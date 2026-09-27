# Windows 版开发说明

Windows 版与 macOS 版功能对应，用 Go 编写，只依赖标准库和 Win32 API，可以在 macOS 上交叉编译成单个 exe。

```text
windows/
├── cmd/xmindautosave/     程序入口、内置 config.json、图标与清单（winres/）
├── internal/core/         与平台无关的逻辑：配置、文件开关存储、保存时机、URL 与标题解析
└── internal/winapp/       Win32 部分：通知区域图标、悬浮开关、输入监听、辅助功能、安装卸载
```

## 构建与测试

```bash
cd windows
go test ./internal/core/...          # 在任何系统上都能跑
GOOS=windows GOARCH=amd64 go vet ./...
../script/package_windows.sh 1.0.3   # 生成 dist/release/XMindAutoSave-Setup-1.0.3.exe
```

只构建 x64：绘制悬浮开关的 GDI+ 函数需要浮点参数，Go 的 `syscall` 只在 windows/amd64 上按调用约定传递它们。ARM 版 Windows 会用系统自带的仿真运行 x64 程序。

在 Windows 上调试时加 `--portable`，程序就地运行、不会安装自己：

```powershell
go run ./cmd/xmindautosave --portable
```

## 工作方式

| 环节 | 做法 |
| --- | --- |
| 找到 XMind | 前台窗口所属进程名在 `processNames` 中（默认 `Xmind.exe`，不区分大小写） |
| 识别当前文件 | 优先读取 XMind 编辑页的辅助功能（MSAA）信息：Chromium 把文档 URL 作为 document 的值公开，其中 `source=` 就是本地路径，与 macOS 版读取的 URL 相同。Chromium 只有在客户端调用过 IAccessible2 后才构建网页内容的辅助功能树，所以读取前会先调用一次（相当于 macOS 版设置 `AXManualAccessibility`）。读不到时，用窗口标题匹配“最近使用”文件夹里的 `.xmind` 快捷方式 |
| 判断有编辑 | 标题中出现 `dirtyIndicators`（默认 `已编辑`、`Edited`、`*`）时以它为准，与 macOS 版一致；从未出现过时，以在 XMind 中的按键和鼠标松开为准 |
| 保存时机 | 每次输入重新计时，停止约 1.2 秒后发送一次 `Ctrl+S`；XMind 不在前台、按着修饰键或鼠标、XMind 正显示菜单、中文输入法可能正在组字时都会等待 |
| 确认保存 | 文件修改时间变化，或标题的未保存标记消失 |
| 文件开关 | `%APPDATA%\XMindAutoSave\preferences.json`，以卷序列号 + 文件 ID 为键（相当于 macOS 的设备号 + inode），文件被替换时再按路径找回 |
| 登录启动 | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`，首次运行时默认开启，尊重“任务管理器 → 启动应用”中的禁用 |
| 安装 | 下载的 exe 把自己复制到 `%LOCALAPPDATA%\Programs\XMindAutoSave`，创建开始菜单快捷方式和“设置 → 应用”中的卸载项，全部在当前用户范围内 |

输入监听使用 Raw Input 而不是低级键盘钩子：只记录“有按键 / 有点击”这件事和按键是否为字母，不记录内容，也不会拖慢系统输入。

## 命令行参数

| 参数 | 作用 |
| --- | --- |
| （无） | 未安装时安装并启动；已安装的副本直接运行 |
| `--portable` | 不安装，就地运行 |
| `--probe` | 生成诊断报告并用记事本打开 |
| `--uninstall` | 卸载（“设置 → 应用”调用） |
| `--autostart` | 登录时由 Run 项传入，无特殊行为 |

环境变量 `XMIND_AUTOSAVE_CONFIG` 可以指向自定义的 `config.json`；`XMIND_AUTOSAVE_SHOW_WHEN_INACTIVE=1` 让悬浮开关在 XMind 不在前台时也显示，便于调试位置。

## 诊断报告

通知区域菜单“生成诊断报告”或 `XMindAutoSave.exe --probe` 会写出 `%APPDATA%\XMindAutoSave\diagnostics.txt`，包括：XMind 窗口（标题、位置、DPI）、子窗口类名、辅助功能树的前 300 个节点、两种文件识别方式的结果、输入法状态、“最近使用”中的 `.xmind` 快捷方式，以及最近日志。它相当于 macOS 版的 `Tools/AXProbe.swift`。

## 需要在真机上确认的事项

这一版在 macOS 上完成了编译、静态检查和核心逻辑的单元测试，Win32 部分还没有在 Windows 上运行过。首次验证时请重点看：

1. 通知区域图标出现，菜单中“当前文件”显示正确的文件名，诊断报告里“辅助功能”一行给出了路径。
2. XMind 的窗口标题在有未保存修改时是否带标记（报告里“标题显示未保存”）；如果标记是别的文字，把它加进 `dirtyIndicators`。
3. 悬浮开关的位置是否落在 XMind 标题栏空白处；不合适就调整 `panel_windows.go` 中的偏移。
4. 连续输入时不会提前保存，停止约 1.2 秒后保存一次；拖动主题后也会保存。
5. 用微软拼音输入中文时，拼音未上屏期间不会被打断。
6. 重新运行新版安装程序能替换旧版；“设置 → 应用”能完整卸载。
