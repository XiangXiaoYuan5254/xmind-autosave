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
XMIND_AUTOSAVE_NETWORK_TESTS=1 go test -run GitHub ./internal/core/   # 另外实际查询一次 GitHub
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
| 检查更新 | 启动 30 秒后及此后每天一次，读取 GitHub 最新 Release 的版本号和文件名（`internal/core/update.go`），只有更新且带 `XMindAutoSave-Setup-*.exe` 的版本才提示：通知只弹一次，下载项一直留在菜单顶部。先用 `HTTPS_PROXY` 等环境变量，否则用“设置 → 网络 → 代理”中的手动代理（不支持 PAC 脚本）。开关和已提醒的版本记在 `HKCU\Software\XMindAutoSave`。版本号不是 x.y.z 的构建（`dev`、CI 的 `ci-…`）不检查 |
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

## 自动化测试

`.github/workflows/windows.yml` 在每次提交时用 GitHub 的 Windows 虚拟机测试：

- 构建与测试：`go vet`、核心逻辑单元测试、Win32 集成测试（结构体布局、注册表、文件 ID、快捷方式、悬浮开关绘制），并上传 exe。
- 端到端（`go test -tags e2e`）：先在 Edge 上验证能从辅助功能树读出文档 URL；再静默安装最新版 XMind，走完首次启动的“What's New”、许可协议（关闭使用统计后同意）、关闭登录提示和 Quick Start，打开文档并运行本程序，验证：
  - 通过辅助功能识别出文件路径，悬浮开关出现；
  - 编辑后约 1.2 秒自动保存，改动确实写进了文件；
  - 连续输入（间隔小于 1.2 秒）期间不提前保存，停下后再保存一次。

每次运行的截图、程序日志和诊断报告都作为 artifact 上传。

## 需要在真机上确认的事项

1. 悬浮开关位置：XMind 的工具栏在文件名和右侧按钮组之间居中，窗口较窄（约 1024 像素宽）时开关会盖住工具栏最左边的按钮；更宽的窗口里它落在空白处。
2. 用微软拼音输入中文时，拼音未上屏期间不会被打断（虚拟机里没有中文输入法）。
3. 安装、升级和“设置 → 应用”卸载（端到端测试以 `--portable` 运行）。

另外，XMind for Windows 的“未保存”标记显示在页面里（文件名下方的 “Edited”），不在窗口标题中，所以 Windows 版按输入活动计时；对没有改动的本地文档，多发的 `Ctrl+S` 不会改写文件。
