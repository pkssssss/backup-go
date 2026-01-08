package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"backup-go/internal/logger"
)

const (
	DefaultKeepDays = 30
)

type Config struct {
	Cos    CosConfig    `toml:"cos"`
	Backup BackupConfig `toml:"backup"`
}

type CosConfig struct {
	SecretID  string `toml:"secret_id"`
	SecretKey string `toml:"secret_key"`
	Bucket    string `toml:"bucket"`
	Region    string `toml:"region"`
	Prefix    string `toml:"prefix"`
	KeepDays  int    `toml:"keep_days"` // COS备份文件保留天数
}

type BackupConfig struct {
	DataDir  string         `toml:"data_dir"`
	Schedule ScheduleConfig `toml:"schedule"`
}

type ScheduleConfig struct {
	Enabled  bool   `toml:"enabled"`
	Hour     int    `toml:"hour"`     // 小时 (0-23)
	Minute   int    `toml:"minute"`   // 分钟 (0-59)
	Timezone string `toml:"timezone"` // 时区，如 "Asia/Shanghai"
}

// SaveConfig 保存配置到文件
func SaveConfig(cfgPath string, cfg *Config) error {
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

// LoadConfig 简化的配置加载函数
func LoadConfig(cfgPath string) (*Config, error) {
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

// GenerateDefaultConfig 生成默认配置
func GenerateDefaultConfig(configPath string) error {
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

	logger.PrintLog("init", "已生成默认配置文件 "+configPath+"，请修改后重新运行。")
	logger.PrintLog("init", "注意: Bucket 需为 'name-appid' 格式，Region 形如 ap-shanghai。")
	return nil
}

// CalculateNextRunTime 计算下次运行时间
func CalculateNextRunTime(schedule ScheduleConfig) time.Time {
	now := time.Now()
	var loc *time.Location
	var err error

	if schedule.Timezone != "" {
		loc, err = time.LoadLocation(schedule.Timezone)
		if err != nil {
			logger.PrintLog("warn", fmt.Sprintf("加载时区失败 '%s'，使用系统默认时区: %v", schedule.Timezone, err))
			loc = time.Local
		}
	} else {
		loc = time.Local
	}

	// 转换当前时间到目标时区
	nowInLoc := now.In(loc)
	
	// 构建今天的目标时间
	nextRun := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), 
		schedule.Hour, schedule.Minute, 0, 0, loc)

	// 如果今天的时间已经过了，推到明天
	if nextRun.Before(nowInLoc) {
		nextRun = nextRun.Add(24 * time.Hour)
	}

	return nextRun
}
