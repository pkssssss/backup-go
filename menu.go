package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/tencentyun/cos-go-sdk-v5"
)

// 常量定义 - 减少硬编码字符串
const (
	// 菜单标题
	TitleConfig     = "🔧 配置管理"
	TitleService    = "📋 服务管理"
	TitleLog        = "📝 日志管理"
	TitleSchedule   = "🕐 定时任务"
	TitleStatus     = "📊 状态查看"

	// 错误消息
	ErrConfigNotFound    = "配置文件不存在，请先配置"
	ErrLoadConfigFailed  = "加载配置失败"
	ErrCOSNotConfigured  = "腾讯云COS认证信息未配置"
	ErrServiceFailed     = "操作失败"
	ErrInvalidInput      = "输入无效"
	ErrOperationFailed  = "操作失败"

	// 成功消息
	SuccessOperationDone = "操作完成！"
	SuccessConfigSaved   = "配置保存成功"
	SuccessServiceStarted = "服务启动成功"
	SuccessServiceStopped = "服务停止成功"
	SuccessBackupDone    = "备份完成"

	// 提示消息
	PromptContinue = "按 Enter 继续..."
	PromptOption   = "请输入选项: "
	PromptRetry    = "是否重试？[y/N]: "

	// 常见错误处理建议
	SuggestCheckConfig       = "请检查配置文件"
	SuggestCheckCosConfig    = "请验证腾讯云COS配置"
	SuggestCheckDir         = "请检查目录权限和路径"
	SuggestTryAgain        = "请稍后重试操作"
	SuggestCheckFilePermission = "检查文件权限和路径"
	SuggestCheckConfigAndNetwork = "检查配置文件和网络连接"

	// 应用程序信息
	AppName     = "Backup-Go 腾讯云 COS 备份工具"
	AppVersion  = "v1.0"
	AppFullName = "🚀 " + AppName + " " + AppVersion

	// 菜单选项文本
	MenuOptionImmediateBackup = "  1. 🎯 立即备份"
	MenuOptionConfig         = "  2. 🔧 配置管理"
	MenuOptionService        = "  3. 📋 服务管理"
	MenuOptionLog            = "  4. 📝 日志管理"
	MenuOptionSchedule       = "  5. 🕐 定时任务"
	MenuOptionStatus         = "  6. 📊 状态查看"
	MenuOptionExit           = "  7. ❌ 退出管理"

	// 状态文本
	StatusConfigExists    = "✅ 配置文件存在"
	StatusConfigMissing   = "❌ 配置文件不存在"
	StatusConfigValid     = "✅ 有效"
	StatusConfigInvalid   = "❌ 解析错误"
	StatusCosConfigured   = "✅ 已配置"
	StatusCosNotConfigured = "❌ 未配置"
	StatusServiceRunning  = "✅ 运行中"
	StatusServiceStopped  = "❌ 未运行"

	// 操作提示
	PromptEnterOption = "请输入选项 [1-7]: "
	ConfirmResetConfig = "确认重置？这将丢失当前配置 [y/N]: "
	ConfirmUninstallService = "确认卸载？这将删除开机自启设置 [y/N]: "

	// 功能描述
	DescConfigManagement = "  • 配置管理: 设置COS认证信息和备份路径"
	DescServiceManagement = "  • 服务管理: 安装开机自启和后台服务"
	DescLogManagement    = "  • 日志管理: 查看操作日志和故障排除"
	DescScheduleManagement = "  • 定时任务: 配置自动备份计划"
	DescStatusDetection  = "  • 状态检测: 主菜单顶部显示实时状态"

	// 状态标识符 - 用于状态检查
	StatusScheduleEnabled  = "✅ 定时任务已启用"
	StatusServiceRunningFlag = true
)

// 显示主交互菜单
func showInteractiveMenu() {
	// 检查是否为交互模式
	stat, _ := os.Stdin.Stat()
	isInteractive := (stat.Mode() & os.ModeCharDevice) != 0

	if !isInteractive {
		printLog("info", "检测到非交互模式，直接执行备份...")
		handleNonInteractiveMode()
		return
	}

	// 交互模式
	for {
		clearScreen()
		showMainMenuHeader()
		showMainMenuOptions()

		choice := getUserInput(PromptEnterOption)

		// 如果获取不到输入，退出程序
		if choice == "" {
			printLog("info", "无法获取用户输入，退出程序")
			break
		}

		switch choice {
		case "1":
			handleImmediateBackup()
		case "2":
			handleConfigMenu()
		case "3":
			handleServiceMenu()
		case "4":
			handleLogMenu()
		case "5":
			handleScheduleMenu()
		case "6":
			showBasicStatus()
		case "7", "0", "q", "Q", "quit", "exit":
			// 获取实际状态
			running, _ := getServiceRunningStatus()
			scheduleStatus := checkScheduleStatus()

			// 根据实际状态显示不同的退出信息
			printLog("info", "感谢使用 Backup-Go！")

			if running {
				printLog("info", "✅ 后台服务正在运行中")
				if strings.Contains(scheduleStatus.text, StatusScheduleEnabled) {
					printLog("info", "⏰ 定时备份任务已启用，将自动执行")
				} else {
					printLog("info", "⚠️  定时备份任务未启用，仅服务运行中")
				}
				printLog("info", "💡 可随时重新运行程序进行管理")
			} else {
				printLog("info", "⚠️  后台服务未运行")
				if strings.Contains(scheduleStatus.text, StatusScheduleEnabled) {
					printLog("info", "⚠️  定时备份任务已配置但服务未启动")
					printLog("info", "💡 请启动服务以启用定时备份功能")
				} else {
					printLog("info", "💡 所有服务已停止，可随时重新启动")
				}
			}
			os.Exit(0)
		default:
			printLog("warn", "无效选项，请重新选择")
			pauseForKey()
		}
	}
}

// 非交互模式处理
func handleNonInteractiveMode() {
	printLog("info", "正在执行一次性备份...")

	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if !fileExists(cfgPath) {
		printLog("error", "配置文件不存在，请先在交互模式下配置")
		os.Exit(1)
	}

	cfg, err := loadConfig(cfgPath)
	exitIfError(err, "加载配置失败")

	if cfg.Cos.SecretID == "" || cfg.Cos.SecretKey == "" {
		printLog("error", "腾讯云 COS 认证信息未配置")
		os.Exit(1)
	}

	client, err := createCOSClient(cfg)
	exitIfError(err, "创建COS客户端失败")

	performOneTimeBackup(cfg, client)
	printLog("info", "备份完成，程序退出")
}

// 显示主菜单头部信息
func showMainMenuHeader() {
	fmt.Println(AppFullName)
	fmt.Println(strings.Repeat("=", 60))

	// 显示详细状态检测栏
	fmt.Println("📊 系统状态检测:")
	showSystemStatus()

	fmt.Println(strings.Repeat("=", 60))
}

// 显示详细系统状态
func showSystemStatus() {
	// 综合配置和COS状态
	configCosStatus := checkConfigAndCOSStatus()
	serviceStatus := checkServiceStatus()

	// 第二行：定时任务和开机自启
	scheduleStatus := checkScheduleStatus()
	autoStartStatus := checkAutoStartStatus()

	// 计算第一部分的最大长度，用于对齐分隔符
	leftPart1 := fmt.Sprintf("🔧 配置与COS: %s", configCosStatus.text)
	leftPart2 := fmt.Sprintf("⏰ 定时任务: %s", scheduleStatus.text)

	maxLeftLength := len(leftPart1)
	if len(leftPart2) > maxLeftLength {
		maxLeftLength = len(leftPart2)
	}

	// 显示第一行（配置与服务）
	padding1 := strings.Repeat(" ", maxLeftLength-len(leftPart1))
	fmt.Printf("  %s%s | 🔄 服务: %s\n", leftPart1, padding1, serviceStatus.text)

	// 显示第二行（定时任务与开机自启）
	padding2 := strings.Repeat(" ", maxLeftLength-len(leftPart2))
	fmt.Printf("  %s%s | 🚀 开机自启: %s\n", leftPart2, padding2, autoStartStatus.text)

	// 第三行：备份路径状态（无需对齐，因为只有一个部分）
	dataStatus := checkBackupPathStatus()
	fmt.Printf("  📁 备份路径: %s\n", dataStatus.text)
}

// 状态信息结构
type StatusInfo struct {
	text    string
	details string
}

// 检查配置状态
func checkConfigStatus() StatusInfo {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if !fileExists(cfgPath) {
		return StatusInfo{
			text:    "❌ 配置文件不存在",
			details: "请先进行配置管理设置COS认证信息",
		}
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return StatusInfo{
			text:    "❌ 配置文件格式错误",
			details: fmt.Sprintf("解析失败: %v", err),
		}
	}

	if cfg.Cos.SecretID == "" || cfg.Cos.SecretKey == "" {
		return StatusInfo{
			text:    "⚠️  COS认证信息未配置",
			details: "请在配置管理中设置SecretID和SecretKey",
		}
	}

	if cfg.Cos.Bucket == "" || !strings.Contains(cfg.Cos.Bucket, "-") {
		return StatusInfo{
			text:    "⚠️  COS桶名称格式错误",
			details: "桶名应为 'name-appid' 格式",
		}
	}

	return StatusInfo{
		text:    "✅ 配置正常",
		details: fmt.Sprintf("桶: %s, 区域: %s", cfg.Cos.Bucket, cfg.Cos.Region),
	}
}

