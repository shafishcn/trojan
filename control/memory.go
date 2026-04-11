package control

import (
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory prototype store for the control plane.
type MemoryStore struct {
	mu              sync.RWMutex
	nextNodeID      uint64
	nextTaskID      uint64
	nextTaskEventID uint64
	nextAuditLogID  uint64
	nodes           map[string]*Node
	tasks           map[string][]Task
	taskEvents      map[uint64][]TaskEvent
	auditLogs       []ControlAuditLog
	usage           map[string][]UsageSnapshot
	users           map[string]*ControlUser
	admins          map[string]*ControlAdmin
	nodeSecrets     map[string]string
	bindings        map[string]map[string]UserBinding
}

// NewMemoryStore creates a new in-memory control-plane store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nextNodeID:      1,
		nextTaskID:      1,
		nextTaskEventID: 1,
		nextAuditLogID:  1,
		nodes:           make(map[string]*Node),
		tasks:           make(map[string][]Task),
		taskEvents:      make(map[uint64][]TaskEvent),
		usage:           make(map[string][]UsageSnapshot),
		users:           make(map[string]*ControlUser),
		admins:          make(map[string]*ControlAdmin),
		nodeSecrets:     make(map[string]string),
		bindings:        make(map[string]map[string]UserBinding),
	}
}

// ExportBackup returns a full in-memory snapshot of the control-plane state.
func (s *MemoryStore) ExportBackup() (*BackupBundle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bundle := &BackupBundle{
		Version:    "control.v1",
		ExportedAt: time.Now(),
		Admins:     make([]BackupAdmin, 0, len(s.admins)),
		Nodes:      make([]BackupNode, 0, len(s.nodes)),
		Users:      make([]ControlUser, 0, len(s.users)),
		Bindings:   make([]UserBinding, 0),
		Tasks:      make([]Task, 0),
		TaskEvents: make([]TaskEvent, 0),
		AuditLogs:  make([]ControlAuditLog, 0, len(s.auditLogs)),
		Usage:      make([]UsageSnapshot, 0),
	}

	for _, admin := range s.admins {
		bundle.Admins = append(bundle.Admins, BackupAdmin{
			Username:     admin.Username,
			Role:         admin.Role,
			Status:       admin.Status,
			PasswordHash: admin.PasswordHash,
			CreatedAt:    admin.CreatedAt,
			UpdatedAt:    admin.UpdatedAt,
		})
	}
	sort.Slice(bundle.Admins, func(i, j int) bool { return bundle.Admins[i].Username < bundle.Admins[j].Username })

	for nodeKey, node := range s.nodes {
		copyNode := cloneNode(node)
		bundle.Nodes = append(bundle.Nodes, BackupNode{
			Node:            *copyNode,
			AgentSecretHash: s.nodeSecrets[nodeKey],
		})
	}
	sort.Slice(bundle.Nodes, func(i, j int) bool { return bundle.Nodes[i].Node.NodeKey < bundle.Nodes[j].Node.NodeKey })

	for _, user := range s.users {
		bundle.Users = append(bundle.Users, *cloneControlUser(user))
	}
	sort.Slice(bundle.Users, func(i, j int) bool { return bundle.Users[i].Username < bundle.Users[j].Username })

	for nodeKey, userBindings := range s.bindings {
		for username, binding := range userBindings {
			copyBinding := binding
			copyBinding.NodeKey = nodeKey
			copyBinding.Username = username
			bundle.Bindings = append(bundle.Bindings, copyBinding)
		}
	}
	sort.Slice(bundle.Bindings, func(i, j int) bool {
		if bundle.Bindings[i].Username == bundle.Bindings[j].Username {
			return bundle.Bindings[i].NodeKey < bundle.Bindings[j].NodeKey
		}
		return bundle.Bindings[i].Username < bundle.Bindings[j].Username
	})

	for _, nodeTasks := range s.tasks {
		for _, task := range nodeTasks {
			bundle.Tasks = append(bundle.Tasks, cloneTask(task))
		}
	}
	sort.Slice(bundle.Tasks, func(i, j int) bool { return bundle.Tasks[i].ID < bundle.Tasks[j].ID })

	for _, events := range s.taskEvents {
		for _, event := range events {
			bundle.TaskEvents = append(bundle.TaskEvents, cloneTaskEvent(event))
		}
	}
	sort.Slice(bundle.TaskEvents, func(i, j int) bool { return bundle.TaskEvents[i].ID < bundle.TaskEvents[j].ID })

	for _, log := range s.auditLogs {
		bundle.AuditLogs = append(bundle.AuditLogs, cloneAuditLog(log))
	}
	sort.Slice(bundle.AuditLogs, func(i, j int) bool { return bundle.AuditLogs[i].ID < bundle.AuditLogs[j].ID })

	for _, snapshots := range s.usage {
		for _, snapshot := range snapshots {
			bundle.Usage = append(bundle.Usage, snapshot)
		}
	}
	sort.Slice(bundle.Usage, func(i, j int) bool {
		if bundle.Usage[i].ReportedAt.Equal(bundle.Usage[j].ReportedAt) {
			if bundle.Usage[i].NodeKey == bundle.Usage[j].NodeKey {
				return bundle.Usage[i].Username < bundle.Usage[j].Username
			}
			return bundle.Usage[i].NodeKey < bundle.Usage[j].NodeKey
		}
		return bundle.Usage[i].ReportedAt.Before(bundle.Usage[j].ReportedAt)
	})

	return bundle, nil
}

