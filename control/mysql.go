package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const controlMigrationTable = "control_schema_migrations"

type controlMigration struct {
	Version int
	Name    string
	Apply   func(tx *sql.Tx) error
}

var controlMigrations = []controlMigration{
	{
		Version: 1,
		Name:    "create-control-core-tables",
		Apply: func(tx *sql.Tx) error {
			return execMigrationStatements(tx,
				`CREATE TABLE IF NOT EXISTS nodes (
					id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
					node_key VARCHAR(64) NOT NULL,
					name VARCHAR(64) NOT NULL,
					region VARCHAR(32) NOT NULL DEFAULT '',
					provider VARCHAR(32) NOT NULL DEFAULT '',
					endpoint VARCHAR(255) NOT NULL DEFAULT '',
					port INT NOT NULL DEFAULT 443,
					trojan_type VARCHAR(16) NOT NULL DEFAULT 'trojan',
					trojan_version VARCHAR(32) NOT NULL DEFAULT '',
					manager_version VARCHAR(32) NOT NULL DEFAULT '',
					public_ip VARCHAR(64) NOT NULL DEFAULT '',
					domain_name VARCHAR(255) NOT NULL DEFAULT '',
					tags JSON DEFAULT NULL,
					status VARCHAR(16) NOT NULL DEFAULT 'active',
					last_seen_at DATETIME DEFAULT NULL,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
					PRIMARY KEY (id),
					UNIQUE KEY uk_nodes_node_key (node_key),
					UNIQUE KEY uk_nodes_name (name),
					KEY idx_nodes_region_status (region, status)
				) DEFAULT CHARSET=utf8mb4`,
				`CREATE TABLE IF NOT EXISTS control_users (
					id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
					username VARCHAR(64) NOT NULL,
					password_show VARCHAR(255) NOT NULL,
					quota BIGINT NOT NULL DEFAULT 0,
					use_days INT UNSIGNED NOT NULL DEFAULT 0,
					expiry_date VARCHAR(16) NOT NULL DEFAULT '',
					status VARCHAR(16) NOT NULL DEFAULT 'active',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
					PRIMARY KEY (id),
					UNIQUE KEY uk_control_users_username (username)
				) DEFAULT CHARSET=utf8mb4`,
				`CREATE TABLE IF NOT EXISTS user_node_bindings (
					id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
					user_id BIGINT UNSIGNED NOT NULL,
					node_id BIGINT UNSIGNED NOT NULL,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id),
					UNIQUE KEY uk_user_node_binding (user_id, node_id),
					KEY idx_user_node_bindings_node_id (node_id),
					CONSTRAINT fk_user_node_bindings_user_id FOREIGN KEY (user_id) REFERENCES control_users(id),
					CONSTRAINT fk_user_node_bindings_node_id FOREIGN KEY (node_id) REFERENCES nodes(id)
				) DEFAULT CHARSET=utf8mb4`,
				`CREATE TABLE IF NOT EXISTS tasks (
					id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
					node_id BIGINT UNSIGNED NOT NULL,
					task_type VARCHAR(32) NOT NULL,
					payload JSON NOT NULL,
					status VARCHAR(16) NOT NULL DEFAULT 'pending',
					result_message VARCHAR(255) NOT NULL DEFAULT '',
					result_details JSON DEFAULT NULL,
					started_at DATETIME DEFAULT NULL,
					finished_at DATETIME DEFAULT NULL,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
					PRIMARY KEY (id),
					KEY idx_tasks_node_status (node_id, status),
					CONSTRAINT fk_tasks_node_id FOREIGN KEY (node_id) REFERENCES nodes(id)
				) DEFAULT CHARSET=utf8mb4`,
				`CREATE TABLE IF NOT EXISTS node_heartbeats (
					id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
					node_id BIGINT UNSIGNED NOT NULL,
					cpu_percent DOUBLE NOT NULL DEFAULT 0,
					memory_percent DOUBLE NOT NULL DEFAULT 0,
					disk_percent DOUBLE NOT NULL DEFAULT 0,
					tcp_count INT NOT NULL DEFAULT 0,
					udp_count INT NOT NULL DEFAULT 0,
					upload_speed BIGINT UNSIGNED NOT NULL DEFAULT 0,
					download_speed BIGINT UNSIGNED NOT NULL DEFAULT 0,
					payload JSON DEFAULT NULL,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id),
					KEY idx_node_heartbeats_node_created (node_id, created_at),
					CONSTRAINT fk_node_heartbeats_node_id FOREIGN KEY (node_id) REFERENCES nodes(id)
				) DEFAULT CHARSET=utf8mb4`,
				`CREATE TABLE IF NOT EXISTS usage_reports (
					id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
					node_id BIGINT UNSIGNED NOT NULL,
					username VARCHAR(64) NOT NULL,
					upload BIGINT UNSIGNED NOT NULL DEFAULT 0,
					download BIGINT UNSIGNED NOT NULL DEFAULT 0,
					quota BIGINT NOT NULL DEFAULT -1,
					expiry_date VARCHAR(16) NOT NULL DEFAULT '',
					reported_at DATETIME NOT NULL,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id),
					KEY idx_usage_reports_node_created (node_id, created_at),
					KEY idx_usage_reports_node_user (node_id, username),
					CONSTRAINT fk_usage_reports_node_id FOREIGN KEY (node_id) REFERENCES nodes(id)
				) DEFAULT CHARSET=utf8mb4`,
			)
		},
	},
	{
		Version: 2,
		Name:    "ensure-node-port-column",
		Apply: func(tx *sql.Tx) error {
			exists, err := columnExists(tx, "nodes", "port")
			if err != nil {
				return err
			}
			if exists {
				return nil
			}
			_, err = tx.Exec(`ALTER TABLE nodes ADD COLUMN port INT NOT NULL DEFAULT 443 AFTER endpoint`)
			return err
		},
	},
	{
		Version: 3,
		Name:    "create-task-events",
		Apply: func(tx *sql.Tx) error {
			return execMigrationStatements(tx,
				`CREATE TABLE IF NOT EXISTS task_events (
					id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
					task_id BIGINT UNSIGNED NOT NULL,
					node_id BIGINT UNSIGNED NOT NULL,
					event_type VARCHAR(32) NOT NULL,
					actor VARCHAR(16) NOT NULL DEFAULT '',
					message VARCHAR(255) NOT NULL DEFAULT '',
					details JSON DEFAULT NULL,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id),
					KEY idx_task_events_task_created (task_id, created_at),
					CONSTRAINT fk_task_events_task_id FOREIGN KEY (task_id) REFERENCES tasks(id),
					CONSTRAINT fk_task_events_node_id FOREIGN KEY (node_id) REFERENCES nodes(id)
				) DEFAULT CHARSET=utf8mb4`,
			)
		},
	},
	{
		Version: 4,
		Name:    "create-control-admins",
		Apply: func(tx *sql.Tx) error {
			return execMigrationStatements(tx,
				`CREATE TABLE IF NOT EXISTS control_admins (
					id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
					username VARCHAR(64) NOT NULL,
					password_hash VARCHAR(255) NOT NULL,
					role VARCHAR(32) NOT NULL DEFAULT 'super_admin',
					status VARCHAR(16) NOT NULL DEFAULT 'active',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
					PRIMARY KEY (id),
					UNIQUE KEY uk_control_admins_username (username)
				) DEFAULT CHARSET=utf8mb4`,
			)
		},
	},
	{
		Version: 5,
		Name:    "create-control-audit-logs",
		Apply: func(tx *sql.Tx) error {
			return execMigrationStatements(tx,
				`CREATE TABLE IF NOT EXISTS control_audit_logs (
					id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
					actor VARCHAR(64) NOT NULL DEFAULT '',
					actor_role VARCHAR(32) NOT NULL DEFAULT '',
					action VARCHAR(64) NOT NULL,
					resource_type VARCHAR(64) NOT NULL DEFAULT '',
					resource_id VARCHAR(128) NOT NULL DEFAULT '',
					message VARCHAR(255) NOT NULL DEFAULT '',
					details JSON DEFAULT NULL,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id),
					KEY idx_control_audit_logs_actor_created (actor, created_at),
					KEY idx_control_audit_logs_action_created (action, created_at),
					KEY idx_control_audit_logs_resource_created (resource_type, created_at)
				) DEFAULT CHARSET=utf8mb4`,
			)
		},
	},
	{
		Version: 6,
		Name:    "add-node-agent-auth-columns",
		Apply: func(tx *sql.Tx) error {
			exists, err := columnExists(tx, "nodes", "agent_secret_hash")
			if err != nil {
				return err
			}
			if !exists {
				if _, err := tx.Exec(`ALTER TABLE nodes ADD COLUMN agent_secret_hash VARCHAR(128) NOT NULL DEFAULT '' AFTER tags`); err != nil {
					return err
				}
			}
			exists, err = columnExists(tx, "nodes", "agent_auth_mode")
			if err != nil {
				return err
			}
			if !exists {
				if _, err := tx.Exec(`ALTER TABLE nodes ADD COLUMN agent_auth_mode VARCHAR(16) NOT NULL DEFAULT 'token' AFTER agent_secret_hash`); err != nil {
					return err
				}
			}
			_, err = tx.Exec(`UPDATE nodes SET agent_auth_mode = IF(agent_secret_hash <> '', 'secret', 'token')`)
			return err
		},
	},
	{
		Version: 7,
		Name:    "add-task-execution-columns",
		Apply: func(tx *sql.Tx) error {
			exists, err := columnExists(tx, "tasks", "attempt_count")
			if err != nil {
				return err
			}
			if !exists {
				if _, err := tx.Exec(`ALTER TABLE tasks ADD COLUMN attempt_count INT NOT NULL DEFAULT 0 AFTER status`); err != nil {
					return err
				}
			}
			exists, err = columnExists(tx, "tasks", "execution_token")
			if err != nil {
				return err
			}
			if !exists {
				if _, err := tx.Exec(`ALTER TABLE tasks ADD COLUMN execution_token VARCHAR(64) NOT NULL DEFAULT '' AFTER attempt_count`); err != nil {
					return err
				}
			}
			return nil
		},
	},
}

