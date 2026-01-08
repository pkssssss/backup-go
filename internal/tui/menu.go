package tui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"backup-go/internal/config"
	"backup-go/internal/logger"
	"backup-go/internal/service"
	"backup-go/internal/task"
)

// ShowMenu 显示主菜单
func ShowMenu(cfgPath string) {
	for {
		clearScreen()
		fmt.Println("Backup-Go 腾讯云 COS 备份工具 (Refactored)")
		fmt.Println(strings.Repeat("=", 60))
		ShowSystemStatus(cfgPath)
		fmt.Println(strings.Repeat("=", 60))

		fmt.Println("请选择操作:")
		fmt.Println("  1. 🎯 立即备份")
		fmt.Println("  2. 🔧 配置管理")
		fmt.Println("  3. 📋 服务管理")
		fmt.Println("  4. 📝 日志管理")
		fmt.Println("  0. ❌ 退出")

		choice := getUserInput("请输入选项: ")

		switch choice {
		case "1":
			handleImmediateBackup(cfgPath)
		case "2":
			handleConfigMenu(cfgPath)
		case "3":
			handleServiceMenu()
		case "4":
			handleLogMenu()
		case "0", "q", "exit":
			logger.PrintLog("info", "退出程序")
			os.Exit(0)
		default:
			fmt.Println("无效选项，请重试")
			pauseForKey()
		}
	}
}

func handleImmediateBackup(cfgPath string) {
	clearScreen()
	fmt.Println("🎯 立即备份")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Printf("❌ 加载配置失败: %v\n", err)
		pauseForKey()
		return
	}

	fmt.Println("正在执行备份...")
	if err := task.RunBackup(cfg); err != nil {
		fmt.Printf("❌ 备份失败: %v\n", err)
	} else {
		fmt.Println("✅ 备份成功完成")
	}
	pauseForKey()
}

func handleConfigMenu(cfgPath string) {
	for {
		clearScreen()
		fmt.Println("🔧 配置管理")
		fmt.Println("  1. 查看配置文件")
		fmt.Println("  2. 编辑配置文件")
		fmt.Println("  3. 测试 COS 连接")
		fmt.Println("  4. 生成默认配置 (覆盖)")
		fmt.Println("  0. 返回上一级")

		switch getUserInput("选项: ") {
		case "1":
			content, err := os.ReadFile(cfgPath)
			if err != nil {
				fmt.Printf("读取失败: %v\n", err)
			} else {
				fmt.Println(string(content))
			}
			pauseForKey()
		case "2":
			editConfigFile(cfgPath)
		case "3":
			CheckConfigAndTestCOS(cfgPath)
			pauseForKey()
		case "4":
			if getUserInput("确认覆盖? (y/n): ") == "y" {
				config.GenerateDefaultConfig(cfgPath)
				fmt.Println("已生成默认配置")
			}
			pauseForKey()
		case "0":
			return
		}
	}
}

func handleServiceMenu() {
	svc := service.GetServiceManager()
	for {
		clearScreen()
		status := svc.Status()
		state := "已停止"
		if status.Running {
			state = fmt.Sprintf("运行中 (PID: %d)", status.PID)
		}
		auto := "禁用"
		if status.AutoStart {
			auto = "启用"
		}

		fmt.Println("📋 服务管理")
		fmt.Printf("当前状态: %s | 开机自启: %s\n", state, auto)
		fmt.Println("  1. 安装服务 (开机自启)")
		fmt.Println("  2. 卸载服务")
		fmt.Println("  3. 启动服务")
		fmt.Println("  4. 停止服务")
		fmt.Println("  5. 重启服务")
		fmt.Println("  0. 返回上一级")

		switch getUserInput("选项: ") {
		case "1":
			if err := svc.Install(); err != nil {
				fmt.Printf("安装失败: %v\n", err)
			}
			pauseForKey()
		case "2":
			if err := svc.Uninstall(); err != nil {
				fmt.Printf("卸载失败: %v\n", err)
			}
			pauseForKey()
		case "3":
			if err := svc.Start(); err != nil {
				fmt.Printf("启动失败: %v\n", err)
			}
			pauseForKey()
		case "4":
			if err := svc.Stop(); err != nil {
				fmt.Printf("停止失败: %v\n", err)
			}
			pauseForKey()
		case "5":
			if err := svc.Restart(); err != nil {
				fmt.Printf("重启失败: %v\n", err)
			}
			pauseForKey()
		case "0":
			return
		}
	}
}

func handleLogMenu() {
	// 简化版：只显示最新日志
	clearScreen()
	fmt.Println("📝 最新日志 (最后 20 行)")
	// 这里假设日志就在 logs 目录
	logDir := "logs"
	entries, _ := os.ReadDir(logDir)
	var latest string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			latest = filepath.Join(logDir, e.Name())
		}
	}
	if latest == "" {
		fmt.Println("暂无日志")
	} else {
		cmd := exec.Command("tail", "-n", "20", latest)
		cmd.Stdout = os.Stdout
		cmd.Run()
	}
	pauseForKey()
}

// editConfigFile 调用系统编辑器
func editConfigFile(cfgPath string) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "nano" // 默认 nano
		}
	}
	cmd := exec.Command(editor, cfgPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("打开编辑器失败: %v\n", err)
	}
}

func clearScreen() {
	fmt.Print("\033[H\033[2J") // ANSI clear screen
}

func getUserInput(prompt string) string {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func pauseForKey() {
	fmt.Println("\n按 Enter 继续...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}
