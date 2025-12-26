package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// 服务管理结构体
type ServiceManager interface {
	Install() error
	Start() error
	Stop() error
	Restart() error
	Status() ServiceStatus
	Uninstall() error
}

// 服务状态
type ServiceStatus struct {
	Running    bool
	PID        int
	AutoStart  bool
	Installed  bool
	LastError  error
}

// 获取当前系统的服务管理器
func getServiceManager() ServiceManager {
	switch runtime.GOOS {
	case "darwin":
		return &MacOSServiceManager{}
	case "linux":
		return &LinuxServiceManager{}
	case "windows":
		return &WindowsServiceManager{}
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
	m.programPath = getCurrentExecutablePath()
}

func (m *MacOSServiceManager) Install() error {
	m.init()

	printLog("info", "正在安装 macOS LaunchAgent 服务...")

	// 确保 LaunchAgents 目录存在
	launchAgentsDir := filepath.Dir(m.plistPath)
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("创建 LaunchAgents 目录失败: %w", err)
	}

	// 生成 plist 文件内容
	plistContent := m.generatePlistContent()

	// 写入 plist 文件
	if err := os.WriteFile(m.plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("写入 plist 文件失败: %w", err)
	}

	// 加载服务
	if err := exec.Command("launchctl", "load", m.plistPath).Run(); err != nil {
		return fmt.Errorf("加载 LaunchAgent 失败: %w", err)
	}

	printLog("info", "✅ macOS LaunchAgent 服务安装成功")
	return nil
}

func (m *MacOSServiceManager) Start() error {
	m.init()

	printLog("info", "正在启动服务...")
	if err := exec.Command("launchctl", "start", m.serviceName).Run(); err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}

	printLog("info", "✅ 服务启动成功")
	return nil
}

func (m *MacOSServiceManager) Stop() error {
	m.init()

	printLog("info", "正在停止服务...")
	if err := exec.Command("launchctl", "stop", m.serviceName).Run(); err != nil {
		return fmt.Errorf("停止服务失败: %w", err)
	}

	printLog("info", "✅ 服务停止成功")
	return nil
}

func (m *MacOSServiceManager) Restart() error {
	if err := m.Stop(); err != nil {
		return err
	}
	// 等待服务完全停止
	exec.Command("sleep", "5").Run()
	return m.Start()
}

func (m *MacOSServiceManager) Status() ServiceStatus {
	m.init()

	status := ServiceStatus{Installed: false, Running: false}

	// 检查 plist 文件是否存在
	if _, err := os.Stat(m.plistPath); err == nil {
		status.Installed = true
	}

	// 使用 launchctl list 检查服务状态
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

	status.AutoStart = status.Installed // launchctl 加载即表示自启
	return status
}

func (m *MacOSServiceManager) Uninstall() error {
	m.init()

	printLog("info", "正在卸载 macOS LaunchAgent 服务...")

	// 停止服务
	m.Stop()

	// 卸载服务
	if err := exec.Command("launchctl", "unload", m.plistPath).Run(); err != nil {
		printLog("warn", "卸载服务失败: "+err.Error())
	}

	// 删除 plist 文件
	if err := os.Remove(m.plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 plist 文件失败: %w", err)
	}

	printLog("info", "✅ macOS LaunchAgent 服务卸载成功")
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
        <string>--daemon</string>
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
	m.programPath = getCurrentExecutablePath()
}

func (m *LinuxServiceManager) Install() error {
	m.init()

	printLog("info", "正在安装 systemd 用户服务...")

	// 确保服务目录存在
	serviceDir := filepath.Dir(m.serviceFile)
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("创建 systemd 服务目录失败: %w", err)
	}

	// 生成服务文件内容
	serviceContent := m.generateServiceContent()

	// 写入服务文件
	if err := os.WriteFile(m.serviceFile, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("写入服务文件失败: %w", err)
	}

	// 重新加载 systemd
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("重新加载 systemd 失败: %w", err)
	}

	// 启用服务
	if err := exec.Command("systemctl", "--user", "enable", m.serviceName).Run(); err != nil {
		return fmt.Errorf("启用服务失败: %w", err)
	}

	printLog("info", "✅ systemd 用户服务安装成功")
	return nil
}

func (m *LinuxServiceManager) Start() error {
	m.init()

	printLog("info", "正在启动服务...")
	if err := exec.Command("systemctl", "--user", "start", m.serviceName).Run(); err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}

	printLog("info", "✅ 服务启动成功")
	return nil
}

func (m *LinuxServiceManager) Stop() error {
	m.init()

	printLog("info", "正在停止服务...")
	if err := exec.Command("systemctl", "--user", "stop", m.serviceName).Run(); err != nil {
		return fmt.Errorf("停止服务失败: %w", err)
	}

	printLog("info", "✅ 服务停止成功")
	return nil
}

func (m *LinuxServiceManager) Restart() error {
	return exec.Command("systemctl", "--user", "restart", m.serviceName).Run()
}

