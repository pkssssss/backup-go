package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"backup-go/internal/logger"
	"backup-go/internal/utils"
)

// ServiceManager 服务管理接口
type ServiceManager interface {
	Install() error
	Start() error
	Stop() error
	Restart() error
	Status() ServiceStatus
	Uninstall() error
}

// ServiceStatus 服务状态
type ServiceStatus struct {
	Running    bool
	PID        int
	AutoStart  bool
	Installed  bool
	LastError  error
}

// GetServiceManager 获取当前系统的服务管理器
func GetServiceManager() ServiceManager {
	switch runtime.GOOS {
	case "darwin":
		return &MacOSServiceManager{}
	case "linux":
		return &LinuxServiceManager{}
	default:
		return &GenericServiceManager{}
	}
}

// macOS 服务管理器 (使用 launchd)
type MacOSServiceManager struct {
	serviceName string
	plistPath   string
	programPath string
}

func (m *MacOSServiceManager) init() {
	m.serviceName = "com.backup-go.daemon"
	homeDir, _ := os.UserHomeDir()
	m.plistPath = filepath.Join(homeDir, "Library", "LaunchAgents", m.serviceName+".plist")
	m.programPath = utils.GetCurrentExecutablePath()
}

func (m *MacOSServiceManager) Install() error {
	m.init()
	logger.PrintLog("info", "正在安装 macOS LaunchAgent 服务...")

	launchAgentsDir := filepath.Dir(m.plistPath)
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("创建 LaunchAgents 目录失败: %w", err)
	}

	plistContent := m.generatePlistContent()
	if err := os.WriteFile(m.plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("写入 plist 文件失败: %w", err)
	}

	// 尝试卸载旧的以防冲突
	exec.Command("launchctl", "unload", m.plistPath).Run()

	if err := exec.Command("launchctl", "load", m.plistPath).Run(); err != nil {
		return fmt.Errorf("加载 LaunchAgent 失败: %w", err)
	}

	logger.PrintLog("info", "✅ macOS LaunchAgent 服务安装成功")
	return nil
}

func (m *MacOSServiceManager) Start() error {
	m.init()
	logger.PrintLog("info", "正在启动服务...")
	if err := exec.Command("launchctl", "start", m.serviceName).Run(); err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}
	logger.PrintLog("info", "✅ 服务启动成功")
	return nil
}

func (m *MacOSServiceManager) Stop() error {
	m.init()
	logger.PrintLog("info", "正在停止服务...")
	if err := exec.Command("launchctl", "stop", m.serviceName).Run(); err != nil {
		return fmt.Errorf("停止服务失败: %w", err)
	}
	logger.PrintLog("info", "✅ 服务停止成功")
	return nil
}

func (m *MacOSServiceManager) Restart() error {
	if err := m.Stop(); err != nil {
		return err
	}
	exec.Command("sleep", "2").Run()
	return m.Start()
}

func (m *MacOSServiceManager) Status() ServiceStatus {
	m.init()
	status := ServiceStatus{Installed: false, Running: false}

	if _, err := os.Stat(m.plistPath); err == nil {
		status.Installed = true
	}

	cmd := exec.Command("launchctl", "list")
	output, err := cmd.Output()
	if err != nil {
		status.LastError = err
		return status
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, m.serviceName) {
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				if pid, err := strconv.Atoi(fields[0]); err == nil && pid > 0 {
					status.Running = true
					status.PID = pid
				}
			}
			break
		}
	}
	status.AutoStart = status.Installed
	return status
}

func (m *MacOSServiceManager) Uninstall() error {
	m.init()
	logger.PrintLog("info", "正在卸载 macOS LaunchAgent 服务...")
	m.Stop()
	if err := exec.Command("launchctl", "unload", m.plistPath).Run(); err != nil {
		logger.PrintLog("warn", "卸载服务失败: "+err.Error())
	}
	if err := os.Remove(m.plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 plist 文件失败: %w", err)
	}
	logger.PrintLog("info", "✅ macOS LaunchAgent 服务卸载成功")
	return nil
}

