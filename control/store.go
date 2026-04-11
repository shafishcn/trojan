package control

import "errors"

var ErrNodeNotFound = errors.New("node not found")
var ErrTaskNotFound = errors.New("task not found")
var ErrUserNotFound = errors.New("user not found")
var ErrBindingNotFound = errors.New("binding not found")
var ErrAdminNotFound = errors.New("admin not found")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrPermissionDenied = errors.New("permission denied")
var ErrInvalidRole = errors.New("invalid role")
var ErrInvalidStatus = errors.New("invalid status")
var ErrTaskConflict = errors.New("task state conflict")
var ErrInvalidBackup = errors.New("invalid backup bundle")

// Store defines the minimum persistence contract for the control plane.
type Store interface {
	EnsureControlAdmin(req EnsureControlAdminRequest) (*ControlAdmin, error)
	ListControlAdmins() ([]ControlAdmin, error)
	GetControlAdmin(username string) (*ControlAdmin, error)
	AuthenticateControlAdmin(username string, password string) (*ControlAdmin, error)
	UpdateControlAdmin(username string, req UpdateControlAdminRequest) (*ControlAdmin, error)
	CleanupHistory(req CleanupRequest) (*CleanupResult, error)
	ExportBackup() (*BackupBundle, error)
	ImportBackup(bundle BackupBundle) error
	AppendAuditLog(req CreateAuditLogRequest) (*ControlAuditLog, error)
	ListAuditLogs(query AuditQuery) ([]ControlAuditLog, error)
	GetNodeAgentCredential(nodeKey string) (*NodeAgentCredential, error)
	RotateNodeAgentSecret(nodeKey string) (string, error)
	RegisterNode(req RegisterNodeRequest) (*Node, error)
	Heartbeat(req HeartbeatRequest) (*Node, error)
	PendingTasks(nodeKey string, limit int) ([]Task, error)
	ListNodes() ([]Node, error)
	CreateTask(req CreateTaskRequest) (*Task, error)
	ListTasks(query TaskQuery) ([]Task, error)
	GetTask(taskID uint64) (*Task, error)
	ListTaskEvents(taskID uint64, limit int) ([]TaskEvent, error)
	RetryTask(taskID uint64) (*Task, error)
	StartTask(taskID uint64, req StartTaskRequest) (*Task, error)
	FinishTask(taskID uint64, req FinishTaskRequest) (*Task, error)
	ReportUsage(req UsageReportRequest) ([]UsageSnapshot, error)
	NodeUsage(nodeKey string, limit int) ([]UsageSnapshot, error)
	ListUsers() ([]ControlUser, error)
	CreateUser(req CreateUserRequest) (*ControlUser, error)
	GetUser(username string) (*ControlUser, error)
	DeleteUser(username string) error
	BindUserToNode(username, nodeKey string) (*UserBinding, error)
	UnbindUserFromNode(username, nodeKey string) error
	NodeUsers(nodeKey string) ([]ControlUser, error)
	UserNodes(username string) ([]Node, error)
	UserUsage(username string, limit int) ([]UsageSnapshot, error)
	SyncNodeUsers(nodeKey string) (*Task, error)
}
