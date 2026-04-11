package control

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
	gopsnet "github.com/shirou/gopsutil/net"

	"trojan/core"
	"trojan/trojan"
)

// AgentExecutor executes control-plane tasks on the local node.
type AgentExecutor struct{}

// NewAgentExecutor creates a new local task executor.
func NewAgentExecutor() *AgentExecutor {
	return &AgentExecutor{}
}

// TaskResult represents the result of a local task execution.
type TaskResult struct {
	Success bool
	Message string
	Details map[string]interface{}
}

// SyncUserPayload describes one user to be synchronized to a node.
type SyncUserPayload struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	Quota      *int64 `json:"quota,omitempty"`
	UseDays    *uint  `json:"useDays,omitempty"`
	ExpiryDate string `json:"expiryDate,omitempty"`
}

// Execute runs a supported task locally.
func (e *AgentExecutor) Execute(task Task) TaskResult {
	switch task.TaskType {
	case "start_trojan":
		trojan.Start()
		return TaskResult{Success: true, Message: "trojan started"}
	case "stop_trojan":
		trojan.Stop()
		return TaskResult{Success: true, Message: "trojan stopped"}
	case "restart_trojan":
		trojan.Restart()
		return TaskResult{Success: true, Message: "trojan restarted"}
	case "switch_trojan_type":
		tType, _ := task.Payload["type"].(string)
		if tType == "" {
			return TaskResult{Message: "type is required"}
		}
		if err := trojan.SwitchType(tType); err != nil {
			return TaskResult{Message: err.Error()}
		}
		return TaskResult{Success: true, Message: "trojan type switched", Details: map[string]interface{}{"type": tType}}
	case "sync_users":
		return e.syncUsers(task.Payload)
	default:
		return TaskResult{Message: "unsupported task type: " + task.TaskType}
	}
}

// CollectHeartbeat gathers a heartbeat snapshot from the local machine.
func CollectHeartbeat(nodeKey string) HeartbeatRequest {
	var (
		cpuPercent    float64
		memoryPercent float64
		diskPercent   float64
		tcpCount      int
		udpCount      int
		uploadSpeed   uint64
		downloadSpeed uint64
	)

	if values, err := cpu.Percent(0, false); err == nil && len(values) > 0 {
		cpuPercent = values[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		memoryPercent = vm.UsedPercent
	}
	if du, err := disk.Usage("/"); err == nil {
		diskPercent = du.UsedPercent
	}
	if tcpConn, err := gopsnet.Connections("tcp"); err == nil {
		tcpCount = len(tcpConn)
	}
	if udpConn, err := gopsnet.Connections("udp"); err == nil {
		udpCount = len(udpConn)
	}
	if counters, err := gopsnet.IOCounters(false); err == nil && len(counters) > 0 {
		uploadSpeed = counters[0].BytesSent
		downloadSpeed = counters[0].BytesRecv
	}

	return HeartbeatRequest{
		NodeKey:       nodeKey,
		CPUPercent:    cpuPercent,
		MemoryPercent: memoryPercent,
		DiskPercent:   diskPercent,
		TCPCount:      tcpCount,
		UDPCount:      udpCount,
		UploadSpeed:   uploadSpeed,
		DownloadSpeed: downloadSpeed,
		Payload: map[string]interface{}{
			"trojanType":    trojan.Type(),
			"trojanVersion": trojan.Version(),
			"trojanStatus":  trojan.Status(false),
			"reportedAt":    time.Now().Format(time.RFC3339),
		},
	}
}

// CollectUsage gathers user usage snapshots from the local Trojan MySQL backend.
func CollectUsage(nodeKey string) (UsageReportRequest, error) {
	mysql := core.GetMysql()
	if mysql == nil {
		return UsageReportRequest{}, fmt.Errorf("mysql config is not available")
	}
	users, err := mysql.GetData()
	if err != nil {
		return UsageReportRequest{}, err
	}

	req := UsageReportRequest{
		NodeKey: nodeKey,
		Users:   make([]UsageReportItem, 0, len(users)),
	}
	for _, user := range users {
		req.Users = append(req.Users, UsageReportItem{
			Username:   user.Username,
			Upload:     user.Upload,
			Download:   user.Download,
			Quota:      user.Quota,
			ExpiryDate: user.ExpiryDate,
		})
	}
	return req, nil
}

func (e *AgentExecutor) syncUsers(payload map[string]interface{}) TaskResult {
	mysql := core.GetMysql()
	if mysql == nil {
		return TaskResult{Message: "mysql config is not available"}
	}

	users, replace, err := parseSyncUserPayload(payload)
	if err != nil {
		return TaskResult{Message: err.Error()}
	}

	if err := syncUsersToNode(mysql, users, replace); err != nil {
		return TaskResult{Message: err.Error()}
	}
	trojan.Restart()
	return TaskResult{
		Success: true,
		Message: "users synchronized",
		Details: map[string]interface{}{
			"userCount": len(users),
			"replace":   replace,
		},
	}
}

func parseSyncUserPayload(payload map[string]interface{}) ([]SyncUserPayload, bool, error) {
	var result []SyncUserPayload
	usersRaw, ok := payload["users"]
	if !ok {
		return nil, false, fmt.Errorf("users is required")
	}
	userList, ok := usersRaw.([]interface{})
	if !ok {
		return nil, false, fmt.Errorf("users must be an array")
	}
	for _, item := range userList {
		userMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("users item must be an object")
		}
		user := SyncUserPayload{}
		if value, _ := userMap["username"].(string); value != "" {
			user.Username = value
		}
		if value, _ := userMap["password"].(string); value != "" {
			user.Password = value
		}
		if user.Username == "" || user.Password == "" {
			return nil, false, fmt.Errorf("username and password are required")
		}
		if value, exists := parseInt64Field(userMap["quota"]); exists {
			user.Quota = &value
		}
		if value, exists := parseUintField(userMap["useDays"]); exists {
			user.UseDays = &value
		}
		if value, _ := userMap["expiryDate"].(string); value != "" {
			user.ExpiryDate = value
		}
		result = append(result, user)
	}

	replace := false
	if raw, ok := payload["replace"].(bool); ok {
		replace = raw
	}
	return result, replace, nil
}

