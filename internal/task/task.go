package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"backup-go/internal/config"
	"backup-go/internal/core/archiver"
	"backup-go/internal/core/uploader"
	"backup-go/internal/logger"
)

const (
	TempDir = "tmp"
)

// RunBackup 执行一次完整备份
func RunBackup(cfg *config.Config) error {
	// 创建 COS 客户端
	client, err := uploader.NewClient(&cfg.Cos)
	if err != nil {
		return fmt.Errorf("创建COS客户端失败: %w", err)
	}

	// 准备临时目录 (使用独立子目录避免冲突)
	taskID := fmt.Sprintf("manual-%d", time.Now().UnixNano())
	taskTempDir := filepath.Join(TempDir, taskID)
	
	if err := os.MkdirAll(taskTempDir, 0755); err != nil {
		return fmt.Errorf("创建任务临时目录失败: %w", err)
	}
	defer os.RemoveAll(taskTempDir) // 任务结束清理

	// 生成文件名
	archiveName := fmt.Sprintf("backup-%s.tar.zst", time.Now().Format("20060102-150405"))
	archivePath := filepath.Join(taskTempDir, archiveName)

	// 1. 压缩
	_, _, err = archiver.Compress(cfg.Backup.DataDir, archivePath)
	if err != nil {
		if strings.Contains(err.Error(), "为空，跳过备份") {
			logger.PrintLog("skip", err.Error())
			return nil
		}
		return fmt.Errorf("压缩失败: %w", err)
	}

	// 2. 上传
	cosPath := cfg.Cos.Prefix + archiveName
	// 归一化 prefix
	if cfg.Cos.Prefix != "" && !strings.HasSuffix(cfg.Cos.Prefix, "/") {
		cosPath = cfg.Cos.Prefix + "/" + archiveName
	}

	if err := uploader.Upload(client, archivePath, cosPath); err != nil {
		return fmt.Errorf("上传失败: %w", err)
	}

	// 3. 清理过期
	if err := uploader.DeleteExpiredBackups(client, cfg.Cos.Bucket, cfg.Cos.Prefix, cfg.Cos.KeepDays); err != nil {
		logger.PrintLog("warn", fmt.Sprintf("清理过期备份失败: %v", err))
	}

	logger.PrintLog("done", "备份流程完成")
	return nil
}
