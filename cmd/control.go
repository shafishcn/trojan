package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"trojan/control"
	"trojan/util"
)

var (
	controlHost               string
	controlPort               int
	controlStore              string
	controlDSN                string
	controlToken              string
	agentToken                string
	metricsToken              string
	controlJWT                string
	adminUser                 string
	adminPass                 string
	sessionHours              int
	loginRate                 int
	agentRate                 int
	auditRetentionDays        int
	taskRetentionDays         int
	usageRetentionDays        int
	cleanupIntervalMin        int
	nodeStaleMinutes          int
	failedTaskAlertThreshold  int
	pendingTaskAlertThreshold int
)

// controlCmd starts the prototype control-plane server.
var controlCmd = &cobra.Command{
	Use:   "control",
	Short: "启动多节点控制中心原型",
	Run: func(cmd *cobra.Command, args []string) {
		store, cleanup, err := buildControlStore()
		if err != nil {
			fmt.Println(err)
			return
		}
		defer cleanup()
		if adminPass != "" {
			admin, err := store.EnsureControlAdmin(control.EnsureControlAdminRequest{
				Username: adminUser,
				Password: adminPass,
				Role:     "super_admin",
			})
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Printf("bootstrapped control admin: %s (%s)\n", admin.Username, admin.Role)
		}
		if controlJWT == "" {
			controlJWT = os.Getenv("TROJAN_CONTROL_JWT_SECRET")
		}
		if controlJWT == "" && (adminPass != "" || controlAdminExists(store, adminUser)) {
			controlJWT = util.RandString(32, util.ALL)
		}
		if controlJWT == "" && controlToken == "" {
			fmt.Println("warning: control auth is disabled; configure --admin-pass or --control-token for production use")
		}
		fmt.Printf("control plane listening on %s:%d\n", controlHost, controlPort)
		startCleanupLoop(store)
		router := control.NewRouter(store, control.ServerOptions{
			ControlToken:              controlToken,
			AgentToken:                agentToken,
			MetricsToken:              metricsToken,
			ControlJWTSecret:          controlJWT,
			ControlSessionTTL:         time.Duration(sessionHours) * time.Hour,
			LoginRateLimit:            loginRate,
			AgentRateLimit:            agentRate,
			AuditRetentionDays:        auditRetentionDays,
			TaskRetentionDays:         taskRetentionDays,
			UsageRetentionDays:        usageRetentionDays,
			CleanupInterval:           time.Duration(cleanupIntervalMin) * time.Minute,
			NodeStaleMinutes:          nodeStaleMinutes,
			FailedTaskAlertThreshold:  failedTaskAlertThreshold,
			PendingTaskAlertThreshold: pendingTaskAlertThreshold,
		})
		if err := router.Run(fmt.Sprintf("%s:%d", controlHost, controlPort)); err != nil {
			fmt.Println(err)
		}
	},
}

func controlAdminExists(store control.Store, username string) bool {
	if store == nil || username == "" {
		return false
	}
	_, err := store.GetControlAdmin(username)
	return err == nil
}

