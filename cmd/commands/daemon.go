package commands

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
)

var (
	pidFile     = "/var/run/zaproxy.pid"
	logFile     = "/var/log/zaproxy.log"
	daemonFlags = struct {
		pidFile string
		logFile string
	}{}
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "以守护进程模式运行zaproxy",
	Long:  `以守护进程模式运行zaproxy，支持start、stop、restart和status操作`,
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "启动zaproxy守护进程",
	Run: func(cmd *cobra.Command, args []string) {
		if isRunning() {
			fmt.Println("zaproxy守护进程已经在运行")
			return
		}
		startDaemon()
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "停止zaproxy守护进程",
	Run: func(cmd *cobra.Command, args []string) {
		stopDaemon()
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "重启zaproxy守护进程",
	Run: func(cmd *cobra.Command, args []string) {
		stopDaemon()
		startDaemon()
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看zaproxy守护进程状态",
	Run: func(cmd *cobra.Command, args []string) {
		checkStatus()
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)

	// 添加守护进程相关的标志
	daemonCmd.PersistentFlags().StringVar(&daemonFlags.pidFile, "pid-file", pidFile, "PID文件路径")
	daemonCmd.PersistentFlags().StringVar(&daemonFlags.logFile, "log-file", logFile, "日志文件路径")
}

func startDaemon() {
	// 获取当前可执行文件的路径
	executable, err := os.Executable()
	if err != nil {
		log.Fatalf("无法获取可执行文件路径: %v", err)
	}

	// 确保日志文件和PID文件的目录存在
	if err := ensureDir(daemonFlags.logFile); err != nil {
		log.Fatalf("无法创建日志文件目录: %v", err)
	}
	if err := ensureDir(daemonFlags.pidFile); err != nil {
		log.Fatalf("无法创建PID文件目录: %v", err)
	}

	// 创建日志文件
	logFd, err := os.OpenFile(daemonFlags.logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("无法创建日志文件: %v", err)
	}

	// 准备子进程
	cmd := exec.Command(executable, "http")
	cmd.Stdout = logFd
	cmd.Stderr = logFd
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // 创建新的会话
	}

	// 启动子进程
	if err := cmd.Start(); err != nil {
		log.Fatalf("启动守护进程失败: %v", err)
	}

	// 写入PID文件
	if err := os.WriteFile(daemonFlags.pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		log.Printf("无法写入PID文件: %v", err)
		cmd.Process.Kill()
		return
	}

	fmt.Printf("zaproxy守护进程已启动，PID: %d\n", cmd.Process.Pid)
}

func stopDaemon() {
	pid, err := readPIDFile()
	if err != nil {
		fmt.Printf("读取PID文件失败: %v\n", err)
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("查找进程失败: %v\n", err)
		return
	}

	// 发送SIGTERM信号
	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("停止进程失败: %v\n", err)
		return
	}

	// 删除PID文件
	if err := os.Remove(daemonFlags.pidFile); err != nil {
		fmt.Printf("删除PID文件失败: %v\n", err)
	}

	fmt.Println("zaproxy守护进程已停止")
}

func isRunning() bool {
	pid, err := readPIDFile()
	if err != nil {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// 在Unix系统中，FindProcess总是成功的，所以我们需要发送信号0来检查进程是否真的存在
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func checkStatus() {
	if !isRunning() {
		fmt.Println("zaproxy守护进程未运行")
		return
	}

	pid, _ := readPIDFile()
	fmt.Printf("zaproxy守护进程正在运行，PID: %d\n", pid)
}

func readPIDFile() (int, error) {
	data, err := os.ReadFile(daemonFlags.pidFile)
	if err != nil {
		return 0, fmt.Errorf("读取PID文件失败: %v", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, fmt.Errorf("解析PID失败: %v", err)
	}

	return pid, nil
}

// 确保目录存在
func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
