package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// 系统状态结构体
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
}

// 获取当前系统状态
func getCurrentSystemStatus() SystemStatus {
	status := SystemStatus{}

	// 加载配置获取基础信息
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if cfg, err := loadConfig(cfgPath); err == nil {
		status.ConfigLoaded = true
		status.DataDir = cfg.Backup.DataDir
		status.Bucket = cfg.Cos.Bucket
		status.Prefix = cfg.Cos.Prefix
		status.ScheduleEnabled = cfg.Backup.Schedule.Enabled

		// 计算下次备份时间
		if status.ScheduleEnabled {
			now := time.Now()
			scheduleTime := time.Date(now.Year(), now.Month(), now.Day(),
				cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute, 0, 0, now.Location())

			// 如果今天的调度时间已过，则设为明天
			if scheduleTime.Before(now) {
				scheduleTime = scheduleTime.Add(24 * time.Hour)
			}
			status.NextBackup = scheduleTime
		}

		// 获取上次备份信息（可以从日志或状态文件读取）
		status.LastBackup, status.LastBackupSuccess = getLastBackupInfo()
	}

	// 检查服务状态
	status.ServiceRunning, status.ServicePID = getServiceRunningStatus()

	return status
}

// 获取服务运行状态
func getServiceRunningStatus() (bool, int) {
	// 根据操作系统使用不同的方法检测服务状态
	switch runtime.GOOS {
	case "darwin":
		return getMacOSServiceStatus()
	case "linux":
		return getLinuxServiceStatus()
	case "windows":
		return getWindowsServiceStatus()
	default:
		// 默认方法：检查进程
		return getProcessStatus()
	}
}

// macOS 服务状态检测
func getMacOSServiceStatus() (bool, int) {
	serviceName := "com.backup-go.daemon"

	// 使用 launchctl list 检查服务
	cmd := exec.Command("launchctl", "list")
	output, err := cmd.Output()
	if err != nil {
		return false, 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, serviceName) {
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				if pid, err := strconv.Atoi(fields[0]); err == nil && pid > 0 {
					return true, pid
				}
			}
		}
	}

	return false, 0
}

// Linux 服务状态检测
func getLinuxServiceStatus() (bool, int) {
	serviceName := "backup-go"

	// 使用 systemctl 检查服务状态
	cmd := exec.Command("systemctl", "is-active", serviceName)
	if err := cmd.Run(); err == nil {
		// 服务正在运行，获取PID
		cmd = exec.Command("systemctl", "show", serviceName, "--property=MainPID")
		if output, err := cmd.Output(); err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "MainPID=") {
					if pidStr := strings.TrimPrefix(line, "MainPID="); pidStr != "" {
						if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
							return true, pid
						}
					}
				}
			}
		}
	}

	return false, 0
}

// Windows 服务状态检测
func getWindowsServiceStatus() (bool, int) {
	// 使用 sc query 检查服务状态
	serviceName := "BackupGo"

	cmd := exec.Command("sc", "query", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return false, 0
	}

	outputStr := string(output)
	if strings.Contains(outputStr, "RUNNING") {
		// 尝试获取PID
		if strings.Contains(outputStr, "PID") {
			lines := strings.Split(outputStr, "\n")
			for _, line := range lines {
				if strings.Contains(line, "PID") {
					fields := strings.Fields(line)
					for i, field := range fields {
						if field == "PID" && i+1 < len(fields) {
							if pid, err := strconv.Atoi(fields[i+1]); err == nil {
								return true, pid
							}
						}
					}
				}
			}
		}
		return true, 0
	}

	return false, 0
}

// 通用进程状态检测
func getProcessStatus() (bool, int) {
	// 使用 pgrep 或 ps 查找进程
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("tasklist", "/fi", "imagename eq backup-go.exe")
	} else {
		cmd = exec.Command("pgrep", "-f", "backup-go")
	}

	output, err := cmd.Output()
	if err != nil {
		// 如果 pgrep 不可用，尝试使用 ps
		cmd = exec.Command("ps", "aux")
		if output, err := cmd.Output(); err == nil {
			if strings.Contains(string(output), "backup-go") {
				// 简单检测，无法获取精确PID
				return true, 0
			}
		}
		return false, 0
	}

	if runtime.GOOS == "windows" {
		return strings.Contains(string(output), "backup-go.exe"), 0
	} else {
		if pidStr := strings.TrimSpace(string(output)); pidStr != "" {
			if pid, err := strconv.Atoi(pidStr); err == nil {
				return true, pid
			}
		}
	}

	return false, 0
}