func (m *MacOSServiceManager) generatePlistContent() string {
	workDir := filepath.Dir(m.programPath)
	logDir := filepath.Join(workDir, "logs")

	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>` + m.serviceName + `</string>
    <key>ProgramArguments</key>
    <array>
        <string>` + m.programPath + `</string>
        <string>server</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>` + workDir + `</string>
    <key>StandardOutPath</key>
    <string>` + filepath.Join(logDir, "daemon.log") + `</string>
    <key>StandardErrorPath</key>
    <string>` + filepath.Join(logDir, "daemon-error.log") + `</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin</string>
    </dict>
    <key>ProcessType</key>
    <string>Background</string>
</dict>
</plist>`
}

// Linux 服务管理器 (使用 systemd)
type LinuxServiceManager struct {
	serviceName string
	serviceFile string
	programPath string
}

func (m *LinuxServiceManager) init() {
	m.serviceName = "backup-go"
	homeDir, _ := os.UserHomeDir()
	m.serviceFile = filepath.Join(homeDir, ".config", "systemd", "user", m.serviceName+".service")
	m.programPath = utils.GetCurrentExecutablePath()
}

func (m *LinuxServiceManager) Install() error {
	m.init()
	logger.PrintLog("info", "正在安装 systemd 用户服务...")

	serviceDir := filepath.Dir(m.serviceFile)
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("创建 systemd 服务目录失败: %w", err)
	}

	serviceContent := m.generateServiceContent()
	if err := os.WriteFile(m.serviceFile, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("写入服务文件失败: %w", err)
	}

	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("重新加载 systemd 失败: %w", err)
	}
	if err := exec.Command("systemctl", "--user", "enable", m.serviceName).Run(); err != nil {
		return fmt.Errorf("启用服务失败: %w", err)
	}

	logger.PrintLog("info", "✅ systemd 用户服务安装成功")
	return nil
}

func (m *LinuxServiceManager) Start() error {
	m.init()
	logger.PrintLog("info", "正在启动服务...")
	if err := exec.Command("systemctl", "--user", "start", m.serviceName).Run(); err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}
	logger.PrintLog("info", "✅ 服务启动成功")
	return nil
}

func (m *LinuxServiceManager) Stop() error {
	m.init()
	logger.PrintLog("info", "正在停止服务...")
	if err := exec.Command("systemctl", "--user", "stop", m.serviceName).Run(); err != nil {
		return fmt.Errorf("停止服务失败: %w", err)
	}
	logger.PrintLog("info", "✅ 服务停止成功")
	return nil
}

func (m *LinuxServiceManager) Restart() error {
	m.init()
	return exec.Command("systemctl", "--user", "restart", m.serviceName).Run()
}

func (m *LinuxServiceManager) Status() ServiceStatus {
	m.init()
	status := ServiceStatus{Installed: false, Running: false}

	if _, err := os.Stat(m.serviceFile); err == nil {
		status.Installed = true
	}

	if output, err := exec.Command("systemctl", "--user", "is-enabled", m.serviceName).Output(); err == nil {
		status.AutoStart = strings.TrimSpace(string(output)) == "enabled"
	}

	if output, err := exec.Command("systemctl", "--user", "is-active", m.serviceName).Output(); err == nil {
		if strings.TrimSpace(string(output)) == "active" {
			status.Running = true
			if showOutput, err := exec.Command("systemctl", "--user", "show", m.serviceName, "--property=MainPID").Output(); err == nil {
				lines := strings.Split(string(showOutput), "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "MainPID=") {
						if pidStr := strings.TrimPrefix(line, "MainPID="); pidStr != "" {
							if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
								status.PID = pid
							}
						}
					}
				}
			}
		}
	}
	return status
}

func (m *LinuxServiceManager) Uninstall() error {
	m.init()
	logger.PrintLog("info", "正在卸载 systemd 用户服务...")
	exec.Command("systemctl", "--user", "disable", m.serviceName).Run()
	m.Stop()
	if err := os.Remove(m.serviceFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除服务文件失败: %w", err)
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	logger.PrintLog("info", "✅ systemd 用户服务卸载成功")
	return nil
}

func (m *LinuxServiceManager) generateServiceContent() string {
	workDir := filepath.Dir(m.programPath)
	logDir := filepath.Join(workDir, "logs")

	return `[Unit]
Description=Backup-Go COS Backup Service
After=network.target

[Service]
Type=simple
ExecStart=` + m.programPath + ` server
WorkingDirectory=` + workDir + `
Restart=always
RestartSec=5
StandardOutput=file:` + filepath.Join(logDir, "daemon.log") + `
StandardError=file:` + filepath.Join(logDir, "daemon-error.log") + `

[Install]
WantedBy=default.target`
}

// 通用服务管理器 (不支持系统服务的平台)
type GenericServiceManager struct{}

func (m *GenericServiceManager) Install() error { return fmt.Errorf("当前平台不支持系统服务安装") }
func (m *GenericServiceManager) Start() error { return fmt.Errorf("当前平台不支持系统服务启动") }
func (m *GenericServiceManager) Stop() error { return fmt.Errorf("当前平台不支持系统服务停止") }
func (m *GenericServiceManager) Restart() error { return fmt.Errorf("当前平台不支持系统服务重启") }
func (m *GenericServiceManager) Status() ServiceStatus { return ServiceStatus{Installed: false, Running: false} }
func (m *GenericServiceManager) Uninstall() error { return fmt.Errorf("当前平台不支持系统服务卸载") }