func init() {
	controlCmd.Flags().StringVar(&controlHost, "host", "0.0.0.0", "控制中心监听地址")
	controlCmd.Flags().IntVarP(&controlPort, "port", "p", 8081, "控制中心监听端口")
	controlCmd.Flags().StringVar(&controlStore, "store", "memory", "控制中心存储后端(memory/mysql)")
	controlCmd.Flags().StringVar(&controlDSN, "dsn", "", "MySQL DSN，示例: root:pass@tcp(127.0.0.1:3306)/trojan_control?parseTime=true")
	controlCmd.Flags().StringVar(&controlToken, "control-token", "", "控制端 Bearer Token")
	controlCmd.Flags().StringVar(&agentToken, "agent-token", "", "Agent Bearer Token")
	controlCmd.Flags().StringVar(&metricsToken, "metrics-token", "", "Prometheus 指标接口 Bearer Token")
	controlCmd.Flags().StringVar(&controlJWT, "jwt-secret", "", "控制中心 JWT Secret，可用 TROJAN_CONTROL_JWT_SECRET")
	controlCmd.Flags().StringVar(&adminUser, "admin-user", "admin", "启动时自动创建的控制中心管理员用户名")
	controlCmd.Flags().StringVar(&adminPass, "admin-pass", "", "启动时自动创建的控制中心管理员密码")
	controlCmd.Flags().IntVar(&sessionHours, "session-hours", 12, "控制中心 JWT 会话有效期(小时)")
	controlCmd.Flags().IntVar(&loginRate, "login-rate-limit", 30, "管理员登录接口每分钟最大请求数，设为负数可关闭")
	controlCmd.Flags().IntVar(&agentRate, "agent-rate-limit", 600, "Agent API 每分钟最大请求数，设为负数可关闭")
	controlCmd.Flags().IntVar(&auditRetentionDays, "audit-retention-days", 90, "控制中心审计日志保留天数，设为0或负数可关闭自动清理")
	controlCmd.Flags().IntVar(&taskRetentionDays, "task-retention-days", 30, "已完成任务和任务事件保留天数，设为0或负数可关闭自动清理")
	controlCmd.Flags().IntVar(&usageRetentionDays, "usage-retention-days", 30, "节点 usage 快照保留天数，设为0或负数可关闭自动清理")
	controlCmd.Flags().IntVar(&cleanupIntervalMin, "cleanup-interval-minutes", 60, "后台历史数据清理间隔(分钟)，设为0或负数可关闭自动清理任务")
	controlCmd.Flags().IntVar(&nodeStaleMinutes, "node-stale-minutes", 10, "节点超过多少分钟未上报心跳时进入告警")
	controlCmd.Flags().IntVar(&failedTaskAlertThreshold, "failed-task-alert-threshold", 1, "失败任务达到多少条时触发告警")
	controlCmd.Flags().IntVar(&pendingTaskAlertThreshold, "pending-task-alert-threshold", 20, "待处理任务达到多少条时触发积压告警")
	rootCmd.AddCommand(controlCmd)
}

func startCleanupLoop(store control.Store) {
	req := control.CleanupRequest{
		AuditRetentionDays: auditRetentionDays,
		TaskRetentionDays:  taskRetentionDays,
		UsageRetentionDays: usageRetentionDays,
	}
	if cleanupIntervalMin <= 0 {
		return
	}
	if req.AuditRetentionDays <= 0 && req.TaskRetentionDays <= 0 && req.UsageRetentionDays <= 0 {
		return
	}

	runCleanup := func() {
		result, err := store.CleanupHistory(req)
		if err != nil {
			fmt.Printf("control cleanup failed: %v\n", err)
			return
		}
		if result.AuditLogsDeleted == 0 && result.TaskEventsDeleted == 0 && result.TasksDeleted == 0 && result.UsageReportsDeleted == 0 {
			return
		}
		fmt.Printf("control cleanup completed: audit=%d task_events=%d tasks=%d usage=%d\n",
			result.AuditLogsDeleted, result.TaskEventsDeleted, result.TasksDeleted, result.UsageReportsDeleted)
	}

	runCleanup()
	go func() {
		ticker := time.NewTicker(time.Duration(cleanupIntervalMin) * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			runCleanup()
		}
	}()
}

func buildControlStore() (control.Store, func(), error) {
	switch controlStore {
	case "memory":
		return control.NewMemoryStore(), func() {}, nil
	case "mysql":
		dsn := controlDSN
		if dsn == "" {
			dsn = os.Getenv("TROJAN_CONTROL_DSN")
		}
		if dsn == "" {
			return nil, func() {}, fmt.Errorf("mysql store requires --dsn or TROJAN_CONTROL_DSN")
		}
		store, err := control.NewMySQLStore(dsn)
		if err != nil {
			return nil, func() {}, err
		}
		return store, func() {
			_ = store.Close()
		}, nil
	default:
		return nil, func() {}, fmt.Errorf("unsupported control store: %s", controlStore)
	}
}
