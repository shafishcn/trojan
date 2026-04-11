package control

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ServerOptions configures control-plane middleware such as auth.
type ServerOptions struct {
	ControlToken              string
	AgentToken                string
	MetricsToken              string
	ControlJWTSecret          string
	ControlSessionTTL         time.Duration
	LoginRateLimit            int
	AgentRateLimit            int
	AuditRetentionDays        int
	TaskRetentionDays         int
	UsageRetentionDays        int
	CleanupInterval           time.Duration
	NodeStaleMinutes          int
	FailedTaskAlertThreshold  int
	PendingTaskAlertThreshold int
}

// NewRouter creates the control-plane router.
func NewRouter(store Store, opts ...ServerOptions) *gin.Engine {
	var option ServerOptions
	if len(opts) > 0 {
		option = opts[0]
	}
	router := gin.Default()
	registerUIRoutes(router)
	buildControlAuthRoutes(router, store, option)
	router.GET("/healthz", func(c *gin.Context) {
		respond(c, http.StatusOK, "success", gin.H{"status": "ok"})
	})
	router.GET("/readyz", func(c *gin.Context) {
		status, err := controlRuntimeStatus(store)
		if err != nil || !status.Healthy {
			respond(c, http.StatusServiceUnavailable, "not ready", status)
			return
		}
		respond(c, http.StatusOK, "ready", status)
	})
	router.GET("/metrics", metricsAuthMiddleware(option.MetricsToken), func(c *gin.Context) {
		snapshot, err := controlMetricsSnapshot(store)
		statusCode := http.StatusOK
		if err != nil || snapshot == nil || !snapshot.Healthy {
			statusCode = http.StatusServiceUnavailable
		}
		c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		c.String(statusCode, renderPrometheusMetrics(snapshot))
	})

	agentLimiter := newFixedWindowLimiter(resolveRateLimitPerMinute(option.AgentRateLimit, 600), time.Minute)

	admin := router.Group("/api/control")
	admin.Use(controlAuthMiddleware(store, option))
	{
		admin.GET("/nodes", requireControlRole("viewer"), func(c *gin.Context) {
			nodes, err := store.ListNodes()
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{"nodes": nodes})
		})

		admin.GET("/runtime/status", requireControlRole("viewer"), func(c *gin.Context) {
			status, err := controlRuntimeStatus(store)
			if err != nil && status == nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{
				"runtime": status,
				"config": gin.H{
					"controlAuthEnabled":        option.ControlJWTSecret != "" || option.ControlToken != "",
					"jwtEnabled":                option.ControlJWTSecret != "",
					"tokenFallback":             option.ControlToken != "",
					"agentTokenEnabled":         option.AgentToken != "",
					"metricsTokenEnabled":       option.MetricsToken != "",
					"loginRateLimit":            resolveRateLimitPerMinute(option.LoginRateLimit, 30),
					"agentRateLimit":            resolveRateLimitPerMinute(option.AgentRateLimit, 600),
					"auditRetentionDays":        option.AuditRetentionDays,
					"taskRetentionDays":         option.TaskRetentionDays,
					"usageRetentionDays":        option.UsageRetentionDays,
					"cleanupIntervalSec":        int(option.CleanupInterval.Seconds()),
					"nodeStaleMinutes":          option.NodeStaleMinutes,
					"failedTaskAlertThreshold":  option.FailedTaskAlertThreshold,
					"pendingTaskAlertThreshold": option.PendingTaskAlertThreshold,
				},
			})
		})

		admin.GET("/alerts/summary", requireControlRole("viewer"), func(c *gin.Context) {
			summary, err := controlAlertSummary(store, option)
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			respond(c, http.StatusOK, "success", summary)
		})

		admin.POST("/nodes/:nodeKey/agent-secret/rotate", requireControlRole("admin"), func(c *gin.Context) {
			nodeKey := c.Param("nodeKey")
			secret, err := store.RotateNodeAgentSecret(nodeKey)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "node.rotate_agent_secret",
				ResourceType: "node",
				ResourceID:   nodeKey,
				Message:      "node agent secret rotated",
			})
			respond(c, http.StatusOK, "success", gin.H{
				"nodeKey":     nodeKey,
				"agentSecret": secret,
			})
		})

		admin.GET("/overview", requireControlRole("viewer"), func(c *gin.Context) {
			nodes, err := store.ListNodes()
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			users, err := store.ListUsers()
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			tasks, err := store.ListTasks(TaskQuery{Limit: 200})
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			taskStatusCount := map[string]int{}
			for _, task := range tasks {
				taskStatusCount[task.Status]++
			}
			var lastSeenAt *string
			for _, node := range nodes {
				if node.LastSeenAt.IsZero() {
					continue
				}
				formatted := node.LastSeenAt.Format(time.RFC3339)
				if lastSeenAt == nil || formatted > *lastSeenAt {
					lastSeenAt = &formatted
				}
			}
			overview := gin.H{
				"nodeCount":      len(nodes),
				"userCount":      len(users),
				"taskCount":      len(tasks),
				"taskStatus":     taskStatusCount,
				"lastNodeSeenAt": lastSeenAt,
			}
			respond(c, http.StatusOK, "success", overview)
		})

		admin.GET("/users", requireControlRole("viewer"), func(c *gin.Context) {
			users, err := store.ListUsers()
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{"users": users})
		})

		admin.POST("/users", requireControlRole("admin"), func(c *gin.Context) {
			var req CreateUserRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			user, err := store.CreateUser(req)
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "user.upsert",
				ResourceType: "user",
				ResourceID:   user.Username,
				Message:      "control user created or updated",
				Details: map[string]interface{}{
					"quota":      user.Quota,
					"useDays":    user.UseDays,
					"expiryDate": user.ExpiryDate,
				},
			})
			respond(c, http.StatusOK, "success", user)
		})

		admin.GET("/users/:username", requireControlRole("viewer"), func(c *gin.Context) {
			user, err := store.GetUser(c.Param("username"))
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", user)
		})

		admin.DELETE("/users/:username", requireControlRole("admin"), func(c *gin.Context) {
			username := c.Param("username")
			if err := store.DeleteUser(username); err != nil {
				handleStoreError(c, err)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "user.delete",
				ResourceType: "user",
				ResourceID:   username,
				Message:      "control user deleted",
			})
			respond(c, http.StatusOK, "success", gin.H{"username": username})
		})

		admin.POST("/users/:username/bindings", requireControlRole("admin"), func(c *gin.Context) {
			var req struct {
				NodeKey string `json:"nodeKey" binding:"required"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			binding, err := store.BindUserToNode(c.Param("username"), req.NodeKey)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "user.bind_node",
				ResourceType: "binding",
				ResourceID:   c.Param("username") + ":" + req.NodeKey,
				Message:      "user bound to node",
			})
			respond(c, http.StatusOK, "success", binding)
		})

		admin.DELETE("/users/:username/bindings/:nodeKey", requireControlRole("admin"), func(c *gin.Context) {
			username := c.Param("username")
			nodeKey := c.Param("nodeKey")
			if err := store.UnbindUserFromNode(username, nodeKey); err != nil {
				handleStoreError(c, err)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "user.unbind_node",
				ResourceType: "binding",
				ResourceID:   username + ":" + nodeKey,
				Message:      "user unbound from node",
			})
			respond(c, http.StatusOK, "success", gin.H{
				"username": username,
				"nodeKey":  nodeKey,
			})
		})

		admin.GET("/users/:username/nodes", requireControlRole("viewer"), func(c *gin.Context) {
			nodes, err := store.UserNodes(c.Param("username"))
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{
				"username": c.Param("username"),
				"nodes":    nodes,
			})
		})

		admin.GET("/users/:username/usage", requireControlRole("viewer"), func(c *gin.Context) {
			limit := 100
			if err := bindLimit(c, &limit); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			usage, err := store.UserUsage(c.Param("username"), limit)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{
				"username": c.Param("username"),
				"usage":    usage,
			})
		})

		admin.GET("/users/:username/subscription/clash", requireControlRole("viewer"), func(c *gin.Context) {
			user, err := store.GetUser(c.Param("username"))
			if err != nil {
				handleStoreError(c, err)
				return
			}
			nodes, err := store.UserNodes(c.Param("username"))
			if err != nil {
				handleStoreError(c, err)
				return
			}
			content, err := BuildClashSubscription(user, nodes)
			if err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			c.Header("content-disposition", fmt.Sprintf("attachment; filename=%s-clash.yaml", user.Username))
			c.Header("content-type", "text/yaml; charset=utf-8")
			c.String(http.StatusOK, content)
		})

		admin.GET("/users/:username/subscription/links", requireControlRole("viewer"), func(c *gin.Context) {
			user, err := store.GetUser(c.Param("username"))
			if err != nil {
				handleStoreError(c, err)
				return
			}
			nodes, err := store.UserNodes(c.Param("username"))
			if err != nil {
				handleStoreError(c, err)
				return
			}
			content, err := BuildTrojanLinks(user, nodes)
			if err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			c.Header("content-disposition", fmt.Sprintf("attachment; filename=%s-links.txt", user.Username))
			c.Header("content-type", "text/plain; charset=utf-8")
			c.String(http.StatusOK, content)
		})

		admin.POST("/tasks", requireControlRole("admin"), func(c *gin.Context) {
			var req CreateTaskRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			task, err := store.CreateTask(req)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "task.create",
				ResourceType: "task",
				ResourceID:   strconv.FormatUint(task.ID, 10),
				Message:      "task created from control plane",
				Details: map[string]interface{}{
					"nodeKey":  task.NodeKey,
					"taskType": task.TaskType,
				},
			})
			respond(c, http.StatusOK, "success", task)
		})

		admin.GET("/tasks", requireControlRole("viewer"), func(c *gin.Context) {
			query, err := bindTaskQuery(c)
			if err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			fetchQuery := query
			fetchQuery.Limit++
			tasks, err := store.ListTasks(fetchQuery)
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			hasMore := len(tasks) > query.Limit
			if hasMore {
				tasks = tasks[:query.Limit]
			}
			respond(c, http.StatusOK, "success", gin.H{
				"tasks":      tasks,
				"limit":      query.Limit,
				"offset":     query.Offset,
				"hasMore":    hasMore,
				"nextOffset": query.Offset + len(tasks),
			})
		})

		admin.GET("/tasks/:id", requireControlRole("viewer"), func(c *gin.Context) {
			taskID, err := parseTaskID(c)
			if err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			task, err := store.GetTask(taskID)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			limit := 100
			if err := bindLimit(c, &limit); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			events, err := store.ListTaskEvents(taskID, limit)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{
				"task":   task,
				"events": events,
			})
		})

		admin.GET("/audit", requireControlRole("admin"), func(c *gin.Context) {
			query, err := bindAuditQuery(c)
			if err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			fetchQuery := query
			fetchQuery.Limit++
			logs, err := store.ListAuditLogs(fetchQuery)
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			hasMore := len(logs) > query.Limit
			if hasMore {
				logs = logs[:query.Limit]
			}
			respond(c, http.StatusOK, "success", gin.H{
				"logs":       logs,
				"limit":      query.Limit,
				"offset":     query.Offset,
				"hasMore":    hasMore,
				"nextOffset": query.Offset + len(logs),
			})
		})

		admin.POST("/maintenance/cleanup", requireControlRole("super_admin"), func(c *gin.Context) {
			var req CleanupRequest
			if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			result, err := store.CleanupHistory(req)
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "maintenance.cleanup",
				ResourceType: "maintenance",
				ResourceID:   "history",
				Message:      "historical control-plane data cleaned up",
				Details: map[string]interface{}{
					"auditRetentionDays": req.AuditRetentionDays,
					"taskRetentionDays":  req.TaskRetentionDays,
					"usageRetentionDays": req.UsageRetentionDays,
				},
			})
			respond(c, http.StatusOK, "success", result)
		})

		admin.GET("/backup/export", requireControlRole("super_admin"), func(c *gin.Context) {
			bundle, err := store.ExportBackup()
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "backup.export",
				ResourceType: "backup",
				ResourceID:   bundle.ExportedAt.Format(time.RFC3339),
				Message:      "control-plane backup exported",
				Details: map[string]interface{}{
					"admins":     len(bundle.Admins),
					"nodes":      len(bundle.Nodes),
					"users":      len(bundle.Users),
					"bindings":   len(bundle.Bindings),
					"tasks":      len(bundle.Tasks),
					"taskEvents": len(bundle.TaskEvents),
					"auditLogs":  len(bundle.AuditLogs),
					"usage":      len(bundle.Usage),
				},
			})
			respond(c, http.StatusOK, "success", gin.H{"backup": bundle})
		})

		admin.POST("/backup/import", requireControlRole("super_admin"), func(c *gin.Context) {
			var bundle BackupBundle
			if err := c.ShouldBindJSON(&bundle); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			if err := store.ImportBackup(bundle); err != nil {
				handleStoreError(c, err)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "backup.import",
				ResourceType: "backup",
				ResourceID:   bundle.ExportedAt.Format(time.RFC3339),
				Message:      "control-plane backup imported",
				Details: map[string]interface{}{
					"admins":     len(bundle.Admins),
					"nodes":      len(bundle.Nodes),
					"users":      len(bundle.Users),
					"bindings":   len(bundle.Bindings),
					"tasks":      len(bundle.Tasks),
					"taskEvents": len(bundle.TaskEvents),
					"auditLogs":  len(bundle.AuditLogs),
					"usage":      len(bundle.Usage),
				},
			})
			respond(c, http.StatusOK, "success", gin.H{
				"admins":     len(bundle.Admins),
				"nodes":      len(bundle.Nodes),
				"users":      len(bundle.Users),
				"bindings":   len(bundle.Bindings),
				"tasks":      len(bundle.Tasks),
				"taskEvents": len(bundle.TaskEvents),
				"auditLogs":  len(bundle.AuditLogs),
				"usage":      len(bundle.Usage),
			})
		})

		admin.POST("/tasks/:id/retry", requireControlRole("admin"), func(c *gin.Context) {
			taskID, err := parseTaskID(c)
			if err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			task, err := store.RetryTask(taskID)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "task.retry",
				ResourceType: "task",
				ResourceID:   strconv.FormatUint(taskID, 10),
				Message:      "task retry requested",
				Details: map[string]interface{}{
					"newTaskId": task.ID,
					"nodeKey":   task.NodeKey,
					"taskType":  task.TaskType,
				},
			})
			respond(c, http.StatusOK, "success", task)
		})

		admin.GET("/nodes/:nodeKey/usage", requireControlRole("viewer"), func(c *gin.Context) {
			limit := 100
			if err := bindLimit(c, &limit); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			usage, err := store.NodeUsage(c.Param("nodeKey"), limit)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{
				"nodeKey": c.Param("nodeKey"),
				"usage":   usage,
			})
		})

		admin.GET("/nodes/:nodeKey/users", requireControlRole("viewer"), func(c *gin.Context) {
			users, err := store.NodeUsers(c.Param("nodeKey"))
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{
				"nodeKey": c.Param("nodeKey"),
				"users":   users,
			})
		})

		admin.POST("/nodes/:nodeKey/sync", requireControlRole("admin"), func(c *gin.Context) {
			nodeKey := c.Param("nodeKey")
			task, err := store.SyncNodeUsers(nodeKey)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "node.sync_users",
				ResourceType: "node",
				ResourceID:   nodeKey,
				Message:      "sync_users task generated for node",
				Details: map[string]interface{}{
					"taskId": task.ID,
				},
			})
			respond(c, http.StatusOK, "success", task)
		})

		admin.GET("/admins", requireControlRole("super_admin"), func(c *gin.Context) {
			admins, err := store.ListControlAdmins()
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{"admins": admins})
		})

		admin.POST("/admins", requireControlRole("super_admin"), func(c *gin.Context) {
			var req EnsureControlAdminRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			if req.Role == "" {
				req.Role = "admin"
			}
			if !controlRoleAllowed(req.Role) {
				respond(c, http.StatusBadRequest, ErrInvalidRole.Error(), nil)
				return
			}
			admin, err := store.EnsureControlAdmin(req)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "admin.create",
				ResourceType: "admin",
				ResourceID:   admin.Username,
				Message:      "control admin created",
				Details: map[string]interface{}{
					"role": admin.Role,
				},
			})
			respond(c, http.StatusOK, "success", admin)
		})

		admin.PATCH("/admins/:username", requireControlRole("super_admin"), func(c *gin.Context) {
			var req UpdateControlAdminRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			if req.Role != "" && !controlRoleAllowed(req.Role) {
				respond(c, http.StatusBadRequest, ErrInvalidRole.Error(), nil)
				return
			}
			if req.Status != "" && !controlStatusAllowed(req.Status) {
				respond(c, http.StatusBadRequest, ErrInvalidStatus.Error(), nil)
				return
			}
			current := currentControlAdmin(c)
			admins, err := store.ListControlAdmins()
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			if err := guardSuperAdminMutation(current, c.Param("username"), req, admins); err != nil {
				handleStoreError(c, err)
				return
			}
			admin, err := store.UpdateControlAdmin(c.Param("username"), req)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			appendAuditLog(store, c, CreateAuditLogRequest{
				Action:       "admin.update",
				ResourceType: "admin",
				ResourceID:   admin.Username,
				Message:      "control admin updated",
				Details: map[string]interface{}{
					"role":   admin.Role,
					"status": admin.Status,
				},
			})
			respond(c, http.StatusOK, "success", admin)
		})
	}

	agent := router.Group("/api/agent")
	agent.Use(rateLimitMiddleware(agentLimiter, agentRateLimitKey, "too many agent requests"))
	agent.Use(agentAuthMiddleware(store, option.AgentToken))
	{
		agent.POST("/register", func(c *gin.Context) {
			var req RegisterNodeRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			node, err := store.RegisterNode(req)
			if err != nil {
				respond(c, http.StatusInternalServerError, err.Error(), nil)
				return
			}
			respond(c, http.StatusOK, "success", node)
		})

		agent.POST("/heartbeat", func(c *gin.Context) {
			var req HeartbeatRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			node, err := store.Heartbeat(req)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", node)
		})

		agent.GET("/tasks/pending", func(c *gin.Context) {
			nodeKey := c.Query("nodeKey")
			if nodeKey == "" {
				respond(c, http.StatusBadRequest, "nodeKey is required", nil)
				return
			}

			limit := 20
			if err := bindLimit(c, &limit); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}

			tasks, err := store.PendingTasks(nodeKey, limit)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{
				"nodeKey": nodeKey,
				"tasks":   tasks,
			})
		})

		agent.POST("/tasks/:id/start", func(c *gin.Context) {
			taskID, err := parseTaskID(c)
			if err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			var req StartTaskRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			task, err := store.StartTask(taskID, req)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", task)
		})

		agent.POST("/tasks/:id/result", func(c *gin.Context) {
			taskID, err := parseTaskID(c)
			if err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			var req FinishTaskRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			task, err := store.FinishTask(taskID, req)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", task)
		})

		agent.POST("/usage", func(c *gin.Context) {
			var req UsageReportRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				respond(c, http.StatusBadRequest, err.Error(), nil)
				return
			}
			usage, err := store.ReportUsage(req)
			if err != nil {
				handleStoreError(c, err)
				return
			}
			respond(c, http.StatusOK, "success", gin.H{
				"nodeKey": req.NodeKey,
				"count":   len(usage),
			})
		})
	}

	return router
}

// Start launches the control-plane HTTP server.
func Start(host string, port int, store Store) error {
	if store == nil {
		store = NewMemoryStore()
	}
	return NewRouter(store).Run(fmt.Sprintf("%s:%d", host, port))
}

func bindLimit(c *gin.Context, limit *int) error {
	raw := c.Query("limit")
	if raw == "" {
		return nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return errors.New("limit must be an integer")
	}
	if parsed <= 0 {
		return errors.New("limit must be positive")
	}
	if parsed > 100 {
		parsed = 100
	}
	*limit = parsed
	return nil
}

func bindOffset(c *gin.Context, offset *int) error {
	raw := c.Query("offset")
	if raw == "" {
		return nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return errors.New("offset must be an integer")
	}
	if parsed < 0 {
		return errors.New("offset must be non-negative")
	}
	*offset = parsed
	return nil
}

func bindTaskQuery(c *gin.Context) (TaskQuery, error) {
	query := TaskQuery{
		Limit:    50,
		Offset:   0,
		NodeKey:  strings.TrimSpace(c.Query("nodeKey")),
		TaskType: strings.TrimSpace(c.Query("taskType")),
		Status:   strings.TrimSpace(c.Query("status")),
	}
	if err := bindLimit(c, &query.Limit); err != nil {
		return TaskQuery{}, err
	}
	if err := bindOffset(c, &query.Offset); err != nil {
		return TaskQuery{}, err
	}
	return query, nil
}

func bindAuditQuery(c *gin.Context) (AuditQuery, error) {
	query := AuditQuery{
		Limit:        100,
		Offset:       0,
		Actor:        strings.TrimSpace(c.Query("actor")),
		Action:       strings.TrimSpace(c.Query("action")),
		ResourceType: strings.TrimSpace(c.Query("resourceType")),
	}
	if err := bindLimit(c, &query.Limit); err != nil {
		return AuditQuery{}, err
	}
	if err := bindOffset(c, &query.Offset); err != nil {
		return AuditQuery{}, err
	}
	return query, nil
}

func handleStoreError(c *gin.Context, err error) {
	if errors.Is(err, ErrNodeNotFound) {
		respond(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	if errors.Is(err, ErrTaskNotFound) {
		respond(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	if errors.Is(err, ErrUserNotFound) {
		respond(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	if errors.Is(err, ErrBindingNotFound) {
		respond(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	if errors.Is(err, ErrAdminNotFound) {
		respond(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	if errors.Is(err, ErrInvalidCredentials) {
		respond(c, http.StatusUnauthorized, err.Error(), nil)
		return
	}
	if errors.Is(err, ErrPermissionDenied) {
		respond(c, http.StatusForbidden, err.Error(), nil)
		return
	}
	if errors.Is(err, ErrInvalidRole) || errors.Is(err, ErrInvalidStatus) {
		respond(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if errors.Is(err, ErrInvalidBackup) {
		respond(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if errors.Is(err, ErrTaskConflict) {
		respond(c, http.StatusConflict, err.Error(), nil)
		return
	}
	respond(c, http.StatusInternalServerError, err.Error(), nil)
}

func parseTaskID(c *gin.Context) (uint64, error) {
	raw := c.Param("id")
	taskID, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, errors.New("invalid task id")
	}
	return taskID, nil
}

func bearerAuthMiddleware(expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expectedToken == "" {
			c.Next()
			return
		}
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			respond(c, http.StatusUnauthorized, "missing bearer token", nil)
			c.Abort()
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
			respond(c, http.StatusUnauthorized, "invalid bearer token", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

func metricsAuthMiddleware(expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expectedToken == "" {
			c.Next()
			return
		}
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

func respond(c *gin.Context, code int, message string, data interface{}) {
	c.JSON(code, ResponseBody{
		Code:    code,
		Message: message,
		Data:    data,
	})
}

func appendAuditLog(store Store, c *gin.Context, req CreateAuditLogRequest) {
	if store == nil {
		return
	}
	if req.Actor == "" || req.ActorRole == "" {
		actor, role := auditActorFromContext(c)
		if req.Actor == "" {
			req.Actor = actor
		}
		if req.ActorRole == "" {
			req.ActorRole = role
		}
	}
	_, _ = store.AppendAuditLog(req)
}

func auditActorFromContext(c *gin.Context) (string, string) {
	if c == nil {
		return "system", "system"
	}
	admin := currentControlAdmin(c)
	if admin == nil {
		return "anonymous", "anonymous"
	}
	return admin.Username, admin.Role
}

func guardSuperAdminMutation(current *ControlAdmin, targetUsername string, req UpdateControlAdminRequest, admins []ControlAdmin) error {
	if targetUsername == "" {
		return ErrAdminNotFound
	}
	var (
		target              *ControlAdmin
		activeSuperAdminNum int
	)
	for i := range admins {
		if admins[i].Role == "super_admin" && admins[i].Status == "active" {
			activeSuperAdminNum++
		}
		if admins[i].Username == targetUsername {
			target = &admins[i]
		}
	}
	if target == nil {
		return ErrAdminNotFound
	}
	roleAfter := target.Role
	statusAfter := target.Status
	if req.Role != "" {
		roleAfter = req.Role
	}
	if req.Status != "" {
		statusAfter = req.Status
	}
	if target.Role == "super_admin" && target.Status == "active" && (roleAfter != "super_admin" || statusAfter != "active") && activeSuperAdminNum <= 1 {
		return ErrPermissionDenied
	}
	if current != nil && current.Username == targetUsername && (roleAfter != current.Role || statusAfter != "active") {
		return ErrPermissionDenied
	}
	return nil
}
