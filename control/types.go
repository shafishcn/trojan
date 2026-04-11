package control

import "time"

// ResponseBody is the common control-plane API response.
type ResponseBody struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// RegisterNodeRequest is sent by a node agent when it first connects.
type RegisterNodeRequest struct {
	NodeKey        string   `json:"nodeKey" binding:"required"`
	Name           string   `json:"name" binding:"required"`
	Region         string   `json:"region"`
	Provider       string   `json:"provider"`
	Endpoint       string   `json:"endpoint"`
	Port           int      `json:"port"`
	TrojanType     string   `json:"trojanType"`
	TrojanVersion  string   `json:"trojanVersion"`
	ManagerVersion string   `json:"managerVersion"`
	PublicIP       string   `json:"publicIp"`
	DomainName     string   `json:"domainName"`
	Tags           []string `json:"tags"`
	AgentSecret    string   `json:"agentSecret,omitempty"`
}

// HeartbeatRequest is sent by a node agent on a schedule.
type HeartbeatRequest struct {
	NodeKey       string                 `json:"nodeKey" binding:"required"`
	CPUPercent    float64                `json:"cpuPercent"`
	MemoryPercent float64                `json:"memoryPercent"`
	DiskPercent   float64                `json:"diskPercent"`
	TCPCount      int                    `json:"tcpCount"`
	UDPCount      int                    `json:"udpCount"`
	UploadSpeed   uint64                 `json:"uploadSpeed"`
	DownloadSpeed uint64                 `json:"downloadSpeed"`
	Payload       map[string]interface{} `json:"payload"`
}

// CreateTaskRequest is sent by the control plane to enqueue a node task.
type CreateTaskRequest struct {
	NodeKey  string                 `json:"nodeKey" binding:"required"`
	TaskType string                 `json:"taskType" binding:"required"`
	Payload  map[string]interface{} `json:"payload"`
}

// TaskQuery filters task listing for control-plane audit views.
type TaskQuery struct {
	Limit    int
	Offset   int
	NodeKey  string
	TaskType string
	Status   string
}

// AuditQuery filters control-plane audit logs.
type AuditQuery struct {
	Limit        int
	Offset       int
	Actor        string
	Action       string
	ResourceType string
}

// CleanupRequest defines retention windows in days for historical data cleanup.
type CleanupRequest struct {
	AuditRetentionDays int `json:"auditRetentionDays"`
	TaskRetentionDays  int `json:"taskRetentionDays"`
	UsageRetentionDays int `json:"usageRetentionDays"`
}

// CleanupResult reports how much historical data was pruned.
type CleanupResult struct {
	AuditLogsDeleted    int64     `json:"auditLogsDeleted"`
	TaskEventsDeleted   int64     `json:"taskEventsDeleted"`
	TasksDeleted        int64     `json:"tasksDeleted"`
	UsageReportsDeleted int64     `json:"usageReportsDeleted"`
	CompletedAt         time.Time `json:"completedAt"`
}

// BackupBundle is a portable snapshot of the control-plane state.
type BackupBundle struct {
	Version    string            `json:"version"`
	ExportedAt time.Time         `json:"exportedAt"`
	Admins     []BackupAdmin     `json:"admins"`
	Nodes      []BackupNode      `json:"nodes"`
	Users      []ControlUser     `json:"users"`
	Bindings   []UserBinding     `json:"bindings"`
	Tasks      []Task            `json:"tasks"`
	TaskEvents []TaskEvent       `json:"taskEvents"`
	AuditLogs  []ControlAuditLog `json:"auditLogs"`
	Usage      []UsageSnapshot   `json:"usage"`
}

