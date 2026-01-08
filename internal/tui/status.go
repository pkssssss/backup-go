package tui

import (
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"backup-go/internal/config"
	"backup-go/internal/core/archiver"
	"backup-go/internal/core/uploader"
	"backup-go/internal/service"
)

// SystemStatus 系统状态结构体
type SystemStatus struct {
	ServiceRunning    bool
	ServicePID        int
	ConfigLoaded      bool
	DataDir           string
	Bucket            string
	Prefix            string
	LastBackup        time.Time
	LastBackupSuccess bool
	NextBackup        time.Time
	ScheduleEnabled   bool
	ServiceAutoStart  bool
}

// GetCurrentSystemStatus 获取当前系统状态
func GetCurrentSystemStatus(cfgPath string) SystemStatus {
	status := SystemStatus{}

	// 加载配置获取基础信息
	if cfg, err := config.LoadConfig(cfgPath); err == nil {
		status.ConfigLoaded = true
		status.DataDir = cfg.Backup.DataDir
		status.Bucket = cfg.Cos.Bucket
		status.Prefix = cfg.Cos.Prefix
		status.ScheduleEnabled = cfg.Backup.Schedule.Enabled

		// 计算下次备份时间
		if status.ScheduleEnabled {
			status.NextBackup = config.CalculateNextRunTime(cfg.Backup.Schedule)
		}
	}

	// 检查服务状态
	svc := service.GetServiceManager()
	svcStatus := svc.Status()
	status.ServiceRunning = svcStatus.Running
	status.ServicePID = svcStatus.PID
	status.ServiceAutoStart = svcStatus.AutoStart

	return status
}

// ShowSystemStatus 显示详细系统状态
func ShowSystemStatus(cfgPath string) {
	status := GetCurrentSystemStatus(cfgPath)

	// 综合配置和COS状态
	configStatus := "❌ 未加载"
	if status.ConfigLoaded {
		configStatus = fmt.Sprintf("✅ 正常 (桶: %s)", status.Bucket)
	}

	// 服务状态
	serviceStatus := "○ 已停止"
	if status.ServiceRunning {
		serviceStatus = fmt.Sprintf("✅ 运行中 (PID: %d)", status.ServicePID)
	}

	// 定时任务
	scheduleStatus := "○ 未启用"
	if status.ScheduleEnabled {
		scheduleStatus = fmt.Sprintf("✅ 已启用 (下次: %s)", status.NextBackup.Format("15:04:05"))
	}

	// 开机自启
	autoStartStatus := "○ 已禁用"
	if status.ServiceAutoStart {
		autoStartStatus = "✅ 已启用"
	}

	// 备份路径状态
	dataStatus := "❌ 未配置"
	if status.ConfigLoaded {
		if size, err := archiver.CalculateDirSize(status.DataDir); err == nil {
			dataStatus = fmt.Sprintf("✅ 就绪 (%s)", humanize.Bytes(uint64(size)))
		} else {
			dataStatus = fmt.Sprintf("⚠️  无法读取 (%s)", status.DataDir)
		}
	}

	fmt.Println("📊 系统状态检测:")
	fmt.Printf("  🔧 配置与COS: %-30s | 🔄 服务: %s\n", configStatus, serviceStatus)
	fmt.Printf("  ⏰ 定时任务: %-30s | 🚀 自启: %s\n", scheduleStatus, autoStartStatus)
	fmt.Printf("  📁 数据目录: %s\n", dataStatus)
}

// CheckConfigAndTestCOS 检查配置并测试 COS
func CheckConfigAndTestCOS(cfgPath string) {
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Printf("❌ 配置文件加载失败: %v\n", err)
		return
	}

	fmt.Println("正在测试 COS 连接...")
	client, err := uploader.NewClient(&cfg.Cos)
	if err != nil {
		fmt.Printf("❌ COS 客户端创建失败: %v\n", err)
		return
	}

	if err := uploader.TestConnection(client, cfg.Cos.Bucket); err != nil {
		fmt.Printf("❌ COS 连接测试失败: %v\n", err)
	} else {
		fmt.Println("✅ COS 连接测试成功！")
	}
}