// 获取上次备份信息
func getLastBackupInfo() (time.Time, bool) {
	// 尝试从状态文件读取上次备份信息
	statusFile := filepath.Join(LogDir, ".backup_status")
	if data, err := os.ReadFile(statusFile); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "last_backup=") {
				if timeStr := strings.TrimPrefix(line, "last_backup="); timeStr != "" {
					if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
						// 查找成功状态
						success := true // 默认成功
						for _, l := range lines {
							if strings.HasPrefix(l, "last_success=") {
								if successStr := strings.TrimPrefix(l, "last_success="); successStr != "" {
									success = successStr == "true"
								}
								break
							}
						}
						return t, success
					}
				}
			}
		}
	}

	// 如果没有状态文件，尝试从日志文件推断
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(LogDir, fmt.Sprintf("backup-%s.log", today))
	if data, err := os.ReadFile(logFile); err == nil {
		logContent := string(data)
		lines := strings.Split(logContent, "\n")

		// 查找最后的备份记录
		var lastBackupTime time.Time
		var success bool

		for i := len(lines) - 1; i >= 0; i-- {
			line := lines[i]
			if strings.Contains(line, "[info]") && strings.Contains(line, "备份完成") {
				// 提取时间戳
				if timeStr := extractTimestampFromLog(line); timeStr != "" {
					if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
						lastBackupTime = t
						success = true
						break
					}
				}
			} else if strings.Contains(line, "[error]") && strings.Contains(line, "备份失败") {
				if timeStr := extractTimestampFromLog(line); timeStr != "" {
					if t, err := time.Parse("2006-01-02 15:04:05", timeStr); err == nil {
						lastBackupTime = t
						success = false
						break
					}
				}
			}
		}

		if !lastBackupTime.IsZero() {
			return lastBackupTime, success
		}
	}

	return time.Time{}, false
}

// 从日志行提取时间戳
func extractTimestampFromLog(logLine string) string {
	// 假设日志格式为: [2025-10-30 14:30:15] [info] 备份完成
	if strings.HasPrefix(logLine, "[") {
		if idx := strings.Index(logLine, "]"); idx > 0 {
			return logLine[1:idx]
		}
	}
	return ""
}

// 获取服务状态文本
func getServiceStatusText(status ServiceStatus) string {
	if status.Running {
		return fmt.Sprintf("● 运行中 (PID: %d)", status.PID)
	}
	return "○ 已停止"
}

// 获取备份状态文本
func getBackupStatusText(success bool) string {
	if success {
		return "成功"
	}
	return "失败"
}

// 显示详细状态信息
func showDetailedStatus() {
	status := getCurrentSystemStatus()

	fmt.Printf("🖥️  系统信息:\n")
	fmt.Printf("   操作系统: %s\n", runtime.GOOS)
	fmt.Printf("   架构: %s\n", runtime.GOARCH)
	fmt.Printf("   程序路径: %s\n", getCurrentExecutablePath())
	fmt.Printf("   工作目录: %s\n", getCurrentWorkingDir())

	fmt.Printf("\n🔧 服务状态:\n")
	if status.ServiceRunning {
		fmt.Printf("   运行状态: ● 运行中\n")
		fmt.Printf("   进程PID: %d\n", status.ServicePID)
	} else {
		fmt.Printf("   运行状态: ○ 已停止\n")
	}
	fmt.Printf("   开机自启: %s\n", getAutoStartStatusText())

	if status.ConfigLoaded {
		fmt.Printf("\n📁 备份配置:\n")
		fmt.Printf("   数据目录: %s\n", status.DataDir)
		fmt.Printf("   COS桶名: %s\n", status.Bucket)
		fmt.Printf("   存储前缀: %s/\n", status.Prefix)

		fmt.Printf("\n⏰ 备份统计:\n")
		if status.ScheduleEnabled {
			fmt.Printf("   定时任务: ✅ 已启用\n")
			if !status.NextBackup.IsZero() {
				fmt.Printf("   下次备份: %s\n", status.NextBackup.Format("2006-01-02 15:04:05"))
			}
		} else {
			fmt.Printf("   定时任务: ○ 已禁用\n")
		}

		if !status.LastBackup.IsZero() {
			fmt.Printf("   上次备份: %s (%s)\n",
				status.LastBackup.Format("2006-01-02 15:04:05"),
				getBackupStatusText(status.LastBackupSuccess))
		} else {
			fmt.Printf("   上次备份: 暂无记录\n")
		}
	} else {
		fmt.Printf("\n⚠️  配置状态: ❌ 未加载或存在错误\n")
	}

	// 显示存储信息
	showStorageInfo()
}

// 显示存储信息
func showStorageInfo() {
	fmt.Printf("\n🗂️  存储信息:\n")

	// 临时目录信息
	if files, err := os.ReadDir(TempDir); err == nil {
		fmt.Printf("   临时目录: %s (%d个文件)\n", TempDir, len(files))
	} else {
		fmt.Printf("   临时目录: %s (无法读取)\n", TempDir)
	}

	// 日志目录信息
	if files, err := os.ReadDir(LogDir); err == nil {
		var logSize int64
		logCount := 0
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".log") {
				logCount++
				if info, err := file.Info(); err == nil {
					logSize += info.Size()
				}
			}
		}
		fmt.Printf("   日志目录: %s (%d个文件, %s)\n",
			LogDir, logCount, formatBytes(int64(logSize)))
	} else {
		fmt.Printf("   日志目录: %s (无法读取)\n", LogDir)
	}
}

// 获取当前可执行文件路径
func getCurrentExecutablePath() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "unknown"
}

// 获取当前工作目录
func getCurrentWorkingDir() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "unknown"
}

// 格式化字节数
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}