// RuntimeStatus reports the current SQL-backed control-plane status.
func (s *MySQLStore) RuntimeStatus() (*RuntimeStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	status := &RuntimeStatus{
		Backend:   "mysql",
		Healthy:   false,
		CheckedAt: time.Now().UTC(),
		Details:   map[string]interface{}{},
	}
	if err := s.db.PingContext(ctx); err != nil {
		status.Details["error"] = err.Error()
		return status, err
	}

	stats := s.db.Stats()
	status.Healthy = true
	status.Details["openConnections"] = stats.OpenConnections
	status.Details["inUse"] = stats.InUse
	status.Details["idle"] = stats.Idle
	status.Details["waitCount"] = stats.WaitCount
	status.Details["waitDurationMs"] = stats.WaitDuration.Milliseconds()
	status.Details["maxIdleClosed"] = stats.MaxIdleClosed
	status.Details["maxIdleTimeClosed"] = stats.MaxIdleTimeClosed
	status.Details["maxLifetimeClosed"] = stats.MaxLifetimeClosed

	applied, err := loadAppliedControlMigrations(s.db)
	if err != nil {
		status.Details["migrationError"] = err.Error()
		return status, nil
	}
	status.Details["appliedMigrationCount"] = len(applied)
	status.Details["expectedMigrationCount"] = len(controlMigrations)
	status.Details["pendingMigrationCount"] = len(pendingControlMigrationVersions(applied))
	return status, nil
}

// MetricsSnapshot reports current SQL-backed metrics for scraping.
func (s *MySQLStore) MetricsSnapshot() (*MetricsSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	snapshot := &MetricsSnapshot{
		Backend:   "mysql",
		Healthy:   false,
		CheckedAt: time.Now().UTC(),
	}
	if err := s.db.PingContext(ctx); err != nil {
		return snapshot, err
	}
	snapshot.Healthy = true

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes`).Scan(&snapshot.NodeCount); err != nil {
		return snapshot, err
	}
	activeCutoff := time.Now().Add(-5 * time.Minute)
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE last_seen_at IS NOT NULL AND last_seen_at >= ?`, activeCutoff).Scan(&snapshot.ActiveNodeCount); err != nil {
		return snapshot, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM control_users`).Scan(&snapshot.UserCount); err != nil {
		return snapshot, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM control_admins`).Scan(&snapshot.AdminCount); err != nil {
		return snapshot, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&snapshot.TaskCount); err != nil {
		return snapshot, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM tasks GROUP BY status`)
	if err != nil {
		return snapshot, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			status string
			count  int64
		)
		if err := rows.Scan(&status, &count); err != nil {
			return snapshot, err
		}
		switch status {
		case "pending":
			snapshot.TaskPendingCount = count
		case "running":
			snapshot.TaskRunningCount = count
		case "succeeded":
			snapshot.TaskSucceededCount = count
		case "failed":
			snapshot.TaskFailedCount = count
		}
	}
	if err := rows.Err(); err != nil {
		return snapshot, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_events`).Scan(&snapshot.TaskEventCount); err != nil {
		return snapshot, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM control_audit_logs`).Scan(&snapshot.AuditLogCount); err != nil {
		return snapshot, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_reports`).Scan(&snapshot.UsageCount); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

// MySQLStore persists control-plane state in MySQL.
type MySQLStore struct {
	db *sql.DB
}

// NewMySQLStore creates a MySQL-backed control-plane store and initializes the minimum schema.
func NewMySQLStore(dsn string) (*MySQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := initMySQLSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &MySQLStore{db: db}, nil
}

func initMySQLSchema(db *sql.DB) error {
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS control_schema_migrations (
	version INT NOT NULL,
	name VARCHAR(128) NOT NULL,
	applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (version)
) DEFAULT CHARSET=utf8mb4
`); err != nil {
		return err
	}

	applied, err := loadAppliedControlMigrations(db)
	if err != nil {
		return err
	}

	for _, migration := range controlMigrations {
		if applied[migration.Version] {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if err := migration.Apply(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply control migration %d (%s): %w", migration.Version, migration.Name, err)
		}
		if _, err := tx.Exec(`
INSERT INTO control_schema_migrations(version, name, applied_at)
VALUES (?, ?, ?)
`, migration.Version, migration.Name, time.Now()); err != nil {
			tx.Rollback()
			return fmt.Errorf("record control migration %d (%s): %w", migration.Version, migration.Name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

// Close releases the MySQL connection pool.
func (s *MySQLStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// EnsureControlAdmin creates the bootstrap admin if it does not already exist.
func (s *MySQLStore) EnsureControlAdmin(req EnsureControlAdminRequest) (*ControlAdmin, error) {
	admin, err := s.findControlAdmin(req.Username)
	if err == nil {
		return admin, nil
	}
	if !errors.Is(err, ErrAdminNotFound) {
		return nil, err
	}
	passwordHash, err := hashControlPassword(req.Password)
	if err != nil {
		return nil, err
	}
	role := req.Role
	if role == "" {
		role = "super_admin"
	}
	_, err = s.db.Exec(`
INSERT INTO control_admins(username, password_hash, role, status)
VALUES (?, ?, ?, 'active')
`, req.Username, passwordHash, role)
	if err != nil {
		return nil, err
	}
	return s.findControlAdmin(req.Username)
}

// GetControlAdmin returns one control-plane administrator.
func (s *MySQLStore) GetControlAdmin(username string) (*ControlAdmin, error) {
	return s.findControlAdmin(username)
}

// AuthenticateControlAdmin validates control-plane admin credentials.
func (s *MySQLStore) AuthenticateControlAdmin(username string, password string) (*ControlAdmin, error) {
	admin, err := s.findControlAdmin(username)
	if err != nil {
		return nil, err
	}
	if admin.Status != "" && admin.Status != "active" {
		return nil, ErrInvalidCredentials
	}
	if err := checkControlPassword(admin.PasswordHash, password); err != nil {
		return nil, ErrInvalidCredentials
	}
	return admin, nil
}

// ListControlAdmins returns all control-plane administrators.
func (s *MySQLStore) ListControlAdmins() ([]ControlAdmin, error) {
	rows, err := s.db.Query(`
SELECT username, password_hash, role, status, created_at, updated_at
FROM control_admins
ORDER BY FIELD(role, 'super_admin', 'admin', 'viewer') ASC, username ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var admins []ControlAdmin
	for rows.Next() {
		var admin ControlAdmin
		if err := rows.Scan(&admin.Username, &admin.PasswordHash, &admin.Role, &admin.Status, &admin.CreatedAt, &admin.UpdatedAt); err != nil {
			return nil, err
		}
		admins = append(admins, admin)
	}
	return admins, rows.Err()
}

