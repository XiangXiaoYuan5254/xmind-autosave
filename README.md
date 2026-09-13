# XMind Auto Save for macOS

给 XMind 本地文档补上接近实时的自动保存，并且每个文件都能独立开关。

[下载最新版 DMG](https://github.com/XiangXiaoYuan5254/xmind-autosave/releases/latest)

## 它能做什么

- 在 XMind 当前文档标题栏旁显示“自动保存”开关。
- 检测到文档出现“已编辑 / Edited”后，持续监听编辑活动；每次键盘输入都会重新计时，停止输入约 1.2 秒后才发送一次 `⌘S`。
- 每个 `.xmind` 文件分别记忆开关；新文件默认关闭。
- 同一磁盘内重命名或移动文件后，原来的开关设置仍然保留。
- 自动注册为登录项，也可以从菜单栏随时关闭“登录时自动启动”。
- XMind 不在前台、进入全屏或演说模式时，悬浮开关自动隐藏。

程序不联网、不上传内容，也不会扫描磁盘。它只读取 XMind 当前窗口的辅助功能信息，并且只向 XMind 发送 `⌘S`。

## 安装（约 1 分钟）

系统要求：macOS 13 或更高版本，支持 Apple 芯片和 Intel Mac。

1. 从 [Releases](https://github.com/XiangXiaoYuan5254/xmind-autosave/releases/latest) 下载 `XMindAutoSave-*.dmg`。
2. 打开 DMG，把 `XMindAutoSave.app` 拖进 `Applications`。
3. 进入“应用程序”，按住 Control 点击 `XMindAutoSave`，选择“打开”，再确认“打开”。
4. 按系统提示，在“系统设置 → 隐私与安全性 → 辅助功能”中允许 `XMindAutoSave`。
5. 回到 XMind，通过标题栏旁的开关为当前文件开启自动保存。

运行后它只显示在 macOS 菜单栏，不会显示 Dock 图标。

### 为什么首次需要右键打开？

当前版本没有 Apple Developer 证书，因此不能进行苹果公证。安装包使用 macOS 临时签名并公开全部源码，但 Gatekeeper 仍会在第一次打开时要求手动确认。获得正式开发者签名后，这一步可以移除。

## 使用

- 标题栏旁的开关只影响当前文件。
- 菜单栏图标中也能切换当前文件、立即检查、打开辅助功能设置及控制登录启动。
- 开关设置保存在 `~/Library/Application Support/XMindAutoSave/preferences.json`。
- 退出程序后，本次登录期间不会自动重启；下次登录是否启动由菜单中的“登录时自动启动”决定。

## 卸载

先在菜单栏取消“登录时自动启动”，再退出程序并删除 `/Applications/XMindAutoSave.app`。如需同时清除文件开关记录，再删除：

```text
~/Library/Application Support/XMindAutoSave
```

也可以在“系统设置 → 通用 → 登录项”中移除或关闭它。

## 从源码构建

项目是 SwiftPM 原生 macOS App：

```bash
swift test
./script/build_and_run.sh --verify
```

生成同时支持 Apple 芯片与 Intel 的 DMG：

```bash
./script/package_release.sh 1.0.3
```

产物位于 `dist/release/`，可以直接上传到 GitHub Release。

## 兼容性说明

当前版本在 XMind 26.02 上验证。它依赖 XMind 的辅助功能界面来识别当前本地文档与“已编辑”状态；如果 XMind 以后大幅调整界面结构，可能需要同步适配。

本项目是独立的社区工具，与 XMind Ltd. 无隶属或官方合作关系；XMind 是其各自权利人的商标。