// 检查服务状态
func checkServiceStatus() StatusInfo {
	running, pid := getServiceRunningStatus()
	if running {
		return StatusInfo{
			text:    "✅ 服务运行中",
			details: fmt.Sprintf("进程PID: %d", pid),
		}
	}
	return StatusInfo{
		text:    "○ 服务已停止",
		details: "可在服务管理中启动后台服务",
	}
}

// 检查定时任务状态
func checkScheduleStatus() StatusInfo {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if !fileExists(cfgPath) {
		return StatusInfo{
			text:    "○ 配置文件不存在",
			details: "",
		}
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return StatusInfo{
			text:    "❌ 读取配置失败",
			details: "",
		}
	}

	if !cfg.Backup.Schedule.Enabled {
		return StatusInfo{
			text:    "○ 定时任务已禁用",
			details: fmt.Sprintf("配置时间: %02d:%02d", cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute),
		}
	}

	nextRun := calculateNextRunTime(cfg.Backup.Schedule)
	if !nextRun.IsZero() {
		duration := time.Until(nextRun)
		return StatusInfo{
			text:    "✅ 定时任务已启用",
			details: fmt.Sprintf("下次执行: %s (距现在 %s)",
				nextRun.Format("15:04:05"),
				formatDuration(duration)),
		}
	}

	return StatusInfo{
		text:    "✅ 定时任务已启用",
		details: fmt.Sprintf("执行时间: 每天 %02d:%02d", cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute),
	}
}

// 检查开机自启状态
func checkAutoStartStatus() StatusInfo {
	manager := getServiceManager()
	status := manager.Status()

	if !status.Installed {
		return StatusInfo{
			text:    "○ 未安装开机自启",
			details: "可在服务管理中安装系统服务",
		}
	}

	if !status.AutoStart {
		return StatusInfo{
			text:    "○ 开机自启已禁用",
			details: "服务已安装但未启用开机自启",
		}
	}

	return StatusInfo{
		text:    "✅ 开机自启已启用",
		details: "系统启动时会自动运行备份服务",
	}
}

// 检查COS连接状态
func checkCOSStatus() StatusInfo {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if !fileExists(cfgPath) {
		return StatusInfo{
			text:    "○ 配置文件不存在",
			details: "",
		}
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil || cfg.Cos.SecretID == "" || cfg.Cos.SecretKey == "" {
		return StatusInfo{
			text:    "○ COS未配置",
			details: "请先配置COS认证信息",
		}
	}

	// 简化检查，不进行实际连接测试以避免启动延迟
	return StatusInfo{
		text:    "✅ COS配置完整",
		details: "可在配置管理中测试连接",
	}
}

// 检查数据目录状态
func checkDataStatus() StatusInfo {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if !fileExists(cfgPath) {
		return StatusInfo{
			text:    "○ 配置文件不存在",
			details: "",
		}
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return StatusInfo{
			text:    "❌ 读取配置失败",
			details: "",
		}
	}

	if !fileExists(cfg.Backup.DataDir) {
		return StatusInfo{
			text:    "❌ 数据目录不存在",
			details: fmt.Sprintf("目录路径: %s", cfg.Backup.DataDir),
		}
	}

	size, err := calculateDirSize(cfg.Backup.DataDir)
	if err != nil {
		return StatusInfo{
			text:    "⚠️  无法计算目录大小",
			details: fmt.Sprintf("目录路径: %s", cfg.Backup.DataDir),
		}
	}

	if size == 0 {
		return StatusInfo{
			text:    "⚠️  数据目录为空",
			details: fmt.Sprintf("目录路径: %s", cfg.Backup.DataDir),
		}
	}

	return StatusInfo{
		text:    "✅ 数据就绪",
		details: fmt.Sprintf("大小: %s, 路径: %s",
			humanize.Bytes(uint64(size)), cfg.Backup.DataDir),
	}
}

// 检查配置和COS连接状态（合并功能）
func checkConfigAndCOSStatus() StatusInfo {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if !fileExists(cfgPath) {
		return StatusInfo{
			text:    "❌ 配置文件不存在",
			details: "请先进行配置管理设置COS认证信息",
		}
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return StatusInfo{
			text:    "❌ 配置文件格式错误",
			details: fmt.Sprintf("解析失败: %v", err),
		}
	}

	if cfg.Cos.SecretID == "" || cfg.Cos.SecretKey == "" {
		return StatusInfo{
			text:    "⚠️  COS认证信息未配置",
			details: "请在配置管理中设置SecretID和SecretKey",
		}
	}

	if cfg.Cos.Bucket == "" || !strings.Contains(cfg.Cos.Bucket, "-") {
		return StatusInfo{
			text:    "⚠️  COS桶名称格式错误",
			details: "桶名应为 'name-appid' 格式",
		}
	}

	// 自动测试COS连接
	client, err := createCOSClient(cfg)
	if err != nil {
		return StatusInfo{
			text:    "⚠️  COS客户端创建失败",
			details: fmt.Sprintf("错误: %v", err),
		}
	}
	if err := testCOSConnection(client, cfg.Cos.Bucket); err != nil {
		return StatusInfo{
			text:    "⚠️  COS连接失败",
			details: fmt.Sprintf("桶: %s, 错误: %v", cfg.Cos.Bucket, err),
		}
	}

	return StatusInfo{
		text:    "✅ 配置正常，COS连接成功",
		details: fmt.Sprintf("桶: %s, 区域: %s", cfg.Cos.Bucket, cfg.Cos.Region),
	}
}

// 检查备份路径状态
func checkBackupPathStatus() StatusInfo {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if !fileExists(cfgPath) {
		return StatusInfo{
			text:    "○ 配置文件不存在",
			details: "",
		}
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return StatusInfo{
			text:    "❌ 读取配置失败",
			details: "",
		}
	}

	backupPath := cfg.Backup.DataDir
	if !fileExists(backupPath) {
		return StatusInfo{
			text:    fmt.Sprintf("❌ 备份路径不存在 (%s)", backupPath),
			details: fmt.Sprintf("目录路径: %s", backupPath),
		}
	}

	size, err := calculateDirSize(backupPath)
	if err != nil {
		return StatusInfo{
			text:    fmt.Sprintf("⚠️  无法读取备份路径 (%s)", backupPath),
			details: fmt.Sprintf("路径: %s, 错误: %v", backupPath, err),
		}
	}

	if size == 0 {
		return StatusInfo{
			text:    fmt.Sprintf("⚠️  备份路径为空 (%s)", backupPath),
			details: fmt.Sprintf("目录路径: %s", backupPath),
		}
	}

	return StatusInfo{
		text:    fmt.Sprintf("✅ 备份路径就绪 (%s)", backupPath),
		details: fmt.Sprintf("大小: %s", humanize.Bytes(uint64(size))),
	}
}

// 显示主菜单选项
func showMainMenuOptions() {
	fmt.Println("请选择操作:")
	fmt.Println(MenuOptionImmediateBackup)
	fmt.Println(MenuOptionConfig)
	fmt.Println(MenuOptionService)
	fmt.Println(MenuOptionLog)
	fmt.Println(MenuOptionSchedule)
	fmt.Println(MenuOptionStatus)
	fmt.Println(MenuOptionExit)
}

// 处理立即备份 - 重构使用通用函数
func handleImmediateBackup() {
	clearScreen()
	fmt.Println("🎯 立即备份")
	fmt.Println(strings.Repeat("-", 30))

	// 使用通用配置加载
	cfg, client, err := loadConfigWithClient()
	if err != nil {
		showErrorAndWait(err, "加载配置失败", "检查配置文件是否存在和格式是否正确")
		return
	}

	fmt.Println("正在执行备份...")
	performOneTimeBackup(cfg, client)

	fmt.Println("\n备份操作完成！")
	pauseForKey()
}

// 简化的状态查看（只显示详细信息，不重复显示状态）
func showBasicStatus() {
	clearScreen()
	fmt.Println("📊 系统状态详情")
	fmt.Println(strings.Repeat("-", 30))

	// 基本系统信息
	fmt.Printf("🖥️  操作系统: %s\n", runtime.GOOS)
	fmt.Printf("⚙️  系统架构: %s\n", runtime.GOARCH)
	fmt.Printf("📁 程序路径: %s\n", getCurrentExecutablePath())

	// 版本信息
	fmt.Printf("🔧 程序版本: Backup-Go v1.0.0\n")

	// Go环境信息
	fmt.Printf("🐹 Go版本: %s\n", runtime.Version())

	fmt.Println()
	fmt.Println("💡 使用提示:")
	fmt.Println(DescConfigManagement)
	fmt.Println(DescServiceManagement)
	fmt.Println(DescLogManagement)
	fmt.Println(DescScheduleManagement)
	fmt.Println(DescStatusDetection)

	pauseForKey()
}

// 工具函数

// 清屏
func clearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// 获取用户输入
func getUserInput(prompt string) string {
	fmt.Print(prompt)

	// 检查标准输入是否可用
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// 标准输入不可用（如后台运行），返回默认值
		fmt.Println("(非交互模式，使用默认选项)")
		return ""  // 返回空字符串，让调用方处理
	}

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("(读取输入失败，使用默认选项)")
		return ""
	}
	return strings.TrimSpace(input)
}

