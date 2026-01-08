package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	LogDir = "logs"
)

// PrintLog 统一日志输出
func PrintLog(level, message string) {
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

// writeToFile 写入日志到文件
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

// ExitIfError 如果发生错误则退出
func ExitIfError(err error, msg string) {
	if err != nil {
		PrintLog("error", fmt.Sprintf("%s: %v", msg, err))
		os.Exit(1)
	}
}

// ExitIfNil 如果返回值为 nil 且有错误则退出
func ExitIfNil[T any](v T, err error, msg string) T {
	if err != nil {
		PrintLog("error", fmt.Sprintf("%s: %v", msg, err))
		os.Exit(1)
	}
	return v
}
