CREATE TABLE IF NOT EXISTS plans (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name VARCHAR(64) NOT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    quota BIGINT NOT NULL DEFAULT -1,
    duration_days INT NOT NULL DEFAULT 0,
    node_strategy VARCHAR(32) NOT NULL DEFAULT 'manual',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_plans_name (name)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    username VARCHAR(64) NOT NULL,
    password_show VARCHAR(255) NOT NULL,
    password_hash CHAR(56) NOT NULL,
    plan_id BIGINT UNSIGNED DEFAULT NULL,
    quota BIGINT NOT NULL DEFAULT -1,
    use_days INT NOT NULL DEFAULT 0,
    expiry_date DATE DEFAULT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    remark VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_username (username),
    KEY idx_users_plan_id (plan_id),
    CONSTRAINT fk_users_plan_id FOREIGN KEY (plan_id) REFERENCES plans(id)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS nodes (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    node_key VARCHAR(64) NOT NULL,
    name VARCHAR(64) NOT NULL,
    region VARCHAR(32) NOT NULL DEFAULT '',
    provider VARCHAR(32) NOT NULL DEFAULT '',
    endpoint VARCHAR(255) NOT NULL DEFAULT '',
    trojan_type VARCHAR(16) NOT NULL DEFAULT 'trojan',
    trojan_version VARCHAR(32) NOT NULL DEFAULT '',
    manager_version VARCHAR(32) NOT NULL DEFAULT '',
    public_ip VARCHAR(64) NOT NULL DEFAULT '',
    domain_name VARCHAR(255) NOT NULL DEFAULT '',
    tags VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    last_seen_at DATETIME DEFAULT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_nodes_node_key (node_key),
    UNIQUE KEY uk_nodes_name (name),
    KEY idx_nodes_region_status (region, status)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS node_tokens (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    node_id BIGINT UNSIGNED NOT NULL,
    token_hash CHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    expires_at DATETIME DEFAULT NULL,
    last_used_at DATETIME DEFAULT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_node_tokens_token_hash (token_hash),
    KEY idx_node_tokens_node_id (node_id),
    CONSTRAINT fk_node_tokens_node_id FOREIGN KEY (node_id) REFERENCES nodes(id)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_node_bindings (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    node_id BIGINT UNSIGNED NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    source VARCHAR(16) NOT NULL DEFAULT 'manual',
    synced_revision BIGINT NOT NULL DEFAULT 0,
    last_synced_at DATETIME DEFAULT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_user_node_binding (user_id, node_id),
    KEY idx_user_node_bindings_node_id (node_id),
    CONSTRAINT fk_user_node_bindings_user_id FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT fk_user_node_bindings_node_id FOREIGN KEY (node_id) REFERENCES nodes(id)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS tasks (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    node_id BIGINT UNSIGNED NOT NULL,
    task_type VARCHAR(32) NOT NULL,
    payload JSON NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    retry_count INT NOT NULL DEFAULT 0,
    max_retry INT NOT NULL DEFAULT 3,
    scheduled_at DATETIME DEFAULT NULL,
    started_at DATETIME DEFAULT NULL,
    finished_at DATETIME DEFAULT NULL,
    created_by VARCHAR(64) NOT NULL DEFAULT 'system',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_tasks_node_status (node_id, status),
    KEY idx_tasks_type_status (task_type, status),
    CONSTRAINT fk_tasks_node_id FOREIGN KEY (node_id) REFERENCES nodes(id)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS task_results (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    task_id BIGINT UNSIGNED NOT NULL,
    node_id BIGINT UNSIGNED NOT NULL,
    success TINYINT(1) NOT NULL DEFAULT 0,
    message VARCHAR(255) NOT NULL DEFAULT '',
    details JSON DEFAULT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_task_results_task_id (task_id),
    KEY idx_task_results_node_id (node_id),
    CONSTRAINT fk_task_results_task_id FOREIGN KEY (task_id) REFERENCES tasks(id),
    CONSTRAINT fk_task_results_node_id FOREIGN KEY (node_id) REFERENCES nodes(id)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS node_heartbeats (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    node_id BIGINT UNSIGNED NOT NULL,
    cpu_percent DECIMAL(5,2) NOT NULL DEFAULT 0,
    memory_percent DECIMAL(5,2) NOT NULL DEFAULT 0,
    disk_percent DECIMAL(5,2) NOT NULL DEFAULT 0,
    tcp_count INT NOT NULL DEFAULT 0,
    udp_count INT NOT NULL DEFAULT 0,
    upload_speed BIGINT UNSIGNED NOT NULL DEFAULT 0,
    download_speed BIGINT UNSIGNED NOT NULL DEFAULT 0,
    payload JSON DEFAULT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_node_heartbeats_node_created (node_id, created_at),
    CONSTRAINT fk_node_heartbeats_node_id FOREIGN KEY (node_id) REFERENCES nodes(id)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS usage_reports (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    node_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    upload BIGINT UNSIGNED NOT NULL DEFAULT 0,
    download BIGINT UNSIGNED NOT NULL DEFAULT 0,
    quota BIGINT NOT NULL DEFAULT -1,
    report_time DATETIME NOT NULL,
    payload JSON DEFAULT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_usage_reports_node_user_time (node_id, user_id, report_time),
    KEY idx_usage_reports_user_time (user_id, report_time),
    CONSTRAINT fk_usage_reports_node_id FOREIGN KEY (node_id) REFERENCES nodes(id),
    CONSTRAINT fk_usage_reports_user_id FOREIGN KEY (user_id) REFERENCES users(id)
) DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS subscriptions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name VARCHAR(64) NOT NULL,
    scope VARCHAR(16) NOT NULL DEFAULT 'user',
    target_id BIGINT UNSIGNED DEFAULT NULL,
    template_type VARCHAR(16) NOT NULL DEFAULT 'clash',
    config JSON NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_subscriptions_scope_target (scope, target_id)
) DEFAULT CHARSET=utf8mb4;