// ImportBackup replaces the in-memory state with the provided snapshot.
func (s *MemoryStore) ImportBackup(bundle BackupBundle) error {
	if err := validateBackupBundle(bundle); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextNodeID = 1
	s.nextTaskID = 1
	s.nextTaskEventID = 1
	s.nextAuditLogID = 1
	s.nodes = make(map[string]*Node)
	s.tasks = make(map[string][]Task)
	s.taskEvents = make(map[uint64][]TaskEvent)
	s.auditLogs = make([]ControlAuditLog, 0, len(bundle.AuditLogs))
	s.usage = make(map[string][]UsageSnapshot)
	s.users = make(map[string]*ControlUser)
	s.admins = make(map[string]*ControlAdmin)
	s.nodeSecrets = make(map[string]string)
	s.bindings = make(map[string]map[string]UserBinding)

	for _, admin := range bundle.Admins {
		copyAdmin := &ControlAdmin{
			Username:     admin.Username,
			Role:         admin.Role,
			Status:       admin.Status,
			PasswordHash: admin.PasswordHash,
			CreatedAt:    admin.CreatedAt,
			UpdatedAt:    admin.UpdatedAt,
		}
		s.admins[admin.Username] = copyAdmin
	}

	var maxNodeID uint64
	for _, backupNode := range bundle.Nodes {
		node := backupNode.Node
		copyNode := cloneNode(&node)
		s.nodes[node.NodeKey] = copyNode
		if backupNode.AgentSecretHash != "" {
			s.nodeSecrets[node.NodeKey] = backupNode.AgentSecretHash
		}
		if node.ID > maxNodeID {
			maxNodeID = node.ID
		}
	}
	s.nextNodeID = maxNodeID + 1

	for _, user := range bundle.Users {
		copyUser := user
		s.users[user.Username] = &copyUser
	}

	for _, binding := range bundle.Bindings {
		if _, exists := s.users[binding.Username]; !exists {
			return ErrUserNotFound
		}
		if _, exists := s.nodes[binding.NodeKey]; !exists {
			return ErrNodeNotFound
		}
		if _, exists := s.bindings[binding.Username]; !exists {
			s.bindings[binding.Username] = make(map[string]UserBinding)
		}
		s.bindings[binding.Username][binding.NodeKey] = binding
	}

	var maxTaskID uint64
	for _, task := range bundle.Tasks {
		if _, exists := s.nodes[task.NodeKey]; !exists {
			return ErrNodeNotFound
		}
		s.tasks[task.NodeKey] = append(s.tasks[task.NodeKey], cloneTask(task))
		if task.ID > maxTaskID {
			maxTaskID = task.ID
		}
	}
	s.nextTaskID = maxTaskID + 1

	var maxTaskEventID uint64
	for _, event := range bundle.TaskEvents {
		s.taskEvents[event.TaskID] = append(s.taskEvents[event.TaskID], cloneTaskEvent(event))
		if event.ID > maxTaskEventID {
			maxTaskEventID = event.ID
		}
	}
	s.nextTaskEventID = maxTaskEventID + 1

	var maxAuditLogID uint64
	for _, log := range bundle.AuditLogs {
		s.auditLogs = append(s.auditLogs, cloneAuditLog(log))
		if log.ID > maxAuditLogID {
			maxAuditLogID = log.ID
		}
	}
	s.nextAuditLogID = maxAuditLogID + 1

	for _, snapshot := range bundle.Usage {
		if _, exists := s.nodes[snapshot.NodeKey]; !exists {
			return ErrNodeNotFound
		}
		s.usage[snapshot.NodeKey] = append(s.usage[snapshot.NodeKey], snapshot)
	}

	return nil
}

