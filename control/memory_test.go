package control

import (
	"testing"
	"time"
)

func TestPendingTasks_FiltersByPendingStatus(t *testing.T) {
	store := NewMemoryStore()

	if _, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey: "node-filter",
		Name:    "filter-node",
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}

	task1, err := store.CreateTask(CreateTaskRequest{
		NodeKey:  "node-filter",
		TaskType: "sync_users",
	})
	if err != nil {
		t.Fatalf("CreateTask(1) error = %v", err)
	}

	task2, err := store.CreateTask(CreateTaskRequest{
		NodeKey:  "node-filter",
		TaskType: "restart",
	})
	if err != nil {
		t.Fatalf("CreateTask(2) error = %v", err)
	}

	// 将 task1 标记为完成
	if _, err := store.StartTask(task1.ID, StartTaskRequest{
		NodeKey:        "node-filter",
		ExecutionToken: "exec-1",
	}); err != nil {
		t.Fatalf("StartTask(1) error = %v", err)
	}
	if _, err := store.FinishTask(task1.ID, FinishTaskRequest{
		NodeKey:        "node-filter",
		ExecutionToken: "exec-1",
		Success:        true,
		Message:        "done",
	}); err != nil {
		t.Fatalf("FinishTask(1) error = %v", err)
	}

	// PendingTasks 应该只返回 task2
	pending, err := store.PendingTasks("node-filter", 10)
	if err != nil {
		t.Fatalf("PendingTasks() error = %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("PendingTasks() returned %d tasks, want 1", len(pending))
	}
	if pending[0].ID != task2.ID {
		t.Errorf("PendingTasks()[0].ID = %d, want %d", pending[0].ID, task2.ID)
	}
	if pending[0].TaskType != "restart" {
		t.Errorf("PendingTasks()[0].TaskType = %q, want %q", pending[0].TaskType, "restart")
	}
}

func TestPendingTasks_LimitWorks(t *testing.T) {
	store := NewMemoryStore()

	if _, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey: "node-limit",
		Name:    "limit-node",
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, err := store.CreateTask(CreateTaskRequest{
			NodeKey:  "node-limit",
			TaskType: "sync",
		}); err != nil {
			t.Fatalf("CreateTask(%d) error = %v", i, err)
		}
	}

	pending, err := store.PendingTasks("node-limit", 2)
	if err != nil {
		t.Fatalf("PendingTasks() error = %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("PendingTasks(limit=2) returned %d, want 2", len(pending))
	}
}

func TestPendingTasks_UnknownNode(t *testing.T) {
	store := NewMemoryStore()

	_, err := store.PendingTasks("unknown-node", 10)
	if err != ErrNodeNotFound {
		t.Errorf("PendingTasks(unknown) error = %v, want ErrNodeNotFound", err)
	}
}

func TestMemoryStore_NodeLifecycle(t *testing.T) {
	store := NewMemoryStore()

	node, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey:    "node-lifecycle",
		Name:       "lifecycle-node",
		Region:     "us-east-1",
		DomainName: "lifecycle.example.com",
		Port:       443,
	})
	if err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}
	if node.NodeKey != "node-lifecycle" {
		t.Errorf("node.NodeKey = %q, want %q", node.NodeKey, "node-lifecycle")
	}

	nodes, err := store.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("ListNodes() returned %d nodes, want 1", len(nodes))
	}

	_, err = store.Heartbeat(HeartbeatRequest{
		NodeKey:       "node-lifecycle",
		CPUPercent:    50.0,
		MemoryPercent: 60.0,
	})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
}

