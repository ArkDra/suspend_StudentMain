package main

import (
	"fmt"
	"os"
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
	EXE_NAME = "StudentMain.exe"

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
		exit()
	}
}

func handleOk(ok bool) {
	if !ok {
		fmt.Printf("%s not found\n", EXE_NAME)
		exit()
	}
}

func exit() {
	err := unregisterGlobalHotKey()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	os.Exit(1)
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

func suspendThread(threadID uint32) error {
	threadHandle, err := openThread(threadID)
	handleError(err)

	_, ret, _ := SuspendThread.Call(threadHandle)
	if ret != 0 {
		return fmt.Errorf("SuspendThread failed")
	}
	// fmt.Println("SuspendThread succeed")
	defer windows.CloseHandle(windows.Handle(threadHandle))
	return nil
}

func resumeThread(threadID uint32) error {
	threadHandle, err := openThread(threadID)
	handleError(err)

	_, ret, _ := ResumeThread.Call(threadHandle)
	if ret != 0 {
		return fmt.Errorf("ResumeThread failed")
	}
	// fmt.Println("ResumeThread succeed")
	defer windows.CloseHandle(windows.Handle(threadHandle))
	return nil
}

func processThreads() {
	processMap, threadMap := getProcessMapAndThereadMap()
	if !isSuspended {
		operateThreads(processMap, threadMap, suspendThread)
		isSuspended = true
	} else {
		operateThreads(processMap, threadMap, resumeThread)
		isSuspended = false
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
	handleOk(ok)
	for _, pid := range pids {
		tids, ok := threadMap[pid]
		handleOk(ok)

		for _, tid := range tids {
			err := operation(tid)
			handleError(err)
		}
	}
}

func main() {
	err := registerGlobalHotKey()
	handleError(err)

	for {
		if getMessage() {
			processThreads()
		} else {
			time.Sleep(5 * time.Millisecond)
		}
	}
}