func (m *LinuxServiceManager) Status() ServiceStatus {
	m.init()

	status := ServiceStatus{Installed: false, Running: false}

	// 检查服务文件是否存在
	if _, err := os.Stat(m.serviceFile); err == nil {
		status.Installed = true
	}

	// 检查服务是否启用
	if output, err := exec.Command("systemctl", "--user", "is-enabled", m.serviceName).Output(); err == nil {
		status.AutoStart = strings.TrimSpace(string(output)) == "enabled"
	}

	// 检查服务是否运行
	if output, err := exec.Command("systemctl", "--user", "is-active", m.serviceName).Output(); err == nil {
		if strings.TrimSpace(string(output)) == "active" {
			status.Running = true

			// 获取 PID
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

	printLog("info", "正在卸载 systemd 用户服务...")

	// 停用并停止服务
	exec.Command("systemctl", "--user", "disable", m.serviceName).Run()
	m.Stop()

	// 删除服务文件
	if err := os.Remove(m.serviceFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除服务文件失败: %w", err)
	}

	// 重新加载 systemd
	exec.Command("systemctl", "--user", "daemon-reload").Run()

	printLog("info", "✅ systemd 用户服务卸载成功")
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
ExecStart=` + m.programPath + ` --daemon
WorkingDirectory=` + workDir + `
Restart=always
RestartSec=5
StandardOutput=file:` + filepath.Join(logDir, "daemon.log") + `
StandardError=file:` + filepath.Join(logDir, "daemon-error.log") + `

[Install]
WantedBy=default.target`
}

// Windows 服务管理器 (使用 sc)
type WindowsServiceManager struct {
	serviceName string
	programPath string
}

func (m *WindowsServiceManager) init() {
	m.serviceName = "BackupGo"
	m.programPath = getCurrentExecutablePath()
}

func (m *WindowsServiceManager) Install() error {
	m.init()

	printLog("info", "正在安装 Windows 服务...")

	// 创建服务命令
	cmd := exec.Command("sc", "create", m.serviceName,
		"binPath=", m.programPath+" --daemon",
		"start=", "auto",
		"displayname=", "Backup-Go COS Backup Service")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("创建 Windows 服务失败: %w", err)
	}

	printLog("info", "✅ Windows 服务安装成功")
	return nil
}

func (m *WindowsServiceManager) Start() error {
	m.init()

	printLog("info", "正在启动服务...")
	if err := exec.Command("sc", "start", m.serviceName).Run(); err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}

	printLog("info", "✅ 服务启动成功")
	return nil
}

func (m *WindowsServiceManager) Stop() error {
	m.init()

	printLog("info", "正在停止服务...")
	if err := exec.Command("sc", "stop", m.serviceName).Run(); err != nil {
		return fmt.Errorf("停止服务失败: %w", err)
	}

	printLog("info", "✅ 服务停止成功")
	return nil
}

func (m *WindowsServiceManager) Restart() error {
	if err := m.Stop(); err != nil {
		return err
	}
	return m.Start()
}

func (m *WindowsServiceManager) Status() ServiceStatus {
	m.init()

	status := ServiceStatus{Installed: false, Running: false}

	// 查询服务状态
	cmd := exec.Command("sc", "query", m.serviceName)
	output, err := cmd.Output()
	if err != nil {
		return status
	}

	outputStr := string(output)
	status.Installed = true

	if strings.Contains(outputStr, "RUNNING") {
		status.Running = true
		status.AutoStart = true // Windows 服务通常是自动启动

		// 尝试获取PID
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "PID") {
				fields := strings.Fields(line)
				for i, field := range fields {
					if field == "PID" && i+1 < len(fields) {
						if pid, err := strconv.Atoi(fields[i+1]); err == nil {
							status.PID = pid
						}
					}
				}
			}
		}
	}

	return status
}

func (m *WindowsServiceManager) Uninstall() error {
	m.init()

	printLog("info", "正在卸载 Windows 服务...")

	// 停止服务
	m.Stop()

	// 删除服务
	if err := exec.Command("sc", "delete", m.serviceName).Run(); err != nil {
		return fmt.Errorf("删除 Windows 服务失败: %w", err)
	}

	printLog("info", "✅ Windows 服务卸载成功")
	return nil
}

// 通用服务管理器 (不支持系统服务的平台)
type GenericServiceManager struct{}

func (m *GenericServiceManager) Install() error {
	return fmt.Errorf("当前平台不支持系统服务安装")
}

func (m *GenericServiceManager) Start() error {
	return fmt.Errorf("当前平台不支持系统服务启动")
}

func (m *GenericServiceManager) Stop() error {
	return fmt.Errorf("当前平台不支持系统服务停止")
}

func (m *GenericServiceManager) Restart() error {
	return fmt.Errorf("当前平台不支持系统服务重启")
}

func (m *GenericServiceManager) Status() ServiceStatus {
	return ServiceStatus{Installed: false, Running: false}
}

func (m *GenericServiceManager) Uninstall() error {
	return fmt.Errorf("当前平台不支持系统服务卸载")
}


func getServiceStatus() ServiceStatus {
	manager := getServiceManager()
	return manager.Status()
}

func getAutoStartStatusText() string {
	status := getServiceStatus()
	if status.AutoStart {
		return "✅ 已启用"
	}
	return "○ 已禁用"
}