// CleanupHistory prunes historical audit, task, and usage records by retention window.
func (s *MemoryStore) CleanupHistory(req CleanupRequest) (*CleanupResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := &CleanupResult{
		CompletedAt: time.Now(),
	}

	if req.AuditRetentionDays > 0 {
		cutoff := result.CompletedAt.Add(-time.Duration(req.AuditRetentionDays) * 24 * time.Hour)
		filtered := s.auditLogs[:0]
		for _, log := range s.auditLogs {
			if log.CreatedAt.Before(cutoff) {
				result.AuditLogsDeleted++
				continue
			}
			filtered = append(filtered, log)
		}
		s.auditLogs = filtered
	}

	if req.TaskRetentionDays > 0 {
		cutoff := result.CompletedAt.Add(-time.Duration(req.TaskRetentionDays) * 24 * time.Hour)
		for nodeKey, tasks := range s.tasks {
			filteredTasks := tasks[:0]
			for _, task := range tasks {
				finishedAt := task.CreatedAt
				if task.FinishedAt != nil {
					finishedAt = *task.FinishedAt
				}
				if (task.Status == "succeeded" || task.Status == "failed") && finishedAt.Before(cutoff) {
					result.TasksDeleted++
					if events, exists := s.taskEvents[task.ID]; exists {
						result.TaskEventsDeleted += int64(len(events))
						delete(s.taskEvents, task.ID)
					}
					continue
				}
				filteredTasks = append(filteredTasks, task)
			}
			s.tasks[nodeKey] = filteredTasks
		}
	}

	if req.UsageRetentionDays > 0 {
		cutoff := result.CompletedAt.Add(-time.Duration(req.UsageRetentionDays) * 24 * time.Hour)
		for nodeKey, usage := range s.usage {
			filteredUsage := usage[:0]
			for _, snapshot := range usage {
				if snapshot.ReportedAt.Before(cutoff) {
					result.UsageReportsDeleted++
					continue
				}
				filteredUsage = append(filteredUsage, snapshot)
			}
			s.usage[nodeKey] = filteredUsage
		}
	}

	return result, nil
}

// AppendAuditLog records one operator audit event.
func (s *MemoryStore) AppendAuditLog(req CreateAuditLogRequest) (*ControlAuditLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log := ControlAuditLog{
		ID:           s.nextAuditLogID,
		Actor:        req.Actor,
		ActorRole:    req.ActorRole,
		Action:       req.Action,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		Message:      req.Message,
		Details:      clonePayload(req.Details),
		CreatedAt:    time.Now(),
	}
	s.nextAuditLogID++
	s.auditLogs = append(s.auditLogs, log)
	copyLog := cloneAuditLog(log)
	return &copyLog, nil
}

// ListAuditLogs returns audit logs ordered by newest first.
func (s *MemoryStore) ListAuditLogs(query AuditQuery) ([]ControlAuditLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	result := make([]ControlAuditLog, 0, len(s.auditLogs))
	for _, log := range s.auditLogs {
		if query.Actor != "" && log.Actor != query.Actor {
			continue
		}
		if query.Action != "" && log.Action != query.Action {
			continue
		}
		if query.ResourceType != "" && log.ResourceType != query.ResourceType {
			continue
		}
		result = append(result, cloneAuditLog(log))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID > result[j].ID
	})
	return paginateAuditLogs(result, query.Offset, query.Limit), nil
}

