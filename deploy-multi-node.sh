#!/usr/bin/env bash
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
REPO="${REPO:-shafishcn/trojan}"
DOWNLOAD_BASE="${DOWNLOAD_BASE:-https://github.com/${REPO}/releases/download}"
LATEST_API="${LATEST_API:-https://api.github.com/repos/${REPO}/releases/latest}"
SERVICE_DIR="/etc/systemd/system"
INSTALL_DIR="/opt/trojan"
START_NOW=1
VERSION=""
MODE=""

CONTROL_HOST="127.0.0.1"
CONTROL_PORT="8081"
CONTROL_STORE="mysql"
CONTROL_DSN=""
CONTROL_ADMIN_USER="admin"
CONTROL_ADMIN_PASS=""
CONTROL_JWT_SECRET=""
CONTROL_AGENT_TOKEN=""
CONTROL_METRICS_TOKEN=""
CONTROL_LOGIN_RATE_LIMIT="30"
CONTROL_AGENT_RATE_LIMIT="600"
CONTROL_AUDIT_RETENTION_DAYS="90"
CONTROL_TASK_RETENTION_DAYS="30"
CONTROL_USAGE_RETENTION_DAYS="30"
CONTROL_CLEANUP_INTERVAL_MINUTES="60"
CONTROL_NODE_STALE_MINUTES="10"
CONTROL_FAILED_TASK_ALERT_THRESHOLD="1"
CONTROL_PENDING_TASK_ALERT_THRESHOLD="20"
CONTROL_SERVICE_NAME="trojan-control"

AGENT_CONTROL_URL=""
AGENT_TOKEN=""
AGENT_NODE_SECRET=""
AGENT_NODE_KEY=""
AGENT_NODE_NAME=""
AGENT_NODE_DOMAIN=""
AGENT_NODE_PORT="443"
AGENT_SERVICE_NAME="trojan-agent"

color_echo() {
	local color="$1"
	shift
	printf '\033[%sm%s\033[0m\n' "$color" "$*"
}

info() {
	color_echo "36" "[INFO] $*"
}

warn() {
	color_echo "33" "[WARN] $*"
}

success() {
	color_echo "32" "[OK] $*"
}

fail() {
	color_echo "31" "[ERR] $*"
	exit 1
}

usage() {
	cat <<'EOF'
多节点控制中心一键部署脚本

用法:
  bash deploy-multi-node.sh control [选项]
  bash deploy-multi-node.sh agent [选项]

公共选项:
  --version <tag>           指定版本, 例如 v2.15.5, 默认自动获取最新 release
  --install-dir <dir>       安装目录, 默认 /opt/trojan
  --no-start                仅写入文件和 systemd, 不执行 enable --now
  -h, --help                显示帮助

control 模式常用参数:
  --host <host>             控制中心监听地址, 默认 127.0.0.1
  --port <port>             控制中心监听端口, 默认 8081
  --store <memory|mysql>    存储类型, 默认 mysql
  --dsn <dsn>               MySQL DSN, store=mysql 时必填
  --admin-user <user>       初始管理员, 默认 admin
  --admin-pass <pass>       初始管理员密码
  --jwt-secret <secret>     JWT 密钥
  --agent-token <token>     Agent 共享 token
  --metrics-token <token>   /metrics token

agent 模式必填参数:
  --control-url <url>       控制中心地址
  --token <token>           Agent token
  --node-secret <secret>    当前节点独立密钥
  --node-key <key>          当前节点唯一 key
  --name <name>             节点显示名称
  --domain <domain>         节点对外域名
  --port <port>             节点服务端口, 默认 443

示例:
  bash deploy-multi-node.sh control \
    --version v2.15.5 \
    --dsn 'root:pass@tcp(127.0.0.1:3306)/trojan_control?parseTime=true&charset=utf8mb4' \
    --admin-pass 'change-me' \
    --jwt-secret 'jwt-secret' \
    --agent-token 'agent-token' \
    --metrics-token 'metrics-token'

  bash deploy-multi-node.sh agent \
    --version v2.15.5 \
    --control-url 'https://control.example.com' \
    --token 'agent-token' \
    --node-secret 'node-secret-001' \
    --node-key 'node-01' \
    --name 'tokyo-01' \
    --domain 'tokyo.example.com' \
    --port 443
EOF
}

ensure_root() {
	if [[ "$(id -u)" != "0" ]]; then
		fail "请使用 root 执行该脚本"
	fi
}

ensure_command() {
	local command_name="$1"
	command -v "$command_name" >/dev/null 2>&1 || fail "缺少依赖: $command_name"
}

