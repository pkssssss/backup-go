# Backup-Go 🚀

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-gray.svg?style=flat-square)]()
[![Tencent Cloud COS](https://img.shields.io/badge/Integration-Tencent%20Cloud%20COS-0052D9?style=flat-square&logo=tencent-cloud)](https://cloud.tencent.com/product/cos)

**Backup-Go** 是一款专为服务器和开发者设计的现代、高效、云原生备份解决方案。它能够将您的本地数据安全地打包、压缩并备份至腾讯云对象存储（COS），同时提供全平台的后台服务管理和定时任务调度。

> **核心理念**：配置一次，遗忘即可。让数据安全成为后台静默运行的守护进程。

---

## ✨ 核心特性 (Key Features)

*   **⚡ 极速压缩引擎**: 采用 Facebook 开源的 **Zstandard (zstd)** 算法，提供远超 Gzip 的压缩速度和压缩率，显著节省带宽和存储成本。
*   **☁️ 原生云集成**: 深度集成腾讯云 COS SDK，支持断点续传（底层）、分块上传，大文件备份稳如磐石。
*   **🤖 智能守护进程**:
    *   **热重载**: 修改配置文件无需重启服务，即刻生效。
    *   **系统服务**: 一键安装为系统服务 —— macOS (LaunchAgent), Linux (Systemd)。
*   **🛡️ 智能保留策略**: 自动清理云端过期的备份文件，精准控制存储成本，无需手动维护。
*   **🖥️ 精美 TUI 交互**: 内置现代化终端交互界面，无需记忆繁琐参数，通过菜单即可完成配置、监控和日志查看。
*   **🔒 安全可靠**: 自动识别并规避循环符号链接、危险路径，确保备份过程安全无误。

## 🛠️ 快速开始 (Quick Start)

### 1. 安装 (Installation)

确保您的环境已安装 Go 1.25+。

```bash
git clone git@github.com:pkssssss/backup-go.git
cd backup-go
go build -o backup-go cmd/backup-go/main.go
```

### 2. 初始化与配置 (Configuration)

首次运行会自动进入交互式向导，或手动生成配置：

```bash
./backup-go
# 或者
./backup-go init
```

程序会自动生成配置文件，您也可以参考 `config/config.toml` 手动配置：

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

或者使用命令行：
```bash
./backup-go install
```

服务安装后，将根据配置的定时任务自动并在后台静默运行。

## 📖 命令参考

```bash
backup-go [命令]

命令:
  server     启动后台服务模式 (通常由系统服务调用)
  once       立即执行一次备份
  init       生成默认配置文件
  install    安装为系统服务
  uninstall  卸载系统服务
  help       显示帮助信息
```

如果不带参数运行，将进入交互式菜单。

## 📂 目录结构 (Refactored)

```text
.
├── cmd/
│   └── backup-go/          # 应用程序入口
├── internal/
│   ├── config/             # 配置管理
│   ├── core/               # 核心业务 (archiver, uploader)
│   ├── logger/             # 日志工具
│   ├── scheduler/          # 调度器 (Server Mode)
│   ├── service/            # 系统服务管理
│   ├── task/               # 任务执行逻辑
│   ├── tui/                # 终端界面
│   └── utils/              # 通用工具
└── config/                 # 配置文件目录
```

## ⚠️ 注意事项

*   **符号链接**: 为了安全起见，备份时**不会跟随**指向外部的绝对路径符号链接，但会保留相对路径的符号链接文件本身。
*   **权限**: 在 Linux/macOS 上安装系统服务可能需要 `sudo` 权限（取决于安装位置，默认用户级服务无需 sudo）。

## 🤝 贡献 (Contributing)

欢迎提交 Issue 和 Pull Request！

## 📄 许可证 (License)

MIT License
