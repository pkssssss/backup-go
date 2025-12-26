package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	ConfigDir  = "config"
	ConfigFile = "config.toml"
	DefaultKeepDays = 30
	TempDir    = "tmp"
	LogDir     = "logs"
)

type Config struct {
	Cos CosConfig `toml:"cos"`
	Backup BackupConfig `toml:"backup"`
}

type CosConfig struct {
	SecretID  string `toml:"secret_id"`
	SecretKey string `toml:"secret_key"`
	Bucket    string `toml:"bucket"`
	Region    string `toml:"region"`
	Prefix    string `toml:"prefix"`
	KeepDays  int    `toml:"keep_days"`    // COS备份文件保留天数
}

type BackupConfig struct {
	DataDir  string `toml:"data_dir"`
	Schedule ScheduleConfig `toml:"schedule"`
}

type ScheduleConfig struct {
	Enabled   bool   `toml:"enabled"`
	Hour      int    `toml:"hour"`       // 小时 (0-23)
	Minute    int    `toml:"minute"`     // 分钟 (0-59)
	Timezone  string `toml:"timezone"`   // 时区，如 "Asia/Shanghai"
}

func printLog(level, message string) {
	// 创建带时间戳的日志消息
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)

	// 输出到控制台
	fmt.Println(msg)

	// 写入到日志文件
	go func() {
		if err := writeToFile(msg); err != nil {
			// 日志写入失败时，输出到控制台但不造成程序崩溃
			fmt.Printf("[ERROR] 日志写入失败: %v\n", err)
		}
	}()
}

// 写入日志到文件
func writeToFile(message string) error {
	// 确保日志目录存在
	if err := os.MkdirAll(LogDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 按日期创建日志文件
	today := time.Now().Format("2006-01-02")
	logFileName := fmt.Sprintf("backup-%s.log", today)
	logFilePath := filepath.Join(LogDir, logFileName)

	// 以追加模式打开文件
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}
	defer file.Close()

	// 写入日志消息
	if _, err := file.WriteString(message + "\n"); err != nil {
		return fmt.Errorf("写入日志失败: %w", err)
	}

	return nil
}

func exitIfError(err error, msg string) {
	if err != nil {
		printLog("error", fmt.Sprintf("%s: %v", msg, err))
		os.Exit(1)
	}
}

func exitIfNil[T any](v T, err error, msg string) T {
	if err != nil {
		printLog("error", fmt.Sprintf("%s: %v", msg, err))
		os.Exit(1)
	}
	return v
}

func normalizePrefix(prefix string) string {
	p := strings.Trim(prefix, "/")
	if p != "" {
		return p + "/"
	}
	return p
}

func ensureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		printLog("info", "创建目录: "+dir)
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

// 保存配置到文件
func saveConfig(cfgPath string, cfg *Config) error {
	// 使用 TOML 编码器保存配置
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("编码配置失败: %w", err)
	}

	// 确保配置文件有适当的权限
	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// 简化的配置加载函数
func loadConfig(cfgPath string) (*Config, error) {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &cfg, nil
}

func generateDefaultConfig(configPath string) error {
	// 手动创建带注释的TOML配置内容
	tomlContent := `# 腾讯云COS对象存储配置
[cos]
secret_id  = "AKID_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"  # 腾讯云访问密钥ID
secret_key = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"       # 腾讯云访问密钥Key
bucket     = "your-bucket-name-appid"                 # COS存储桶名称（必须是 name-appid 格式）
region     = "ap-shanghai"                            # COS地域（如：ap-shanghai, ap-beijing）
prefix     = "backup/"                                # COS存储目录前缀
keep_days  = 30                                       # COS备份文件保留天数

# 本地备份配置
[backup]
data_dir = "./data"                                   # 本地需要备份的源目录（支持相对路径或绝对路径）

# 定时任务配置
[backup.schedule]
enabled  = false                                      # 是否启用定时任务
hour     = 2                                          # 执行小时（24小时制，0-23）
minute   = 0                                          # 执行分钟（0-59）
timezone = "Asia/Shanghai"                            # 时区设置
`

	file, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("创建配置文件失败: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(tomlContent); err != nil {
		return fmt.Errorf("写入配置文件内容失败: %w", err)
	}

	printLog("init", "已生成默认配置文件 "+configPath+"，请修改后重新运行。")
	printLog("init", "注意: Bucket 需为 'name-appid' 格式，Region 形如 ap-shanghai。")
	return nil
}

// 获取临时目录路径
func getTempDir() string {
	return TempDir
}

// 准备临时目录
func prepareTempDir() error {
	if err := os.RemoveAll(TempDir); err != nil {
		return fmt.Errorf("清理临时目录失败: %w", err)
	}
	if err := os.MkdirAll(TempDir, 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	return nil
}

// 清理临时目录
func cleanupTempDir() error {
	return os.RemoveAll(TempDir)
}

// 确保必要目录存在
func setupDirectories() error {
	// 创建配置目录
	if err := os.MkdirAll(ConfigDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	// 创建日志目录
	if err := os.MkdirAll(LogDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 创建临时目录
	if err := os.MkdirAll(TempDir, 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}

	return nil
}