// EnsureControlAdmin creates the bootstrap admin if it does not already exist.
func (s *MemoryStore) EnsureControlAdmin(req EnsureControlAdminRequest) (*ControlAdmin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if admin, exists := s.admins[req.Username]; exists {
		return cloneControlAdmin(admin), nil
	}
	passwordHash, err := hashControlPassword(req.Password)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	role := req.Role
	if role == "" {
		role = "super_admin"
	}
	admin := &ControlAdmin{
		Username:     req.Username,
		Role:         role,
		Status:       "active",
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.admins[req.Username] = admin
	return cloneControlAdmin(admin), nil
}

// ListControlAdmins returns all control-plane administrators.
func (s *MemoryStore) ListControlAdmins() ([]ControlAdmin, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ControlAdmin, 0, len(s.admins))
	for _, admin := range s.admins {
		result = append(result, *cloneControlAdmin(admin))
	}
	sort.Slice(result, func(i, j int) bool {
		if controlRoleRank(result[i].Role) == controlRoleRank(result[j].Role) {
			return result[i].Username < result[j].Username
		}
		return controlRoleRank(result[i].Role) > controlRoleRank(result[j].Role)
	})
	return result, nil
}

// GetControlAdmin returns one control-plane administrator.
func (s *MemoryStore) GetControlAdmin(username string) (*ControlAdmin, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	admin, exists := s.admins[username]
	if !exists {
		return nil, ErrAdminNotFound
	}
	return cloneControlAdmin(admin), nil
}

// AuthenticateControlAdmin validates control-plane admin credentials.
func (s *MemoryStore) AuthenticateControlAdmin(username string, password string) (*ControlAdmin, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	admin, exists := s.admins[username]
	if !exists {
		return nil, ErrAdminNotFound
	}
	if admin.Status != "" && admin.Status != "active" {
		return nil, ErrInvalidCredentials
	}
	if err := checkControlPassword(admin.PasswordHash, password); err != nil {
		return nil, ErrInvalidCredentials
	}
	return cloneControlAdmin(admin), nil
}

// UpdateControlAdmin updates an existing control-plane admin.
func (s *MemoryStore) UpdateControlAdmin(username string, req UpdateControlAdminRequest) (*ControlAdmin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	admin, exists := s.admins[username]
	if !exists {
		return nil, ErrAdminNotFound
	}
	if req.Password != "" {
		passwordHash, err := hashControlPassword(req.Password)
		if err != nil {
			return nil, err
		}
		admin.PasswordHash = passwordHash
	}
	if req.Role != "" {
		admin.Role = req.Role
	}
	if req.Status != "" {
		admin.Status = req.Status
	}
	admin.UpdatedAt = time.Now()
	return cloneControlAdmin(admin), nil
}

// GetNodeAgentCredential returns the internal auth material for a node.
func (s *MemoryStore) GetNodeAgentCredential(nodeKey string) (*NodeAgentCredential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, exists := s.nodes[nodeKey]
	if !exists {
		return nil, ErrNodeNotFound
	}
	secret := s.nodeSecrets[nodeKey]
	return &NodeAgentCredential{
		NodeKey:     nodeKey,
		SecretHash:  secret,
		UpdatedAt:   node.UpdatedAt,
		AuthEnabled: secret != "",
	}, nil
}

// RotateNodeAgentSecret rotates the per-node signing secret and returns the new plaintext secret.
func (s *MemoryStore) RotateNodeAgentSecret(nodeKey string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, exists := s.nodes[nodeKey]
	if !exists {
		return "", ErrNodeNotFound
	}
	secret := GenerateAgentSecret()
	derived, err := hashAgentSecret(secret)
	if err != nil {
		return "", err
	}
	s.nodeSecrets[nodeKey] = derived
	node.AgentAuthMode = "secret"
	node.UpdatedAt = time.Now()
	return secret, nil
}

// RegisterNode creates or updates a node record.
func (s *MemoryStore) RegisterNode(req RegisterNodeRequest) (*Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	node, exists := s.nodes[req.NodeKey]
	if !exists {
		node = &Node{
			ID:        s.nextNodeID,
			NodeKey:   req.NodeKey,
			Status:    "active",
			CreatedAt: now,
		}
		s.nextNodeID++
		s.nodes[req.NodeKey] = node
	}

	node.Name = req.Name
	node.Region = req.Region
	node.Provider = req.Provider
	node.Endpoint = req.Endpoint
	node.Port = req.Port
	node.TrojanType = req.TrojanType
	node.TrojanVersion = req.TrojanVersion
	node.ManagerVersion = req.ManagerVersion
	node.PublicIP = req.PublicIP
	node.DomainName = req.DomainName
	node.Tags = cloneTags(req.Tags)
	if existingSecret := s.nodeSecrets[req.NodeKey]; existingSecret != "" {
		node.AgentAuthMode = "secret"
	} else if req.AgentSecret != "" {
		derived, err := hashAgentSecret(req.AgentSecret)
		if err != nil {
			return nil, err
		}
		s.nodeSecrets[req.NodeKey] = derived
		node.AgentAuthMode = "secret"
	} else {
		node.AgentAuthMode = "token"
	}
	node.LastSeenAt = now
	node.UpdatedAt = now

	return cloneNode(node), nil
}

