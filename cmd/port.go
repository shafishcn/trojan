package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"trojan/trojan"
)

var portCmd = &cobra.Command{
	Use:   "port [port]",
	Short: "修改trojan端口",
	Long: `修改trojan端口
直接传入端口号: trojan port 443
不传入则进入交互模式: trojan port`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 1 {
			port, err := strconv.Atoi(args[0])
			if err != nil {
				fmt.Println("端口号必须是数字")
				return
			}
			if port < 1 || port > 65535 {
				fmt.Println("端口号必须在1-65535范围内")
				return
			}
			trojan.ChangePortByNum(port)
		} else {
			trojan.ChangePort()
		}
	},
}

func init() {
	rootCmd.AddCommand(portCmd)
}