detect_asset_name() {
	local arch
	arch="$(uname -m)"
	case "$arch" in
	x86_64 | amd64)
		echo "trojan-linux-amd64"
		;;
	aarch64 | arm64)
		echo "trojan-linux-arm64"
		;;
	*)
		fail "暂不支持的架构: $arch"
		;;
	esac
}

fetch_latest_version() {
	curl -fsSL "$LATEST_API" | grep '"tag_name"' | head -n1 | cut -d'"' -f4
}

escape_env_value() {
	printf "%s" "$1" | sed "s/'/'\\\\''/g"
}

backup_file() {
	local target="$1"
	if [[ -f "$target" ]]; then
		local stamp
		stamp="$(date +%Y%m%d-%H%M%S)"
		cp "$target" "${target}.bak.${stamp}"
	fi
}

require_non_empty() {
	local name="$1"
	local value="$2"
	if [[ -z "$value" ]]; then
		fail "缺少必填参数: $name"
	fi
}

download_binary() {
	local version="$1"
	local asset_name="$2"
	local target_dir="$3"
	local tmp_file
	tmp_file="$(mktemp)"
	trap 'rm -f "$tmp_file"' RETURN

	info "下载 $version / $asset_name"
	curl -fsSL "${DOWNLOAD_BASE}/${version}/${asset_name}" -o "$tmp_file"
	install -m 0755 "$tmp_file" "${target_dir}/trojan"
	success "已安装二进制到 ${target_dir}/trojan"
}

write_control_env() {
	local env_file="/etc/trojan-control.env"
	backup_file "$env_file"
	cat >"$env_file" <<EOF
TROJAN_CONTROL_HOST='$(escape_env_value "$CONTROL_HOST")'
TROJAN_CONTROL_PORT='$(escape_env_value "$CONTROL_PORT")'
TROJAN_CONTROL_STORE='$(escape_env_value "$CONTROL_STORE")'
TROJAN_CONTROL_DSN='$(escape_env_value "$CONTROL_DSN")'
TROJAN_ADMIN_USER='$(escape_env_value "$CONTROL_ADMIN_USER")'
TROJAN_ADMIN_PASS='$(escape_env_value "$CONTROL_ADMIN_PASS")'
TROJAN_JWT_SECRET='$(escape_env_value "$CONTROL_JWT_SECRET")'
TROJAN_AGENT_TOKEN='$(escape_env_value "$CONTROL_AGENT_TOKEN")'
TROJAN_METRICS_TOKEN='$(escape_env_value "$CONTROL_METRICS_TOKEN")'
TROJAN_LOGIN_RATE_LIMIT='$(escape_env_value "$CONTROL_LOGIN_RATE_LIMIT")'
TROJAN_AGENT_RATE_LIMIT='$(escape_env_value "$CONTROL_AGENT_RATE_LIMIT")'
TROJAN_AUDIT_RETENTION_DAYS='$(escape_env_value "$CONTROL_AUDIT_RETENTION_DAYS")'
TROJAN_TASK_RETENTION_DAYS='$(escape_env_value "$CONTROL_TASK_RETENTION_DAYS")'
TROJAN_USAGE_RETENTION_DAYS='$(escape_env_value "$CONTROL_USAGE_RETENTION_DAYS")'
TROJAN_CLEANUP_INTERVAL_MINUTES='$(escape_env_value "$CONTROL_CLEANUP_INTERVAL_MINUTES")'
TROJAN_NODE_STALE_MINUTES='$(escape_env_value "$CONTROL_NODE_STALE_MINUTES")'
TROJAN_FAILED_TASK_ALERT_THRESHOLD='$(escape_env_value "$CONTROL_FAILED_TASK_ALERT_THRESHOLD")'
TROJAN_PENDING_TASK_ALERT_THRESHOLD='$(escape_env_value "$CONTROL_PENDING_TASK_ALERT_THRESHOLD")'
EOF
	chmod 600 "$env_file"
	success "已写入 $env_file"
}