// Heartbeat updates the latest node heartbeat information.
func (s *MemoryStore) Heartbeat(req HeartbeatRequest) (*Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, exists := s.nodes[req.NodeKey]
	if !exists {
		return nil, ErrNodeNotFound
	}

	now := time.Now()
	node.LastSeenAt = now
	node.UpdatedAt = now
	node.LastHeartbeat = &HeartbeatSnapshot{
		CPUPercent:    req.CPUPercent,
		MemoryPercent: req.MemoryPercent,
		DiskPercent:   req.DiskPercent,
		TCPCount:      req.TCPCount,
		UDPCount:      req.UDPCount,
		UploadSpeed:   req.UploadSpeed,
		DownloadSpeed: req.DownloadSpeed,
		Payload:       clonePayload(req.Payload),
		ReportedAt:    now,
	}

	return cloneNode(node), nil
}

// PendingTasks returns pending tasks for the given node.
func (s *MemoryStore) PendingTasks(nodeKey string, limit int) ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.nodes[nodeKey]; !exists {
		return nil, ErrNodeNotFound
	}

	if limit <= 0 {
		limit = 20
	}

	allTasks := s.tasks[nodeKey]
	if len(allTasks) < limit {
		limit = len(allTasks)
	}

	result := make([]Task, 0, limit)
	for _, task := range allTasks[:limit] {
		result = append(result, cloneTask(task))
	}
	return result, nil
}

// ListNodes returns all registered nodes ordered by creation sequence.
func (s *MemoryStore) ListNodes() ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		result = append(result, *cloneNode(node))
	}
	return result, nil
}

// ListTasks returns tasks across all nodes ordered by creation sequence.
func (s *MemoryStore) ListTasks(query TaskQuery) ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	result := make([]Task, 0)
	for nodeKey, tasks := range s.tasks {
		if query.NodeKey != "" && nodeKey != query.NodeKey {
			continue
		}
		for _, task := range tasks {
			if query.TaskType != "" && task.TaskType != query.TaskType {
				continue
			}
			if query.Status != "" && task.Status != query.Status {
				continue
			}
			result = append(result, cloneTask(task))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID > result[j].ID
	})
	return paginateTasks(result, query.Offset, query.Limit), nil
}

// GetTask returns one task by id.
func (s *MemoryStore) GetTask(taskID uint64) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, tasks := range s.tasks {
		for _, task := range tasks {
			if task.ID == taskID {
				copyTask := cloneTask(task)
				return &copyTask, nil
			}
		}
	}
	return nil, ErrTaskNotFound
}

// ListTaskEvents returns the audit trail for one task.
func (s *MemoryStore) ListTaskEvents(taskID uint64, limit int) ([]TaskEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events, exists := s.taskEvents[taskID]
	if !exists {
		if _, err := s.getTaskLocked(taskID); err != nil {
			return nil, err
		}
		return []TaskEvent{}, nil
	}
	if limit <= 0 {
		limit = 100
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	result := make([]TaskEvent, len(events))
	for i, event := range events {
		result[i] = cloneTaskEvent(event)
	}
	return result, nil
}

// CreateTask adds a pending task for a node.
func (s *MemoryStore) CreateTask(req CreateTaskRequest) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.nodes[req.NodeKey]; !exists {
		return nil, ErrNodeNotFound
	}

	task := Task{
		ID:        s.nextTaskID,
		NodeKey:   req.NodeKey,
		TaskType:  req.TaskType,
		Payload:   clonePayload(req.Payload),
		Status:    "pending",
		Attempt:   0,
		CreatedAt: time.Now(),
	}
	s.nextTaskID++
	s.tasks[req.NodeKey] = append(s.tasks[req.NodeKey], task)
	s.recordTaskEventLocked(task.ID, task.NodeKey, "queued", "control", "task queued", map[string]interface{}{
		"taskType": task.TaskType,
	})

	copyTask := cloneTask(task)
	return &copyTask, nil
}