// 等待按键继续
func pauseForKey() {
	fmt.Println("\n按 Enter 继续...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}

// 通用菜单处理器 - 减少重复代码
func handleMenuLoop(title string, options []MenuOption, showExitOption bool) {
	for {
		clearScreen()
		fmt.Printf("%s\n", title)
		fmt.Println(strings.Repeat("-", len(title)))

		// 显示选项
		for _, option := range options {
			fmt.Printf("  %s\n", option.Text)
		}

		if showExitOption {
			fmt.Println("  0. 返回上一级")
		}

		choice := getUserInput("请输入选项: ")

		// 处理选项
		handled := false
		for _, option := range options {
			if option.Key == choice {
				if option.Handler != nil {
					option.Handler()
				}
				handled = true
				break
			}
		}

		// 通用退出选项处理
		if choice == "0" || choice == "q" || choice == "Q" || choice == "quit" || choice == "exit" {
			break
		}

		if !handled {
			printLog("warn", "无效选项，请重新选择")
			pauseForKey()
		}
	}
}

// 菜单选项结构
type MenuOption struct {
	Key     string
	Text    string
	Handler func()
}

// 通用配置加载和验证 - 减少重复代码
func loadAndValidateConfig() (*Config, error) {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if !fileExists(cfgPath) {
		return nil, fmt.Errorf("配置文件不存在: %s", cfgPath)
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	// 验证COS配置
	if cfg.Cos.SecretID == "" || cfg.Cos.SecretKey == "" {
		return nil, fmt.Errorf("腾讯云COS认证信息未配置")
	}

	if cfg.Cos.Bucket == "" || !strings.Contains(cfg.Cos.Bucket, "-") {
		return nil, fmt.Errorf("COS桶名称格式错误，应为'name-appid'格式")
	}

	return cfg, nil
}

// 通用配置加载和COS客户端创建
func loadConfigWithClient() (*Config, *cos.Client, error) {
	cfg, err := loadAndValidateConfig()
	if err != nil {
		return nil, nil, err
	}

	client, err := createCOSClient(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("创建COS客户端失败: %w", err)
	}

	return cfg, client, nil
}

// 统一的错误处理函数 - 提供一致的错误处理体验
func handleError(context string, err error, action string) error {
	if err == nil {
		return nil
	}

	// 构建详细的错误信息
	var errorMsg string
	if context != "" {
		errorMsg = fmt.Sprintf("%s失败: %s", context, err.Error())
	} else {
		errorMsg = fmt.Sprintf("操作失败: %s", err.Error())
	}

	// 记录错误日志
	printLog("error", errorMsg)

	// 如果有指定后续操作，返回错误让调用者处理
	if action != "" {
		return fmt.Errorf("%s: %w", context, err)
	}

	// 默认返回错误
	return err
}

// 显示错误信息并等待用户确认
func showErrorAndWaitBasic(message string) {
	printLog("error", message)
	pauseForKey()
}

// 显示错误信息和建议并等待用户确认
func showErrorAndWait(err error, operation string, suggestions ...string) {
	printLog("error", fmt.Sprintf("%s: %v", operation, err))

	if len(suggestions) > 0 {
		printLog("info", "建议解决方案:")
		for i, suggestion := range suggestions {
			printLog("info", fmt.Sprintf("  %d. %s", i+1, suggestion))
		}
	}

	pauseForKey()
}

// 处理操作并显示结果
func handleOperation(operation string, action func() error) bool {
	err := action()
	if err != nil {
		showErrorAndWait(err, operation)
		return false
	}
	printLog("info", fmt.Sprintf("%s成功", operation))
	return true
}

// 安全错误处理 - 检查错误类型并提供恢复建议
func safeErrorCheck(err error, context string, suggestions ...string) {
	if err == nil {
		return
	}

	printLog("error", fmt.Sprintf("%s错误: %s", context, err.Error()))

	if len(suggestions) > 0 {
		printLog("info", "建议解决方案:")
		for i, suggestion := range suggestions {
			printLog("info", fmt.Sprintf("  %d. %s", i+1, suggestion))
		}
	}
}

// 处理配置菜单
func handleConfigMenu() {
	for {
		clearScreen()
		fmt.Println("🔧 配置管理")
		fmt.Println(strings.Repeat("-", 30))

		// 显示配置状态
		cfgPath := filepath.Join(ConfigDir, ConfigFile)
		configExists := fileExists(cfgPath)

		if configExists {
			fmt.Println("当前配置状态: " + StatusConfigExists)

			// 尝试加载配置并验证
			if cfg, err := loadConfig(cfgPath); err == nil {
				fmt.Println("配置文件格式: " + StatusConfigValid)

				// 验证关键配置项
				if cfg.Cos.SecretID == "" || cfg.Cos.SecretKey == "" {
					fmt.Println("COS认证信息: " + StatusCosNotConfigured)
				} else {
					fmt.Println("COS认证信息: " + StatusCosConfigured)
				}

				if cfg.Cos.Bucket == "" {
					fmt.Println("COS桶名称: " + StatusCosNotConfigured)
				} else {
					fmt.Printf("COS桶名称: ✅ %s\n", cfg.Cos.Bucket)
				}

				if cfg.Backup.DataDir == "" {
					fmt.Println("备份目录: " + StatusCosNotConfigured)
				} else {
					fmt.Printf("备份目录: ✅ %s\n", cfg.Backup.DataDir)
				}
			} else {
				fmt.Println("配置文件格式: " + StatusConfigInvalid)
			}
		} else {
			fmt.Println("当前配置状态: " + StatusConfigMissing)
			fmt.Println("COS连接状态: ❌ 无法测试")
		}

		fmt.Println("\n请选择操作:")
		fmt.Println("  1. 编辑配置文件")
		fmt.Println("  2. 验证配置有效性")
		fmt.Println("  3. 重置为默认配置")
		fmt.Println("  4. 测试 COS 连接")
		fmt.Println("  5. 查看配置文件内容")
		fmt.Println("  0. 返回主菜单")

		choice := getUserInput("请输入选项 [0-5]: ")

		switch choice {
		case "1":
			editConfigFile()
		case "2":
			validateConfig()
		case "3":
			resetConfig()
		case "4":
			testCOSConnectivity()
		case "5":
			viewConfigFile()
		case "0":
			return
		default:
			printLog("warn", "无效选项，请重新选择")
			pauseForKey()
		}
	}
}

// 编辑配置文件
func editConfigFile() {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	printLog("info", "正在打开配置文件编辑器...")

	if err := safeOpenEditor(cfgPath); err != nil {
		if strings.Contains(err.Error(), "Linux系统下请手动编辑") {
			printLog("info", err.Error())
			if editor := os.Getenv("EDITOR"); editor != "" {
				printLog("info", fmt.Sprintf("检测到环境变量 EDITOR=%s，可使用该编辑器", editor))
			}
			printLog("info", "设置环境变量 EDITOR 后可使用编辑器：export EDITOR=nano")
		} else {
			showErrorAndWait(err, "无法打开编辑器",
				"1. 检查文件路径是否正确",
				"2. 确有编辑器权限",
				"3. 手动编辑配置文件: "+cfgPath)
		}
	} else {
		printLog("info", "配置文件编辑完成")
	}
	pauseForKey()
}

// 验证文件路径是否安全
func isSafePath(path string) bool {
	// 规范化路径
	cleanPath := filepath.Clean(path)

	// 检查是否包含危险字符
	dangerousChars := []string{"..", ";", "&", "|", "`", "$", "(", ")", "<", ">", "\"", "'"}
	for _, char := range dangerousChars {
		if strings.Contains(cleanPath, char) {
			return false
		}
	}

	// 检查是否在预期的目录内
	configDirAbs, err := filepath.Abs(ConfigDir)
	if err != nil {
		return false
	}

	fileAbs, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}

	// 确保文件在配置目录内
	return strings.HasPrefix(fileAbs, configDirAbs)
}

// 安全地打开编辑器
func safeOpenEditor(filePath string) error {
	// 验证路径安全性
	if !isSafePath(filePath) {
		return fmt.Errorf("文件路径不安全: %s", filePath)
	}

	// 验证文件存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在: %s", filePath)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Windows: 使用记事本，不传递路径参数
		cmd = exec.Command("notepad", filePath)
	} else if runtime.GOOS == "darwin" {
		// macOS: 使用系统默认编辑器
		cmd = exec.Command("open", "-t", filePath) // -t 表示使用默认文本编辑器
	} else {
		// Linux: 不直接调用编辑器，提示用户手动编辑
		return fmt.Errorf("Linux系统下请手动编辑配置文件: %s", filePath)
	}

	// 设置进程组，防止子进程继承信号
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	return cmd.Run()
}

// 验证配置
func validateConfig() {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		showErrorAndWait(err, "配置文件解析失败", "检查配置文件格式和内容")
		return
	}

	printLog("info", "✅ 配置文件格式正确")

	var hasErrors bool

	// 验证COS配置
	if cfg.Cos.SecretID == "" {
		printLog("warn", "⚠️  腾讯云 COS SecretID 未配置")
		hasErrors = true
	} else {
		printLog("info", "✅ 腾讯云 COS SecretID 已配置")
	}

	if cfg.Cos.SecretKey == "" {
		printLog("warn", "⚠️  腾讯云 COS SecretKey 未配置")
		hasErrors = true
	} else {
		printLog("info", "✅ 腾讯云 COS SecretKey 已配置")
	}

	if cfg.Cos.Region == "" {
		printLog("warn", "⚠️  COS 区域未配置，将使用默认值")
	} else {
		printLog("info", "✅ COS 区域已配置: "+cfg.Cos.Region)
	}

	if cfg.Cos.Bucket == "" {
		printLog("warn", "⚠️  COS 桶名称未配置")
		hasErrors = true
	} else {
		printLog("info", "✅ COS 桶名称已配置: "+cfg.Cos.Bucket)
	}

	if cfg.Cos.Prefix == "" {
		printLog("warn", "⚠️  COS 存储前缀未配置，将使用默认值")
	} else {
		printLog("info", "✅ COS 存储前缀已配置: "+cfg.Cos.Prefix)
	}

	// 验证备份配置
	if cfg.Backup.DataDir == "" {
		printLog("warn", "⚠️  备份目录未配置")
		hasErrors = true
	} else {
		if _, err := os.Stat(cfg.Backup.DataDir); os.IsNotExist(err) {
			printLog("warn", "⚠️  备份目录不存在: "+cfg.Backup.DataDir)
			hasErrors = true
		} else {
			printLog("info", "✅ 备份目录已配置: "+cfg.Backup.DataDir)
		}
	}

	// 验证定时任务配置
	if cfg.Backup.Schedule.Enabled {
		// 验证时间范围
		if cfg.Backup.Schedule.Hour < 0 || cfg.Backup.Schedule.Hour > 23 {
			printLog("warn", "⚠️  定时任务小时配置无效")
			hasErrors = true
		} else if cfg.Backup.Schedule.Minute < 0 || cfg.Backup.Schedule.Minute > 59 {
			printLog("warn", "⚠️  定时任务分钟配置无效")
			hasErrors = true
		} else {
			printLog("info", fmt.Sprintf("✅ 定时任务已配置: %02d:%02d",
				cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute))
		}

		// 验证时区配置
		if cfg.Backup.Schedule.Timezone == "" {
			printLog("warn", "⚠️  定时任务时区未配置，将使用系统时区")
		} else {
			// 验证时区是否有效
			if _, err := time.LoadLocation(cfg.Backup.Schedule.Timezone); err != nil {
				printLog("warn", fmt.Sprintf("⚠️  定时任务时区配置无效: %s", cfg.Backup.Schedule.Timezone))
				printLog("warn", "⚠️  支持的时区格式: Asia/Shanghai, America/New_York, Europe/London 等")
				hasErrors = true
			} else {
				printLog("info", fmt.Sprintf("✅ 定时任务时区已配置: %s", cfg.Backup.Schedule.Timezone))
			}
		}
	} else {
		printLog("info", "📋 定时任务已禁用")
	}

	if !hasErrors {
		printLog("info", "🎉 配置验证通过！")
	} else {
		printLog("warn", "⚠️  配置存在问题，请修复后重试")
	}

	pauseForKey()
}

