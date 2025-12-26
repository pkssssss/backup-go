# Backup-Go 🚀

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-gray.svg?style=flat-square)]()
[![Tencent Cloud COS](https://img.shields.io/badge/Integration-Tencent%20Cloud%20COS-0052D9?style=flat-square&logo=tencent-cloud)](https://cloud.tencent.com/product/cos)

**Backup-Go** 是一款专为服务器和开发者设计的现代、高效、云原生备份解决方案。它能够将您的本地数据安全地打包、压缩并备份至腾讯云对象存储（COS），同时提供全平台的后台服务管理和定时任务调度。

> **核心理念**：配置一次，遗忘即可。让数据安全成为后台静默运行的守护进程。

---

## ✨ 核心特性 (Key Features)

*   **⚡ 极速压缩引擎**: 采用 Facebook 开源的 **Zstandard (zstd)** 算法，提供远超 Gzip 的压缩速度和压缩率，显著节省带宽和存储成本。
*   **☁️ 原生云集成**: 深度集成腾讯云 COS SDK，支持断点续传（底层）、分块上传，大文件备份稳如磐石。
*   **🤖 智能守护进程**:
    *   **热重载**: 修改配置文件无需重启服务，即刻生效。
    *   **全平台服务**: 一键安装为系统服务 —— macOS (LaunchAgent), Linux (Systemd), Windows (Service)。
*   **🛡️ 智能保留策略**: 自动清理云端过期的备份文件，精准控制存储成本，无需手动维护。
*   **🖥️ 精美 TUI 交互**: 内置现代化终端交互界面，无需记忆繁琐参数，通过菜单即可完成配置、监控和日志查看。
*   **🔒 安全可靠**: 自动识别并规避循环符号链接、危险路径，确保备份过程安全无误。

## 🛠️ 快速开始 (Quick Start)

### 1. 安装 (Installation)

确保您的环境已安装 Go 1.25+。

```bash
git clone git@github.com:pkssssss/backup-go.git
cd backup-go
go build -o backup-go .
```

### 2. 初始化与配置 (Configuration)

首次运行会自动进入交互式向导，或手动指定初始化：

```bash
./backup-go
```

程序会自动生成配置文件，您也可以参考 `config/config.toml.example` 手动配置：

```toml
[cos]
secret_id  = "你的腾讯云SecretID"
secret_key = "你的腾讯云SecretKey"
bucket     = "backup-1250000000"  # 格式: name-appid
region     = "ap-shanghai"
prefix     = "server-backup/"
keep_days  = 30                   # 备份保留30天

[backup]
data_dir = "/path/to/your/data"   # 需要备份的目录

[backup.schedule]
enabled  = true
hour     = 2                      # 每天凌晨 2:00 执行
minute   = 0
timezone = "Asia/Shanghai"
```

### 3. 安装为后台服务 (Run as Service)

无需编写 Service 文件，Backup-Go 自动接管一切。

在主菜单中选择：
`3. 📋 服务管理` -> `1. 安装开机自启`

或者使用命令行（进阶）：
*服务安装后，将根据配置的定时任务自动并在后台静默运行。*

## 📖 详细功能

### 交互式菜单 (Interactive Menu)

运行 `./backup-go` 即可唤起控制台：

```text
Backup-Go 腾讯云 COS 备份工具 v1.0
============================================================
📊 系统状态检测:
  🔧 配置与COS: ✅ 配置正常，COS连接成功    | 🔄 服务: ✅ 运行中 (PID: 12345)
  ⏰ 定时任务: ✅ 下次执行: 02:00:00       | 🚀 开机自启: ✅ 已启用
  📁 备份路径: ✅ 数据就绪 (2.4 GB)
============================================================
请选择操作:
  1. 🎯 立即备份
  2. 🔧 配置管理
  3. 📋 服务管理
  4. 📝 日志管理
  5. 🕐 定时任务
  6. 📊 状态查看
  7. ❌ 退出管理
```

### 守护进程模式 (Daemon Mode)

如果您希望手动运行守护进程（通常由系统服务管理器调用）：

```bash
./backup-go --daemon
```
在此模式下，程序会监听配置文件变化 (`fsnotify`)，一旦您修改了备份时间或路径，守护进程会自动热更新，无需重启。

## 📂 目录结构

```text
.
├── backup.go           # 主入口与核心备份逻辑
├── service.go          # 全平台服务管理 (Systemd/Launchd/SC)
├── cosio.go            # 腾讯云 COS 交互封装
├── menu.go             # TUI 交互界面逻辑
├── config.go           # 配置加载与热重载
├── config/             # 配置文件目录
└── logs/               # 运行日志
```

## ⚠️ 注意事项

*   **符号链接**: 为了安全起见，备份时**不会跟随**指向外部的绝对路径符号链接，但会保留相对路径的符号链接文件本身。
*   **权限**: 在 Linux/macOS 上安装系统服务可能需要 `sudo` 权限（取决于安装位置，默认用户级服务无需 sudo）。

## 🤝 贡献 (Contributing)

欢迎提交 Issue 和 Pull Request！

## 📄 许可证 (License)

MIT License
