# 多节点控制中心生产部署指南

这份文档面向已经准备把控制中心落到生产环境的场景，目标是提供一套能直接套用的最小方案。

## 推荐拓扑

- 1 台控制中心主机，运行 `trojan control`
- 1 台 MySQL 实例，独立持久化控制中心数据
- 多台 VPS 节点，运行 `trojan agent`
- 1 个反向代理入口，例如 `nginx` 或 `caddy`
- 1 套监控采集，例如 Prometheus 抓取 `/metrics`

推荐角色拆分：

- 控制中心只暴露 `443`
- MySQL 不暴露公网
- Agent 主动连接控制中心，不开放额外公网入口

## 启动前准备

如果你不想手工创建 `systemd` 和环境变量文件，现在也可以直接使用仓库根目录的：

- [deploy-multi-node.sh](../deploy-multi-node.sh)

它支持：

- `source <(curl -sL https://raw.githubusercontent.com/shafishcn/trojan/master/install.sh) --control ...`
- `source <(curl -sL https://raw.githubusercontent.com/shafishcn/trojan/master/install.sh) --agent ...`

建议至少准备这些密钥：

- `--jwt-secret`
- `--agent-token`
- `--metrics-token`
- 每个节点单独的 `--node-secret`

建议保留的基础配置：

- `--login-rate-limit 30`
- `--agent-rate-limit 600`
- `--audit-retention-days 90`
- `--task-retention-days 30`
- `--usage-retention-days 30`
- `--node-stale-minutes 10`
- `--failed-task-alert-threshold 1`
- `--pending-task-alert-threshold 20`

## MySQL 初始化

创建数据库时建议显式打开 `parseTime=true`：

```text
root:strong-pass@tcp(127.0.0.1:3306)/trojan_control?parseTime=true&charset=utf8mb4
```

控制中心启动后会自动执行 migration，不需要手工导表。

## 控制中心 systemd

如果你希望一键部署而不是手工落文件，可直接执行：

```bash
source <(curl -sL https://raw.githubusercontent.com/shafishcn/trojan/master/install.sh) --control \
  --version v2.15.5 \
  --dsn 'root:pass@tcp(127.0.0.1:3306)/trojan_control?parseTime=true&charset=utf8mb4' \
  --admin-pass 'change-me' \
  --jwt-secret 'jwt-secret' \
  --agent-token 'agent-token' \
  --metrics-token 'metrics-token'
```

可直接参考：

- [docs/examples/control.service](examples/control.service)

部署时重点替换：

- 二进制路径
- 工作目录
- `--dsn`
- `--admin-pass`
- `--jwt-secret`
- `--agent-token`
- `--metrics-token`

推荐做法：

- 先用 `EnvironmentFile=/etc/trojan-control.env`
- 把敏感参数放进 env 文件
- `systemctl daemon-reload && systemctl enable --now trojan-control`

## 节点 Agent systemd

Agent 也可以直接一键部署：

```bash
source <(curl -sL https://raw.githubusercontent.com/shafishcn/trojan/master/install.sh) --agent \
  --version v2.15.5 \
  --control-url 'https://control.example.com' \
  --token 'agent-token' \
  --node-secret 'node-secret-001' \
  --node-key 'node-01' \
  --name 'tokyo-01' \
  --domain 'tokyo.example.com' \
  --port 443
```

可直接参考：

- [docs/examples/agent.service](examples/agent.service)

每个节点至少要有不同的：

- `--node-key`
- `--name`
- `--domain`
- `--node-secret`

## 反向代理

推荐把控制中心挂到 HTTPS 下，再把 `/metrics` 单独做一层限制。

可直接参考：

- [docs/examples/nginx-control.conf](examples/nginx-control.conf)
- [docs/examples/Caddyfile](examples/Caddyfile)

建议策略：

- `/` 和 `/api/` 走主站域名
- `/metrics` 只允许 Prometheus 源地址访问
- 只开放 `443`

如果更偏向自动 HTTPS，可以直接用 Caddy，配置样例已放在：

- [docs/examples/Caddyfile](examples/Caddyfile)

## Prometheus 抓取

可直接参考：

- [docs/examples/prometheus-scrape.yml](examples/prometheus-scrape.yml)

注意：

- 如果 `/metrics` 开了 `metrics token`，抓取端要带 `Authorization: Bearer <token>`
- 如果经反代，Prometheus 应抓代理后的 HTTPS 地址

## 备份与恢复演练

建议每次升级前都执行一轮：

1. 调用 `GET /api/control/backup/export`
2. 保存导出的 JSON 文件
3. 在测试环境启动一个新控制中心
4. 调用 `POST /api/control/backup/import`
5. 验证管理员、节点、用户、绑定、任务和审计是否恢复

建议至少保留：

- 每日一次完整备份
- 最近 7 天热备
- 最近 30 天冷备

仓库里也提供了脚本模板：

- [docs/examples/backup-control-plane.sh](examples/backup-control-plane.sh)
- [docs/examples/restore-control-plane.sh](examples/restore-control-plane.sh)
- [docs/examples/backup-control-plane.cron](examples/backup-control-plane.cron)
- [docs/disaster-recovery-drill.md](disaster-recovery-drill.md)

建议先赋执行权限：

```bash
chmod +x docs/examples/backup-control-plane.sh docs/examples/restore-control-plane.sh
```

备份示例：

```bash
export TROJAN_CONTROL_URL="https://control.example.com"
export TROJAN_CONTROL_TOKEN="your-super-admin-token"
export BACKUP_DIR="/var/backups/trojan-control"

./docs/examples/backup-control-plane.sh
```

恢复示例：

```bash
export TROJAN_CONTROL_URL="https://control.example.com"
export TROJAN_CONTROL_TOKEN="your-super-admin-token"

./docs/examples/restore-control-plane.sh /var/backups/trojan-control/control-backup-20260411-120000.json
```

恢复脚本会自动兼容两种输入：

- 直接保存的 `backup` JSON
- `GET /api/control/backup/export` 的完整响应 JSON

如果你希望直接落到定时任务，可以参考：

- [docs/examples/backup-control-plane.cron](examples/backup-control-plane.cron)

建议把恢复演练也制度化，清单模板见：

- [docs/disaster-recovery-drill.md](disaster-recovery-drill.md)

如果准备把发布升级流程也标准化，可继续参考：

- [docs/release-upgrade-sop.md](release-upgrade-sop.md)
- [docs/rollback-runbook.md](rollback-runbook.md)

## 上线检查清单

- 控制中心和 Agent 都已跑在 `systemd`
- 控制中心前面已有 HTTPS 反代
- MySQL 不暴露公网
- `jwt-secret`、`agent-token`、`metrics-token` 已替换默认值
- 每个节点已启用独立 `node-secret`
- `/readyz`、`/metrics`、`/api/control/alerts/summary` 已接入监控
- 备份导出与导入已至少演练一次

## 升级建议

推荐顺序：

1. 导出控制中心备份
2. 备份 MySQL
3. 在测试环境验证新版本 migration
4. 先升级控制中心
5. 再分批升级 Agent
6. 检查 `/readyz`、`/metrics`、告警摘要和任务下发

如果控制中心升级后异常：

- 优先回滚控制中心二进制
- 用最近备份恢复控制中心状态
- 暂停批量 Agent 升级

更完整的升级和回滚步骤见：

- [docs/release-upgrade-sop.md](release-upgrade-sop.md)
- [docs/rollback-runbook.md](rollback-runbook.md)