// 重置配置
func resetConfig() {
	fmt.Println("⚠️  警告: 即将重置配置文件为默认设置")
	fmt.Println("这将丢失当前的所有配置")
	fmt.Println()
	confirm := getUserInput("确认重置？这将丢失当前配置 [y/N]: ")
	if strings.ToLower(confirm) == "y" || strings.ToLower(confirm) == "yes" {
		cfgPath := filepath.Join(ConfigDir, ConfigFile)
		if err := generateDefaultConfig(cfgPath); err != nil {
			showErrorAndWait(err, "重置配置失败", "检查文件权限和磁盘空间")
		} else {
			printLog("info", "✅ 配置文件已重置为默认设置")
		}
	} else {
		printLog("info", "取消重置配置")
	}
	pauseForKey()
}

// 测试COS连接
func testCOSConnectivity() {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		showErrorAndWait(err, "加载配置失败", "检查配置文件是否存在和格式是否正确")
		return
	}

	// 检查必要的配置
	if cfg.Cos.SecretID == "" || cfg.Cos.SecretKey == "" {
		printLog("error", "❌ 腾讯云 COS 认证信息未配置")
		pauseForKey()
		return
	}

	if cfg.Cos.Bucket == "" {
		printLog("error", "❌ COS 桶名称未配置")
		pauseForKey()
		return
	}

	printLog("info", "正在测试 COS 连接...")

	client, err := createCOSClient(cfg)
	if err != nil {
		showErrorAndWait(err, "创建COS客户端失败", "检查配置文件和网络连接")
		return
	}

	if err := testCOSConnection(client, cfg.Cos.Bucket); err != nil {
		showErrorAndWait(err, "COS 连接测试失败",
			"1. 检查 SecretID 和 SecretKey 是否正确",
			"2. 确认桶名称格式为 'name-appid'",
			"3. 验证区域配置是否正确",
			"4. 检查网络连接是否正常")
	} else {
		printLog("info", "✅ COS 连接测试成功")
		printLog("info", "✅ 认证信息正确")
		printLog("info", "✅ 桶访问权限正常")
	}
	pauseForKey()
}

// 查看配置文件内容
func viewConfigFile() {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	if !fileExists(cfgPath) {
		printLog("error", "配置文件不存在")
		pauseForKey()
		return
	}

	printLog("info", "配置文件内容:")
	printLog("info", "文件路径: "+cfgPath)
	fmt.Println(strings.Repeat("=", 50))

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		showErrorAndWait(err, "读取配置文件失败", "检查文件权限和路径")
		return
	}

	fmt.Println(string(data))
	fmt.Println(strings.Repeat("=", 50))
	pauseForKey()
}

// 处理服务管理菜单
func handleServiceMenu() {
	for {
		clearScreen()
		fmt.Println("📋 服务管理")
		fmt.Println(strings.Repeat("-", 30))

		// 显示当前服务状态
		running, pid := getServiceRunningStatus()
		autoStartStatus := getAutoStartStatusText()

		fmt.Printf("当前状态: %s\n", func() string {
			if running {
				if pid > 0 {
					return fmt.Sprintf("● 运行中 (PID: %d)", pid)
				}
				return "● 运行中"
			}
			return "○ 已停止"
		}())
		fmt.Printf("开机自启: %s\n", autoStartStatus)

		fmt.Println("\n请选择操作:")
		fmt.Println("  1. 安装开机自启")
		fmt.Println("  2. 启动服务")
		fmt.Println("  3. 停止服务")
		fmt.Println("  4. 重启服务")
		fmt.Println("  5. 查看详细状态")
		fmt.Println("  6. 卸载服务")
		fmt.Println("  0. 返回主菜单")

		choice := getUserInput("请输入选项 [0-6]: ")

		switch choice {
		case "1":
			installAutoStart()
		case "2":
			startService()
		case "3":
			stopService()
		case "4":
			restartService()
		case "5":
			showDetailedServiceStatus()
		case "6":
			uninstallService()
		case "0":
			return
		default:
			printLog("warn", "无效选项，请重新选择")
			pauseForKey()
		}
	}
}


// 显示详细服务状态
func showDetailedServiceStatus() {
	fmt.Println("📊 详细服务状态")
	fmt.Println(strings.Repeat("-", 30))

	manager := getServiceManager()
	status := manager.Status()

	fmt.Printf("安装状态: %s\n", func() string {
		if status.Installed { return "✅ 已安装" } else { return "○ 未安装" }
	}())

	running, pid := getServiceRunningStatus()
	fmt.Printf("运行状态: %s\n", func() string {
		if running {
			if pid > 0 {
				return fmt.Sprintf("● 运行中 (PID: %d)", pid)
			}
			return "● 运行中"
		}
		return "○ 已停止"
	}())

	fmt.Printf("开机自启: %s\n", func() string {
		if status.AutoStart { return "✅ 已启用" } else { return "○ 已禁用" }
	}())

	if running && pid > 0 {
		fmt.Printf("进程PID: %d\n", pid)

		// 尝试获取进程信息
		if runtime.GOOS != "windows" {
			if cmd := exec.Command("ps", "-p", strconv.Itoa(pid)); cmd != nil {
				if output, err := cmd.Output(); err == nil {
					lines := strings.Split(string(output), "\n")
					if len(lines) > 1 {
						fmt.Printf("进程信息: %s\n", strings.TrimSpace(lines[1]))
					}
				}
			}
		}
	}

	// 显示系统特定信息
	if runtime.GOOS == "darwin" {
		fmt.Println("\nmacOS LaunchAgent 信息:")
		homeDir, _ := os.UserHomeDir()
		plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", "com.backup-go.daemon.plist")
		if fileExists(plistPath) {
			fmt.Printf("配置文件: ✅ %s\n", plistPath)
		} else {
			fmt.Printf("配置文件: ❌ %s\n", plistPath)
		}
	} else if runtime.GOOS == "linux" {
		fmt.Println("\nLinux Systemd 信息:")
		fmt.Println("用户服务: backup-go")
		fmt.Println("服务文件: ~/.config/systemd/user/backup-go.service")
	} else if runtime.GOOS == "windows" {
		fmt.Println("\nWindows Service 信息:")
		fmt.Println("服务名称: BackupGo")
		fmt.Println("查看命令: sc query BackupGo")
	}

	if status.LastError != nil {
		fmt.Printf("\n最近错误: %v\n", status.LastError)
	}

	pauseForKey()
}