// UpdateControlAdmin updates an existing control-plane administrator.
func (s *MySQLStore) UpdateControlAdmin(username string, req UpdateControlAdminRequest) (*ControlAdmin, error) {
	admin, err := s.findControlAdmin(username)
	if err != nil {
		return nil, err
	}
	passwordHash := admin.PasswordHash
	if req.Password != "" {
		passwordHash, err = hashControlPassword(req.Password)
		if err != nil {
			return nil, err
		}
	}
	role := admin.Role
	if req.Role != "" {
		role = req.Role
	}
	status := admin.Status
	if req.Status != "" {
		status = req.Status
	}
	result, err := s.db.Exec(`
UPDATE control_admins
SET password_hash = ?, role = ?, status = ?, updated_at = ?
WHERE username = ?
`, passwordHash, role, status, time.Now(), username)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrAdminNotFound
	}
	return s.findControlAdmin(username)
}

// CleanupHistory prunes historical audit, task, and usage records by retention window.
func (s *MySQLStore) CleanupHistory(req CleanupRequest) (*CleanupResult, error) {
	result := &CleanupResult{
		CompletedAt: time.Now(),
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if req.AuditRetentionDays > 0 {
		cutoff := result.CompletedAt.Add(-time.Duration(req.AuditRetentionDays) * 24 * time.Hour)
		res, execErr := tx.Exec(`DELETE FROM control_audit_logs WHERE created_at < ?`, cutoff)
		if execErr != nil {
			err = execErr
			return nil, err
		}
		deleted, execErr := res.RowsAffected()
		if execErr != nil {
			err = execErr
			return nil, err
		}
		result.AuditLogsDeleted = deleted
	}

	if req.TaskRetentionDays > 0 {
		cutoff := result.CompletedAt.Add(-time.Duration(req.TaskRetentionDays) * 24 * time.Hour)
		res, execErr := tx.Exec(`
DELETE e
FROM task_events e
JOIN tasks t ON t.id = e.task_id
WHERE t.status IN ('succeeded', 'failed') AND COALESCE(t.finished_at, t.created_at) < ?
`, cutoff)
		if execErr != nil {
			err = execErr
			return nil, err
		}
		deletedEvents, execErr := res.RowsAffected()
		if execErr != nil {
			err = execErr
			return nil, err
		}
		result.TaskEventsDeleted = deletedEvents

		res, execErr = tx.Exec(`
DELETE FROM tasks
WHERE status IN ('succeeded', 'failed') AND COALESCE(finished_at, created_at) < ?
`, cutoff)
		if execErr != nil {
			err = execErr
			return nil, err
		}
		deletedTasks, execErr := res.RowsAffected()
		if execErr != nil {
			err = execErr
			return nil, err
		}
		result.TasksDeleted = deletedTasks
	}

	if req.UsageRetentionDays > 0 {
		cutoff := result.CompletedAt.Add(-time.Duration(req.UsageRetentionDays) * 24 * time.Hour)
		res, execErr := tx.Exec(`DELETE FROM usage_reports WHERE reported_at < ?`, cutoff)
		if execErr != nil {
			err = execErr
			return nil, err
		}
		deleted, execErr := res.RowsAffected()
		if execErr != nil {
			err = execErr
			return nil, err
		}
		result.UsageReportsDeleted = deleted
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// ExportBackup returns a full SQL-backed snapshot of the control-plane state.
func (s *MySQLStore) ExportBackup() (*BackupBundle, error) {
	bundle := &BackupBundle{
		Version:    "control.v1",
		ExportedAt: time.Now(),
	}

	adminRows, err := s.db.Query(`
SELECT username, password_hash, role, status, created_at, updated_at
FROM control_admins
ORDER BY username ASC
`)
	if err != nil {
		return nil, err
	}
	defer adminRows.Close()
	for adminRows.Next() {
		var admin BackupAdmin
		if err := adminRows.Scan(&admin.Username, &admin.PasswordHash, &admin.Role, &admin.Status, &admin.CreatedAt, &admin.UpdatedAt); err != nil {
			return nil, err
		}
		bundle.Admins = append(bundle.Admins, admin)
	}
	if err := adminRows.Err(); err != nil {
		return nil, err
	}

	nodeRows, err := s.db.Query(`
SELECT id, node_key, name, region, provider, endpoint, port, trojan_type, trojan_version, manager_version, public_ip, domain_name, tags, agent_secret_hash, agent_auth_mode, status, last_seen_at, created_at, updated_at
FROM nodes
ORDER BY id ASC
`)
	if err != nil {
		return nil, err
	}
	defer nodeRows.Close()
	for nodeRows.Next() {
		var (
			node            Node
			tagsJSON        []byte
			agentSecretHash string
			lastSeenAt      sql.NullTime
		)
		if err := nodeRows.Scan(&node.ID, &node.NodeKey, &node.Name, &node.Region, &node.Provider, &node.Endpoint, &node.Port, &node.TrojanType, &node.TrojanVersion, &node.ManagerVersion, &node.PublicIP, &node.DomainName, &tagsJSON, &agentSecretHash, &node.AgentAuthMode, &node.Status, &lastSeenAt, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, err
		}
		node.Tags = decodeStringSlice(tagsJSON)
		if lastSeenAt.Valid {
			node.LastSeenAt = lastSeenAt.Time
		}
		heartbeat, err := s.lastHeartbeat(node.ID)
		if err != nil {
			return nil, err
		}
		node.LastHeartbeat = heartbeat
		bundle.Nodes = append(bundle.Nodes, BackupNode{
			Node:            node,
			AgentSecretHash: agentSecretHash,
		})
	}
	if err := nodeRows.Err(); err != nil {
		return nil, err
	}

	userRows, err := s.db.Query(`
SELECT username, password_show, quota, use_days, expiry_date, status, created_at, updated_at
FROM control_users
ORDER BY username ASC
`)
	if err != nil {
		return nil, err
	}
	defer userRows.Close()
	for userRows.Next() {
		var user ControlUser
		if err := userRows.Scan(&user.Username, &user.Password, &user.Quota, &user.UseDays, &user.ExpiryDate, &user.Status, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		bundle.Users = append(bundle.Users, user)
	}
	if err := userRows.Err(); err != nil {
		return nil, err
	}

	bindingRows, err := s.db.Query(`
SELECT u.username, n.node_key, b.created_at
FROM user_node_bindings b
JOIN control_users u ON u.id = b.user_id
JOIN nodes n ON n.id = b.node_id
ORDER BY u.username ASC, n.node_key ASC
`)
	if err != nil {
		return nil, err
	}
	defer bindingRows.Close()
	for bindingRows.Next() {
		var binding UserBinding
		if err := bindingRows.Scan(&binding.Username, &binding.NodeKey, &binding.CreatedAt); err != nil {
			return nil, err
		}
		bundle.Bindings = append(bundle.Bindings, binding)
	}
	if err := bindingRows.Err(); err != nil {
		return nil, err
	}

	taskRows, err := s.db.Query(`
SELECT t.id, n.node_key, t.task_type, t.payload, t.status, t.attempt_count, t.execution_token, t.result_message, t.result_details, t.created_at, t.started_at, t.finished_at
FROM tasks t
JOIN nodes n ON n.id = t.node_id
ORDER BY t.id ASC
`)
	if err != nil {
		return nil, err
	}
	defer taskRows.Close()
	for taskRows.Next() {
		var (
			task              Task
			payloadJSON       []byte
			executionToken    string
			resultDetailsJSON []byte
			startedAt         sql.NullTime
			finishedAt        sql.NullTime
		)
		if err := taskRows.Scan(&task.ID, &task.NodeKey, &task.TaskType, &payloadJSON, &task.Status, &task.Attempt, &executionToken, &task.ResultMessage, &resultDetailsJSON, &task.CreatedAt, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		task.Payload = decodePayload(payloadJSON)
		task.ExecutionToken = executionToken
		task.ResultDetails = decodePayload(resultDetailsJSON)
		if startedAt.Valid {
			task.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			task.FinishedAt = &finishedAt.Time
		}
		bundle.Tasks = append(bundle.Tasks, task)
	}
	if err := taskRows.Err(); err != nil {
		return nil, err
	}

	eventRows, err := s.db.Query(`
SELECT e.id, e.task_id, n.node_key, e.event_type, e.actor, e.message, e.details, e.created_at
FROM task_events e
JOIN nodes n ON n.id = e.node_id
ORDER BY e.id ASC
`)
	if err != nil {
		return nil, err
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var (
			event       TaskEvent
			detailsJSON []byte
		)
		if err := eventRows.Scan(&event.ID, &event.TaskID, &event.NodeKey, &event.EventType, &event.Actor, &event.Message, &detailsJSON, &event.CreatedAt); err != nil {
			return nil, err
		}
		event.Details = decodePayload(detailsJSON)
		bundle.TaskEvents = append(bundle.TaskEvents, event)
	}
	if err := eventRows.Err(); err != nil {
		return nil, err
	}

	auditRows, err := s.db.Query(`
SELECT id, actor, actor_role, action, resource_type, resource_id, message, details, created_at
FROM control_audit_logs
ORDER BY id ASC
`)
	if err != nil {
		return nil, err
	}
	defer auditRows.Close()
	for auditRows.Next() {
		var (
			log         ControlAuditLog
			detailsJSON []byte
		)
		if err := auditRows.Scan(&log.ID, &log.Actor, &log.ActorRole, &log.Action, &log.ResourceType, &log.ResourceID, &log.Message, &detailsJSON, &log.CreatedAt); err != nil {
			return nil, err
		}
		log.Details = decodePayload(detailsJSON)
		bundle.AuditLogs = append(bundle.AuditLogs, log)
	}
	if err := auditRows.Err(); err != nil {
		return nil, err
	}

	usageRows, err := s.db.Query(`
SELECT n.node_key, u.username, u.upload, u.download, u.quota, u.expiry_date, u.reported_at
FROM usage_reports u
JOIN nodes n ON n.id = u.node_id
ORDER BY u.reported_at ASC, n.node_key ASC, u.username ASC
`)
	if err != nil {
		return nil, err
	}
	defer usageRows.Close()
	for usageRows.Next() {
		var snapshot UsageSnapshot
		if err := usageRows.Scan(&snapshot.NodeKey, &snapshot.Username, &snapshot.Upload, &snapshot.Download, &snapshot.Quota, &snapshot.ExpiryDate, &snapshot.ReportedAt); err != nil {
			return nil, err
		}
		bundle.Usage = append(bundle.Usage, snapshot)
	}
	if err := usageRows.Err(); err != nil {
		return nil, err
	}

	return bundle, nil
}

// ImportBackup replaces the SQL-backed control-plane state with the provided snapshot.
func (s *MySQLStore) ImportBackup(bundle BackupBundle) error {
	if err := validateBackupBundle(bundle); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err = execMigrationStatements(tx,
		`DELETE FROM task_events`,
		`DELETE FROM usage_reports`,
		`DELETE FROM node_heartbeats`,
		`DELETE FROM tasks`,
		`DELETE FROM user_node_bindings`,
		`DELETE FROM control_audit_logs`,
		`DELETE FROM control_users`,
		`DELETE FROM control_admins`,
		`DELETE FROM nodes`,
	); err != nil {
		return err
	}

	nodeIDMap := make(map[string]uint64, len(bundle.Nodes))
	for _, backupNode := range bundle.Nodes {
		tags, err := encodeJSON(backupNode.Node.Tags)
		if err != nil {
			return err
		}
		lastSeen := nullableTime(backupNode.Node.LastSeenAt)
		if _, err = tx.Exec(`
INSERT INTO nodes(id, node_key, name, region, provider, endpoint, port, trojan_type, trojan_version, manager_version, public_ip, domain_name, tags, agent_secret_hash, agent_auth_mode, status, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, backupNode.Node.ID, backupNode.Node.NodeKey, backupNode.Node.Name, backupNode.Node.Region, backupNode.Node.Provider, backupNode.Node.Endpoint, backupNode.Node.Port, backupNode.Node.TrojanType, backupNode.Node.TrojanVersion, backupNode.Node.ManagerVersion, backupNode.Node.PublicIP, backupNode.Node.DomainName, tags, backupNode.AgentSecretHash, backupNode.Node.AgentAuthMode, backupNode.Node.Status, lastSeen, backupNode.Node.CreatedAt, backupNode.Node.UpdatedAt); err != nil {
			return err
		}
		nodeIDMap[backupNode.Node.NodeKey] = backupNode.Node.ID
		if backupNode.Node.LastHeartbeat != nil {
			payload, err := encodeJSON(backupNode.Node.LastHeartbeat.Payload)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(`
INSERT INTO node_heartbeats(node_id, cpu_percent, memory_percent, disk_percent, tcp_count, udp_count, upload_speed, download_speed, payload, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, backupNode.Node.ID, backupNode.Node.LastHeartbeat.CPUPercent, backupNode.Node.LastHeartbeat.MemoryPercent, backupNode.Node.LastHeartbeat.DiskPercent, backupNode.Node.LastHeartbeat.TCPCount, backupNode.Node.LastHeartbeat.UDPCount, backupNode.Node.LastHeartbeat.UploadSpeed, backupNode.Node.LastHeartbeat.DownloadSpeed, payload, backupNode.Node.LastHeartbeat.ReportedAt); err != nil {
				return err
			}
		}
	}

	for _, admin := range bundle.Admins {
		if _, err = tx.Exec(`
INSERT INTO control_admins(username, password_hash, role, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`, admin.Username, admin.PasswordHash, admin.Role, admin.Status, admin.CreatedAt, admin.UpdatedAt); err != nil {
			return err
		}
	}

	userIDMap := make(map[string]uint64, len(bundle.Users))
	for _, user := range bundle.Users {
		res, err := tx.Exec(`
INSERT INTO control_users(username, password_show, quota, use_days, expiry_date, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, user.Username, user.Password, user.Quota, user.UseDays, user.ExpiryDate, user.Status, user.CreatedAt, user.UpdatedAt)
		if err != nil {
			return err
		}
		lastID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		userIDMap[user.Username] = uint64(lastID)
	}

	for _, binding := range bundle.Bindings {
		userID, ok := userIDMap[binding.Username]
		if !ok {
			return ErrUserNotFound
		}
		nodeID, ok := nodeIDMap[binding.NodeKey]
		if !ok {
			return ErrNodeNotFound
		}
		if _, err = tx.Exec(`
INSERT INTO user_node_bindings(user_id, node_id, created_at)
VALUES (?, ?, ?)
`, userID, nodeID, binding.CreatedAt); err != nil {
			return err
		}
	}

	for _, task := range bundle.Tasks {
		nodeID, ok := nodeIDMap[task.NodeKey]
		if !ok {
			return ErrNodeNotFound
		}
		payload, err := encodeJSON(task.Payload)
		if err != nil {
			return err
		}
		resultDetails, err := encodeJSON(task.ResultDetails)
		if err != nil {
			return err
		}
		updatedAt := task.CreatedAt
		if task.FinishedAt != nil {
			updatedAt = *task.FinishedAt
		} else if task.StartedAt != nil {
			updatedAt = *task.StartedAt
		}
		if _, err = tx.Exec(`
INSERT INTO tasks(id, node_id, task_type, payload, status, attempt_count, execution_token, result_message, result_details, started_at, finished_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, task.ID, nodeID, task.TaskType, payload, task.Status, task.Attempt, task.ExecutionToken, task.ResultMessage, resultDetails, nullableTimePtr(task.StartedAt), nullableTimePtr(task.FinishedAt), task.CreatedAt, updatedAt); err != nil {
			return err
		}
	}

	for _, event := range bundle.TaskEvents {
		nodeID, ok := nodeIDMap[event.NodeKey]
		if !ok {
			return ErrNodeNotFound
		}
		details, err := encodeJSON(event.Details)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`
INSERT INTO task_events(id, task_id, node_id, event_type, actor, message, details, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, event.ID, event.TaskID, nodeID, event.EventType, event.Actor, event.Message, details, event.CreatedAt); err != nil {
			return err
		}
	}

	for _, log := range bundle.AuditLogs {
		details, err := encodeJSON(log.Details)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`
INSERT INTO control_audit_logs(id, actor, actor_role, action, resource_type, resource_id, message, details, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, log.ID, log.Actor, log.ActorRole, log.Action, log.ResourceType, log.ResourceID, log.Message, details, log.CreatedAt); err != nil {
			return err
		}
	}

	for _, snapshot := range bundle.Usage {
		nodeID, ok := nodeIDMap[snapshot.NodeKey]
		if !ok {
			return ErrNodeNotFound
		}
		if _, err = tx.Exec(`
INSERT INTO usage_reports(node_id, username, upload, download, quota, expiry_date, reported_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, nodeID, snapshot.Username, snapshot.Upload, snapshot.Download, snapshot.Quota, snapshot.ExpiryDate, snapshot.ReportedAt, snapshot.ReportedAt); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// AppendAuditLog records one control-plane audit event.
func (s *MySQLStore) AppendAuditLog(req CreateAuditLogRequest) (*ControlAuditLog, error) {
	details, err := encodeJSON(req.Details)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result, err := s.db.Exec(`
INSERT INTO control_audit_logs(actor, actor_role, action, resource_type, resource_id, message, details, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, req.Actor, req.ActorRole, req.Action, req.ResourceType, req.ResourceID, req.Message, details, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &ControlAuditLog{
		ID:           uint64(id),
		Actor:        req.Actor,
		ActorRole:    req.ActorRole,
		Action:       req.Action,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		Message:      req.Message,
		Details:      clonePayload(req.Details),
		CreatedAt:    now,
	}, nil
}

// ListAuditLogs returns control-plane audit logs ordered by newest first.
func (s *MySQLStore) ListAuditLogs(query AuditQuery) ([]ControlAuditLog, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	sqlQuery := `
SELECT id, actor, actor_role, action, resource_type, resource_id, message, details, created_at
FROM control_audit_logs
`
	args := make([]interface{}, 0, 5)
	conditions := make([]string, 0, 3)
	if query.Actor != "" {
		conditions = append(conditions, "actor = ?")
		args = append(args, query.Actor)
	}
	if query.Action != "" {
		conditions = append(conditions, "action = ?")
		args = append(args, query.Action)
	}
	if query.ResourceType != "" {
		conditions = append(conditions, "resource_type = ?")
		args = append(args, query.ResourceType)
	}
	if len(conditions) > 0 {
		sqlQuery += "WHERE " + joinConditions(conditions) + "\n"
	}
	sqlQuery += "ORDER BY id DESC\nLIMIT ? OFFSET ?"
	args = append(args, query.Limit, query.Offset)

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []ControlAuditLog
	for rows.Next() {
		var (
			log         ControlAuditLog
			detailsJSON []byte
		)
		if err := rows.Scan(&log.ID, &log.Actor, &log.ActorRole, &log.Action, &log.ResourceType, &log.ResourceID, &log.Message, &detailsJSON, &log.CreatedAt); err != nil {
			return nil, err
		}
		log.Details = decodePayload(detailsJSON)
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// GetNodeAgentCredential returns the internal auth material for a node.
func (s *MySQLStore) GetNodeAgentCredential(nodeKey string) (*NodeAgentCredential, error) {
	row := s.db.QueryRow(`
SELECT node_key, agent_secret_hash, updated_at
FROM nodes
WHERE node_key = ?
`, nodeKey)
	var (
		credential NodeAgentCredential
		updatedAt  sql.NullTime
	)
	if err := row.Scan(&credential.NodeKey, &credential.SecretHash, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}
	if updatedAt.Valid {
		credential.UpdatedAt = updatedAt.Time
	}
	credential.AuthEnabled = credential.SecretHash != ""
	return &credential, nil
}

// RotateNodeAgentSecret rotates the per-node signing secret and returns the new plaintext secret.
func (s *MySQLStore) RotateNodeAgentSecret(nodeKey string) (string, error) {
	if _, err := s.findNodeByKey(nodeKey); err != nil {
		return "", err
	}
	secret := GenerateAgentSecret()
	derived, err := hashAgentSecret(secret)
	if err != nil {
		return "", err
	}
	result, err := s.db.Exec(`
UPDATE nodes
SET agent_secret_hash = ?, agent_auth_mode = 'secret', updated_at = ?
WHERE node_key = ?
`, derived, time.Now(), nodeKey)
	if err != nil {
		return "", err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if affected == 0 {
		return "", ErrNodeNotFound
	}
	return secret, nil
}

// RegisterNode creates or updates a node row.
func (s *MySQLStore) RegisterNode(req RegisterNodeRequest) (*Node, error) {
	tags, err := encodeJSON(req.Tags)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	secretHash := ""
	if req.AgentSecret != "" {
		secretHash, err = hashAgentSecret(req.AgentSecret)
		if err != nil {
			return nil, err
		}
	}

	_, err = s.db.Exec(`
INSERT INTO nodes(node_key, name, region, provider, endpoint, port, trojan_type, trojan_version, manager_version, public_ip, domain_name, tags, agent_secret_hash, agent_auth_mode, status, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, IF(? <> '', 'secret', 'token'), 'active', ?)
ON DUPLICATE KEY UPDATE
	name = VALUES(name),
	region = VALUES(region),
	provider = VALUES(provider),
	endpoint = VALUES(endpoint),
	port = VALUES(port),
	trojan_type = VALUES(trojan_type),
	trojan_version = VALUES(trojan_version),
	manager_version = VALUES(manager_version),
	public_ip = VALUES(public_ip),
	domain_name = VALUES(domain_name),
	tags = VALUES(tags),
	agent_secret_hash = IF(nodes.agent_secret_hash = '' AND VALUES(agent_secret_hash) <> '', VALUES(agent_secret_hash), nodes.agent_secret_hash),
	agent_auth_mode = IF(IF(nodes.agent_secret_hash = '' AND VALUES(agent_secret_hash) <> '', VALUES(agent_secret_hash), nodes.agent_secret_hash) <> '', 'secret', 'token'),
	status = 'active',
	last_seen_at = VALUES(last_seen_at)
`, req.NodeKey, req.Name, req.Region, req.Provider, req.Endpoint, req.Port, req.TrojanType, req.TrojanVersion, req.ManagerVersion, req.PublicIP, req.DomainName, tags, secretHash, secretHash, now)
	if err != nil {
		return nil, err
	}
	return s.findNodeByKey(req.NodeKey)
}

// Heartbeat records the latest heartbeat and updates node last_seen_at.
func (s *MySQLStore) Heartbeat(req HeartbeatRequest) (*Node, error) {
	node, err := s.findNodeByKey(req.NodeKey)
	if err != nil {
		return nil, err
	}

	payload, err := encodeJSON(req.Payload)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	if _, err := s.db.Exec(`
INSERT INTO node_heartbeats(node_id, cpu_percent, memory_percent, disk_percent, tcp_count, udp_count, upload_speed, download_speed, payload, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, node.ID, req.CPUPercent, req.MemoryPercent, req.DiskPercent, req.TCPCount, req.UDPCount, req.UploadSpeed, req.DownloadSpeed, payload, now); err != nil {
		return nil, err
	}

	if _, err := s.db.Exec(`UPDATE nodes SET last_seen_at = ?, updated_at = ? WHERE id = ?`, now, now, node.ID); err != nil {
		return nil, err
	}

	return s.findNodeByKey(req.NodeKey)
}

// PendingTasks lists pending tasks for a node.
func (s *MySQLStore) PendingTasks(nodeKey string, limit int) ([]Task, error) {
	node, err := s.findNodeByKey(nodeKey)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(`
SELECT id, task_type, payload, status, created_at
       , attempt_count, execution_token, result_message, result_details, started_at, finished_at
FROM tasks
WHERE node_id = ? AND status = 'pending'
ORDER BY id ASC
LIMIT ?
`, node.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var (
			task              Task
			payloadJSON       []byte
			executionToken    string
			resultDetailsJSON []byte
			startedAt         sql.NullTime
			finishedAt        sql.NullTime
		)
		if err := rows.Scan(&task.ID, &task.TaskType, &payloadJSON, &task.Status, &task.CreatedAt, &task.Attempt, &executionToken, &task.ResultMessage, &resultDetailsJSON, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		task.NodeKey = nodeKey
		task.Payload = decodePayload(payloadJSON)
		task.ExecutionToken = executionToken
		task.ResultDetails = decodePayload(resultDetailsJSON)
		if startedAt.Valid {
			task.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			task.FinishedAt = &finishedAt.Time
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// ListNodes returns all registered nodes.
func (s *MySQLStore) ListNodes() ([]Node, error) {
	rows, err := s.db.Query(`
SELECT id, node_key, name, region, provider, endpoint, port, trojan_type, trojan_version, manager_version, public_ip, domain_name, tags, agent_auth_mode, status, last_seen_at, created_at, updated_at
FROM nodes
ORDER BY id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		heartbeat, err := s.lastHeartbeat(node.ID)
		if err != nil {
			return nil, err
		}
		node.LastHeartbeat = heartbeat
		nodes = append(nodes, *node)
	}
	return nodes, rows.Err()
}

// CreateTask enqueues a pending task for a node.
func (s *MySQLStore) CreateTask(req CreateTaskRequest) (*Task, error) {
	node, err := s.findNodeByKey(req.NodeKey)
	if err != nil {
		return nil, err
	}

	payload, err := encodeJSON(req.Payload)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result, err := s.db.Exec(`
INSERT INTO tasks(node_id, task_type, payload, status, attempt_count, execution_token, created_at, updated_at)
VALUES (?, ?, ?, 'pending', 0, '', ?, ?)
`, node.ID, req.TaskType, payload, now, now)
	if err != nil {
		return nil, err
	}

	taskID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	task := &Task{
		ID:        uint64(taskID),
		NodeKey:   req.NodeKey,
		TaskType:  req.TaskType,
		Payload:   clonePayload(req.Payload),
		Status:    "pending",
		Attempt:   0,
		CreatedAt: now,
	}
	if err := s.insertTaskEvent(task.ID, node.ID, "queued", "control", "task queued", map[string]interface{}{
		"taskType": task.TaskType,
	}); err != nil {
		return nil, err
	}
	return task, nil
}

// RetryTask creates a new pending copy of an existing task.
func (s *MySQLStore) RetryTask(taskID uint64) (*Task, error) {
	task, err := s.findTask(taskID)
	if err != nil {
		return nil, err
	}
	retryTask, err := s.CreateTask(CreateTaskRequest{
		NodeKey:  task.NodeKey,
		TaskType: task.TaskType,
		Payload:  task.Payload,
	})
	if err != nil {
		return nil, err
	}
	node, err := s.findNodeByKey(task.NodeKey)
	if err != nil {
		return nil, err
	}
	if err := s.insertTaskEvent(task.ID, node.ID, "retry_requested", "control", "task retry requested", map[string]interface{}{
		"retryTaskId": retryTask.ID,
	}); err != nil {
		return nil, err
	}
	if err := s.insertTaskEvent(retryTask.ID, node.ID, "retried_from", "control", "task re-queued from previous task", map[string]interface{}{
		"sourceTaskId": task.ID,
		"taskType":     retryTask.TaskType,
	}); err != nil {
		return nil, err
	}
	return retryTask, nil
}

// ListTasks returns recent tasks across all nodes.
func (s *MySQLStore) ListTasks(query TaskQuery) ([]Task, error) {
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	sqlQuery := `
SELECT t.id, n.node_key, t.task_type, t.payload, t.status, t.attempt_count, t.execution_token, t.result_message, t.result_details, t.created_at, t.started_at, t.finished_at
FROM tasks t
JOIN nodes n ON n.id = t.node_id
`
	args := make([]interface{}, 0, 5)
	conditions := make([]string, 0, 3)
	if query.NodeKey != "" {
		conditions = append(conditions, "n.node_key = ?")
		args = append(args, query.NodeKey)
	}
	if query.TaskType != "" {
		conditions = append(conditions, "t.task_type = ?")
		args = append(args, query.TaskType)
	}
	if query.Status != "" {
		conditions = append(conditions, "t.status = ?")
		args = append(args, query.Status)
	}
	if len(conditions) > 0 {
		sqlQuery += "WHERE " + joinConditions(conditions) + "\n"
	}
	sqlQuery += "ORDER BY t.id DESC\nLIMIT ? OFFSET ?"
	args = append(args, query.Limit, query.Offset)

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var (
			task              Task
			payloadJSON       []byte
			executionToken    string
			resultDetailsJSON []byte
			startedAt         sql.NullTime
			finishedAt        sql.NullTime
		)
		if err := rows.Scan(&task.ID, &task.NodeKey, &task.TaskType, &payloadJSON, &task.Status, &task.Attempt, &executionToken, &task.ResultMessage, &resultDetailsJSON, &task.CreatedAt, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		task.Payload = decodePayload(payloadJSON)
		task.ExecutionToken = executionToken
		task.ResultDetails = decodePayload(resultDetailsJSON)
		if startedAt.Valid {
			task.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			task.FinishedAt = &finishedAt.Time
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// GetTask returns one task by id.
func (s *MySQLStore) GetTask(taskID uint64) (*Task, error) {
	return s.findTask(taskID)
}

// ListTaskEvents returns the task audit trail ordered by creation time.
func (s *MySQLStore) ListTaskEvents(taskID uint64, limit int) ([]TaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if _, err := s.findTask(taskID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT e.id, e.task_id, n.node_key, e.event_type, e.actor, e.message, e.details, e.created_at
FROM task_events e
JOIN nodes n ON n.id = e.node_id
WHERE e.task_id = ?
ORDER BY e.id ASC
LIMIT ?
`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TaskEvent
	for rows.Next() {
		var (
			event       TaskEvent
			detailsJSON []byte
		)
		if err := rows.Scan(&event.ID, &event.TaskID, &event.NodeKey, &event.EventType, &event.Actor, &event.Message, &detailsJSON, &event.CreatedAt); err != nil {
			return nil, err
		}
		event.Details = decodePayload(detailsJSON)
		events = append(events, event)
	}
	return events, rows.Err()
}

// StartTask marks a task as running.
func (s *MySQLStore) StartTask(taskID uint64, req StartTaskRequest) (*Task, error) {
	task, err := s.findTask(taskID)
	if err != nil {
		return nil, err
	}
	if task.NodeKey != req.NodeKey {
		if _, err := s.findNodeByKey(req.NodeKey); err != nil {
			return nil, err
		}
		return nil, ErrTaskNotFound
	}
	if task.Status == "running" && task.ExecutionToken == req.ExecutionToken {
		return task, nil
	}
	if task.Status != "pending" {
		return nil, ErrTaskConflict
	}
	node, err := s.findNodeByKey(req.NodeKey)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result, err := s.db.Exec(`
UPDATE tasks
SET status = 'running', started_at = ?, updated_at = ?, attempt_count = attempt_count + 1, execution_token = ?
WHERE id = ? AND status = 'pending'
`, now, now, req.ExecutionToken, taskID)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		refreshed, err := s.findTask(taskID)
		if err != nil {
			return nil, err
		}
		if refreshed.Status == "running" && refreshed.ExecutionToken == req.ExecutionToken {
			return refreshed, nil
		}
		return nil, ErrTaskConflict
	}
	if err := s.insertTaskEvent(taskID, node.ID, "started", "agent", "task started by node agent", map[string]interface{}{
		"attempt":        task.Attempt + 1,
		"executionToken": req.ExecutionToken,
	}); err != nil {
		return nil, err
	}
	return s.findTask(taskID)
}

// FinishTask marks a task as finished and stores the result.
func (s *MySQLStore) FinishTask(taskID uint64, req FinishTaskRequest) (*Task, error) {
	task, err := s.findTask(taskID)
	if err != nil {
		return nil, err
	}
	if task.NodeKey != req.NodeKey {
		if _, err := s.findNodeByKey(req.NodeKey); err != nil {
			return nil, err
		}
		return nil, ErrTaskNotFound
	}
	if task.ExecutionToken != req.ExecutionToken {
		return nil, ErrTaskConflict
	}
	if task.Status == "succeeded" || task.Status == "failed" {
		return task, nil
	}
	if task.Status != "running" {
		return nil, ErrTaskConflict
	}
	now := time.Now()
	status := "failed"
	if req.Success {
		status = "succeeded"
	}
	node, err := s.findNodeByKey(req.NodeKey)
	if err != nil {
		return nil, err
	}
	resultDetails, err := encodeJSON(req.Details)
	if err != nil {
		return nil, err
	}
	result, err := s.db.Exec(`
UPDATE tasks
SET status = ?, result_message = ?, result_details = ?, finished_at = ?, updated_at = ?
WHERE id = ? AND status = 'running' AND execution_token = ?
`, status, req.Message, resultDetails, now, now, taskID, req.ExecutionToken)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		refreshed, err := s.findTask(taskID)
		if err != nil {
			return nil, err
		}
		if (refreshed.Status == "succeeded" || refreshed.Status == "failed") && refreshed.ExecutionToken == req.ExecutionToken {
			return refreshed, nil
		}
		return nil, ErrTaskConflict
	}
	eventType := "failed"
	message := "task finished with failure"
	if req.Success {
		eventType = "succeeded"
		message = "task finished successfully"
	}
	if err := s.insertTaskEvent(taskID, node.ID, eventType, "agent", message, req.Details); err != nil {
		return nil, err
	}
	return s.findTask(taskID)
}

// ReportUsage stores node usage snapshots.
func (s *MySQLStore) ReportUsage(req UsageReportRequest) ([]UsageSnapshot, error) {
	node, err := s.findNodeByKey(req.NodeKey)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	snapshots := make([]UsageSnapshot, 0, len(req.Users))
	for _, user := range req.Users {
		if _, err := s.db.Exec(`
INSERT INTO usage_reports(node_id, username, upload, download, quota, expiry_date, reported_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, node.ID, user.Username, user.Upload, user.Download, user.Quota, user.ExpiryDate, now); err != nil {
			return nil, err
		}
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
	return snapshots, nil
}

// NodeUsage returns the latest usage snapshots for a node.
func (s *MySQLStore) NodeUsage(nodeKey string, limit int) ([]UsageSnapshot, error) {
	node, err := s.findNodeByKey(nodeKey)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`
SELECT username, upload, download, quota, expiry_date, reported_at
FROM usage_reports
WHERE node_id = ?
ORDER BY id DESC
LIMIT ?
`, node.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usage []UsageSnapshot
	for rows.Next() {
		var item UsageSnapshot
		if err := rows.Scan(&item.Username, &item.Upload, &item.Download, &item.Quota, &item.ExpiryDate, &item.ReportedAt); err != nil {
			return nil, err
		}
		item.NodeKey = nodeKey
		usage = append(usage, item)
	}
	return usage, rows.Err()
}

// ListUsers returns control-plane users.
func (s *MySQLStore) ListUsers() ([]ControlUser, error) {
	rows, err := s.db.Query(`
SELECT username, password_show, quota, use_days, expiry_date, status, created_at, updated_at
FROM control_users
ORDER BY username ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []ControlUser
	for rows.Next() {
		var user ControlUser
		if err := rows.Scan(&user.Username, &user.Password, &user.Quota, &user.UseDays, &user.ExpiryDate, &user.Status, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// CreateUser creates or updates a control-plane user.
func (s *MySQLStore) CreateUser(req CreateUserRequest) (*ControlUser, error) {
	_, err := s.db.Exec(`
INSERT INTO control_users(username, password_show, quota, use_days, expiry_date, status)
VALUES (?, ?, ?, ?, ?, 'active')
ON DUPLICATE KEY UPDATE
	password_show = VALUES(password_show),
	quota = VALUES(quota),
	use_days = VALUES(use_days),
	expiry_date = VALUES(expiry_date),
	status = 'active'
`, req.Username, req.Password, req.Quota, req.UseDays, req.ExpiryDate)
	if err != nil {
		return nil, err
	}
	return s.findControlUser(req.Username)
}

// GetUser returns one control-plane user.
func (s *MySQLStore) GetUser(username string) (*ControlUser, error) {
	return s.findControlUser(username)
}

// DeleteUser removes a control-plane user and its bindings.
func (s *MySQLStore) DeleteUser(username string) error {
	userID, err := s.findControlUserID(username)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM user_node_bindings WHERE user_id = ?`, userID); err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM control_users WHERE id = ?`, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrUserNotFound
	}
	return nil
}

// BindUserToNode binds a control-plane user to a node.
func (s *MySQLStore) BindUserToNode(username, nodeKey string) (*UserBinding, error) {
	userID, err := s.findControlUserID(username)
	if err != nil {
		return nil, err
	}
	node, err := s.findNodeByKey(nodeKey)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`
INSERT INTO user_node_bindings(user_id, node_id)
VALUES (?, ?)
ON DUPLICATE KEY UPDATE node_id = VALUES(node_id)
`, userID, node.ID)
	if err != nil {
		return nil, err
	}
	return s.findBinding(username, nodeKey)
}

// UnbindUserFromNode removes a user-node binding.
func (s *MySQLStore) UnbindUserFromNode(username, nodeKey string) error {
	userID, err := s.findControlUserID(username)
	if err != nil {
		return err
	}
	node, err := s.findNodeByKey(nodeKey)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(`DELETE FROM user_node_bindings WHERE user_id = ? AND node_id = ?`, userID, node.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrBindingNotFound
	}
	return nil
}

// NodeUsers returns users bound to the node.
func (s *MySQLStore) NodeUsers(nodeKey string) ([]ControlUser, error) {
	node, err := s.findNodeByKey(nodeKey)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT u.username, u.password_show, u.quota, u.use_days, u.expiry_date, u.status, u.created_at, u.updated_at
FROM user_node_bindings b
JOIN control_users u ON u.id = b.user_id
WHERE b.node_id = ?
ORDER BY u.username ASC
`, node.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []ControlUser
	for rows.Next() {
		var user ControlUser
		if err := rows.Scan(&user.Username, &user.Password, &user.Quota, &user.UseDays, &user.ExpiryDate, &user.Status, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// UserNodes returns nodes bound to a control-plane user.
func (s *MySQLStore) UserNodes(username string) ([]Node, error) {
	if _, err := s.findControlUser(username); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT n.id, n.node_key, n.name, n.region, n.provider, n.endpoint, n.port, n.trojan_type, n.trojan_version, n.manager_version, n.public_ip, n.domain_name, n.tags, n.agent_auth_mode, n.status, n.last_seen_at, n.created_at, n.updated_at
FROM user_node_bindings b
JOIN control_users u ON u.id = b.user_id
JOIN nodes n ON n.id = b.node_id
WHERE u.username = ?
ORDER BY n.node_key ASC
`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		heartbeat, err := s.lastHeartbeat(node.ID)
		if err != nil {
			return nil, err
		}
		node.LastHeartbeat = heartbeat
		nodes = append(nodes, *node)
	}
	return nodes, rows.Err()
}

// UserUsage returns usage snapshots for one user across nodes.
func (s *MySQLStore) UserUsage(username string, limit int) ([]UsageSnapshot, error) {
	if _, err := s.findControlUser(username); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`
SELECT n.node_key, u.username, u.upload, u.download, u.quota, u.expiry_date, u.reported_at
FROM usage_reports u
JOIN nodes n ON n.id = u.node_id
WHERE u.username = ?
ORDER BY u.id DESC
LIMIT ?
`, username, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usage []UsageSnapshot
	for rows.Next() {
		var item UsageSnapshot
		if err := rows.Scan(&item.NodeKey, &item.Username, &item.Upload, &item.Download, &item.Quota, &item.ExpiryDate, &item.ReportedAt); err != nil {
			return nil, err
		}
		usage = append(usage, item)
	}
	return usage, rows.Err()
}

// SyncNodeUsers creates a sync_users task from current bound users.
func (s *MySQLStore) SyncNodeUsers(nodeKey string) (*Task, error) {
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

func (s *MySQLStore) findNodeByKey(nodeKey string) (*Node, error) {
	row := s.db.QueryRow(`
SELECT id, node_key, name, region, provider, endpoint, port, trojan_type, trojan_version, manager_version, public_ip, domain_name, tags, agent_auth_mode, status, last_seen_at, created_at, updated_at
FROM nodes
WHERE node_key = ?
`, nodeKey)

	node, err := scanNode(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNodeNotFound
		}
		return nil, err
	}
	heartbeat, err := s.lastHeartbeat(node.ID)
	if err != nil {
		return nil, err
	}
	node.LastHeartbeat = heartbeat
	return node, nil
}

func (s *MySQLStore) findTask(taskID uint64) (*Task, error) {
	row := s.db.QueryRow(`
SELECT t.id, n.node_key, t.task_type, t.payload, t.status, t.attempt_count, t.execution_token, t.result_message, t.result_details, t.created_at, t.started_at, t.finished_at
FROM tasks t
JOIN nodes n ON n.id = t.node_id
WHERE t.id = ?
`, taskID)

	var (
		task              Task
		payloadJSON       []byte
		executionToken    string
		resultDetailsJSON []byte
		startedAt         sql.NullTime
		finishedAt        sql.NullTime
	)
	if err := row.Scan(&task.ID, &task.NodeKey, &task.TaskType, &payloadJSON, &task.Status, &task.Attempt, &executionToken, &task.ResultMessage, &resultDetailsJSON, &task.CreatedAt, &startedAt, &finishedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	task.Payload = decodePayload(payloadJSON)
	task.ExecutionToken = executionToken
	task.ResultDetails = decodePayload(resultDetailsJSON)
	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		task.FinishedAt = &finishedAt.Time
	}
	return &task, nil
}

func (s *MySQLStore) findControlUser(username string) (*ControlUser, error) {
	row := s.db.QueryRow(`
SELECT username, password_show, quota, use_days, expiry_date, status, created_at, updated_at
FROM control_users
WHERE username = ?
`, username)
	var user ControlUser
	if err := row.Scan(&user.Username, &user.Password, &user.Quota, &user.UseDays, &user.ExpiryDate, &user.Status, &user.CreatedAt, &user.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (s *MySQLStore) findControlAdmin(username string) (*ControlAdmin, error) {
	row := s.db.QueryRow(`
SELECT username, password_hash, role, status, created_at, updated_at
FROM control_admins
WHERE username = ?
`, username)
	var admin ControlAdmin
	if err := row.Scan(&admin.Username, &admin.PasswordHash, &admin.Role, &admin.Status, &admin.CreatedAt, &admin.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAdminNotFound
		}
		return nil, err
	}
	return &admin, nil
}

func (s *MySQLStore) findControlUserID(username string) (uint64, error) {
	row := s.db.QueryRow(`SELECT id FROM control_users WHERE username = ?`, username)
	var id uint64
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrUserNotFound
		}
		return 0, err
	}
	return id, nil
}

func (s *MySQLStore) findBinding(username, nodeKey string) (*UserBinding, error) {
	row := s.db.QueryRow(`
SELECT u.username, n.node_key, b.created_at
FROM user_node_bindings b
JOIN control_users u ON u.id = b.user_id
JOIN nodes n ON n.id = b.node_id
WHERE u.username = ? AND n.node_key = ?
`, username, nodeKey)
	var binding UserBinding
	if err := row.Scan(&binding.Username, &binding.NodeKey, &binding.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			if _, err := s.findControlUser(username); err != nil {
				return nil, err
			}
			if _, err := s.findNodeByKey(nodeKey); err != nil {
				return nil, err
			}
			return nil, ErrBindingNotFound
		}
		return nil, err
	}
	return &binding, nil
}

func (s *MySQLStore) insertTaskEvent(taskID uint64, nodeID uint64, eventType string, actor string, message string, details map[string]interface{}) error {
	payload, err := encodeJSON(details)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO task_events(task_id, node_id, event_type, actor, message, details, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, taskID, nodeID, eventType, actor, message, payload, time.Now())
	return err
}

func (s *MySQLStore) lastHeartbeat(nodeID uint64) (*HeartbeatSnapshot, error) {
	row := s.db.QueryRow(`
SELECT cpu_percent, memory_percent, disk_percent, tcp_count, udp_count, upload_speed, download_speed, payload, created_at
FROM node_heartbeats
WHERE node_id = ?
ORDER BY id DESC
LIMIT 1
`, nodeID)

	var (
		snapshot    HeartbeatSnapshot
		payloadJSON []byte
	)
	if err := row.Scan(&snapshot.CPUPercent, &snapshot.MemoryPercent, &snapshot.DiskPercent, &snapshot.TCPCount, &snapshot.UDPCount, &snapshot.UploadSpeed, &snapshot.DownloadSpeed, &payloadJSON, &snapshot.ReportedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	snapshot.Payload = decodePayload(payloadJSON)
	return &snapshot, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanNode(scanner rowScanner) (*Node, error) {
	var (
		node       Node
		tagsJSON   []byte
		lastSeenAt sql.NullTime
	)
	if err := scanner.Scan(&node.ID, &node.NodeKey, &node.Name, &node.Region, &node.Provider, &node.Endpoint, &node.Port, &node.TrojanType, &node.TrojanVersion, &node.ManagerVersion, &node.PublicIP, &node.DomainName, &tagsJSON, &node.AgentAuthMode, &node.Status, &lastSeenAt, &node.CreatedAt, &node.UpdatedAt); err != nil {
		return nil, err
	}
	if lastSeenAt.Valid {
		node.LastSeenAt = lastSeenAt.Time
	}
	node.Tags = decodeTags(tagsJSON)
	return &node, nil
}

func loadAppliedControlMigrations(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query(`SELECT version FROM ` + controlMigrationTable + ` ORDER BY version ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func pendingControlMigrationVersions(applied map[int]bool) []int {
	var pending []int
	for _, migration := range controlMigrations {
		if !applied[migration.Version] {
			pending = append(pending, migration.Version)
		}
	}
	return pending
}

func execMigrationStatements(tx *sql.Tx, statements ...string) error {
	for _, stmt := range statements {
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func columnExists(tx *sql.Tx, tableName string, columnName string) (bool, error) {
	row := tx.QueryRow(`
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?
`, tableName, columnName)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func splitStatements(schema string) []string {
	var (
		statements []string
		current    []rune
	)
	for _, char := range schema {
		if char == ';' {
			statements = append(statements, string(current))
			current = current[:0]
			continue
		}
		current = append(current, char)
	}
	if len(current) > 0 {
		statements = append(statements, string(current))
	}
	return statements
}

func joinConditions(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	result := conditions[0]
	for i := 1; i < len(conditions); i++ {
		result += " AND " + conditions[i]
	}
	return result
}

func encodeJSON(value interface{}) ([]byte, error) {
	if value == nil {
		return []byte("null"), nil
	}
	result, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	return result, nil
}

func decodePayload(data []byte) map[string]interface{} {
	if len(data) == 0 || string(data) == "null" {
		return map[string]interface{}{}
	}
	result := map[string]interface{}{}
	if err := json.Unmarshal(data, &result); err != nil {
		return map[string]interface{}{}
	}
	return result
}

func decodeTags(data []byte) []string {
	if len(data) == 0 || string(data) == "null" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal(data, &tags); err != nil {
		return []string{}
	}
	return tags
}

func decodeStringSlice(data []byte) []string {
	return decodeTags(data)
}

func nullableTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

func nullableTimePtr(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
