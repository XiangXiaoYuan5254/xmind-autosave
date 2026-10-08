# XMind Auto Save

给 XMind 本地文档补上接近实时的自动保存，并且每个文件都能独立开关。支持 macOS；[Windows 版](#windows-版测试版)正在测试。

[下载最新版](https://helloxxy.com/works/xmind-autosave/)：macOS 选 `XMindAutoSave-*.dmg`，Windows 选 `XMindAutoSave-Setup-*.exe`。也可以在 [GitHub Releases](https://github.com/XiangXiaoYuan5254/xmind-autosave/releases/latest) 下载。

## 它能做什么

- 在 XMind 当前文档标题栏旁显示“自动保存”开关。
- 检测到文档出现“已编辑 / Edited”后，持续监听编辑活动；每次键盘输入都会重新计时，停止输入约 1.2 秒后才发送一次 `⌘S`。
- 每个 `.xmind` 文件分别记忆开关；新文件默认关闭。
- 同一磁盘内重命名或移动文件后，原来的开关设置仍然保留。
- 自动注册为登录项，也可以从菜单栏随时关闭“登录时自动启动”。
- XMind 不在前台、进入全屏或演说模式时，悬浮开关自动隐藏。

程序不上传任何内容，也不会扫描磁盘。它只读取 XMind 当前窗口的辅助功能信息，并且只向 XMind 发送 `⌘S`。唯一的联网是[检查更新](#更新)：每天向官网 helloxxy.com 查询一次最新版本号，可以在菜单中关闭。

## 安装（约 1 分钟）

系统要求：macOS 13 或更高版本，支持 Apple 芯片和 Intel Mac。

1. 从[官网](https://helloxxy.com/works/xmind-autosave/#download)下载 `XMindAutoSave-*.dmg`。
2. 打开 DMG，把 `XMindAutoSave.app` 拖进 `Applications`。
3. 进入“应用程序”，双击打开 `XMindAutoSave`。第一次打开时 macOS 会确认一次“这是从互联网下载的 App”，点“打开”。
4. 按系统提示，在“系统设置 → 隐私与安全性 → 辅助功能”中允许 `XMindAutoSave`。
5. 回到 XMind，通过标题栏旁的开关为当前文件开启自动保存。

运行后它只显示在 macOS 菜单栏，不会显示 Dock 图标。

安装包使用 Apple Developer ID 签名，并已通过 Apple 公证，Gatekeeper 不会再拦下第一次打开。签名身份固定以后，更新到新版本也不用重新授权辅助功能；从临时签名的旧版本（2026-10 之前下载的）换过来时需要重新授权一次。

## 使用

- 标题栏旁的开关只影响当前文件。
- 菜单栏图标中也能切换当前文件、立即检查、打开辅助功能设置、控制登录启动及检查更新。
- 开关设置保存在 `~/Library/Application Support/XMindAutoSave/preferences.json`。
- 退出程序后，本次登录期间不会自动重启；下次登录是否启动由菜单中的“登录时自动启动”决定。

## 更新

程序启动约 30 秒后以及之后每天一次，会读取官网上的 [`latest.json`](https://helloxxy.com/works/xmind-autosave/downloads/latest.json) 查询最新版本号。它只读取版本号和安装包文件名，不发送你的任何数据。发现新版本时会提醒一次，之后菜单顶部会一直显示“有新版本…，前往下载”。也可以随时点菜单里的“检查更新…”；不想让程序联网，取消勾选“自动检查更新”即可。

macOS 上更新：

1. 下载新的 `XMindAutoSave-*.dmg`。
2. 在菜单栏退出 XMind 自动保存。
3. 把新版 `XMindAutoSave.app` 拖进 `Applications`，选择“替换”，再打开它。
4. 如果更新后自动保存不工作，到“系统设置 → 隐私与安全性 → 辅助功能”中选中 `XMindAutoSave`，点“−”移除，再重新打开程序，按提示授权。从临时签名的旧版本换到 Developer ID 签名的版本时，系统会把新版当成另一个程序。

各文件的开关设置会保留。1.0.3 及更早的版本还没有检查更新功能，需要手动下载一次新版，之后就会自动提醒。1.0.4 向 GitHub 查询新版本，更新到 1.0.5 之后改向官网查询。

## 卸载

先在菜单栏取消“登录时自动启动”，再退出程序并删除 `/Applications/XMindAutoSave.app`。如需同时清除文件开关记录，再删除：

```text
~/Library/Application Support/XMindAutoSave
```

也可以在“系统设置 → 通用 → 登录项”中移除或关闭它。

## Windows 版（测试版）

系统要求：Windows 10 1809 或更高版本、Windows 11（x64；ARM 设备通过系统自带的 x64 仿真运行）。

1. 从[官网](https://helloxxy.com/works/xmind-autosave/#download)下载 `XMindAutoSave-Setup-*.exe` 并双击运行。
2. 如果出现“Windows 已保护你的电脑”，点“更多信息 → 仍要运行”。Windows 版还没有代码签名证书。
3. 程序会安装到 `%LOCALAPPDATA%\Programs\XMindAutoSave`（不需要管理员权限），在开始菜单添加“XMind 自动保存”，随后在任务栏右下角的通知区域运行（图标可能收在 `^` 里），并随 Windows 登录自动启动。
4. 打开 XMind 本地文档，通过标题栏旁的“自动保存”开关为当前文件开启。

单击通知区域的图标可以切换当前文件、立即保存、控制登录启动、检查更新。开关设置保存在 `%APPDATA%\XMindAutoSave\preferences.json`。

和 macOS 版的区别：

- Windows 不允许向后台窗口发送快捷键，所以只在 XMind 位于前台时发送 `Ctrl+S`。编辑后不到 1.2 秒就切到别的程序时，会在回到 XMind 后补存。
- 如果 XMind 的窗口标题带有“未保存”标记，就像 macOS 版一样以它为准；否则以你在 XMind 中的按键和点击为准（只用来计时，不记录按了什么）。对没有改动的本地文档按 `Ctrl+S` 不会产生任何影响。
- 用中文输入法打拼音、还没上屏时不会保存，以免打断输入。

程序同样不上传任何内容，只读取 XMind 窗口的辅助功能信息，并且只向 XMind 发送 `Ctrl+S`；唯一的联网也是每天一次检查更新，可以在菜单中关闭。发现新版本时会弹出一次通知，单击即可前往下载。下载新的 `XMindAutoSave-Setup-*.exe` 并运行，它会自动退出并替换旧版，设置保留。

识别不到当前文件或开关位置不对时，请单击通知区域图标 →“生成诊断报告”，检查后把 `diagnostics.txt` 附在 Issue 里。

卸载：设置 → 应用 → 已安装的应用 → XMind 自动保存 → 卸载。

## 从源码构建

项目是 SwiftPM 原生 macOS App：

```bash
swift test
./script/build_and_run.sh --verify
```

生成同时支持 Apple 芯片与 Intel 的 DMG：

```bash
./script/package_release.sh 1.0.5
```

产物位于 `dist/release/`，可以直接上传到 GitHub Release。脚本用钥匙串里的 Developer ID 证书（`Developer ID Application: Li Ming wang (46AL7LQ9T8)`，`XMIND_SIGN_IDENTITY` 可换成别的）签名并开启 hardened runtime，再把 App 和 DMG 交给 Apple 公证、贴上票据（几分钟，期间别让 Mac 锁屏）。公证用登录钥匙串里名为 `helloxxy-notary` 的凭据（`xcrun notarytool store-credentials` 保存，`XMIND_NOTARY_PROFILE` 可换成别的）。钥匙串里没有这张证书时退回临时签名、跳过公证，只适合自己用。

发布时还要把安装包放到官网 `downloads/`，并更新同目录的 `latest.json`，已安装的程序靠它发现新版本。`files` 只列这个版本实际提供的安装包，没有 DMG 或 Windows 安装程序的版本不会提示对应系统的用户更新：

```json
{ "version": "1.0.5", "date": "2026-09-28", "files": ["XMindAutoSave-1.0.5.dmg", "XMindAutoSave-Setup-1.0.5.exe"] }
```

1.0.4 仍向 GitHub 查询最新 Release，所以 GitHub Release 也要照常发布。

Windows 版用 Go 编写，在 macOS 上即可交叉编译（需要 Go 1.22+）：

```bash
./script/package_windows.sh 1.0.5
```

生成 `dist/release/XMindAutoSave-Setup-1.0.5.exe`。代码结构和调试方法见 [windows/README.md](windows/README.md)。

## 兼容性说明

当前版本在 XMind 26.02 上验证。它依赖 XMind 的辅助功能界面来识别当前本地文档与“已编辑”状态；如果 XMind 以后大幅调整界面结构，可能需要同步适配。Windows 版尚未在真机上完成验证，识别方式见 [windows/README.md](windows/README.md)。

本项目是独立的社区工具，与 XMind Ltd. 无隶属或官方合作关系；XMind 是其各自权利人的商标。

## 许可证

本项目基于 [MIT 许可证](LICENSE) 开源。© 2026 向小园