// RetryTask creates a new pending copy of an existing task.
func (s *MemoryStore) RetryTask(taskID uint64) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for nodeKey, tasks := range s.tasks {
		for _, task := range tasks {
			if task.ID == taskID {
				retryTask := Task{
					ID:        s.nextTaskID,
					NodeKey:   nodeKey,
					TaskType:  task.TaskType,
					Payload:   clonePayload(task.Payload),
					Status:    "pending",
					Attempt:   0,
					CreatedAt: time.Now(),
				}
				s.nextTaskID++
				s.tasks[nodeKey] = append(s.tasks[nodeKey], retryTask)
				s.recordTaskEventLocked(task.ID, nodeKey, "retry_requested", "control", "task retry requested", map[string]interface{}{
					"retryTaskId": retryTask.ID,
				})
				s.recordTaskEventLocked(retryTask.ID, nodeKey, "retried_from", "control", "task re-queued from previous task", map[string]interface{}{
					"sourceTaskId": task.ID,
					"taskType":     retryTask.TaskType,
				})
				copyTask := cloneTask(retryTask)
				return &copyTask, nil
			}
		}
	}
	return nil, ErrTaskNotFound
}

// StartTask marks a task as running.
func (s *MemoryStore) StartTask(taskID uint64, req StartTaskRequest) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, err := s.findTaskLocked(taskID, req.NodeKey)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if task.Status == "running" && task.ExecutionToken == req.ExecutionToken {
		copyTask := cloneTask(*task)
		return &copyTask, nil
	}
	if task.Status != "pending" {
		return nil, ErrTaskConflict
	}
	task.Status = "running"
	task.Attempt++
	task.ExecutionToken = req.ExecutionToken
	task.StartedAt = &now
	s.recordTaskEventLocked(task.ID, task.NodeKey, "started", "agent", "task started by node agent", nil)
	copyTask := cloneTask(*task)
	return &copyTask, nil
}

// FinishTask marks a task as finished and stores its result.
func (s *MemoryStore) FinishTask(taskID uint64, req FinishTaskRequest) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, err := s.findTaskLocked(taskID, req.NodeKey)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if task.ExecutionToken != req.ExecutionToken {
		return nil, ErrTaskConflict
	}
	if task.Status == "succeeded" || task.Status == "failed" {
		copyTask := cloneTask(*task)
		return &copyTask, nil
	}
	if task.Status != "running" {
		return nil, ErrTaskConflict
	}
	if req.Success {
		task.Status = "succeeded"
	} else {
		task.Status = "failed"
	}
	task.ResultMessage = req.Message
	task.ResultDetails = clonePayload(req.Details)
	task.FinishedAt = &now
	eventType := "failed"
	message := "task finished with failure"
	if req.Success {
		eventType = "succeeded"
		message = "task finished successfully"
	}
	s.recordTaskEventLocked(task.ID, task.NodeKey, eventType, "agent", message, clonePayload(req.Details))
	copyTask := cloneTask(*task)
	return &copyTask, nil
}

// ReportUsage stores usage snapshots for a node.
func (s *MemoryStore) ReportUsage(req UsageReportRequest) ([]UsageSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.nodes[req.NodeKey]; !exists {
		return nil, ErrNodeNotFound
	}
	now := time.Now()
	snapshots := make([]UsageSnapshot, 0, len(req.Users))
	for _, user := range req.Users {
		snapshots = append(snapshots, UsageSnapshot{
			NodeKey:    req.NodeKey,
			Username:   user.Username,
			Upload:     user.Upload,
			Download:   user.Download,
			Quota:      user.Quota,
			ExpiryDate: user.ExpiryDate,
			ReportedAt: now,
		})
	}
	s.usage[req.NodeKey] = append([]UsageSnapshot(nil), snapshots...)
	return snapshots, nil
}

func paginateTasks(tasks []Task, offset int, limit int) []Task {
	if offset >= len(tasks) {
		return []Task{}
	}
	end := offset + limit
	if end > len(tasks) {
		end = len(tasks)
	}
	return tasks[offset:end]
}

func paginateAuditLogs(logs []ControlAuditLog, offset int, limit int) []ControlAuditLog {
	if offset >= len(logs) {
		return []ControlAuditLog{}
	}
	end := offset + limit
	if end > len(logs) {
		end = len(logs)
	}
	return logs[offset:end]
}