write_control_service() {
	local service_file="${SERVICE_DIR}/${CONTROL_SERVICE_NAME}.service"
	backup_file "$service_file"
	cat >"$service_file" <<EOF
[Unit]
Description=Trojan Control Plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=/etc/trojan-control.env
ExecStart=${INSTALL_DIR}/trojan control \\
  --host \${TROJAN_CONTROL_HOST} \\
  --port \${TROJAN_CONTROL_PORT} \\
  --store \${TROJAN_CONTROL_STORE} \\
  --dsn \${TROJAN_CONTROL_DSN} \\
  --admin-user \${TROJAN_ADMIN_USER} \\
  --admin-pass \${TROJAN_ADMIN_PASS} \\
  --jwt-secret \${TROJAN_JWT_SECRET} \\
  --agent-token \${TROJAN_AGENT_TOKEN} \\
  --metrics-token \${TROJAN_METRICS_TOKEN} \\
  --login-rate-limit \${TROJAN_LOGIN_RATE_LIMIT} \\
  --agent-rate-limit \${TROJAN_AGENT_RATE_LIMIT} \\
  --audit-retention-days \${TROJAN_AUDIT_RETENTION_DAYS} \\
  --task-retention-days \${TROJAN_TASK_RETENTION_DAYS} \\
  --usage-retention-days \${TROJAN_USAGE_RETENTION_DAYS} \\
  --cleanup-interval-minutes \${TROJAN_CLEANUP_INTERVAL_MINUTES} \\
  --node-stale-minutes \${TROJAN_NODE_STALE_MINUTES} \\
  --failed-task-alert-threshold \${TROJAN_FAILED_TASK_ALERT_THRESHOLD} \\
  --pending-task-alert-threshold \${TROJAN_PENDING_TASK_ALERT_THRESHOLD}
Restart=always
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
	success "已写入 $service_file"
}

write_agent_env() {
	local env_file="/etc/trojan-agent.env"
	backup_file "$env_file"
	cat >"$env_file" <<EOF
TROJAN_CONTROL_URL='$(escape_env_value "$AGENT_CONTROL_URL")'
TROJAN_AGENT_TOKEN='$(escape_env_value "$AGENT_TOKEN")'
TROJAN_NODE_SECRET='$(escape_env_value "$AGENT_NODE_SECRET")'
TROJAN_NODE_KEY='$(escape_env_value "$AGENT_NODE_KEY")'
TROJAN_NODE_NAME='$(escape_env_value "$AGENT_NODE_NAME")'
TROJAN_NODE_DOMAIN='$(escape_env_value "$AGENT_NODE_DOMAIN")'
TROJAN_NODE_PORT='$(escape_env_value "$AGENT_NODE_PORT")'
EOF
	chmod 600 "$env_file"
	success "已写入 $env_file"
}

write_agent_service() {
	local service_file="${SERVICE_DIR}/${AGENT_SERVICE_NAME}.service"
	backup_file "$service_file"
	cat >"$service_file" <<EOF
[Unit]
Description=Trojan Node Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=/etc/trojan-agent.env
ExecStart=${INSTALL_DIR}/trojan agent \\
  --control-url \${TROJAN_CONTROL_URL} \\
  --token \${TROJAN_AGENT_TOKEN} \\
  --node-secret \${TROJAN_NODE_SECRET} \\
  --node-key \${TROJAN_NODE_KEY} \\
  --name \${TROJAN_NODE_NAME} \\
  --domain \${TROJAN_NODE_DOMAIN} \\
  --port \${TROJAN_NODE_PORT}
Restart=always
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
	success "已写入 $service_file"
}

reload_and_start() {
	local service_name="$1"
	systemctl daemon-reload
	if [[ "$START_NOW" == "1" ]]; then
		systemctl enable --now "$service_name"
		systemctl status "$service_name" --no-pager || true
	else
		systemctl enable "$service_name"
		warn "已跳过启动，请手动执行: systemctl start ${service_name}"
	fi
}

install_control() {
	if [[ "$CONTROL_STORE" == "mysql" ]]; then
		require_non_empty "--dsn" "$CONTROL_DSN"
	fi
	require_non_empty "--admin-pass" "$CONTROL_ADMIN_PASS"
	require_non_empty "--jwt-secret" "$CONTROL_JWT_SECRET"
	require_non_empty "--agent-token" "$CONTROL_AGENT_TOKEN"
	require_non_empty "--metrics-token" "$CONTROL_METRICS_TOKEN"

	mkdir -p "$INSTALL_DIR"
	download_binary "$VERSION" "$(detect_asset_name)" "$INSTALL_DIR"
	write_control_env
	write_control_service
	reload_and_start "$CONTROL_SERVICE_NAME"

	cat <<EOF

控制中心部署完成。

常用检查:
  systemctl status ${CONTROL_SERVICE_NAME}
  curl -fsSL http://${CONTROL_HOST}:${CONTROL_PORT}/readyz

建议下一步:
  1. 用 nginx 或 caddy 反代到 ${CONTROL_HOST}:${CONTROL_PORT}
  2. 只对 Prometheus 暴露 /metrics
  3. 尽快导出一次备份并验证 /api/control/backup/export
EOF
}

