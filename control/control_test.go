package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAgentRegisterHeartbeatAndPendingTasks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	router := NewRouter(store)

	registerResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/register", map[string]interface{}{
		"nodeKey":        "node-1",
		"name":           "Tokyo-01",
		"region":         "ap-northeast-1",
		"trojanType":     "trojan",
		"trojanVersion":  "1.0.0",
		"managerVersion": "v2.15.4",
		"tags":           []string{"tokyo", "premium"},
	})
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d", registerResp.Code, http.StatusOK)
	}

	heartbeatResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/heartbeat", map[string]interface{}{
		"nodeKey":       "node-1",
		"cpuPercent":    12.5,
		"memoryPercent": 28.2,
		"diskPercent":   66.4,
		"tcpCount":      10,
		"udpCount":      3,
		"uploadSpeed":   1024,
		"downloadSpeed": 2048,
	})
	if heartbeatResp.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, want %d", heartbeatResp.Code, http.StatusOK)
	}

	createTaskResp := performJSONRequest(t, router, http.MethodPost, "/api/control/tasks", map[string]interface{}{
		"nodeKey":  "node-1",
		"taskType": "sync_users",
		"payload": map[string]interface{}{
			"users": []interface{}{
				map[string]interface{}{"username": "alice"},
			},
		},
	})
	if createTaskResp.Code != http.StatusOK {
		t.Fatalf("create task status = %d, want %d", createTaskResp.Code, http.StatusOK)
	}

	nodesResp := performJSONRequest(t, router, http.MethodGet, "/api/control/nodes", nil)
	if nodesResp.Code != http.StatusOK {
		t.Fatalf("list nodes status = %d, want %d", nodesResp.Code, http.StatusOK)
	}

	taskResp := performJSONRequest(t, router, http.MethodGet, "/api/agent/tasks/pending?nodeKey=node-1&limit=1", nil)
	if taskResp.Code != http.StatusOK {
		t.Fatalf("pending task status = %d, want %d", taskResp.Code, http.StatusOK)
	}

	var body ResponseBody
	if err := json.Unmarshal(taskResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("body.Data type = %T, want map[string]interface{}", body.Data)
	}
	tasks, ok := data["tasks"].([]interface{})
	if !ok {
		t.Fatalf("tasks type = %T, want []interface{}", data["tasks"])
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}

	taskMap, ok := tasks[0].(map[string]interface{})
	if !ok {
		t.Fatalf("task item type = %T, want map[string]interface{}", tasks[0])
	}
	taskID := uint64(taskMap["id"].(float64))

	startTaskResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/tasks/"+itoa(taskID)+"/start", map[string]interface{}{
		"nodeKey":        "node-1",
		"executionToken": "exec-1",
	})
	if startTaskResp.Code != http.StatusOK {
		t.Fatalf("start task status = %d, want %d", startTaskResp.Code, http.StatusOK)
	}

	finishTaskResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/tasks/"+itoa(taskID)+"/result", map[string]interface{}{
		"nodeKey":        "node-1",
		"executionToken": "exec-1",
		"success":        true,
		"message":        "done",
		"details": map[string]interface{}{
			"userCount": 1,
		},
	})
	if finishTaskResp.Code != http.StatusOK {
		t.Fatalf("finish task status = %d, want %d", finishTaskResp.Code, http.StatusOK)
	}

	listTasksResp := performJSONRequest(t, router, http.MethodGet, "/api/control/tasks?limit=5", nil)
	if listTasksResp.Code != http.StatusOK {
		t.Fatalf("list tasks status = %d, want %d", listTasksResp.Code, http.StatusOK)
	}

	filteredTasksResp := performJSONRequest(t, router, http.MethodGet, "/api/control/tasks?limit=5&nodeKey=node-1&taskType=sync_users&status=succeeded", nil)
	if filteredTasksResp.Code != http.StatusOK {
		t.Fatalf("filtered tasks status = %d, want %d", filteredTasksResp.Code, http.StatusOK)
	}

	taskDetailResp := performJSONRequest(t, router, http.MethodGet, "/api/control/tasks/"+itoa(taskID)+"?limit=10", nil)
	if taskDetailResp.Code != http.StatusOK {
		t.Fatalf("task detail status = %d, want %d", taskDetailResp.Code, http.StatusOK)
	}

	var detailBody ResponseBody
	if err := json.Unmarshal(taskDetailResp.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("task detail json.Unmarshal() error = %v", err)
	}
	detailData, ok := detailBody.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("detail body.Data type = %T, want map[string]interface{}", detailBody.Data)
	}
	events, ok := detailData["events"].([]interface{})
	if !ok || len(events) < 3 {
		t.Fatalf("task events missing: %#v", detailData["events"])
	}

	usageResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/usage", map[string]interface{}{
		"nodeKey": "node-1",
		"users": []interface{}{
			map[string]interface{}{
				"username":   "alice",
				"upload":     100,
				"download":   200,
				"quota":      1024,
				"expiryDate": "2026-05-01",
			},
		},
	})
	if usageResp.Code != http.StatusOK {
		t.Fatalf("usage report status = %d, want %d", usageResp.Code, http.StatusOK)
	}

	nodeUsageResp := performJSONRequest(t, router, http.MethodGet, "/api/control/nodes/node-1/usage?limit=10", nil)
	if nodeUsageResp.Code != http.StatusOK {
		t.Fatalf("node usage status = %d, want %d", nodeUsageResp.Code, http.StatusOK)
	}
}