func TestMemoryStore_UserCRUD(t *testing.T) {
	store := NewMemoryStore()

	user, err := store.CreateUser(CreateUserRequest{
		Username: "testuser",
		Password: "dGVzdC1wYXNz",
		Quota:    1024,
		UseDays:  30,
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if user.Username != "testuser" {
		t.Errorf("user.Username = %q, want %q", user.Username, "testuser")
	}

	got, err := store.GetUser("testuser")
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if got.Quota != 1024 {
		t.Errorf("user.Quota = %d, want 1024", got.Quota)
	}

	users, err := store.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(users) != 1 {
		t.Errorf("ListUsers() returned %d users, want 1", len(users))
	}

	err = store.DeleteUser("testuser")
	if err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	_, err = store.GetUser("testuser")
	if err != ErrUserNotFound {
		t.Errorf("GetUser(deleted) error = %v, want ErrUserNotFound", err)
	}
}

func TestMemoryStore_AuditLog(t *testing.T) {
	store := NewMemoryStore()

	log, err := store.AppendAuditLog(CreateAuditLogRequest{
		Actor:        "admin",
		ActorRole:    "super_admin",
		Action:       "user.create",
		ResourceType: "user",
		ResourceID:   "alice",
		Message:      "created user alice",
	})
	if err != nil {
		t.Fatalf("AppendAuditLog() error = %v", err)
	}
	if log.Actor != "admin" {
		t.Errorf("log.Actor = %q, want %q", log.Actor, "admin")
	}

	logs, err := store.ListAuditLogs(AuditQuery{
		Limit:  10,
		Action: "user.create",
	})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("len(logs) = %d, want 1", len(logs))
	}
}

func TestMemoryStore_CleanupHistory(t *testing.T) {
	store := NewMemoryStore()

	if _, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey: "node-cleanup",
		Name:    "cleanup-node",
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}

	task, _ := store.CreateTask(CreateTaskRequest{
		NodeKey:  "node-cleanup",
		TaskType: "sync",
	})
	store.StartTask(task.ID, StartTaskRequest{
		NodeKey:        "node-cleanup",
		ExecutionToken: "exec-1",
	})
	store.FinishTask(task.ID, FinishTaskRequest{
		NodeKey:        "node-cleanup",
		ExecutionToken: "exec-1",
		Success:        true,
		Message:        "done",
	})

	store.AppendAuditLog(CreateAuditLogRequest{
		Actor: "admin", ActorRole: "super_admin",
		Action: "test", ResourceType: "test", Message: "test",
	})
	store.ReportUsage(UsageReportRequest{
		NodeKey: "node-cleanup",
		Users:   []UsageReportItem{{Username: "alice", Upload: 1, Download: 2}},
	})

	oldTime := time.Now().Add(-100 * 24 * time.Hour)
	store.mu.Lock()
	store.auditLogs[0].CreatedAt = oldTime
	for nodeKey, tasks := range store.tasks {
		for i := range tasks {
			tasks[i].CreatedAt = oldTime
			tasks[i].FinishedAt = &oldTime
		}
		store.tasks[nodeKey] = tasks
	}
	for taskID, events := range store.taskEvents {
		for i := range events {
			events[i].CreatedAt = oldTime
		}
		store.taskEvents[taskID] = events
	}
	for nodeKey, usage := range store.usage {
		for i := range usage {
			usage[i].ReportedAt = oldTime
		}
		store.usage[nodeKey] = usage
	}
	store.mu.Unlock()

	result, err := store.CleanupHistory(CleanupRequest{
		AuditRetentionDays: 30,
		TaskRetentionDays:  30,
		UsageRetentionDays: 30,
	})
	if err != nil {
		t.Fatalf("CleanupHistory() error = %v", err)
	}
	if result.TasksDeleted != 1 {
		t.Errorf("TasksDeleted = %d, want 1", result.TasksDeleted)
	}
	if result.AuditLogsDeleted != 1 {
		t.Errorf("AuditLogsDeleted = %d, want 1", result.AuditLogsDeleted)
	}
	if result.UsageReportsDeleted != 1 {
		t.Errorf("UsageReportsDeleted = %d, want 1", result.UsageReportsDeleted)
	}
}

func TestMemoryStore_UsageReport(t *testing.T) {
	store := NewMemoryStore()

	if _, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey: "node-usage",
		Name:    "usage-node",
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}

	_, err := store.ReportUsage(UsageReportRequest{
		NodeKey: "node-usage",
		Users: []UsageReportItem{
			{Username: "alice", Upload: 100, Download: 200, Quota: 1024},
			{Username: "bob", Upload: 50, Download: 100, Quota: 512},
		},
	})
	if err != nil {
		t.Fatalf("ReportUsage() error = %v", err)
	}

	snapshots, err := store.NodeUsage("node-usage", 10)
	if err != nil {
		t.Fatalf("NodeUsage() error = %v", err)
	}
	if len(snapshots) != 2 {
		t.Errorf("NodeUsage() returned %d snapshots, want 2", len(snapshots))
	}
}

func TestMemoryStore_BindUserToNode(t *testing.T) {
	store := NewMemoryStore()

	if _, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey: "node-bind",
		Name:    "bind-node",
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}
	if _, err := store.CreateUser(CreateUserRequest{
		Username: "alice",
		Password: "dGVzdA==",
	}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	// 绑定
	_, err := store.BindUserToNode("alice", "node-bind")
	if err != nil {
		t.Fatalf("BindUserToNode() error = %v", err)
	}

	// 获取节点用户
	users, err := store.NodeUsers("node-bind")
	if err != nil {
		t.Fatalf("NodeUsers() error = %v", err)
	}
	if len(users) != 1 || users[0].Username != "alice" {
		t.Errorf("NodeUsers() = %v, want [alice]", users)
	}

	// 解绑
	err = store.UnbindUserFromNode("alice", "node-bind")
	if err != nil {
		t.Fatalf("UnbindUserFromNode() error = %v", err)
	}

	users, _ = store.NodeUsers("node-bind")
	if len(users) != 0 {
		t.Errorf("NodeUsers() after unbind = %v, want empty", users)
	}
}

func TestMemoryStore_AdminManagement(t *testing.T) {
	store := NewMemoryStore()

	// 创建 admin
	admin, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "root",
		Password: "root-pass",
		Role:     "super_admin",
	})
	if err != nil {
		t.Fatalf("EnsureControlAdmin() error = %v", err)
	}
	if admin.Username != "root" {
		t.Errorf("admin.Username = %q, want %q", admin.Username, "root")
	}
	if admin.Role != "super_admin" {
		t.Errorf("admin.Role = %q, want %q", admin.Role, "super_admin")
	}

	// 重复创建同一 admin 不应报错
	admin2, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "root",
		Password: "root-pass-new",
		Role:     "super_admin",
	})
	if err != nil {
		t.Fatalf("EnsureControlAdmin(dup) error = %v", err)
	}
	if admin2.Username != "root" {
		t.Errorf("dup admin.Username = %q, want %q", admin2.Username, "root")
	}

	// 验证密码
	validated, err := store.AuthenticateControlAdmin("root", "root-pass")
	if err != nil {
		t.Fatalf("AuthenticateControlAdmin() error = %v", err)
	}
	if validated.Username != "root" {
		t.Errorf("validated.Username = %q, want %q", validated.Username, "root")
	}

	// 错误密码
	_, err = store.AuthenticateControlAdmin("root", "wrong-pass")
	if err == nil {
		t.Error("AuthenticateControlAdmin(wrong) should return error")
	}
}