install_agent() {
	require_non_empty "--control-url" "$AGENT_CONTROL_URL"
	require_non_empty "--token" "$AGENT_TOKEN"
	require_non_empty "--node-secret" "$AGENT_NODE_SECRET"
	require_non_empty "--node-key" "$AGENT_NODE_KEY"
	require_non_empty "--name" "$AGENT_NODE_NAME"
	require_non_empty "--domain" "$AGENT_NODE_DOMAIN"

	mkdir -p "$INSTALL_DIR"
	download_binary "$VERSION" "$(detect_asset_name)" "$INSTALL_DIR"
	write_agent_env
	write_agent_service
	reload_and_start "$AGENT_SERVICE_NAME"

	cat <<EOF

节点 Agent 部署完成。

常用检查:
  systemctl status ${AGENT_SERVICE_NAME}
  journalctl -u ${AGENT_SERVICE_NAME} -f

建议下一步:
  1. 在控制中心确认节点是否已注册并上报心跳
  2. 轮换为独立 node-secret
  3. 验证 sync_users 任务可以正常下发
EOF
}

parse_args() {
	if [[ $# -eq 0 ]]; then
		usage
		exit 1
	fi

	case "$1" in
	control | agent)
		MODE="$1"
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		fail "未知模式: $1"
		;;
	esac

	while [[ $# -gt 0 ]]; do
		case "$1" in
		--version)
			VERSION="$2"
			shift 2
			;;
		--install-dir)
			INSTALL_DIR="$2"
			shift 2
			;;
		--no-start)
			START_NOW=0
			shift
			;;
		--host)
			CONTROL_HOST="$2"
			shift 2
			;;
		--port)
			if [[ "$MODE" == "control" ]]; then
				CONTROL_PORT="$2"
			else
				AGENT_NODE_PORT="$2"
			fi
			shift 2
			;;
		--store)
			CONTROL_STORE="$2"
			shift 2
			;;
		--dsn)
			CONTROL_DSN="$2"
			shift 2
			;;
		--admin-user)
			CONTROL_ADMIN_USER="$2"
			shift 2
			;;
		--admin-pass)
			CONTROL_ADMIN_PASS="$2"
			shift 2
			;;
		--jwt-secret)
			CONTROL_JWT_SECRET="$2"
			shift 2
			;;
		--agent-token)
			CONTROL_AGENT_TOKEN="$2"
			shift 2
			;;
		--metrics-token)
			CONTROL_METRICS_TOKEN="$2"
			shift 2
			;;
		--login-rate-limit)
			CONTROL_LOGIN_RATE_LIMIT="$2"
			shift 2
			;;
		--agent-rate-limit)
			CONTROL_AGENT_RATE_LIMIT="$2"
			shift 2
			;;
		--audit-retention-days)
			CONTROL_AUDIT_RETENTION_DAYS="$2"
			shift 2
			;;
		--task-retention-days)
			CONTROL_TASK_RETENTION_DAYS="$2"
			shift 2
			;;
		--usage-retention-days)
			CONTROL_USAGE_RETENTION_DAYS="$2"
			shift 2
			;;
		--cleanup-interval-minutes)
			CONTROL_CLEANUP_INTERVAL_MINUTES="$2"
			shift 2
			;;
		--node-stale-minutes)
			CONTROL_NODE_STALE_MINUTES="$2"
			shift 2
			;;
		--failed-task-alert-threshold)
			CONTROL_FAILED_TASK_ALERT_THRESHOLD="$2"
			shift 2
			;;
		--pending-task-alert-threshold)
			CONTROL_PENDING_TASK_ALERT_THRESHOLD="$2"
			shift 2
			;;
		--control-url)
			AGENT_CONTROL_URL="$2"
			shift 2
			;;
		--token)
			AGENT_TOKEN="$2"
			shift 2
			;;
		--node-secret)
			AGENT_NODE_SECRET="$2"
			shift 2
			;;
		--node-key)
			AGENT_NODE_KEY="$2"
			shift 2
			;;
		--name)
			AGENT_NODE_NAME="$2"
			shift 2
			;;
		--domain)
			AGENT_NODE_DOMAIN="$2"
			shift 2
			;;
		--service-name)
			if [[ "$MODE" == "control" ]]; then
				CONTROL_SERVICE_NAME="$2"
			else
				AGENT_SERVICE_NAME="$2"
			fi
			shift 2
			;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			fail "未知参数: $1"
			;;
		esac
	done
}

main() {
	parse_args "$@"
	ensure_root
	ensure_command curl
	ensure_command systemctl

	if [[ -z "$VERSION" ]]; then
		info "自动获取最新 release 版本"
		VERSION="$(fetch_latest_version)"
		[[ -n "$VERSION" ]] || fail "获取最新版本失败，请改用 --version 手动指定"
	fi
	info "部署版本: $VERSION"

	case "$MODE" in
	control)
		install_control
		;;
	agent)
		install_agent
		;;
	esac
}

main "$@"