func syncUsersToNode(mysql *core.Mysql, users []SyncUserPayload, replace bool) error {
	expected := make(map[string]SyncUserPayload, len(users))
	for _, user := range users {
		expected[user.Username] = user
	}

	if replace {
		existing, err := mysql.GetData()
		if err != nil {
			return err
		}
		for _, user := range existing {
			if _, ok := expected[user.Username]; !ok {
				if err := mysql.DeleteUser(user.ID); err != nil {
					return err
				}
			}
		}
	}

	for _, user := range users {
		originPassBytes, err := base64.StdEncoding.DecodeString(user.Password)
		if err != nil {
			return fmt.Errorf("decode password for %s: %w", user.Username, err)
		}
		originPass := string(originPassBytes)

		existing := mysql.GetUserByName(user.Username)
		if existing == nil {
			if err := mysql.CreateUser(user.Username, user.Password, originPass); err != nil {
				return err
			}
			existing = mysql.GetUserByName(user.Username)
			if existing == nil {
				return fmt.Errorf("user %s created but not found", user.Username)
			}
		} else if existing.Password != user.Password {
			if err := mysql.UpdateUser(existing.ID, user.Username, user.Password, originPass); err != nil {
				return err
			}
		}

		if user.Quota != nil {
			if err := mysql.SetQuota(existing.ID, int(*user.Quota)); err != nil {
				return err
			}
		}
		if user.UseDays != nil {
			if err := mysql.SetExpire(existing.ID, *user.UseDays); err != nil {
				return err
			}
		} else if user.ExpiryDate == "" {
			if err := mysql.CancelExpire(existing.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseInt64Field(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case string:
		if typed == "" {
			return 0, false
		}
		result, err := strconv.ParseInt(typed, 10, 64)
		if err == nil {
			return result, true
		}
	}
	return 0, false
}

func parseUintField(value interface{}) (uint, bool) {
	switch typed := value.(type) {
	case float64:
		return uint(typed), true
	case int:
		return uint(typed), true
	case uint:
		return typed, true
	case string:
		if typed == "" {
			return 0, false
		}
		result, err := strconv.ParseUint(typed, 10, 64)
		if err == nil {
			return uint(result), true
		}
	}
	return 0, false
}

// UserSyncPreview is a stable summary for logs and tests.
func UserSyncPreview(users []SyncUserPayload) []string {
	result := make([]string, 0, len(users))
	for _, user := range users {
		result = append(result, user.Username)
	}
	sort.Strings(result)
	return result
}
