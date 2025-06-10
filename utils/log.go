package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RotateLogFile 轮转日志文件
func RotateLogFile(logFilePath string) error {
	// 检查日志文件是否存在
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		return nil // 如果日志文件不存在，不需要轮转
	}

	// 生成带时间戳的新文件名
	timestamp := time.Now().Format("20060102-150405")
	rotatedFilePath := logFilePath + "." + timestamp

	// 重命名当前日志文件
	if err := os.Rename(logFilePath, rotatedFilePath); err != nil {
		return fmt.Errorf("重命名日志文件失败: %w", err)
	}

	// 创建新的日志文件
	newFile, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("创建新日志文件失败: %w", err)
	}
	newFile.Close() // 我们只是创建文件，实际的写入将由日志记录器处理

	return nil
}

// CleanOldLogs 清理旧的日志文件，只保留最新的 maxFiles 个
func CleanOldLogs(logFilePath string, maxFiles int) error {
	if maxFiles <= 0 {
		maxFiles = 7 // 默认保留7个文件
	}

	// 获取日志文件所在的目录
	dir := "."
	if lastSlash := strings.LastIndex(logFilePath, "/"); lastSlash != -1 {
		dir = logFilePath[:lastSlash]
	}

	// 获取日志文件的基本名称
	baseName := logFilePath
	if lastSlash := strings.LastIndex(logFilePath, "/"); lastSlash != -1 {
		baseName = logFilePath[lastSlash+1:]
	}

	// 读取目录中的所有文件
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取日志目录失败: %w", err)
	}

	// 查找匹配的日志文件并按时间排序
	var logFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), baseName+".") {
			logFiles = append(logFiles, filepath.Join(dir, file.Name()))
		}
	}

	// 如果日志文件少于或等于maxFiles个，不需要清理
	if len(logFiles) <= maxFiles {
		return nil
	}

	// 按修改时间排序
	sort.Slice(logFiles, func(i, j int) bool {
		infoI, _ := os.Stat(logFiles[i])
		infoJ, _ := os.Stat(logFiles[j])
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	// 删除最旧的日志文件，保留最新的maxFiles个
	for i := 0; i < len(logFiles)-maxFiles; i++ {
		if err := os.Remove(logFiles[i]); err != nil {
			return fmt.Errorf("删除旧日志文件 %s 失败: %w", logFiles[i], err)
		}
	}

	return nil
}