func TestControlUserBindingAndSyncTask(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	router := NewRouter(store)

	registerResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/register", map[string]interface{}{
		"nodeKey":    "node-sync",
		"name":       "sync-node",
		"domainName": "sync.example.com",
		"port":       443,
	})
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d", registerResp.Code, http.StatusOK)
	}

	createUserResp := performJSONRequest(t, router, http.MethodPost, "/api/control/users", map[string]interface{}{
		"username": "alice",
		"password": "YWxpY2UtcGFzcw==",
		"quota":    4096,
		"useDays":  30,
	})
	if createUserResp.Code != http.StatusOK {
		t.Fatalf("create user status = %d, want %d", createUserResp.Code, http.StatusOK)
	}

	bindResp := performJSONRequest(t, router, http.MethodPost, "/api/control/users/alice/bindings", map[string]interface{}{
		"nodeKey": "node-sync",
	})
	if bindResp.Code != http.StatusOK {
		t.Fatalf("bind user status = %d, want %d", bindResp.Code, http.StatusOK)
	}

	nodeUsersResp := performJSONRequest(t, router, http.MethodGet, "/api/control/nodes/node-sync/users", nil)
	if nodeUsersResp.Code != http.StatusOK {
		t.Fatalf("node users status = %d, want %d", nodeUsersResp.Code, http.StatusOK)
	}

	syncResp := performJSONRequest(t, router, http.MethodPost, "/api/control/nodes/node-sync/sync", nil)
	if syncResp.Code != http.StatusOK {
		t.Fatalf("sync node status = %d, want %d", syncResp.Code, http.StatusOK)
	}

	taskResp := performJSONRequest(t, router, http.MethodGet, "/api/agent/tasks/pending?nodeKey=node-sync&limit=5", nil)
	if taskResp.Code != http.StatusOK {
		t.Fatalf("pending sync task status = %d, want %d", taskResp.Code, http.StatusOK)
	}

	var body ResponseBody
	if err := json.Unmarshal(taskResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("body.Data type = %T, want map[string]interface{}", body.Data)
	}
	tasks, ok := data["tasks"].([]interface{})
	if !ok || len(tasks) == 0 {
		t.Fatalf("sync tasks missing: %#v", data["tasks"])
	}
	taskMap, ok := tasks[0].(map[string]interface{})
	if !ok {
		t.Fatalf("sync task item type = %T", tasks[0])
	}
	if taskMap["taskType"] != "sync_users" {
		t.Fatalf("taskType = %v, want sync_users", taskMap["taskType"])
	}

	overviewResp := performJSONRequest(t, router, http.MethodGet, "/api/control/overview", nil)
	if overviewResp.Code != http.StatusOK {
		t.Fatalf("overview status = %d, want %d", overviewResp.Code, http.StatusOK)
	}

	userNodesResp := performJSONRequest(t, router, http.MethodGet, "/api/control/users/alice/nodes", nil)
	if userNodesResp.Code != http.StatusOK {
		t.Fatalf("user nodes status = %d, want %d", userNodesResp.Code, http.StatusOK)
	}

	userUsageResp := performJSONRequest(t, router, http.MethodGet, "/api/control/users/alice/usage?limit=5", nil)
	if userUsageResp.Code != http.StatusOK {
		t.Fatalf("user usage status = %d, want %d", userUsageResp.Code, http.StatusOK)
	}

	subscriptionResp := performJSONRequest(t, router, http.MethodGet, "/api/control/users/alice/subscription/clash", nil)
	if subscriptionResp.Code != http.StatusOK {
		t.Fatalf("subscription status = %d, want %d", subscriptionResp.Code, http.StatusOK)
	}
	subscriptionBody := subscriptionResp.Body.String()
	if !strings.Contains(subscriptionBody, "sync.example.com") {
		t.Fatalf("subscription does not contain node domain: %s", subscriptionBody)
	}
	if !strings.Contains(subscriptionBody, "alice-pass") {
		t.Fatalf("subscription does not contain decoded password: %s", subscriptionBody)
	}

	linksResp := performJSONRequest(t, router, http.MethodGet, "/api/control/users/alice/subscription/links", nil)
	if linksResp.Code != http.StatusOK {
		t.Fatalf("links subscription status = %d, want %d", linksResp.Code, http.StatusOK)
	}
	if !strings.Contains(linksResp.Body.String(), "trojan://alice-pass@sync.example.com:443") {
		t.Fatalf("links subscription missing trojan link: %s", linksResp.Body.String())
	}

	retryResp := performJSONRequest(t, router, http.MethodPost, "/api/control/tasks/"+itoa(taskIDFromTaskMap(taskMap))+"/retry", nil)
	if retryResp.Code != http.StatusOK {
		t.Fatalf("retry task status = %d, want %d", retryResp.Code, http.StatusOK)
	}

	unbindResp := performJSONRequest(t, router, http.MethodDelete, "/api/control/users/alice/bindings/node-sync", nil)
	if unbindResp.Code != http.StatusOK {
		t.Fatalf("unbind user status = %d, want %d", unbindResp.Code, http.StatusOK)
	}

	deleteResp := performJSONRequest(t, router, http.MethodDelete, "/api/control/users/alice", nil)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete user status = %d, want %d", deleteResp.Code, http.StatusOK)
	}

	getDeletedResp := performJSONRequest(t, router, http.MethodGet, "/api/control/users/alice", nil)
	if getDeletedResp.Code != http.StatusNotFound {
		t.Fatalf("get deleted user status = %d, want %d", getDeletedResp.Code, http.StatusNotFound)
	}
}

func TestHeartbeatUnknownNode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	router := NewRouter(store)

	resp := performJSONRequest(t, router, http.MethodPost, "/api/agent/heartbeat", map[string]interface{}{
		"nodeKey": "missing-node",
	})
	if resp.Code != http.StatusNotFound {
		t.Fatalf("heartbeat status = %d, want %d", resp.Code, http.StatusNotFound)
	}
}

func TestTaskExecutionIdempotencyAndConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	router := NewRouter(store)

	registerResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/register", map[string]interface{}{
		"nodeKey": "node-task",
		"name":    "task-node",
	})
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d", registerResp.Code, http.StatusOK)
	}

	createTaskResp := performJSONRequest(t, router, http.MethodPost, "/api/control/tasks", map[string]interface{}{
		"nodeKey":  "node-task",
		"taskType": "restart_trojan",
	})
	if createTaskResp.Code != http.StatusOK {
		t.Fatalf("create task status = %d, want %d", createTaskResp.Code, http.StatusOK)
	}

	taskResp := performJSONRequest(t, router, http.MethodGet, "/api/agent/tasks/pending?nodeKey=node-task&limit=1", nil)
	if taskResp.Code != http.StatusOK {
		t.Fatalf("pending task status = %d, want %d", taskResp.Code, http.StatusOK)
	}

	var body ResponseBody
	if err := json.Unmarshal(taskResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("body.Data type = %T, want map[string]interface{}", body.Data)
	}
	tasks, ok := data["tasks"].([]interface{})
	if !ok || len(tasks) != 1 {
		t.Fatalf("tasks = %#v, want single pending task", data["tasks"])
	}
	taskMap, ok := tasks[0].(map[string]interface{})
	if !ok {
		t.Fatalf("task item type = %T, want map[string]interface{}", tasks[0])
	}
	taskID := itoa(taskIDFromTaskMap(taskMap))

	startResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/tasks/"+taskID+"/start", map[string]interface{}{
		"nodeKey":        "node-task",
		"executionToken": "exec-1",
	})
	if startResp.Code != http.StatusOK {
		t.Fatalf("start task status = %d, want %d", startResp.Code, http.StatusOK)
	}
	if extractTaskAttempt(t, startResp) != 1 {
		t.Fatalf("first start attempt = %d, want 1", extractTaskAttempt(t, startResp))
	}

	replayedStartResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/tasks/"+taskID+"/start", map[string]interface{}{
		"nodeKey":        "node-task",
		"executionToken": "exec-1",
	})
	if replayedStartResp.Code != http.StatusOK {
		t.Fatalf("replayed start status = %d, want %d", replayedStartResp.Code, http.StatusOK)
	}
	if extractTaskAttempt(t, replayedStartResp) != 1 {
		t.Fatalf("replayed start attempt = %d, want 1", extractTaskAttempt(t, replayedStartResp))
	}

	conflictStartResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/tasks/"+taskID+"/start", map[string]interface{}{
		"nodeKey":        "node-task",
		"executionToken": "exec-2",
	})
	if conflictStartResp.Code != http.StatusConflict {
		t.Fatalf("conflicting start status = %d, want %d", conflictStartResp.Code, http.StatusConflict)
	}

	wrongFinishResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/tasks/"+taskID+"/result", map[string]interface{}{
		"nodeKey":        "node-task",
		"executionToken": "exec-2",
		"success":        true,
		"message":        "done",
	})
	if wrongFinishResp.Code != http.StatusConflict {
		t.Fatalf("wrong token finish status = %d, want %d", wrongFinishResp.Code, http.StatusConflict)
	}

	finishResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/tasks/"+taskID+"/result", map[string]interface{}{
		"nodeKey":        "node-task",
		"executionToken": "exec-1",
		"success":        true,
		"message":        "done",
	})
	if finishResp.Code != http.StatusOK {
		t.Fatalf("finish task status = %d, want %d", finishResp.Code, http.StatusOK)
	}

	replayedFinishResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/tasks/"+taskID+"/result", map[string]interface{}{
		"nodeKey":        "node-task",
		"executionToken": "exec-1",
		"success":        true,
		"message":        "done",
	})
	if replayedFinishResp.Code != http.StatusOK {
		t.Fatalf("replayed finish status = %d, want %d", replayedFinishResp.Code, http.StatusOK)
	}

	conflictFinishResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/tasks/"+taskID+"/result", map[string]interface{}{
		"nodeKey":        "node-task",
		"executionToken": "exec-3",
		"success":        true,
		"message":        "done",
	})
	if conflictFinishResp.Code != http.StatusConflict {
		t.Fatalf("conflicting finish status = %d, want %d", conflictFinishResp.Code, http.StatusConflict)
	}
}

func TestTaskAndAuditPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	router := NewRouter(store)

	registerResp := performJSONRequest(t, router, http.MethodPost, "/api/agent/register", map[string]interface{}{
		"nodeKey": "node-page",
		"name":    "page-node",
	})
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d", registerResp.Code, http.StatusOK)
	}

	for i := 0; i < 4; i++ {
		taskResp := performJSONRequest(t, router, http.MethodPost, "/api/control/tasks", map[string]interface{}{
			"nodeKey":  "node-page",
			"taskType": fmt.Sprintf("sync_users_%d", i),
		})
		if taskResp.Code != http.StatusOK {
			t.Fatalf("create task %d status = %d, want %d", i, taskResp.Code, http.StatusOK)
		}
		if _, err := store.AppendAuditLog(CreateAuditLogRequest{
			Actor:        "tester",
			ActorRole:    "admin",
			Action:       fmt.Sprintf("task.page.%d", i),
			ResourceType: "task",
			ResourceID:   fmt.Sprintf("%d", i),
			Message:      "pagination test",
		}); err != nil {
			t.Fatalf("AppendAuditLog() error = %v", err)
		}
	}

	taskListResp := performJSONRequest(t, router, http.MethodGet, "/api/control/tasks?limit=2&offset=1", nil)
	if taskListResp.Code != http.StatusOK {
		t.Fatalf("task list status = %d, want %d", taskListResp.Code, http.StatusOK)
	}
	var taskListBody ResponseBody
	if err := json.Unmarshal(taskListResp.Body.Bytes(), &taskListBody); err != nil {
		t.Fatalf("task list json.Unmarshal() error = %v", err)
	}
	taskListData, ok := taskListBody.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("task list body.Data type = %T", taskListBody.Data)
	}
	tasks, ok := taskListData["tasks"].([]interface{})
	if !ok || len(tasks) != 2 {
		t.Fatalf("paginated tasks = %#v, want 2 rows", taskListData["tasks"])
	}
	if hasMore, _ := taskListData["hasMore"].(bool); !hasMore {
		t.Fatalf("task list hasMore = %#v, want true", taskListData["hasMore"])
	}
	if offset, _ := taskListData["offset"].(float64); int(offset) != 1 {
		t.Fatalf("task list offset = %#v, want 1", taskListData["offset"])
	}

	auditResp := performJSONRequest(t, router, http.MethodGet, "/api/control/audit?limit=2&offset=1", nil)
	if auditResp.Code != http.StatusOK {
		t.Fatalf("audit list status = %d, want %d", auditResp.Code, http.StatusOK)
	}
	var auditBody ResponseBody
	if err := json.Unmarshal(auditResp.Body.Bytes(), &auditBody); err != nil {
		t.Fatalf("audit list json.Unmarshal() error = %v", err)
	}
	auditData, ok := auditBody.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("audit body.Data type = %T", auditBody.Data)
	}
	logs, ok := auditData["logs"].([]interface{})
	if !ok || len(logs) != 2 {
		t.Fatalf("paginated logs = %#v, want 2 rows", auditData["logs"])
	}
	if hasMore, _ := auditData["hasMore"].(bool); !hasMore {
		t.Fatalf("audit hasMore = %#v, want true", auditData["hasMore"])
	}
}

func TestCleanupHistoryRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	if _, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "root",
		Password: "root-pass",
		Role:     "super_admin",
	}); err != nil {
		t.Fatalf("EnsureControlAdmin(root) error = %v", err)
	}
	if _, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey: "node-clean",
		Name:    "clean-node",
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}
	oldTask, err := store.CreateTask(CreateTaskRequest{
		NodeKey:  "node-clean",
		TaskType: "sync_users",
	})
	if err != nil {
		t.Fatalf("CreateTask(old) error = %v", err)
	}
	if _, err := store.StartTask(oldTask.ID, StartTaskRequest{
		NodeKey:        "node-clean",
		ExecutionToken: "cleanup-old",
	}); err != nil {
		t.Fatalf("StartTask(old) error = %v", err)
	}
	if _, err := store.FinishTask(oldTask.ID, FinishTaskRequest{
		NodeKey:        "node-clean",
		ExecutionToken: "cleanup-old",
		Success:        true,
		Message:        "done",
	}); err != nil {
		t.Fatalf("FinishTask(old) error = %v", err)
	}
	if _, err := store.CreateTask(CreateTaskRequest{
		NodeKey:  "node-clean",
		TaskType: "restart_trojan",
	}); err != nil {
		t.Fatalf("CreateTask(active) error = %v", err)
	}
	if _, err := store.AppendAuditLog(CreateAuditLogRequest{
		Actor:        "root",
		ActorRole:    "super_admin",
		Action:       "history.old",
		ResourceType: "maintenance",
		ResourceID:   "old",
		Message:      "old log",
	}); err != nil {
		t.Fatalf("AppendAuditLog() error = %v", err)
	}

	oldTime := time.Now().Add(-45 * 24 * time.Hour)
	store.mu.Lock()
	store.auditLogs[0].CreatedAt = oldTime
	for nodeKey, tasks := range store.tasks {
		for i := range tasks {
			if tasks[i].ID != oldTask.ID {
				continue
			}
			tasks[i].CreatedAt = oldTime
			tasks[i].FinishedAt = &oldTime
		}
		store.tasks[nodeKey] = tasks
	}
	events := store.taskEvents[oldTask.ID]
	for i := range events {
		events[i].CreatedAt = oldTime
	}
	store.taskEvents[oldTask.ID] = events
	store.usage["node-clean"] = []UsageSnapshot{
		{
			NodeKey:    "node-clean",
			Username:   "alice",
			Upload:     1,
			Download:   2,
			Quota:      3,
			ReportedAt: oldTime,
		},
	}
	store.mu.Unlock()

	router := NewRouter(store, ServerOptions{ControlJWTSecret: "jwt-secret"})
	rootToken := loginForToken(t, router, "root", "root-pass")

	cleanupResp := performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/control/maintenance/cleanup", map[string]interface{}{
		"auditRetentionDays": 30,
		"taskRetentionDays":  30,
		"usageRetentionDays": 30,
	}, map[string]string{"Authorization": "Bearer " + rootToken})
	if cleanupResp.Code != http.StatusOK {
		t.Fatalf("cleanup status = %d, want %d", cleanupResp.Code, http.StatusOK)
	}

	var cleanupBody ResponseBody
	if err := json.Unmarshal(cleanupResp.Body.Bytes(), &cleanupBody); err != nil {
		t.Fatalf("cleanup json.Unmarshal() error = %v", err)
	}
	cleanupData, ok := cleanupBody.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("cleanup body.Data type = %T", cleanupBody.Data)
	}
	if deleted, _ := cleanupData["tasksDeleted"].(float64); int(deleted) != 1 {
		t.Fatalf("tasksDeleted = %#v, want 1", cleanupData["tasksDeleted"])
	}
	if deleted, _ := cleanupData["taskEventsDeleted"].(float64); int(deleted) == 0 {
		t.Fatalf("taskEventsDeleted = %#v, want > 0", cleanupData["taskEventsDeleted"])
	}
	if deleted, _ := cleanupData["auditLogsDeleted"].(float64); int(deleted) != 1 {
		t.Fatalf("auditLogsDeleted = %#v, want 1", cleanupData["auditLogsDeleted"])
	}
	if deleted, _ := cleanupData["usageReportsDeleted"].(float64); int(deleted) != 1 {
		t.Fatalf("usageReportsDeleted = %#v, want 1", cleanupData["usageReportsDeleted"])
	}

	tasks, err := store.ListTasks(TaskQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskType != "restart_trojan" {
		t.Fatalf("remaining tasks = %#v, want only active task", tasks)
	}

	auditResp := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/audit?action=maintenance.cleanup", nil, map[string]string{
		"Authorization": "Bearer " + rootToken,
	})
	if auditResp.Code != http.StatusOK {
		t.Fatalf("cleanup audit status = %d, want %d", auditResp.Code, http.StatusOK)
	}
}

func TestBackupExportImportRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	if _, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "root",
		Password: "root-pass",
		Role:     "super_admin",
	}); err != nil {
		t.Fatalf("EnsureControlAdmin(root) error = %v", err)
	}
	if _, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey:     "node-backup",
		Name:        "backup-node",
		AgentSecret: "node-secret-backup",
		DomainName:  "backup.example.com",
		Port:        443,
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}
	if _, err := store.CreateUser(CreateUserRequest{
		Username: "alice",
		Password: "YWxpY2UtcGFzcw==",
		Quota:    2048,
		UseDays:  30,
	}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := store.BindUserToNode("alice", "node-backup"); err != nil {
		t.Fatalf("BindUserToNode() error = %v", err)
	}
	task, err := store.CreateTask(CreateTaskRequest{
		NodeKey:  "node-backup",
		TaskType: "sync_users",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := store.StartTask(task.ID, StartTaskRequest{
		NodeKey:        "node-backup",
		ExecutionToken: "backup-exec",
	}); err != nil {
		t.Fatalf("StartTask() error = %v", err)
	}
	if _, err := store.FinishTask(task.ID, FinishTaskRequest{
		NodeKey:        "node-backup",
		ExecutionToken: "backup-exec",
		Success:        true,
		Message:        "done",
	}); err != nil {
		t.Fatalf("FinishTask() error = %v", err)
	}
	if _, err := store.AppendAuditLog(CreateAuditLogRequest{
		Actor:        "root",
		ActorRole:    "super_admin",
		Action:       "backup.seed",
		ResourceType: "backup",
		ResourceID:   "seed",
		Message:      "seed log",
	}); err != nil {
		t.Fatalf("AppendAuditLog() error = %v", err)
	}
	if _, err := store.ReportUsage(UsageReportRequest{
		NodeKey: "node-backup",
		Users: []UsageReportItem{
			{
				Username: "alice",
				Upload:   10,
				Download: 20,
				Quota:    2048,
			},
		},
	}); err != nil {
		t.Fatalf("ReportUsage() error = %v", err)
	}

	router := NewRouter(store, ServerOptions{ControlJWTSecret: "jwt-secret"})
	rootToken := loginForToken(t, router, "root", "root-pass")

	exportResp := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/backup/export", nil, map[string]string{
		"Authorization": "Bearer " + rootToken,
	})
	if exportResp.Code != http.StatusOK {
		t.Fatalf("backup export status = %d, want %d", exportResp.Code, http.StatusOK)
	}

	var exportBody ResponseBody
	if err := json.Unmarshal(exportResp.Body.Bytes(), &exportBody); err != nil {
		t.Fatalf("export json.Unmarshal() error = %v", err)
	}
	exportData, ok := exportBody.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("export body.Data type = %T", exportBody.Data)
	}
	backup, err := decodeBackupBundle(exportData["backup"])
	if err != nil {
		t.Fatalf("decodeBackupBundle() error = %v", err)
	}
	if len(backup.Nodes) != 1 || backup.Nodes[0].AgentSecretHash == "" {
		t.Fatalf("backup nodes = %#v, want node with secret hash", backup.Nodes)
	}
	if backup.Nodes[0].Node.NodeKey != "node-backup" {
		t.Fatalf("backup node key = %q, want node-backup", backup.Nodes[0].Node.NodeKey)
	}
	roundTripStore := NewMemoryStore()
	if err := roundTripStore.ImportBackup(backup); err != nil {
		t.Fatalf("ImportBackup() validation error = %v", err)
	}

	if _, err := store.CreateUser(CreateUserRequest{
		Username: "bob",
		Password: "Ym9iLXBhc3M=",
	}); err != nil {
		t.Fatalf("CreateUser(bob) error = %v", err)
	}
	if err := store.DeleteUser("alice"); err != nil {
		t.Fatalf("DeleteUser(alice) error = %v", err)
	}

	importResp := performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/control/backup/import", backup, map[string]string{
		"Authorization": "Bearer " + rootToken,
	})
	if importResp.Code != http.StatusOK {
		t.Fatalf("backup import status = %d, want %d, body=%s", importResp.Code, http.StatusOK, importResp.Body.String())
	}

	aliceResp := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/users/alice", nil, map[string]string{
		"Authorization": "Bearer " + rootToken,
	})
	if aliceResp.Code != http.StatusOK {
		t.Fatalf("restored alice status = %d, want %d", aliceResp.Code, http.StatusOK)
	}

	bobResp := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/users/bob", nil, map[string]string{
		"Authorization": "Bearer " + rootToken,
	})
	if bobResp.Code != http.StatusNotFound {
		t.Fatalf("bob after import status = %d, want %d", bobResp.Code, http.StatusNotFound)
	}
}

func TestAgentAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	router := NewRouter(store, ServerOptions{AgentToken: "agent-secret"})

	resp := performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/agent/register", map[string]interface{}{
		"nodeKey": "node-1",
		"name":    "tokyo-01",
	}, nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("register without token status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}

	resp = performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/agent/register", map[string]interface{}{
		"nodeKey": "node-1",
		"name":    "tokyo-01",
	}, map[string]string{"Authorization": "Bearer agent-secret"})
	if resp.Code != http.StatusOK {
		t.Fatalf("register with token status = %d, want %d", resp.Code, http.StatusOK)
	}
}

func TestControlLoginRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	if _, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "admin",
		Password: "secret-pass",
		Role:     "super_admin",
	}); err != nil {
		t.Fatalf("EnsureControlAdmin() error = %v", err)
	}
	router := NewRouter(store, ServerOptions{
		ControlJWTSecret: "jwt-secret",
		LoginRateLimit:   1,
	})

	firstResp := performJSONRequest(t, router, http.MethodPost, "/api/control/auth/login", map[string]interface{}{
		"username": "admin",
		"password": "wrong-pass",
	})
	if firstResp.Code != http.StatusUnauthorized {
		t.Fatalf("first login attempt status = %d, want %d", firstResp.Code, http.StatusUnauthorized)
	}

	secondResp := performJSONRequest(t, router, http.MethodPost, "/api/control/auth/login", map[string]interface{}{
		"username": "admin",
		"password": "wrong-pass",
	})
	if secondResp.Code != http.StatusTooManyRequests {
		t.Fatalf("second login attempt status = %d, want %d", secondResp.Code, http.StatusTooManyRequests)
	}
}

func TestAgentRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	if _, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey: "node-rate",
		Name:    "rate-node",
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}
	router := NewRouter(store, ServerOptions{
		AgentToken:     "agent-secret",
		AgentRateLimit: 1,
	})

	firstResp := performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/agent/heartbeat", map[string]interface{}{
		"nodeKey": "node-rate",
	}, map[string]string{"Authorization": "Bearer agent-secret"})
	if firstResp.Code != http.StatusOK {
		t.Fatalf("first heartbeat status = %d, want %d", firstResp.Code, http.StatusOK)
	}

	secondResp := performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/agent/heartbeat", map[string]interface{}{
		"nodeKey": "node-rate",
	}, map[string]string{"Authorization": "Bearer agent-secret"})
	if secondResp.Code != http.StatusTooManyRequests {
		t.Fatalf("second heartbeat status = %d, want %d", secondResp.Code, http.StatusTooManyRequests)
	}
}

func TestControlJWTLoginAndProtectedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	if _, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "admin",
		Password: "secret-pass",
		Role:     "super_admin",
	}); err != nil {
		t.Fatalf("EnsureControlAdmin() error = %v", err)
	}
	router := NewRouter(store, ServerOptions{ControlJWTSecret: "jwt-secret"})

	unauthorizedResp := performJSONRequest(t, router, http.MethodGet, "/api/control/users", nil)
	if unauthorizedResp.Code != http.StatusUnauthorized {
		t.Fatalf("protected control route status = %d, want %d", unauthorizedResp.Code, http.StatusUnauthorized)
	}

	loginResp := performJSONRequest(t, router, http.MethodPost, "/api/control/auth/login", map[string]interface{}{
		"username": "admin",
		"password": "secret-pass",
	})
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", loginResp.Code, http.StatusOK)
	}

	var body ResponseBody
	if err := json.Unmarshal(loginResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("login json.Unmarshal() error = %v", err)
	}
	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("login body.Data type = %T, want map[string]interface{}", body.Data)
	}
	token, ok := data["token"].(string)
	if !ok || token == "" {
		t.Fatalf("login token missing: %#v", data["token"])
	}

	meResp := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/auth/me", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})
	if meResp.Code != http.StatusOK {
		t.Fatalf("auth me status = %d, want %d", meResp.Code, http.StatusOK)
	}

	authorizedResp := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/users", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})
	if authorizedResp.Code != http.StatusOK {
		t.Fatalf("protected route with jwt status = %d, want %d", authorizedResp.Code, http.StatusOK)
	}
}

func TestControlRBACAndAdminManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	if _, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "root",
		Password: "root-pass",
		Role:     "super_admin",
	}); err != nil {
		t.Fatalf("EnsureControlAdmin(root) error = %v", err)
	}
	if _, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "viewer",
		Password: "viewer-pass",
		Role:     "viewer",
	}); err != nil {
		t.Fatalf("EnsureControlAdmin(viewer) error = %v", err)
	}
	router := NewRouter(store, ServerOptions{ControlJWTSecret: "jwt-secret"})

	rootToken := loginForToken(t, router, "root", "root-pass")
	viewerToken := loginForToken(t, router, "viewer", "viewer-pass")

	createUserForbidden := performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/control/users", map[string]interface{}{
		"username": "alice",
		"password": "YWxpY2UtcGFzcw==",
	}, map[string]string{"Authorization": "Bearer " + viewerToken})
	if createUserForbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer create user status = %d, want %d", createUserForbidden.Code, http.StatusForbidden)
	}

	listUsersAllowed := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/users", nil, map[string]string{
		"Authorization": "Bearer " + viewerToken,
	})
	if listUsersAllowed.Code != http.StatusOK {
		t.Fatalf("viewer list users status = %d, want %d", listUsersAllowed.Code, http.StatusOK)
	}

	createAdminResp := performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/control/admins", map[string]interface{}{
		"username": "ops",
		"password": "ops-pass",
		"role":     "admin",
	}, map[string]string{"Authorization": "Bearer " + rootToken})
	if createAdminResp.Code != http.StatusOK {
		t.Fatalf("create admin status = %d, want %d", createAdminResp.Code, http.StatusOK)
	}

	listAdminsForbidden := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/admins", nil, map[string]string{
		"Authorization": "Bearer " + viewerToken,
	})
	if listAdminsForbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer list admins status = %d, want %d", listAdminsForbidden.Code, http.StatusForbidden)
	}

	auditForbidden := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/audit", nil, map[string]string{
		"Authorization": "Bearer " + viewerToken,
	})
	if auditForbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer audit status = %d, want %d", auditForbidden.Code, http.StatusForbidden)
	}

	auditResp := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/audit?action=admin.create&resourceType=admin", nil, map[string]string{
		"Authorization": "Bearer " + rootToken,
	})
	if auditResp.Code != http.StatusOK {
		t.Fatalf("audit list status = %d, want %d", auditResp.Code, http.StatusOK)
	}

	var auditBody ResponseBody
	if err := json.Unmarshal(auditResp.Body.Bytes(), &auditBody); err != nil {
		t.Fatalf("audit json.Unmarshal() error = %v", err)
	}
	auditData, ok := auditBody.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("audit body.Data type = %T", auditBody.Data)
	}
	logs, ok := auditData["logs"].([]interface{})
	if !ok || len(logs) == 0 {
		t.Fatalf("audit logs missing: %#v", auditData["logs"])
	}

	updateSelfForbidden := performJSONRequestWithHeaders(t, router, http.MethodPatch, "/api/control/admins/root", map[string]interface{}{
		"role":   "admin",
		"status": "disabled",
	}, map[string]string{"Authorization": "Bearer " + rootToken})
	if updateSelfForbidden.Code != http.StatusForbidden {
		t.Fatalf("disable last super admin status = %d, want %d", updateSelfForbidden.Code, http.StatusForbidden)
	}

	promoteOpsResp := performJSONRequestWithHeaders(t, router, http.MethodPatch, "/api/control/admins/ops", map[string]interface{}{
		"role": "super_admin",
	}, map[string]string{"Authorization": "Bearer " + rootToken})
	if promoteOpsResp.Code != http.StatusOK {
		t.Fatalf("promote ops status = %d, want %d", promoteOpsResp.Code, http.StatusOK)
	}

	disableRootResp := performJSONRequestWithHeaders(t, router, http.MethodPatch, "/api/control/admins/root", map[string]interface{}{
		"status": "disabled",
	}, map[string]string{"Authorization": "Bearer " + rootToken})
	if disableRootResp.Code != http.StatusForbidden {
		t.Fatalf("disable current super admin self status = %d, want %d", disableRootResp.Code, http.StatusForbidden)
	}
}