// 安装开机自启
func installAutoStart() {
	fmt.Println("🔧 安装开机自启服务")
	fmt.Println(strings.Repeat("-", 30))

	// 获取服务管理器
	manager := getServiceManager()

	printLog("info", "正在安装系统服务...")
	if err := manager.Install(); err != nil {
		var suggestions []string
		if runtime.GOOS == "darwin" {
			suggestions = []string{
				"确保已安装 Command Line Tools",
				"检查 ~/Library/LaunchAgents 目录权限",
				"尝试手动执行: launchctl load ~/Library/LaunchAgents/com.backup-go.daemon.plist",
			}
		} else if runtime.GOOS == "linux" {
			suggestions = []string{
				"确保系统支持 systemd",
				"检查用户权限",
				"尝试手动执行: systemctl --user enable backup-go",
			}
		} else if runtime.GOOS == "windows" {
			suggestions = []string{
				"以管理员权限运行程序",
				"检查 Windows 服务是否正常",
			}
		}
		showErrorAndWait(err, "安装服务失败", suggestions...)
	} else {
		printLog("info", "✅ 开机自启服务安装成功")

		// 显示状态
		status := manager.Status()
		if status.Installed {
			printLog("info", "✅ 服务已注册到系统")
		}
		if status.AutoStart {
			printLog("info", "✅ 开机自启已启用")
		}
		pauseForKey()
	}
}

// 启动服务
func startService() {
	fmt.Println("▶️  启动服务")
	fmt.Println(strings.Repeat("-", 30))

	manager := getServiceManager()
	printLog("info", "正在启动服务...")

	if err := manager.Start(); err != nil {
		showErrorAndWait(err, "启动服务失败",
			"检查服务是否已安装",
			"查看服务日志获取详细错误信息",
			"尝试重启整个系统")
	} else {
		printLog("info", "✅ 服务启动成功")
		printLog("info", "正在等待服务启动...")

		// 等待服务启动完成
		time.Sleep(3 * time.Second)

		// 显示新状态
		status := manager.Status()
		if status.Running {
			printLog("info", fmt.Sprintf("✅ 服务正在运行 (PID: %d)", status.PID))
		} else {
			printLog("warn", "⚠️  服务已停止，请检查配置")
		}
		pauseForKey()
	}
}

// 停止服务
func stopService() {
	fmt.Println("⏹️  停止服务")
	fmt.Println(strings.Repeat("-", 30))

	manager := getServiceManager()
	printLog("info", "正在停止服务...")

	if err := manager.Stop(); err != nil {
		showErrorAndWait(err, "停止服务失败",
			"检查服务是否正在运行",
			"确认有足够的权限停止服务",
			"查看服务日志了解详细错误")
	} else {
		printLog("info", "✅ 服务停止成功")

		// 验证状态
		status := manager.Status()
		if !status.Running {
			printLog("info", "✅ 服务已停止")
		}
		pauseForKey()
	}
}

// 重启服务
func restartService() {
	fmt.Println("🔄 重启服务")
	fmt.Println(strings.Repeat("-", 30))

	manager := getServiceManager()
	printLog("info", "正在重启服务...")

	if err := manager.Restart(); err != nil {
		showErrorAndWait(err, "重启服务失败", "检查服务状态和系统权限")
	} else {
		printLog("info", "✅ 服务重启成功")
		printLog("info", "正在等待服务启动...")

		// 等待服务启动完成
		time.Sleep(3 * time.Second)

		// 显示新状态
		status := manager.Status()
		if status.Running {
			printLog("info", fmt.Sprintf("✅ 服务正在运行 (PID: %d)", status.PID))
		} else {
			printLog("warn", "⚠️  服务已停止，请检查配置")
		}
	}

	pauseForKey()
}

// 卸载服务
func uninstallService() {
	fmt.Println("🗑️  卸载服务")
	fmt.Println(strings.Repeat("-", 30))
	fmt.Println("⚠️  警告: 即将卸载备份服务")
	fmt.Println("这将删除开机自启设置，但不会删除你的备份文件")
	fmt.Println()

	confirm := getUserInput("确认卸载？这将删除服务配置 [y/N]: ")
	if strings.ToLower(confirm) == "y" || strings.ToLower(confirm) == "yes" {
		printLog("info", "正在卸载服务...")

		manager := getServiceManager()

		// 先停止服务
		printLog("info", "正在停止服务...")
		manager.Stop()

		// 卸载服务
		printLog("info", "正在删除服务配置...")
		if err := manager.Uninstall(); err != nil {
			var suggestions []string
			if runtime.GOOS == "darwin" {
				suggestions = []string{
					"手动删除: rm ~/Library/LaunchAgents/com.backup-go.daemon.plist",
					"卸载服务: launchctl unload ~/Library/LaunchAgents/com.backup-go.daemon.plist",
				}
			} else if runtime.GOOS == "linux" {
				suggestions = []string{
					"禁用服务: systemctl --user disable backup-go",
					"停止服务: systemctl --user stop backup-go",
					"删除文件: rm ~/.config/systemd/user/backup-go.service",
				}
			} else {
				suggestions = []string{"以管理员权限运行程序", "检查系统权限设置"}
			}
			showErrorAndWait(err, "卸载服务失败", suggestions...)
		} else {
			printLog("info", "✅ 服务卸载成功")

			// 验证状态
			status := manager.Status()
			if !status.Installed {
				printLog("info", "✅ 服务配置已删除")
			}
		}
	} else {
		printLog("info", "取消卸载服务")
	}

	pauseForKey()
}

// 处理日志管理菜单
func handleLogMenu() {
	for {
		clearScreen()
		fmt.Println("📝 日志管理")
		fmt.Println(strings.Repeat("-", 30))

		// 显示日志状态
		showLogStatus()

		fmt.Println("\n请选择操作:")
		fmt.Println("  1. 查看最新日志")
		fmt.Println("  2. 查看历史日志文件")
		fmt.Println("  3. 搜索日志内容")
		fmt.Println("  4. 清理过期日志")
		fmt.Println("  5. 导出日志")
		fmt.Println("  6. 日志设置")
		fmt.Println("  0. 返回主菜单")

		choice := getUserInput("请输入选项 [0-6]: ")

		switch choice {
		case "1":
			viewLatestLogs()
		case "2":
			viewLogFiles()
		case "3":
			searchLogs()
		case "4":
			cleanupLogs()
		case "5":
			exportLogs()
		case "6":
			logSettings()
		case "0":
			return
		default:
			printLog("warn", "无效选项，请重新选择")
			pauseForKey()
		}
	}
}

// 显示日志状态
func showLogStatus() {
	logFiles, err := getLogFiles()
	if err != nil {
		fmt.Printf("❌ 无法读取日志目录: %v\n", err)
		return
	}

	if len(logFiles) == 0 {
		fmt.Println("📋 日志状态: 暂无日志文件")
		return
	}

	var totalSize int64
	for _, file := range logFiles {
		totalSize += file.Size
	}

	fmt.Printf("📋 日志状态: 共 %d 个日志文件，总大小 %s\n",
		len(logFiles), humanize.Bytes(uint64(totalSize)))

	if len(logFiles) > 0 {
		latestFile := logFiles[len(logFiles)-1]
		fmt.Printf("📄 最新日志: %s (%s)\n",
			latestFile.Name, humanize.Bytes(uint64(latestFile.Size)))
	}
}

// 获取日志文件列表
func getLogFiles() ([]LogFileInfo, error) {
	entries, err := os.ReadDir(LogDir)
	if err != nil {
		return nil, err
	}

	var files []LogFileInfo
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".log") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			files = append(files, LogFileInfo{
				Name:    entry.Name(),
				Size:    info.Size(),
				ModTime: info.ModTime(),
			})
		}
	}

	// 按修改时间排序
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.Before(files[j].ModTime)
	})

	return files, nil
}

// LogFileInfo 日志文件信息
type LogFileInfo struct {
	Name    string
	Size    int64
	ModTime time.Time
}

// 查看最新日志
func viewLatestLogs() {
	clearScreen()
	fmt.Println("📄 查看最新日志")
	fmt.Println(strings.Repeat("-", 30))

	logFiles, err := getLogFiles()
	if err != nil {
		printLog("error", fmt.Sprintf("读取日志文件失败: %v", err))
		pauseForKey()
		return
	}

	if len(logFiles) == 0 {
		fmt.Println("📋 暂无日志文件")
		pauseForKey()
		return
	}

	latestFile := filepath.Join(LogDir, logFiles[len(logFiles)-1].Name)
	fmt.Printf("显示最新日志文件: %s\n\n", logFiles[len(logFiles)-1].Name)

	// 读取最后50行
	if err := displayTailLog(latestFile, 50); err != nil {
		printLog("error", fmt.Sprintf("读取日志内容失败: %v", err))
	}

	pauseForKey()
}

// 显示日志文件末尾内容
func displayTailLog(filename string, lines int) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	var content []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		content = append(content, scanner.Text())
		if len(content) > lines*2 { // 限制内存使用
			content = content[lines:]
		}
	}

	if len(content) == 0 {
		fmt.Println("(日志文件为空)")
		return nil
	}

	start := 0
	if len(content) > lines {
		start = len(content) - lines
		fmt.Printf("... 显示最后 %d 行 ...\n\n", lines)
	}

	for i := start; i < len(content); i++ {
		fmt.Println(content[i])
	}

	return scanner.Err()
}

