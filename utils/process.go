package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// IsRunning 检查指定PID的进程是否正在运行
func IsRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// 在Unix系统中，FindProcess总是返回一个非nil的Process，
	// 所以我们需要发送一个信号来检查进程是否真的存在
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// ReadPIDFile 从PID文件中读取进程ID
func ReadPIDFile(pidFile string) (int, error) {
	content, err := ioutil.ReadFile(pidFile)
	if err != nil {
		return 0, fmt.Errorf("读取PID文件失败: %w", err)
	}

	pid, err := strconv.Atoi(string(content))
	if err != nil {
		return 0, fmt.Errorf("解析PID失败: %w", err)
	}

	return pid, nil
}

// WritePIDFile 将当前进程的PID写入文件
func WritePIDFile(pidFile string) error {
	pid := os.Getpid()
	return ioutil.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
}

// IsZombieProcess 检查指定PID的进程是否是僵尸进程
func IsZombieProcess(pid int) (bool, error) {
	procPath := fmt.Sprintf("/proc/%d/status", pid)
	content, err := ioutil.ReadFile(procPath)
	if err != nil {
		return false, err
	}

	// 在状态文件中查找 "State:" 行
	lines := string(content)
	for _, line := range strings.Split(lines, "\n") {
		if strings.HasPrefix(line, "State:") {
			// 如果状态是 'Z' 或 'z'，则是僵尸进程
			return strings.Contains(line, " Z ") || strings.Contains(line, " z "), nil
		}
	}

	return false, fmt.Errorf("无法确定进程状态")
}

// CleanZombieProcess 清理僵尸进程
func CleanZombieProcess(pidFile string) error {
	pid, err := ReadPIDFile(pidFile)
	if err != nil {
		return err
	}

	isZombie, err := IsZombieProcess(pid)
	if err != nil {
		return err
	}

	if isZombie {
		// 尝试终止僵尸进程
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}

		// 发送SIGTERM信号
		if err := process.Signal(syscall.SIGTERM); err != nil {
			// 如果SIGTERM失败，尝试SIGKILL
			if err := process.Signal(syscall.SIGKILL); err != nil {
				return fmt.Errorf("无法终止僵尸进程: %w", err)
			}
		}

		// 删除PID文件
		if err := os.Remove(pidFile); err != nil {
			return fmt.Errorf("删除PID文件失败: %w", err)
		}
	}

	return nil
}