// NodeUsage returns the latest stored usage snapshots for a node.
func (s *MemoryStore) NodeUsage(nodeKey string, limit int) ([]UsageSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.nodes[nodeKey]; !exists {
		return nil, ErrNodeNotFound
	}
	if limit <= 0 {
		limit = 100
	}
	usage := s.usage[nodeKey]
	if len(usage) > limit {
		usage = usage[:limit]
	}
	result := make([]UsageSnapshot, len(usage))
	copy(result, usage)
	return result, nil
}

// ListUsers returns all control-plane users.
func (s *MemoryStore) ListUsers() ([]ControlUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ControlUser, 0, len(s.users))
	for _, user := range s.users {
		result = append(result, *cloneControlUser(user))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Username < result[j].Username
	})
	return result, nil
}

// CreateUser creates or updates a control-plane user.
func (s *MemoryStore) CreateUser(req CreateUserRequest) (*ControlUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	user, exists := s.users[req.Username]
	if !exists {
		user = &ControlUser{
			Username:  req.Username,
			Status:    "active",
			CreatedAt: now,
		}
		s.users[req.Username] = user
	}
	user.Password = req.Password
	user.Quota = req.Quota
	user.UseDays = req.UseDays
	user.ExpiryDate = req.ExpiryDate
	user.UpdatedAt = now

	return cloneControlUser(user), nil
}

// GetUser returns one control-plane user.
func (s *MemoryStore) GetUser(username string) (*ControlUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[username]
	if !exists {
		return nil, ErrUserNotFound
	}
	return cloneControlUser(user), nil
}

// DeleteUser removes a control-plane user and all its bindings.
func (s *MemoryStore) DeleteUser(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; !exists {
		return ErrUserNotFound
	}
	delete(s.users, username)
	for nodeKey := range s.bindings {
		delete(s.bindings[nodeKey], username)
		if len(s.bindings[nodeKey]) == 0 {
			delete(s.bindings, nodeKey)
		}
	}
	return nil
}

// BindUserToNode binds an existing user to an existing node.
func (s *MemoryStore) BindUserToNode(username, nodeKey string) (*UserBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.nodes[nodeKey]; !exists {
		return nil, ErrNodeNotFound
	}
	if _, exists := s.users[username]; !exists {
		return nil, ErrUserNotFound
	}
	if _, exists := s.bindings[nodeKey]; !exists {
		s.bindings[nodeKey] = make(map[string]UserBinding)
	}
	binding, exists := s.bindings[nodeKey][username]
	if !exists {
		binding = UserBinding{
			Username:  username,
			NodeKey:   nodeKey,
			CreatedAt: time.Now(),
		}
		s.bindings[nodeKey][username] = binding
	}
	copyBinding := binding
	return &copyBinding, nil
}

// UnbindUserFromNode removes a user-node binding.
func (s *MemoryStore) UnbindUserFromNode(username, nodeKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.nodes[nodeKey]; !exists {
		return ErrNodeNotFound
	}
	if _, exists := s.users[username]; !exists {
		return ErrUserNotFound
	}
	bindings, exists := s.bindings[nodeKey]
	if !exists {
		return ErrBindingNotFound
	}
	if _, exists := bindings[username]; !exists {
		return ErrBindingNotFound
	}
	delete(bindings, username)
	if len(bindings) == 0 {
		delete(s.bindings, nodeKey)
	}
	return nil
}

// NodeUsers returns control-plane users bound to a node.
func (s *MemoryStore) NodeUsers(nodeKey string) ([]ControlUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.nodes[nodeKey]; !exists {
		return nil, ErrNodeNotFound
	}
	bindings := s.bindings[nodeKey]
	users := make([]ControlUser, 0, len(bindings))
	for username := range bindings {
		if user, ok := s.users[username]; ok {
			users = append(users, *cloneControlUser(user))
		}
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})
	return users, nil
}

// UserNodes returns nodes bound to a control-plane user.
func (s *MemoryStore) UserNodes(username string) ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.users[username]; !exists {
		return nil, ErrUserNotFound
	}
	var nodes []Node
	for nodeKey, bindings := range s.bindings {
		if _, ok := bindings[username]; ok {
			if node, exists := s.nodes[nodeKey]; exists {
				nodes = append(nodes, *cloneNode(node))
			}
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeKey < nodes[j].NodeKey
	})
	return nodes, nil
}

