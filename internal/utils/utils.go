package utils

import (
	"os"
)

// GetCurrentExecutablePath 获取当前可执行文件路径
func GetCurrentExecutablePath() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "unknown"
}

// GetCurrentWorkingDir 获取当前工作目录
func GetCurrentWorkingDir() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "unknown"
}
