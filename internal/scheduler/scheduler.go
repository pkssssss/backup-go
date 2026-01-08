package scheduler

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"backup-go/internal/config"
	"backup-go/internal/logger"
	"backup-go/internal/task"
)

// Run 启动调度器 (阻塞模式)
func Run(cfgPath string) {
	// 加载初始配置
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		logger.PrintLog("error", fmt.Sprintf("加载配置失败: %v", err))
		os.Exit(1)
	}

	logger.PrintLog("daemon", "=== 启动备份服务 (Server Mode) ===")
	logger.PrintLog("daemon", fmt.Sprintf("定时任务配置: 每天 %02d:%02d",
		cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute))
	logger.PrintLog("daemon", fmt.Sprintf("时区: %s", cfg.Backup.Schedule.Timezone))
	logger.PrintLog("daemon", "配置文件监控已启用")

	// 创建配置文件监控器
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.PrintLog("error", fmt.Sprintf("创建文件监控器失败: %v", err))
		return
	}
	defer watcher.Close()

	if err := watcher.Add(cfgPath); err != nil {
		logger.PrintLog("error", fmt.Sprintf("监控配置文件失败: %v", err))
		return
	}

	// 信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 配置重载通道
	configReloadChan := make(chan bool, 1)

	// 启动文件监控协程
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					logger.PrintLog("daemon", "检测到配置文件变化，正在重载...")
					configReloadChan <- true
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.PrintLog("error", fmt.Sprintf("文件监控错误: %v", err))
			}
		}
	}()

	// 主循环
	for {
		nextRunTime := config.CalculateNextRunTime(cfg.Backup.Schedule)
		now := time.Now()

		if cfg.Backup.Schedule.Enabled {
			duration := nextRunTime.Sub(now)
			logger.PrintLog("daemon", fmt.Sprintf("下次执行时间: %s (等待 %v)",
				nextRunTime.Format("2006-01-02 15:04:05"), duration))

			select {
			case <-time.After(duration):
				logger.PrintLog("daemon", "开始执行定时备份...")
				if err := task.RunBackup(cfg); err != nil {
					logger.PrintLog("error", fmt.Sprintf("定时备份执行失败: %v", err))
				}
			case <-configReloadChan:
				cfg = reloadConfigSafe(cfgPath, cfg)
			case <-sigChan:
				logger.PrintLog("daemon", "收到停止信号，正在退出...")
				return
			}
		} else {
			logger.PrintLog("daemon", "定时任务未启用，等待配置文件变化...")
			select {
			case <-configReloadChan:
				cfg = reloadConfigSafe(cfgPath, cfg)
			case <-sigChan:
				logger.PrintLog("daemon", "收到停止信号，正在退出...")
				return
			}
		}
	}
}

func reloadConfigSafe(cfgPath string, currentCfg *config.Config) *config.Config {
	newCfg, err := config.LoadConfig(cfgPath)
	if err == nil {
		logger.PrintLog("daemon", "配置重载成功")
		return newCfg
	}
	logger.PrintLog("error", fmt.Sprintf("配置重载失败: %v", err))
	return currentCfg
}