// 查看历史日志文件
func viewLogFiles() {
	clearScreen()
	fmt.Println("📂 查看历史日志文件")
	fmt.Println(strings.Repeat("-", 30))

	logFiles, err := getLogFiles()
	if err != nil {
		printLog("error", fmt.Sprintf("读取日志文件失败: %v", err))
		pauseForKey()
		return
	}

	if len(logFiles) == 0 {
		fmt.Println("📋 暂无日志文件")
		pauseForKey()
		return
	}

	fmt.Printf("共找到 %d 个日志文件:\n\n", len(logFiles))

	for i, file := range logFiles {
		fmt.Printf("%d. %s\n", i+1, file.Name)
		fmt.Printf("   大小: %s\n", humanize.Bytes(uint64(file.Size)))
		fmt.Printf("   修改时间: %s\n", file.ModTime.Format("2006-01-02 15:04:05"))
		fmt.Println()
	}

	choice := getUserInput("请选择要查看的日志文件编号 [直接回车返回]: ")
	if choice == "" {
		return
	}

	index, err := strconv.Atoi(choice)
	if err != nil || index < 1 || index > len(logFiles) {
		printLog("warn", "无效的文件编号")
		pauseForKey()
		return
	}

	selectedFile := filepath.Join(LogDir, logFiles[index-1].Name)

	// 询问显示行数
	lines := getUserInput("显示行数 [默认100]: ")
	displayLines := 100
	if lines != "" {
		if num, err := strconv.Atoi(lines); err == nil && num > 0 {
			displayLines = num
		}
	}

	fmt.Printf("\n显示日志文件: %s\n\n", logFiles[index-1].Name)
	if err := displayTailLog(selectedFile, displayLines); err != nil {
		printLog("error", fmt.Sprintf("读取日志失败: %v", err))
	}

	pauseForKey()
}

// 搜索日志内容
func searchLogs() {
	clearScreen()
	fmt.Println("🔍 搜索日志内容")
	fmt.Println(strings.Repeat("-", 30))

	keyword := getUserInput("请输入搜索关键词: ")
	if keyword == "" {
		printLog("warn", "搜索关键词不能为空")
		pauseForKey()
		return
	}

	logFiles, err := getLogFiles()
	if err != nil {
		printLog("error", fmt.Sprintf("读取日志文件失败: %v", err))
		pauseForKey()
		return
	}

	fmt.Printf("\n正在搜索关键词 '%s'...\n\n", keyword)

	var totalMatches int
	for _, file := range logFiles {
		matches, err := searchInLogFile(filepath.Join(LogDir, file.Name), keyword)
		if err != nil {
			printLog("warn", fmt.Sprintf("搜索文件 %s 失败: %v", file.Name, err))
			continue
		}

		if len(matches) > 0 {
			fmt.Printf("📄 %s (找到 %d 处匹配):\n", file.Name, len(matches))
			for _, match := range matches {
				fmt.Printf("  [%s] %s\n", match.Time, match.Content)
			}
			fmt.Println()
			totalMatches += len(matches)
		}
	}

	if totalMatches == 0 {
		fmt.Printf("❌ 未找到包含 '%s' 的日志\n", keyword)
	} else {
		fmt.Printf("✅ 共找到 %d 处匹配\n", totalMatches)
	}

	pauseForKey()
}

// LogMatch 日志匹配结果
type LogMatch struct {
	Time    string
	Content string
}

// 在单个日志文件中搜索
func searchInLogFile(filename, keyword string) ([]LogMatch, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var matches []LogMatch
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
			// 尝试提取时间戳
			timeStr := extractTimestamp(line)
			if timeStr == "" {
				timeStr = fmt.Sprintf("行%d", lineNum)
			}

			// 限制匹配内容长度
			if len(line) > 200 {
				line = line[:200] + "..."
			}

			matches = append(matches, LogMatch{
				Time:    timeStr,
				Content: line,
			})

			// 限制匹配数量
			if len(matches) >= 50 {
				break
			}
		}
	}

	return matches, scanner.Err()
}

// 提取日志时间戳
func extractTimestamp(line string) string {
	// 尝试匹配 [2025-10-31 14:30:00] 格式
	if strings.HasPrefix(line, "[") {
		if end := strings.Index(line, "]"); end > 0 && len(line) > end+2 {
			return line[1:end]
		}
	}
	return ""
}

// 清理过期日志
func cleanupLogs() {
	clearScreen()
	fmt.Println("🗑️  清理过期日志")
	fmt.Println(strings.Repeat("-", 30))

	logFiles, err := getLogFiles()
	if err != nil {
		printLog("error", fmt.Sprintf("读取日志文件失败: %v", err))
		pauseForKey()
		return
	}

	if len(logFiles) == 0 {
		fmt.Println("📋 暂无日志文件需要清理")
		pauseForKey()
		return
	}

	fmt.Printf("当前共有 %d 个日志文件\n\n", len(logFiles))

	// 计算总大小
	var totalSize int64
	for _, file := range logFiles {
		totalSize += file.Size
	}
	fmt.Printf("总占用空间: %s\n\n", humanize.Bytes(uint64(totalSize)))

	fmt.Println("清理选项:")
	fmt.Println("  1. 清理7天前的日志")
	fmt.Println("  2. 清理30天前的日志")
	fmt.Println("  3. 只保留最新10个文件")
	fmt.Println("  4. 自定义天数")

	choice := getUserInput("请选择清理选项 [1-4]: ")

	var days int
	var filesToKeep int

	switch choice {
	case "1":
		days = 7
	case "2":
		days = 30
	case "3":
		filesToKeep = 10
	case "4":
		customDays := getUserInput("请输入天数: ")
		if d, err := strconv.Atoi(customDays); err == nil && d > 0 {
			days = d
		} else {
			printLog("warn", "无效的天数")
			pauseForKey()
			return
		}
	default:
		printLog("warn", "无效选项")
		pauseForKey()
		return
	}

	cutoffTime := time.Now().AddDate(0, 0, -days)
	var filesToDelete []string

	if filesToKeep > 0 {
		// 按文件数量清理
		if len(logFiles) > filesToKeep {
			for i := 0; i < len(logFiles)-filesToKeep; i++ {
				filesToDelete = append(filesToDelete, filepath.Join(LogDir, logFiles[i].Name))
			}
		}
	} else {
		// 按时间清理
		for _, file := range logFiles {
			if file.ModTime.Before(cutoffTime) {
				filesToDelete = append(filesToDelete, filepath.Join(LogDir, file.Name))
			}
		}
	}

	if len(filesToDelete) == 0 {
		fmt.Println("✅ 没有需要清理的日志文件")
		pauseForKey()
		return
	}

	fmt.Printf("\n将删除以下 %d 个日志文件:\n", len(filesToDelete))
	var deleteSize int64
	for _, file := range filesToDelete {
		if info, err := os.Stat(file); err == nil {
			deleteSize += info.Size()
		}
		fmt.Printf("  %s\n", filepath.Base(file))
	}
	fmt.Printf("\n将释放空间: %s\n", humanize.Bytes(uint64(deleteSize)))

	confirm := getUserInput("确认删除？[y/N]: ")
	if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
		fmt.Println("取消清理")
		pauseForKey()
		return
	}

	var deletedCount int
	for _, file := range filesToDelete {
		if err := os.Remove(file); err != nil {
			printLog("warn", fmt.Sprintf("删除文件失败 %s: %v", filepath.Base(file), err))
		} else {
			deletedCount++
		}
	}

	fmt.Printf("\n✅ 成功删除 %d 个日志文件，释放空间 %s\n",
		deletedCount, humanize.Bytes(uint64(deleteSize)))

	pauseForKey()
}

// 导出日志
func exportLogs() {
	clearScreen()
	fmt.Println("📤 导出日志")
	fmt.Println(strings.Repeat("-", 30))

	logFiles, err := getLogFiles()
	if err != nil {
		printLog("error", fmt.Sprintf("读取日志文件失败: %v", err))
		pauseForKey()
		return
	}

	if len(logFiles) == 0 {
		fmt.Println("📋 暂无日志文件可导出")
		pauseForKey()
		return
	}

	// 生成导出文件名
	timestamp := time.Now().Format("20060102-150405")
	exportFile := fmt.Sprintf("logs-export-%s.tar.gz", timestamp)

	fmt.Printf("导出选项:\n")
	fmt.Println("  1. 导出所有日志文件")
	fmt.Println("  2. 导出最近7天的日志")
	fmt.Println("  3. 导出最近30天的日志")

	choice := getUserInput("请选择导出选项 [1-3]: ")

	var selectedFiles []string
	cutoffTime := time.Now()

	switch choice {
	case "1":
		selectedFiles = make([]string, len(logFiles))
		for i, file := range logFiles {
			selectedFiles[i] = filepath.Join(LogDir, file.Name)
		}
	case "2":
		cutoffTime = time.Now().AddDate(0, 0, -7)
		for _, file := range logFiles {
			if file.ModTime.After(cutoffTime) {
				selectedFiles = append(selectedFiles, filepath.Join(LogDir, file.Name))
			}
		}
	case "3":
		cutoffTime = time.Now().AddDate(0, 0, -30)
		for _, file := range logFiles {
			if file.ModTime.After(cutoffTime) {
				selectedFiles = append(selectedFiles, filepath.Join(LogDir, file.Name))
			}
		}
	default:
		printLog("warn", "无效选项")
		pauseForKey()
		return
	}

	if len(selectedFiles) == 0 {
		fmt.Println("❌ 没有符合条件的日志文件")
		pauseForKey()
		return
	}

	fmt.Printf("\n准备导出 %d 个日志文件到: %s\n", len(selectedFiles), exportFile)

	// 创建tar.gz文件
	if err := createTarGzExport(exportFile, selectedFiles); err != nil {
		printLog("error", fmt.Sprintf("导出失败: %v", err))
		pauseForKey()
		return
	}

	// 检查导出文件大小
	if info, err := os.Stat(exportFile); err == nil {
		fmt.Printf("✅ 导出完成！\n")
		fmt.Printf("📁 文件: %s\n", exportFile)
		fmt.Printf("📊 大小: %s\n", humanize.Bytes(uint64(info.Size())))
	}

	pauseForKey()
}

