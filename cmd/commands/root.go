package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// 配置文件路径
	cfgFile string

	// 版本信息
	Version   = "1.0.0"
	BuildTime = "unknown"
	GitCommit = "unknown"

	// 全局配置选项
	listenAddr string
	timeout    int
	logLevel   string
	authFile   string
)

var rootCmd = &cobra.Command{
	Use:   "zaproxy",
	Short: "高性能HTTP/HTTPS代理服务器",
	Long: `zaproxy 是一个高性能的HTTP/HTTPS代理服务器，支持：

- HTTP/HTTPS代理
- 基本认证
- 自定义超时
- 详细日志记录
- 守护进程模式`,
	Version: Version,
	Run: func(cmd *cobra.Command, args []string) {
		// 显示帮助信息
		if err := cmd.Help(); err != nil {
			fmt.Fprintf(os.Stderr, "显示帮助信息时出错：%v\n", err)
		}
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	// 全局标志
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认为 $HOME/.zaproxy.yaml)")
	rootCmd.PersistentFlags().BoolP("daemon", "d", false, "以守护进程模式运行")
	rootCmd.PersistentFlags().StringVarP(&listenAddr, "listen", "l", ":8080", "监听地址和端口")
	rootCmd.PersistentFlags().IntVarP(&timeout, "timeout", "t", 300, "连接超时时间(秒)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "日志级别 (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&authFile, "auth-file", "", "认证文件路径 (格式：username:password)")

	// 绑定配置
	viper.BindPFlag("listen", rootCmd.PersistentFlags().Lookup("listen"))
	viper.BindPFlag("timeout", rootCmd.PersistentFlags().Lookup("timeout"))
	viper.BindPFlag("log_level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("auth_file", rootCmd.PersistentFlags().Lookup("auth-file"))
	viper.BindPFlag("daemon", rootCmd.PersistentFlags().Lookup("daemon"))
}

// initConfig 读取配置文件和环境变量
func initConfig() {
	if cfgFile != "" {
		// 使用指定的配置文件
		viper.SetConfigFile(cfgFile)
	} else {
		// 查找主目录
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// 在主目录中查找配置
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".zaproxy")
	}

	// 读取环境变量
	viper.SetEnvPrefix("ZAPROXY")
	viper.AutomaticEnv()

	// 如果找到配置文件，读取它
	if err := viper.ReadInConfig(); err == nil {
		fmt.Printf("使用配置文件：%s\n", viper.ConfigFileUsed())
	}
}

// Execute 执行根命令
func Execute() {
	// 添加版本信息
	rootCmd.SetVersionTemplate(fmt.Sprintf(`版本：%s
构建时间：%s
Git提交：%s
`, Version, BuildTime, GitCommit))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "执行出错：%v\n", err)
		os.Exit(1)
	}
}

// createDefaultConfigIfNotExists 如果配置文件不存在，创建默认配置
func createDefaultConfigIfNotExists() error {
	if cfgFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		cfgFile = filepath.Join(home, ".zaproxy.yaml")
	}

	// 检查文件是否存在
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		defaultConfig := []byte(`# zaproxy 默认配置
listen: ":8080"
timeout: 300
log_level: "info"
auth_file: ""
daemon: false
`)
		return os.WriteFile(cfgFile, defaultConfig, 0644)
	}
	return nil
}
