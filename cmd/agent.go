package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"trojan/control"
	"trojan/trojan"
)

var (
	agentControlURL        string
	agentAccessToken       string
	agentNodeSecret        string
	agentNodeKey           string
	agentName              string
	agentRegion            string
	agentProvider          string
	agentEndpoint          string
	agentPort              int
	agentPublicIP          string
	agentDomainName        string
	agentTags              string
	agentHeartbeatInterval time.Duration
	agentTaskPollInterval  time.Duration
	agentUsageInterval     time.Duration
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "启动多节点节点代理",
	Run: func(cmd *cobra.Command, args []string) {
		config := control.AgentConfig{
			ControlURL:        agentControlURL,
			AgentToken:        agentAccessToken,
			NodeSecret:        agentNodeSecret,
			NodeKey:           agentNodeKey,
			Name:              agentName,
			Region:            agentRegion,
			Provider:          agentProvider,
			Endpoint:          agentEndpoint,
			Port:              agentPort,
			TrojanType:        trojan.Type(),
			TrojanVersion:     trojan.Version(),
			ManagerVersion:    trojan.MVersion,
			PublicIP:          agentPublicIP,
			DomainName:        agentDomainName,
			Tags:              splitCSV(agentTags),
			HeartbeatInterval: agentHeartbeatInterval,
			TaskPollInterval:  agentTaskPollInterval,
			UsageInterval:     agentUsageInterval,
		}
		runtime := control.NewAgentRuntime(config, control.NewAgentExecutor())
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		fmt.Printf("agent connected to %s as %s\n", agentControlURL, agentNodeKey)
		if err := runtime.Run(ctx); err != nil {
			fmt.Println(err)
		}
	},
}

func init() {
	agentCmd.Flags().StringVar(&agentControlURL, "control-url", "http://127.0.0.1:8081", "控制中心地址")
	agentCmd.Flags().StringVar(&agentAccessToken, "token", "", "Agent Bearer Token")
	agentCmd.Flags().StringVar(&agentNodeSecret, "node-secret", "", "节点独立签名密钥，配置后优先使用签名鉴权")
	agentCmd.Flags().StringVar(&agentNodeKey, "node-key", hostnameOr("node-local"), "节点唯一标识")
	agentCmd.Flags().StringVar(&agentName, "name", hostnameOr("node-local"), "节点显示名称")
	agentCmd.Flags().StringVar(&agentRegion, "region", "", "节点地域")
	agentCmd.Flags().StringVar(&agentProvider, "provider", "", "节点提供商")
	agentCmd.Flags().StringVar(&agentEndpoint, "endpoint", "", "节点入口地址")
	agentCmd.Flags().IntVar(&agentPort, "port", 443, "节点 trojan 端口")
	agentCmd.Flags().StringVar(&agentPublicIP, "public-ip", "", "节点公网IP")
	agentCmd.Flags().StringVar(&agentDomainName, "domain", "", "节点域名")
	agentCmd.Flags().StringVar(&agentTags, "tags", "", "节点标签，逗号分隔")
	agentCmd.Flags().DurationVar(&agentHeartbeatInterval, "heartbeat-interval", 30*time.Second, "心跳间隔")
	agentCmd.Flags().DurationVar(&agentTaskPollInterval, "task-poll-interval", 15*time.Second, "任务轮询间隔")
	agentCmd.Flags().DurationVar(&agentUsageInterval, "usage-interval", time.Minute, "流量上报间隔")
	rootCmd.AddCommand(agentCmd)
}

func splitCSV(input string) []string {
	if strings.TrimSpace(input) == "" {
		return []string{}
	}
	items := strings.Split(input, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func hostnameOr(defaultValue string) string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return defaultValue
	}
	return host
}