// 创建tar.gz导出文件
func createTarGzExport(exportFile string, files []string) error {
	// 导入gzip包需要添加到import中
	// 这里简化实现，直接复制文件
	return fmt.Errorf("导出功能需要额外依赖包，暂未实现")
}

// 日志设置
func logSettings() {
	clearScreen()
	fmt.Println("⚙️  日志设置")
	fmt.Println(strings.Repeat("-", 30))

	fmt.Println("当前日志配置:")
	fmt.Printf("  📁 日志目录: %s\n", LogDir)
	fmt.Printf("  📄 日志格式: 文本格式\n")
	fmt.Printf("  🔄 日志轮转: 按文件大小\n")
	fmt.Printf("  🗑️  默认保留: 30天\n")

	fmt.Println("\n注意: 日志设置目前需要手动编辑配置文件")
	fmt.Println("计划中的功能:")
	fmt.Println("  - 日志级别设置")
	fmt.Println("  - 自动日志轮转")
	fmt.Println("  - 日志压缩存档")
	fmt.Println("  - 远程日志推送")

	pauseForKey()
}

// 处理定时任务菜单
func handleScheduleMenu() {
	for {
		clearScreen()
		fmt.Println("🕐 定时任务管理")
		fmt.Println(strings.Repeat("-", 30))

		// 显示当前定时任务状态
		showScheduleStatus()

		fmt.Println("\n请选择操作:")
		fmt.Println("  1. 启用定时任务")
		fmt.Println("  2. 禁用定时任务")
		fmt.Println("  3. 修改定时时间")
		fmt.Println("  4. 查看下次执行时间")
		fmt.Println("  5. 立即执行定时备份")
		fmt.Println("  6. 查看执行历史")
		fmt.Println("  7. 定时任务设置")
		fmt.Println("  0. 返回主菜单")

		choice := getUserInput("请输入选项 [0-7]: ")

		switch choice {
		case "1":
			enableSchedule()
		case "2":
			disableSchedule()
		case "3":
			modifyScheduleTime()
		case "4":
			showNextRunTime()
		case "5":
			runScheduledBackupNow()
		case "6":
			viewScheduleHistory()
		case "7":
			scheduleSettings()
		case "0":
			return
		default:
			printLog("warn", "无效选项，请重新选择")
			pauseForKey()
		}
	}
}

// 显示定时任务状态
func showScheduleStatus() {
	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		fmt.Printf("❌ 无法读取配置: %v\n", err)
		return
	}

	fmt.Printf("📋 定时任务状态: ")
	if cfg.Backup.Schedule.Enabled {
		fmt.Printf("✅ 已启用\n")
		fmt.Printf("⏰ 执行时间: 每天 %02d:%02d\n", cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute)
		fmt.Printf("🌍 时区: %s\n", cfg.Backup.Schedule.Timezone)

		// 计算下次执行时间
		nextRun := calculateNextRunTime(cfg.Backup.Schedule)
		if !nextRun.IsZero() {
			fmt.Printf("📅 下次执行: %s\n", nextRun.Format("2006-01-02 15:04:05"))
			duration := time.Until(nextRun)
			if duration > 0 {
				fmt.Printf("⏱️  距离下次: %s\n", formatDuration(duration))
			}
		}
	} else {
		fmt.Printf("❌ 已禁用\n")
		fmt.Printf("⏰ 配置时间: 每天 %02d:%02d\n", cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute)
	}

	// 检查服务状态
	running, pid := getServiceRunningStatus()
	if running {
		fmt.Printf("🔄 守护进程: ● 运行中 (PID: %d)\n", pid)
	} else {
		fmt.Printf("🔄 守护进程: ○ 已停止\n")
	}
}

// 启用定时任务
func enableSchedule() {
	clearScreen()
	fmt.Println("✅ 启用定时任务")
	fmt.Println(strings.Repeat("-", 30))

	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		printLog("error", fmt.Sprintf("读取配置失败: %v", err))
		pauseForKey()
		return
	}

	if cfg.Backup.Schedule.Enabled {
		fmt.Println("⚠️  定时任务已经启用")
		pauseForKey()
		return
	}

	fmt.Printf("将启用定时任务: 每天 %02d:%02d\n",
		cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute)
	fmt.Printf("时区: %s\n", cfg.Backup.Schedule.Timezone)

	confirm := getUserInput("确认启用？[y/N]: ")
	if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
		fmt.Println("取消启用")
		pauseForKey()
		return
	}

	// 更新配置
	cfg.Backup.Schedule.Enabled = true
	if err := saveConfig(cfgPath, cfg); err != nil {
		printLog("error", fmt.Sprintf("保存配置失败: %v", err))
		pauseForKey()
		return
	}

	fmt.Println("✅ 定时任务已启用")

	// 提示用户重启服务
	running, _ := getServiceRunningStatus()
	if running {
		fmt.Println("\n💡 提示:")
		fmt.Println("定时任务已启用，但需要重启服务使配置生效")
		fmt.Println("请在'服务管理'中重启服务")
	} else {
		fmt.Println("\n💡 提示:")
		fmt.Println("请启动服务以开始定时备份")
	}

	pauseForKey()
}

// 禁用定时任务
func disableSchedule() {
	clearScreen()
	fmt.Println("❌ 禁用定时任务")
	fmt.Println(strings.Repeat("-", 30))

	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		printLog("error", fmt.Sprintf("读取配置失败: %v", err))
		pauseForKey()
		return
	}

	if !cfg.Backup.Schedule.Enabled {
		fmt.Println("⚠️  定时任务已经禁用")
		pauseForKey()
		return
	}

	fmt.Println("⚠️  警告: 即将禁用定时任务")
	fmt.Println("禁用后，系统将不会自动执行备份")

	confirm := getUserInput("确认禁用？[y/N]: ")
	if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
		fmt.Println("取消禁用")
		pauseForKey()
		return
	}

	// 更新配置
	cfg.Backup.Schedule.Enabled = false
	if err := saveConfig(cfgPath, cfg); err != nil {
		printLog("error", fmt.Sprintf("保存配置失败: %v", err))
		pauseForKey()
		return
	}

	fmt.Println("✅ 定时任务已禁用")

	pauseForKey()
}

// 修改定时时间
func modifyScheduleTime() {
	clearScreen()
	fmt.Println("⏰ 修改定时时间")
	fmt.Println(strings.Repeat("-", 30))

	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		printLog("error", fmt.Sprintf("读取配置失败: %v", err))
		pauseForKey()
		return
	}

	fmt.Printf("当前定时时间: 每天 %02d:%02d\n",
		cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute)
	fmt.Printf("当前时区: %s\n", cfg.Backup.Schedule.Timezone)
	fmt.Println()

	// 输入新的时间
	fmt.Println("请输入新的执行时间 (24小时制):")

	hourStr := getUserInput("小时 [0-23]: ")
	hour, err := strconv.Atoi(hourStr)
	if err != nil || hour < 0 || hour > 23 {
		printLog("error", "无效的小时数，请输入0-23之间的数字")
		pauseForKey()
		return
	}

	minuteStr := getUserInput("分钟 [0-59]: ")
	minute, err := strconv.Atoi(minuteStr)
	if err != nil || minute < 0 || minute > 59 {
		printLog("error", "无效的分钟数，请输入0-59之间的数字")
		pauseForKey()
		return
	}

	fmt.Printf("\n新的定时时间: 每天 %02d:%02d\n", hour, minute)
	fmt.Printf("时区: %s\n", cfg.Backup.Schedule.Timezone)

	confirm := getUserInput("确认修改？[y/N]: ")
	if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
		fmt.Println("取消修改")
		pauseForKey()
		return
	}

	// 更新配置
	cfg.Backup.Schedule.Hour = hour
	cfg.Backup.Schedule.Minute = minute
	if err := saveConfig(cfgPath, cfg); err != nil {
		printLog("error", fmt.Sprintf("保存配置失败: %v", err))
		pauseForKey()
		return
	}

	fmt.Println("✅ 定时时间已修改")

	// 显示下次执行时间
	nextRun := calculateNextRunTime(cfg.Backup.Schedule)
	if !nextRun.IsZero() {
		fmt.Printf("📅 下次执行: %s\n", nextRun.Format("2006-01-02 15:04:05"))
	}

	// 提示重启服务
	if cfg.Backup.Schedule.Enabled {
		fmt.Println("\n💡 提示: 需要重启服务使新的定时时间生效")
	}

	pauseForKey()
}