func TestAgentPerNodeSecretBootstrapAndRotation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	if _, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "root",
		Password: "root-pass",
		Role:     "super_admin",
	}); err != nil {
		t.Fatalf("EnsureControlAdmin(root) error = %v", err)
	}
	router := NewRouter(store, ServerOptions{
		AgentToken:       "bootstrap-token",
		ControlJWTSecret: "jwt-secret",
	})

	registerResp := performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/agent/register", map[string]interface{}{
		"nodeKey":     "node-secure",
		"name":        "secure-node",
		"agentSecret": "node-secret-1",
	}, map[string]string{"Authorization": "Bearer bootstrap-token"})
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register secure node status = %d, want %d", registerResp.Code, http.StatusOK)
	}

	unsignedHeartbeat := performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/agent/heartbeat", map[string]interface{}{
		"nodeKey": "node-secure",
	}, map[string]string{"Authorization": "Bearer bootstrap-token"})
	if unsignedHeartbeat.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned heartbeat status = %d, want %d", unsignedHeartbeat.Code, http.StatusUnauthorized)
	}

	signedHeartbeat := performSignedJSONRequest(t, router, http.MethodPost, "/api/agent/heartbeat", "node-secure", "node-secret-1", map[string]interface{}{
		"nodeKey": "node-secure",
	})
	if signedHeartbeat.Code != http.StatusOK {
		t.Fatalf("signed heartbeat status = %d, want %d", signedHeartbeat.Code, http.StatusOK)
	}

	rootToken := loginForToken(t, router, "root", "root-pass")
	rotateResp := performJSONRequestWithHeaders(t, router, http.MethodPost, "/api/control/nodes/node-secure/agent-secret/rotate", nil, map[string]string{
		"Authorization": "Bearer " + rootToken,
	})
	if rotateResp.Code != http.StatusOK {
		t.Fatalf("rotate secret status = %d, want %d", rotateResp.Code, http.StatusOK)
	}

	var body ResponseBody
	if err := json.Unmarshal(rotateResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("rotate secret json.Unmarshal() error = %v", err)
	}
	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("rotate secret body.Data type = %T", body.Data)
	}
	newSecret, ok := data["agentSecret"].(string)
	if !ok || newSecret == "" {
		t.Fatalf("rotated secret missing: %#v", data["agentSecret"])
	}

	oldSecretHeartbeat := performSignedJSONRequest(t, router, http.MethodPost, "/api/agent/heartbeat", "node-secure", "node-secret-1", map[string]interface{}{
		"nodeKey": "node-secure",
	})
	if oldSecretHeartbeat.Code != http.StatusUnauthorized {
		t.Fatalf("old secret heartbeat status = %d, want %d", oldSecretHeartbeat.Code, http.StatusUnauthorized)
	}

	rotatedHeartbeat := performSignedJSONRequest(t, router, http.MethodPost, "/api/agent/heartbeat", "node-secure", newSecret, map[string]interface{}{
		"nodeKey": "node-secure",
	})
	if rotatedHeartbeat.Code != http.StatusOK {
		t.Fatalf("rotated signed heartbeat status = %d, want %d", rotatedHeartbeat.Code, http.StatusOK)
	}
}

func TestControlUIRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	router := NewRouter(store)

	resp := performJSONRequest(t, router, http.MethodGet, "/", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("ui route status = %d, want %d", resp.Code, http.StatusOK)
	}
	if !strings.Contains(resp.Body.String(), "Trojan Control Center") {
		t.Fatalf("ui route body missing title: %s", resp.Body.String())
	}
}

func TestReadyzAndRuntimeStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	if _, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "root",
		Password: "root-pass",
		Role:     "super_admin",
	}); err != nil {
		t.Fatalf("EnsureControlAdmin(root) error = %v", err)
	}
	router := NewRouter(store, ServerOptions{
		ControlJWTSecret:   "jwt-secret",
		LoginRateLimit:     15,
		AgentRateLimit:     120,
		AuditRetentionDays: 90,
		TaskRetentionDays:  30,
		UsageRetentionDays: 14,
		CleanupInterval:    45 * time.Minute,
	})

	readyResp := performJSONRequest(t, router, http.MethodGet, "/readyz", nil)
	if readyResp.Code != http.StatusOK {
		t.Fatalf("readyz status = %d, want %d", readyResp.Code, http.StatusOK)
	}

	rootToken := loginForToken(t, router, "root", "root-pass")
	runtimeResp := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/runtime/status", nil, map[string]string{
		"Authorization": "Bearer " + rootToken,
	})
	if runtimeResp.Code != http.StatusOK {
		t.Fatalf("runtime status code = %d, want %d", runtimeResp.Code, http.StatusOK)
	}

	var body ResponseBody
	if err := json.Unmarshal(runtimeResp.Body.Bytes(), &body); err != nil {
		t.Fatalf("runtime json.Unmarshal() error = %v", err)
	}
	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("runtime body.Data type = %T", body.Data)
	}
	runtimeData, ok := data["runtime"].(map[string]interface{})
	if !ok {
		t.Fatalf("runtime data type = %T", data["runtime"])
	}
	if backend, _ := runtimeData["backend"].(string); backend != "memory" {
		t.Fatalf("runtime backend = %#v, want memory", runtimeData["backend"])
	}
	if healthy, _ := runtimeData["healthy"].(bool); !healthy {
		t.Fatalf("runtime healthy = %#v, want true", runtimeData["healthy"])
	}
	configData, ok := data["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("config data type = %T", data["config"])
	}
	if secs, _ := configData["cleanupIntervalSec"].(float64); int(secs) != 2700 {
		t.Fatalf("cleanupIntervalSec = %#v, want 2700", configData["cleanupIntervalSec"])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	if _, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey: "node-metrics",
		Name:    "metrics-node",
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}
	if _, err := store.Heartbeat(HeartbeatRequest{
		NodeKey: "node-metrics",
	}); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if _, err := store.CreateUser(CreateUserRequest{
		Username: "alice",
		Password: "YWxpY2UtcGFzcw==",
	}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "root",
		Password: "root-pass",
		Role:     "super_admin",
	}); err != nil {
		t.Fatalf("EnsureControlAdmin() error = %v", err)
	}
	if _, err := store.CreateTask(CreateTaskRequest{
		NodeKey:  "node-metrics",
		TaskType: "sync_users",
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	router := NewRouter(store, ServerOptions{MetricsToken: "metrics-secret"})

	unauthorizedResp := performJSONRequest(t, router, http.MethodGet, "/metrics", nil)
	if unauthorizedResp.Code != http.StatusUnauthorized {
		t.Fatalf("metrics without token status = %d, want %d", unauthorizedResp.Code, http.StatusUnauthorized)
	}

	metricsResp := performJSONRequestWithHeaders(t, router, http.MethodGet, "/metrics", nil, map[string]string{
		"Authorization": "Bearer metrics-secret",
	})
	if metricsResp.Code != http.StatusOK {
		t.Fatalf("metrics with token status = %d, want %d", metricsResp.Code, http.StatusOK)
	}
	body := metricsResp.Body.String()
	if !strings.Contains(body, "trojan_control_up 1") {
		t.Fatalf("metrics missing health gauge: %s", body)
	}
	if !strings.Contains(body, "trojan_control_store_info{backend=\"memory\"} 1") {
		t.Fatalf("metrics missing backend gauge: %s", body)
	}
	if !strings.Contains(body, "trojan_control_nodes_total 1") {
		t.Fatalf("metrics missing node count: %s", body)
	}
	if !strings.Contains(body, "trojan_control_users_total 1") {
		t.Fatalf("metrics missing user count: %s", body)
	}
	if !strings.Contains(body, "trojan_control_tasks_total{status=\"pending\"} 1") {
		t.Fatalf("metrics missing pending task count: %s", body)
	}
}

func TestAlertsSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := NewMemoryStore()
	if _, err := store.EnsureControlAdmin(EnsureControlAdminRequest{
		Username: "root",
		Password: "root-pass",
		Role:     "super_admin",
	}); err != nil {
		t.Fatalf("EnsureControlAdmin() error = %v", err)
	}
	if _, err := store.RegisterNode(RegisterNodeRequest{
		NodeKey: "node-alert",
		Name:    "alert-node",
	}); err != nil {
		t.Fatalf("RegisterNode() error = %v", err)
	}
	task, err := store.CreateTask(CreateTaskRequest{
		NodeKey:  "node-alert",
		TaskType: "sync_users",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := store.StartTask(task.ID, StartTaskRequest{
		NodeKey:        "node-alert",
		ExecutionToken: "alert-exec",
	}); err != nil {
		t.Fatalf("StartTask() error = %v", err)
	}
	if _, err := store.FinishTask(task.ID, FinishTaskRequest{
		NodeKey:        "node-alert",
		ExecutionToken: "alert-exec",
		Success:        false,
		Message:        "boom",
	}); err != nil {
		t.Fatalf("FinishTask() error = %v", err)
	}
	store.mu.Lock()
	node := store.nodes["node-alert"]
	node.LastSeenAt = time.Now().Add(-15 * time.Minute)
	store.mu.Unlock()

	router := NewRouter(store, ServerOptions{
		ControlJWTSecret:          "jwt-secret",
		NodeStaleMinutes:          10,
		FailedTaskAlertThreshold:  1,
		PendingTaskAlertThreshold: 10,
	})
	rootToken := loginForToken(t, router, "root", "root-pass")

	resp := performJSONRequestWithHeaders(t, router, http.MethodGet, "/api/control/alerts/summary", nil, map[string]string{
		"Authorization": "Bearer " + rootToken,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("alerts summary status = %d, want %d", resp.Code, http.StatusOK)
	}

	var body ResponseBody
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	summary, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("body.Data type = %T", body.Data)
	}
	if status, _ := summary["status"].(string); status != "alert" {
		t.Fatalf("summary status = %#v, want alert", summary["status"])
	}
	issues, ok := summary["issues"].([]interface{})
	if !ok || len(issues) < 2 {
		t.Fatalf("issues = %#v, want stale_nodes and failed_tasks", summary["issues"])
	}
}

func performJSONRequest(t *testing.T, router *gin.Engine, method string, target string, body interface{}) *httptest.ResponseRecorder {
	return performJSONRequestWithHeaders(t, router, method, target, body, nil)
}

func loginForToken(t *testing.T, router *gin.Engine, username string, password string) string {
	t.Helper()
	resp := performJSONRequest(t, router, http.MethodPost, "/api/control/auth/login", map[string]interface{}{
		"username": username,
		"password": password,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("login(%s) status = %d, want %d", username, resp.Code, http.StatusOK)
	}
	var body ResponseBody
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("login(%s) json.Unmarshal() error = %v", username, err)
	}
	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("login(%s) body.Data type = %T", username, body.Data)
	}
	token, ok := data["token"].(string)
	if !ok || token == "" {
		t.Fatalf("login(%s) token missing", username)
	}
	return token
}

func performSignedJSONRequest(t *testing.T, router *gin.Engine, method string, target string, nodeKey string, secret string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
	}
	signingKey, err := hashAgentSecret(secret)
	if err != nil {
		t.Fatalf("hashAgentSecret() error = %v", err)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	headers := map[string]string{
		agentNodeKeyHeader:   nodeKey,
		agentTimestampHeader: timestamp,
		agentSignatureHeader: signAgentRequest(method, target, nodeKey, timestamp, payload, signingKey),
	}
	return performJSONRequestWithHeaders(t, router, method, target, body, headers)
}

func performJSONRequestWithHeaders(t *testing.T, router *gin.Engine, method string, target string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func itoa(v uint64) string {
	return fmt.Sprintf("%d", v)
}

func taskIDFromTaskMap(taskMap map[string]interface{}) uint64 {
	return uint64(taskMap["id"].(float64))
}

func extractTaskAttempt(t *testing.T, resp *httptest.ResponseRecorder) int {
	t.Helper()

	var body ResponseBody
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("body.Data type = %T, want map[string]interface{}", body.Data)
	}
	attempt, ok := data["attempt"].(float64)
	if !ok {
		t.Fatalf("attempt type = %T, want float64", data["attempt"])
	}
	return int(attempt)
}

func decodeBackupBundle(value interface{}) (BackupBundle, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return BackupBundle{}, err
	}
	var bundle BackupBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return BackupBundle{}, err
	}
	return bundle, nil
}
