# 官网

XMind 自动保存的官网。纯静态页面：没有构建步骤，也不依赖任何外部 CDN 或字体，国内访问同样很快。

```text
website/
├── index.html          页面
├── styles.css          样式（自动适配浅色 / 深色模式）
├── main.js             首屏演示动画、交互演示、复制按钮
├── assets/             图标、favicon、分享预览图 og.png
└── downloads/          DMG 安装包与 SHA256SUMS.txt
```

## 本地预览

```bash
python3 -m http.server 8080 -d website
```

然后打开 <http://localhost:8080>。

## 发布新版本后更新官网

先按原流程打包，再同步到官网：

```bash
./script/package_release.sh 1.0.4
./script/sync_website_release.sh 1.0.4
```

同步脚本会：

- 把 `dist/release/` 里对应版本的 DMG 复制到 `website/downloads/`，并删除旧版本；
- 重新生成 `website/downloads/SHA256SUMS.txt`；
- 更新页面上的版本号、下载链接、文件大小和发布日期；
- 从 `CHANGELOG.md` 的 `## 1.0.4` 小节读取条目，替换“最近更新”。

Windows 版目前在下载区显示为“即将推出”。发布后，按 `index.html` 里下载按钮旁的注释把占位按钮换成真正的下载链接。

省略版本号时，自动使用 `dist/release/` 里版本最高的 DMG。

## 部署

把 `website/` 目录原样部署到任意静态托管即可，**不需要构建命令**。

| 平台 | 设置 |
| --- | --- |
| Vercel | Root Directory 设为 `website`，Framework 选 Other，Build Command 留空 |
| Netlify | Base directory 设为 `website`，Publish directory 设为 `website`，Build command 留空 |
| Cloudflare Pages | Build output directory 设为 `website`，Build command 留空 |
| 自己的服务器 | 把 `website/` 里的文件放到 Nginx / Caddy 的站点根目录 |

国内服务器或对象存储（如阿里云 OSS、腾讯云 COS）同样适用，上传整个目录并开启静态网站托管即可。

安装包放在 `website/downloads/` 并随仓库提交，这样连接 Git 仓库自动部署时，下载文件也会一起发布。

## 上线后建议

拿到正式域名后，把 `index.html` 里的分享信息改成绝对地址，微信、Twitter 等平台才能正确抓取预览图：

```html
<meta property="og:url" content="https://你的域名/">
<meta property="og:image" content="https://你的域名/assets/og.png">
<link rel="canonical" href="https://你的域名/">
```