// 显示下次执行时间
func showNextRunTime() {
	clearScreen()
	fmt.Println("📅 下次执行时间")
	fmt.Println(strings.Repeat("-", 30))

	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		printLog("error", fmt.Sprintf("读取配置失败: %v", err))
		pauseForKey()
		return
	}

	if !cfg.Backup.Schedule.Enabled {
		fmt.Println("❌ 定时任务未启用")
		fmt.Println("请先启用定时任务")
		pauseForKey()
		return
	}

	fmt.Printf("定时配置: 每天 %02d:%02d (%s)\n",
		cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute, cfg.Backup.Schedule.Timezone)

	nextRun := calculateNextRunTime(cfg.Backup.Schedule)
	if nextRun.IsZero() {
		fmt.Println("❌ 无法计算下次执行时间")
		pauseForKey()
		return
	}

	fmt.Printf("\n📅 下次执行时间: %s\n", nextRun.Format("2006-01-02 15:04:05"))

	duration := time.Until(nextRun)
	if duration > 0 {
		fmt.Printf("⏱️  距离下次执行: %s\n", formatDuration(duration))

		fmt.Printf("\n📊 时间详情:\n")
		days := int(duration.Hours()) / 24
		hours := int(duration.Hours()) % 24
		minutes := int(duration.Minutes()) % 60

		if days > 0 {
			fmt.Printf("  - %d 天 %d 小时 %d 分钟\n", days, hours, minutes)
		} else if hours > 0 {
			fmt.Printf("  - %d 小时 %d 分钟\n", hours, minutes)
		} else {
			fmt.Printf("  - %d 分钟\n", minutes)
		}
	} else {
		fmt.Printf("⏱️  应该在 %s 前执行\n", formatDuration(-duration))
	}

	// 显示未来7天的执行计划
	fmt.Println("\n📋 未来7天执行计划:")
	for i := 0; i < 7; i++ {
		futureTime := nextRun.AddDate(0, 0, i)
		fmt.Printf("  %s: %s\n",
			futureTime.Format("2006-01-02 Monday"),
			futureTime.Format("15:04:05"))
	}

	pauseForKey()
}

// 立即执行定时备份
func runScheduledBackupNow() {
	clearScreen()
	fmt.Println("🚀 立即执行定时备份")
	fmt.Println(strings.Repeat("-", 30))

	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		printLog("error", fmt.Sprintf("读取配置失败: %v", err))
		pauseForKey()
		return
	}

	if !cfg.Backup.Schedule.Enabled {
		fmt.Println("❌ 定时任务未启用")
		fmt.Println("请先在定时任务管理中启用定时备份")
		pauseForKey()
		return
	}

	fmt.Println("⚠️  即将立即执行一次定时备份")
	fmt.Println("这与立即执行备份功能相同，但会记录为定时备份")

	confirm := getUserInput("确认执行？[y/N]: ")
	if strings.ToLower(confirm) != "y" && strings.ToLower(confirm) != "yes" {
		fmt.Println("取消执行")
		pauseForKey()
		return
	}

	fmt.Println("🚀 开始执行定时备份...")

	// 执行备份逻辑
	exitIfError(prepareTempDir(), "准备临时目录失败")

	// 检查COS配置
	if cfg.Cos.SecretID == "" || cfg.Cos.SecretKey == "" {
		printLog("error", "腾讯云 COS 认证信息未配置")
		_ = os.RemoveAll(getTempDir())
		pauseForKey()
		return
	}

	client, err := createCOSClient(cfg)
	if err != nil {
		printLog("error", fmt.Sprintf("创建COS客户端失败: %v", err))
		_ = os.RemoveAll(getTempDir())
		pauseForKey()
		return
	}

	archivePath := generateArchivePath()
	if err := performBackup(cfg, client, archivePath); err != nil {
		printLog("error", fmt.Sprintf("定时备份失败: %v", err))
		_ = os.RemoveAll(getTempDir())
		pauseForKey()
		return
	}

	// 清理过期备份
	if err := deleteExpiredBackups(client, cfg.Cos.Bucket, cfg.Cos.Prefix, cfg.Cos.KeepDays); err != nil {
		printLog("warn", fmt.Sprintf("清理过期备份失败: %v", err))
	}

	exitIfError(cleanupTempDir(), "清理临时目录失败")

	// 记录执行历史
	recordScheduleExecution(true, "")

	fmt.Println("✅ 定时备份执行完成！")

	// 显示下次执行时间
	nextRun := calculateNextRunTime(cfg.Backup.Schedule)
	if !nextRun.IsZero() {
		fmt.Printf("📅 下次自动执行: %s\n", nextRun.Format("2006-01-02 15:04:05"))
	}

	pauseForKey()
}

// 查看执行历史
func viewScheduleHistory() {
	clearScreen()
	fmt.Println("📜 定时备份执行历史")
	fmt.Println(strings.Repeat("-", 30))

	// 这里简化实现，读取日志中的备份记录
	fmt.Println("📋 正在读取执行历史...")

	historyFile := filepath.Join(LogDir, "schedule-history.log")
	if !fileExists(historyFile) {
		fmt.Println("📋 暂无执行历史记录")
		fmt.Println("历史记录将在定时备份执行后自动生成")
		pauseForKey()
		return
	}

	// 读取并显示历史记录
	if err := displayScheduleHistory(historyFile); err != nil {
		printLog("error", fmt.Sprintf("读取历史记录失败: %v", err))
		pauseForKey()
		return
	}

	pauseForKey()
}

// 显示执行历史
func displayScheduleHistory(historyFile string) error {
	file, err := os.Open(historyFile)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Println("\n📊 最近10次执行记录:")
	fmt.Println(strings.Repeat("-", 50))

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > 100 { // 限制内存
			lines = lines[1:]
		}
	}

	// 显示最近10条记录
	start := 0
	if len(lines) > 10 {
		start = len(lines) - 10
	}

	for i := start; i < len(lines); i++ {
		fmt.Printf("%d. %s\n", i-start+1, lines[i])
	}

	if len(lines) == 0 {
		fmt.Println("(暂无记录)")
	}

	return scanner.Err()
}

// 定时任务设置
func scheduleSettings() {
	clearScreen()
	fmt.Println("⚙️  定时任务设置")
	fmt.Println(strings.Repeat("-", 30))

	cfgPath := filepath.Join(ConfigDir, ConfigFile)
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		printLog("error", fmt.Sprintf("读取配置失败: %v", err))
		pauseForKey()
		return
	}

	fmt.Println("当前定时任务配置:")
	fmt.Printf("  🔘 状态: %s\n", func() string {
		if cfg.Backup.Schedule.Enabled {
			return "✅ 已启用"
		}
		return "❌ 已禁用"
	}())
	fmt.Printf("  ⏰ 执行时间: 每天 %02d:%02d\n", cfg.Backup.Schedule.Hour, cfg.Backup.Schedule.Minute)
	fmt.Printf("  🌍 时区: %s\n", cfg.Backup.Schedule.Timezone)

	fmt.Println("\n🔧 高级设置:")
	fmt.Println("  • 失败重试: 自动重试3次")
	fmt.Println("  • 并发控制: 防止重复执行")
	fmt.Println("  • 执行超时: 24小时")
	fmt.Println("  • 日志记录: 详细执行日志")

	fmt.Println("\n💡 提示:")
	fmt.Println("  • 修改配置后需要重启服务生效")
	fmt.Println("  • 建议在系统负载较低时执行")
	fmt.Println("  • 确保COS存储空间充足")

	pauseForKey()
}

// 计算下次执行时间
func calculateNextRunTime(schedule ScheduleConfig) time.Time {
	// 加载配置的时区，如果时区为空则使用本地时区
	var loc *time.Location
	if schedule.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(schedule.Timezone)
		if err != nil {
			printLog("error", fmt.Sprintf("加载时区失败: %v，使用本地时区", err))
			loc = time.Local
		}
	} else {
		loc = time.Local
	}

	// 获取指定时区的当前时间
	now := time.Now().In(loc)

	// 创建今天的执行时间（使用配置的时区）
	todayRun := time.Date(now.Year(), now.Month(), now.Day(),
		schedule.Hour, schedule.Minute, 0, 0, loc)

	// 如果今天的执行时间还未到，返回今天的执行时间
	if todayRun.After(now) {
		return todayRun
	}

	// 否则返回明天的执行时间
	return todayRun.AddDate(0, 0, 1)
}

// 格式化持续时间
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d秒", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%d分钟", int(d.Minutes()))
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%d小时%d分钟", hours, minutes)
	} else {
		days := int(d.Hours()) / 24
		hours := int(d.Hours()) % 24
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%d天%d小时%d分钟", days, hours, minutes)
	}
}

// 记录定时备份执行
func recordScheduleExecution(success bool, errorMsg string) {
	historyFile := filepath.Join(LogDir, "schedule-history.log")

	// 确保日志目录存在
	if err := os.MkdirAll(LogDir, 0755); err != nil {
		return
	}

	// 创建或追加到历史文件
	file, err := os.OpenFile(historyFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	status := "✅ 成功"
	if !success {
		status = "❌ 失败: " + errorMsg
	}

	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, status)
	file.WriteString(logEntry)
}

// 检查文件是否存在
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}