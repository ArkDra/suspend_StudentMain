# suspend_StudentMain

通过全局热键 `Ctrl + Space`，对目标进程 `StudentMain.exe` 执行线程挂起/恢复切换。

## 构建

### 调试构建

保留控制台，便于查看错误信息：

```bash
go build -o suspendStudentMain.exe main.go
```

### 发布构建（无控制台窗口）

```bash
go build -o suspendStudentMain.exe -ldflags "-H=windowsgui" main.go
```

## 运行与使用

1. 先启动目标程序 `StudentMain.exe`。
2. 再启动 `suspendStudentMain.exe`。
3. 按 `Ctrl + Space` 触发切换：
   - 当前未挂起：执行挂起
   - 当前已挂起：执行恢复

## 可配置项（需修改代码后重新编译）

在 `main.go` 中可修改：

- `EXE_NAME`：目标进程名（默认 `StudentMain.exe`）
- 热键：`MOD_CONTROL + VK_SPACE`（默认 `Ctrl + Space`）
- `MUTEX_NAME`：单实例互斥锁名称

## 常见问题

### 1. 热键无效

- 可能被其他程序占用，导致 `RegisterHotKey` 失败。
- 建议先用“调试构建”运行，查看控制台输出。

### 2. 没有任何效果

- 确认目标进程名是否与 `EXE_NAME` 完全一致（区分大小写行为取决于系统返回值，建议完全一致）。
- 确认目标进程确实存在且有线程可操作。

### 3. 启动后立刻退出

- 若已有同名实例在运行，会因为单实例机制直接退出。

## 快速验证步骤

1. 启动 `StudentMain.exe`。
2. 启动本工具（建议先用调试构建）。
3. 按一次 `Ctrl + Space`，观察目标程序是否被挂起。
4. 再按一次 `Ctrl + Space`，观察目标程序是否恢复。
