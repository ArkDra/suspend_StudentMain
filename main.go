package main

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")

	RegisterHotKey           = user32.NewProc("RegisterHotKey")
	UnregisterHotKey         = user32.NewProc("UnregisterHotKey")
	PeekMessage              = user32.NewProc("PeekMessageW")
	createToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	OpenThread               = kernel32.NewProc("OpenThread")
	SuspendThread            = kernel32.NewProc("SuspendThread")
	ResumeThread             = kernel32.NewProc("ResumeThread")

	msg         MSG
	isSuspended = false
)

const (
	EXE_NAME   = "StudentMain.exe"
	MUTEX_NAME = "suspend_StudentMain_single_instance_mutex"

	MOD_CONTROL            = 0x0002
	VK_SPACE               = 0x20
	WM_HOTKEY              = 0x0312
	PM_REMOVE              = 0x0001
	HOTKEY_ID              = 1024
	TH32CS_SNAPPROCESS     = 0x00000002
	TH32CS_SNAPTHREAD      = 0x00000004
	THREADS_SUSPEND_RESUME = 0x0002
)

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct {
		X int32
		Y int32
	}
}

func handleError(err error) {
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func registerGlobalHotKey() error {
	ret, _, _ := RegisterHotKey.Call(uintptr(0), HOTKEY_ID, uintptr(MOD_CONTROL), uintptr(VK_SPACE))
	if ret == 0 {
		return fmt.Errorf("RegisterHotKey failed")
	}
	fmt.Println("RegisterHotKey succeed")
	return nil
}

func unregisterGlobalHotKey() error {
	ret, _, _ := UnregisterHotKey.Call(uintptr(0), HOTKEY_ID)
	if ret == 0 {
		return fmt.Errorf("UnregisterHotKey failed")
	}
	fmt.Println("UnregisterHotKey succeed")
	return nil
}

func getMessage() bool {
	ret, _, _ := PeekMessage.Call(uintptr(unsafe.Pointer(&msg)), uintptr(0), 0, 0, PM_REMOVE)
	if ret == 0 {
		return false
	}
	if msg.Message == WM_HOTKEY && msg.WParam == uintptr(HOTKEY_ID) {
		fmt.Println("HotKey pressed")
		return true
	} else {
		return false
	}
}

func getProcessSnapshot() (windows.Handle, error) {
	processSnapshotHandle, _, _ := createToolhelp32Snapshot.Call(uintptr(TH32CS_SNAPPROCESS), 0)
	if processSnapshotHandle == uintptr(windows.InvalidHandle) {
		return windows.InvalidHandle, fmt.Errorf("getProcessSnapshot failed")
	}
	return windows.Handle(processSnapshotHandle), nil
}

func enumProcesses(processSnapshotHandle windows.Handle) (map[string][]uint32, error) {
	processMap := make(map[string][]uint32)
	var pe32 windows.ProcessEntry32
	pe32.Size = uint32(unsafe.Sizeof(pe32))

	for {
		err := windows.Process32Next(processSnapshotHandle, &pe32)
		if err != nil {
			break
		}

		processID := pe32.ProcessID
		processName := windows.UTF16ToString(pe32.ExeFile[:])
		processMap[processName] = append(processMap[processName], processID)
	}

	defer windows.CloseHandle(processSnapshotHandle)
	return processMap, nil
}

func getThreadSnapshot() (windows.Handle, error) {
	threadSnapshotHandle, _, _ := createToolhelp32Snapshot.Call(uintptr(TH32CS_SNAPTHREAD), 0)
	if threadSnapshotHandle == uintptr(windows.InvalidHandle) {
		return windows.InvalidHandle, fmt.Errorf("getThreadSnapshot failed")
	}

	return windows.Handle(threadSnapshotHandle), nil
}

func enumThreads(threadSnapshotHandle windows.Handle) (map[uint32][]uint32, error) {
	threadMap := make(map[uint32][]uint32)
	var te32 windows.ThreadEntry32
	te32.Size = uint32(unsafe.Sizeof(te32))

	for {
		err := windows.Thread32Next(threadSnapshotHandle, &te32)
		if err != nil {
			break
		}

		threadID := te32.ThreadID
		processID := te32.OwnerProcessID
		threadMap[processID] = append(threadMap[processID], threadID)
	}

	defer windows.CloseHandle(threadSnapshotHandle)
	return threadMap, nil
}

func openThread(threadID uint32) (uintptr, error) {
	threadHandle, _, _ := OpenThread.Call(uintptr(THREADS_SUSPEND_RESUME), 0, uintptr(threadID))
	if threadHandle == 0 {
		return uintptr(0), fmt.Errorf("OpenThread failed")
	}
	return threadHandle, nil
}

func createSingleInstanceMutex() (windows.Handle, error) {
	namePtr, err := windows.UTF16PtrFromString(MUTEX_NAME)
	if err != nil {
		return 0, fmt.Errorf("UTF16PtrFromString failed: %w", err)
	}

	mutexHandle, err := windows.CreateMutex(nil, false, namePtr)
	if err != nil {
		if mutexHandle != 0 {
			_ = windows.CloseHandle(mutexHandle)
		}
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return 0, fmt.Errorf("another instance is already running")
		}
		return 0, fmt.Errorf("CreateMutex failed: %w", err)
	}

	return mutexHandle, nil
}

