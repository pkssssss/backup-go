# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

这是一个用 Go 语言编写的腾讯云 COS 对象存储备份工具。项目支持压缩数据目录并自动上传到腾讯云对象存储，同时具备自动清理过期备份的功能。

## 构建和运行

### 基本命令
```bash
# 构建可执行文件
go build -o backup-go

# 运行备份程序
./backup-go

# 直接运行（开发模式）
go run .
```

### 依赖管理
```bash
# 下载依赖
go mod download

# 整理依赖
go mod tidy

# 验证依赖
go mod verify
```

## 项目架构

### 核心模块
- **backup.go**: 核心备份逻辑，包含目录扫描、tar 打包、zstd 压缩和主函数
- **config.go**: 配置管理，包含配置文件生成、加载和目录初始化
- **cosio.go**: 腾讯云 COS 对象存储接口，包含上传、下载、清理等功能

### 数据流
1. **初始化阶段**: 创建必要目录（data/、config/、tmp/），生成默认配置文件
2. **配置加载**: 从 `config/config.json` 加载腾讯云 COS 认证信息
3. **备份执行**: 扫描 data/ 目录 → 创建 tar 归档 → zstd 压缩 → 上传到 COS
4. **清理阶段**: 删除本地临时文件，清理 COS 上的过期备份

### 关键配置
- **数据目录**: `data/` - 需要备份的数据存放位置
- **配置文件**: `config/config.json` - 腾讯云 COS 配置
- **临时目录**: `tmp/` - 压缩文件临时存储位置
- **COS 路径前缀**: 默认为 `backup`，可配置
- **保留天数**: 默认 30 天，可配置

### 依赖库
- `github.com/tencentyun/cos-go-sdk-v5`: 腾讯云 COS 官方 SDK
- `github.com/klauspost/compress/zstd`: zstd 压缩算法
- `github.com/dustin/go-humanize`: 文件大小人性化显示

## 开发注意事项

### 首次运行
程序首次运行时会自动创建必要的目录结构和默认配置文件，需要手动编辑 `config/config.json` 填入正确的腾讯云 COS 认证信息。

### 错误处理
项目使用统一的错误处理机制：
- `exitIfError()`: 遇到错误直接退出程序
- `exitIfNil()`: 处理可能返回 nil 的函数调用
- `printLog()`: 统一的日志输出格式

### 备份逻辑
- 只压缩常规文件，跳过符号链接和特殊文件
- 使用 zstd 压缩算法平衡压缩率和速度
- 支持进度显示和断点续传（通过 COS SDK）
- 自动跳过空目录的备份操作