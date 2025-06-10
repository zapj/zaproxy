package commands

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:   "zaproxy",
	Short: "代理服务器",
	Long:  `简单的代理服务器`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
		err := cmd.Help()
		if err != nil {
			return
		}
	},
}

func init() {
	rootCmd.PersistentFlags().BoolP("daemon", "d", false, "start daemon")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