func suspendThread(threadID uint32) error {
	threadHandle, err := openThread(threadID)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(windows.Handle(threadHandle))

	count, _, _ := SuspendThread.Call(threadHandle)
	if count == 0xFFFFFFFF {
		return fmt.Errorf("SuspendThread failed")
	}
	// fmt.Println("SuspendThread succeed")
	isSuspended = true
	return nil
}

func resumeThread(threadID uint32) error {
	threadHandle, err := openThread(threadID)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(windows.Handle(threadHandle))

	count, _, _ := ResumeThread.Call(threadHandle)
	if count == 0xFFFFFFFF {
		return fmt.Errorf("ResumeThread failed")
	}
	if count > 1 {
		isSuspended = true
		return nil
	}
	// fmt.Println("ResumeThread succeed")
	isSuspended = false
	return nil
}

func processThreads() {
	processMap, threadMap := getProcessMapAndThereadMap()
	if !isSuspended {
		operateThreads(processMap, threadMap, suspendThread)
	} else {
		operateThreads(processMap, threadMap, resumeThread)
	}
}

func getProcessMapAndThereadMap() (map[string][]uint32, map[uint32][]uint32) {
	processSnapshotHandle, err := getProcessSnapshot()
	handleError(err)
	processMap, err := enumProcesses(processSnapshotHandle)
	handleError(err)
	threadSnapshotHandle, err := getThreadSnapshot()
	handleError(err)
	threadMap, err := enumThreads(threadSnapshotHandle)
	handleError(err)

	return processMap, threadMap
}

func operateThreads(processMap map[string][]uint32, threadMap map[uint32][]uint32, operation func(uint32) error) {
	pids, ok := processMap[EXE_NAME]
	if !ok || len(pids) == 0 {
		fmt.Printf("%s not found, waiting...\n", EXE_NAME)
		isSuspended = false
		return
	}

	hasThread := false
	for _, pid := range pids {
		tids, ok := threadMap[pid]
		if !ok || len(tids) == 0 {
			continue
		}

		hasThread = true
		for _, tid := range tids {
			err := operation(tid)
			handleError(err)
		}
	}

	if !hasThread {
		fmt.Printf("%s has no available threads, waiting...\n", EXE_NAME)
		isSuspended = false
	}
}

func main() {
	mutexHandle, err := createSingleInstanceMutex()
	if err != nil {
		handleError(err)
		return
	}
	defer windows.CloseHandle(mutexHandle)

	err = registerGlobalHotKey()
	if err != nil {
		handleError(err)
		return
	}
	defer func() {
		handleError(unregisterGlobalHotKey())
	}()

	for {
		if getMessage() {
			processThreads()
		} else {
			time.Sleep(5 * time.Millisecond)
		}
	}
}