// UserUsage returns usage snapshots for a user across nodes.
func (s *MemoryStore) UserUsage(username string, limit int) ([]UsageSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.users[username]; !exists {
		return nil, ErrUserNotFound
	}
	if limit <= 0 {
		limit = 100
	}
	var result []UsageSnapshot
	for _, usageList := range s.usage {
		for _, usage := range usageList {
			if usage.Username == username {
				result = append(result, usage)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ReportedAt.After(result[j].ReportedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// SyncNodeUsers generates a sync_users task from current bindings.
func (s *MemoryStore) SyncNodeUsers(nodeKey string) (*Task, error) {
	users, err := s.NodeUsers(nodeKey)
	if err != nil {
		return nil, err
	}
	payloadUsers := make([]interface{}, 0, len(users))
	for _, user := range users {
		payloadUsers = append(payloadUsers, map[string]interface{}{
			"username":   user.Username,
			"password":   user.Password,
			"quota":      user.Quota,
			"useDays":    user.UseDays,
			"expiryDate": user.ExpiryDate,
		})
	}
	return s.CreateTask(CreateTaskRequest{
		NodeKey:  nodeKey,
		TaskType: "sync_users",
		Payload: map[string]interface{}{
			"replace": true,
			"users":   payloadUsers,
		},
	})
}

// AddTask seeds a pending task for a node. It is mainly used by tests and demos.
func (s *MemoryStore) AddTask(nodeKey string, taskType string, payload map[string]interface{}) (Task, error) {
	task, err := s.CreateTask(CreateTaskRequest{
		NodeKey:  nodeKey,
		TaskType: taskType,
		Payload:  payload,
	})
	if err != nil {
		return Task{}, err
	}
	return *task, nil
}

func cloneNode(node *Node) *Node {
	if node == nil {
		return nil
	}
	copyNode := *node
	copyNode.Tags = cloneTags(node.Tags)
	if node.LastHeartbeat != nil {
		heartbeat := *node.LastHeartbeat
		heartbeat.Payload = clonePayload(node.LastHeartbeat.Payload)
		copyNode.LastHeartbeat = &heartbeat
	}
	return &copyNode
}

func cloneTask(task Task) Task {
	task.Payload = clonePayload(task.Payload)
	task.ResultDetails = clonePayload(task.ResultDetails)
	return task
}

func cloneTaskEvent(event TaskEvent) TaskEvent {
	event.Details = clonePayload(event.Details)
	return event
}

func cloneAuditLog(log ControlAuditLog) ControlAuditLog {
	log.Details = clonePayload(log.Details)
	return log
}

func cloneTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}
	result := make([]string, len(tags))
	copy(result, tags)
	return result
}

func clonePayload(payload map[string]interface{}) map[string]interface{} {
	if len(payload) == 0 {
		return map[string]interface{}{}
	}
	result := make(map[string]interface{}, len(payload))
	for k, v := range payload {
		result[k] = v
	}
	return result
}

func cloneControlUser(user *ControlUser) *ControlUser {
	if user == nil {
		return nil
	}
	copyUser := *user
	return &copyUser
}

func cloneControlAdmin(admin *ControlAdmin) *ControlAdmin {
	if admin == nil {
		return nil
	}
	copyAdmin := *admin
	return &copyAdmin
}

func (s *MemoryStore) findTaskLocked(taskID uint64, nodeKey string) (*Task, error) {
	if _, exists := s.nodes[nodeKey]; !exists {
		return nil, ErrNodeNotFound
	}
	tasks := s.tasks[nodeKey]
	for i := range tasks {
		if tasks[i].ID == taskID {
			return &s.tasks[nodeKey][i], nil
		}
	}
	return nil, ErrTaskNotFound
}

func (s *MemoryStore) getTaskLocked(taskID uint64) (*Task, error) {
	for nodeKey, tasks := range s.tasks {
		for i := range tasks {
			if tasks[i].ID == taskID {
				return &s.tasks[nodeKey][i], nil
			}
		}
	}
	return nil, ErrTaskNotFound
}

func (s *MemoryStore) recordTaskEventLocked(taskID uint64, nodeKey string, eventType string, actor string, message string, details map[string]interface{}) {
	event := TaskEvent{
		ID:        s.nextTaskEventID,
		TaskID:    taskID,
		NodeKey:   nodeKey,
		EventType: eventType,
		Actor:     actor,
		Message:   message,
		Details:   clonePayload(details),
		CreatedAt: time.Now(),
	}
	s.nextTaskEventID++
	s.taskEvents[taskID] = append(s.taskEvents[taskID], event)
}