// BackupAdmin contains the serializable control-admin state including hash.
type BackupAdmin struct {
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	PasswordHash string    `json:"passwordHash"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// BackupNode contains node metadata plus the persisted agent-secret hash.
type BackupNode struct {
	Node            Node   `json:"node"`
	AgentSecretHash string `json:"agentSecretHash,omitempty"`
}

// RuntimeStatus summarizes current control-plane health and runtime metadata.
type RuntimeStatus struct {
	Backend   string                 `json:"backend"`
	Healthy   bool                   `json:"healthy"`
	CheckedAt time.Time              `json:"checkedAt"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// AlertSummary is a derived operational alert view for the control plane.
type AlertSummary struct {
	Status     string                 `json:"status"`
	CheckedAt  time.Time              `json:"checkedAt"`
	Thresholds map[string]interface{} `json:"thresholds"`
	Issues     []AlertIssue           `json:"issues"`
}

// AlertIssue describes one active operational issue.
type AlertIssue struct {
	Severity string                 `json:"severity"`
	Kind     string                 `json:"kind"`
	Message  string                 `json:"message"`
	Count    int                    `json:"count"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// MetricsSnapshot is the normalized control-plane metric set.
type MetricsSnapshot struct {
	Backend            string    `json:"backend"`
	Healthy            bool      `json:"healthy"`
	CheckedAt          time.Time `json:"checkedAt"`
	NodeCount          int64     `json:"nodeCount"`
	ActiveNodeCount    int64     `json:"activeNodeCount"`
	UserCount          int64     `json:"userCount"`
	AdminCount         int64     `json:"adminCount"`
	TaskCount          int64     `json:"taskCount"`
	TaskPendingCount   int64     `json:"taskPendingCount"`
	TaskRunningCount   int64     `json:"taskRunningCount"`
	TaskSucceededCount int64     `json:"taskSucceededCount"`
	TaskFailedCount    int64     `json:"taskFailedCount"`
	TaskEventCount     int64     `json:"taskEventCount"`
	AuditLogCount      int64     `json:"auditLogCount"`
	UsageCount         int64     `json:"usageCount"`
}

// StartTaskRequest marks a task as running for a node.
type StartTaskRequest struct {
	NodeKey        string `json:"nodeKey" binding:"required"`
	ExecutionToken string `json:"executionToken" binding:"required"`
}

// FinishTaskRequest reports the final execution result for a task.
type FinishTaskRequest struct {
	NodeKey        string                 `json:"nodeKey" binding:"required"`
	ExecutionToken string                 `json:"executionToken" binding:"required"`
	Success        bool                   `json:"success"`
	Message        string                 `json:"message"`
	Details        map[string]interface{} `json:"details"`
}

// UsageReportItem represents one user's usage snapshot on a node.
type UsageReportItem struct {
	Username   string `json:"username" binding:"required"`
	Upload     uint64 `json:"upload"`
	Download   uint64 `json:"download"`
	Quota      int64  `json:"quota"`
	ExpiryDate string `json:"expiryDate,omitempty"`
}

// UsageReportRequest is sent by an agent to report current user usage.
type UsageReportRequest struct {
	NodeKey string            `json:"nodeKey" binding:"required"`
	Users   []UsageReportItem `json:"users" binding:"required"`
}

// UsageSnapshot is the normalized stored usage data.
type UsageSnapshot struct {
	NodeKey    string    `json:"nodeKey"`
	Username   string    `json:"username"`
	Upload     uint64    `json:"upload"`
	Download   uint64    `json:"download"`
	Quota      int64     `json:"quota"`
	ExpiryDate string    `json:"expiryDate,omitempty"`
	ReportedAt time.Time `json:"reportedAt"`
}

// ControlUser is the control-plane user source of truth.
type ControlUser struct {
	Username   string    `json:"username"`
	Password   string    `json:"password"`
	Quota      int64     `json:"quota"`
	UseDays    uint      `json:"useDays"`
	ExpiryDate string    `json:"expiryDate,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// CreateUserRequest creates or updates a control-plane user.
type CreateUserRequest struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	Quota      int64  `json:"quota"`
	UseDays    uint   `json:"useDays"`
	ExpiryDate string `json:"expiryDate,omitempty"`
}

// ControlAdmin is a control-plane administrator account.
type ControlAdmin struct {
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// EnsureControlAdminRequest bootstraps a control-plane admin account.
type EnsureControlAdminRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role"`
}

// UpdateControlAdminRequest updates an existing control-plane admin.
type UpdateControlAdminRequest struct {
	Password string `json:"password"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

// CreateAuditLogRequest writes one control-plane audit log entry.
type CreateAuditLogRequest struct {
	Actor        string                 `json:"actor"`
	ActorRole    string                 `json:"actorRole"`
	Action       string                 `json:"action"`
	ResourceType string                 `json:"resourceType"`
	ResourceID   string                 `json:"resourceId"`
	Message      string                 `json:"message"`
	Details      map[string]interface{} `json:"details"`
}

// ControlAuditLog is a persisted audit trail record for operator actions.
type ControlAuditLog struct {
	ID           uint64                 `json:"id"`
	Actor        string                 `json:"actor"`
	ActorRole    string                 `json:"actorRole"`
	Action       string                 `json:"action"`
	ResourceType string                 `json:"resourceType"`
	ResourceID   string                 `json:"resourceId"`
	Message      string                 `json:"message"`
	Details      map[string]interface{} `json:"details,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
}

// UserBinding binds a control-plane user to a node.
type UserBinding struct {
	Username  string    `json:"username"`
	NodeKey   string    `json:"nodeKey"`
	CreatedAt time.Time `json:"createdAt"`
}

// Node represents a managed VPS node in the control plane.
type Node struct {
	ID             uint64             `json:"id"`
	NodeKey        string             `json:"nodeKey"`
	Name           string             `json:"name"`
	Region         string             `json:"region"`
	Provider       string             `json:"provider"`
	Endpoint       string             `json:"endpoint"`
	Port           int                `json:"port"`
	TrojanType     string             `json:"trojanType"`
	TrojanVersion  string             `json:"trojanVersion"`
	ManagerVersion string             `json:"managerVersion"`
	PublicIP       string             `json:"publicIp"`
	DomainName     string             `json:"domainName"`
	Tags           []string           `json:"tags"`
	AgentAuthMode  string             `json:"agentAuthMode"`
	Status         string             `json:"status"`
	LastSeenAt     time.Time          `json:"lastSeenAt"`
	LastHeartbeat  *HeartbeatSnapshot `json:"lastHeartbeat,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

// NodeAgentCredential contains the internal auth material for a node agent.
type NodeAgentCredential struct {
	NodeKey     string
	SecretHash  string
	UpdatedAt   time.Time
	AuthEnabled bool
}

// HeartbeatSnapshot stores the latest heartbeat sent by a node.
type HeartbeatSnapshot struct {
	CPUPercent    float64                `json:"cpuPercent"`
	MemoryPercent float64                `json:"memoryPercent"`
	DiskPercent   float64                `json:"diskPercent"`
	TCPCount      int                    `json:"tcpCount"`
	UDPCount      int                    `json:"udpCount"`
	UploadSpeed   uint64                 `json:"uploadSpeed"`
	DownloadSpeed uint64                 `json:"downloadSpeed"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
	ReportedAt    time.Time              `json:"reportedAt"`
}

// Task is a control-plane task waiting for a node agent.
type Task struct {
	ID             uint64                 `json:"id"`
	NodeKey        string                 `json:"nodeKey"`
	TaskType       string                 `json:"taskType"`
	Payload        map[string]interface{} `json:"payload"`
	Status         string                 `json:"status"`
	Attempt        int                    `json:"attempt"`
	ExecutionToken string                 `json:"executionToken,omitempty"`
	ResultMessage  string                 `json:"resultMessage,omitempty"`
	ResultDetails  map[string]interface{} `json:"resultDetails,omitempty"`
	CreatedAt      time.Time              `json:"createdAt"`
	StartedAt      *time.Time             `json:"startedAt,omitempty"`
	FinishedAt     *time.Time             `json:"finishedAt,omitempty"`
}

// TaskEvent records the task lifecycle for auditing and troubleshooting.
type TaskEvent struct {
	ID        uint64                 `json:"id"`
	TaskID    uint64                 `json:"taskId"`
	NodeKey   string                 `json:"nodeKey"`
	EventType string                 `json:"eventType"`
	Actor     string                 `json:"actor"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
}
