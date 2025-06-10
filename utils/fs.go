package utils

import (
	"os"
	"path/filepath"
)

// EnsureDir 确保目录存在，如果不存在则创建
func EnsureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

// FileExists 检查文件是否存在
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// GetAbsPath 获取绝对路径
func GetAbsPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

// RemoveFileIfExists 如果文件存在则删除
func RemoveFileIfExists(path string) error {
	if FileExists(path) {
		return os.Remove(path)
	}
	return nil
}
