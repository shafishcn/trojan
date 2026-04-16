# trojan
![](https://img.shields.io/github/v/release/Jrohy/trojan.svg) 
![](https://img.shields.io/docker/pulls/jrohy/trojan.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/Jrohy/trojan)](https://goreportcard.com/report/github.com/Jrohy/trojan)
[![Downloads](https://img.shields.io/github/downloads/Jrohy/trojan/total.svg)](https://img.shields.io/github/downloads/Jrohy/trojan/total.svg)
[![License](https://img.shields.io/badge/license-GPL%20V3-blue.svg?longCache=true)](https://www.gnu.org/licenses/gpl-3.0.en.html)


trojan多用户管理部署程序

## 功能
- 在线web页面和命令行两种方式管理trojan多用户
- 启动 / 停止 / 重启 trojan 服务端
- 支持流量统计和流量限制
- 命令行模式管理, 支持命令补全
- 集成acme.sh证书申请
- 生成客户端配置文件
- 在线实时查看trojan日志
- 在线trojan和trojan-go随时切换
- 支持trojan://分享链接和二维码分享(仅限web页面)
- 支持转化为clash订阅地址并导入到[clash_for_windows](https://github.com/Fndroid/clash_for_windows_pkg/releases)(仅限web页面)
- 限制用户使用期限

## 安装方式
*trojan使用请提前准备好服务器可用的域名*  

###  a. 一键脚本安装
```
#安装/更新
source <(curl -sL https://raw.githubusercontent.com/shafishcn/trojan/master/install.sh)

#卸载
source <(curl -sL https://raw.githubusercontent.com/shafishcn/trojan/master/install.sh) --remove
```
安装完后输入'trojan'可进入管理程序   
浏览器访问 https://域名 可在线web页面管理trojan用户  
前端页面源码地址: [trojan-web](https://github.com/shafishcn/trojan-web)

### b. docker运行
1. 安装mysql  

因为mariadb内存使用比mysql至少减少一半, 所以推荐使用mariadb数据库
```
docker run --name trojan-mariadb --restart=always -p 3306:3306 -v /home/mariadb:/var/lib/mysql -e MYSQL_ROOT_PASSWORD=trojan -e MYSQL_ROOT_HOST=% -e MYSQL_DATABASE=trojan -d mariadb:10.2
```
端口和root密码以及持久化目录都可以改成其他的

2. 安装trojan
```
docker run -it -d --name trojan --net=host --restart=always --privileged jrohy/trojan init
```
运行完后进入容器 `docker exec -it trojan bash`, 然后输入'trojan'即可进行初始化安装   

启动web服务: `systemctl start trojan-web`   

设置自启动: `systemctl enable trojan-web`

更新管理程序: `source <(curl -sL https://git.io/trojan-install)`

## 部署说明

下面把当前项目最常见的 3 种部署方式集中说明，尽量做到只看 README 就能直接落地。

### 1. 单机生产部署

适用场景：

- 1 台 VPS 上直接部署 Trojan 服务端
- 通过命令行或内置 Web 面板管理用户
- 不需要统一管理多台节点

部署前准备：

- 一台 Linux 服务器，建议 `root` 用户操作
- 一个已经解析到服务器公网 IP 的域名
- 放通 `80`、`443` 以及你计划给 Web 面板使用的端口
- 如果系统开启了额外安全组、防火墙或云厂商 ACL，也要同步放行

推荐步骤：

1. 安装/更新程序

```bash
source <(curl -sL https://raw.githubusercontent.com/shafishcn/trojan/master/install.sh)
```

2. 安装完成后执行：

```bash
trojan
```

3. 根据交互提示完成：

- 证书申请
- Trojan 服务端初始化
- MySQL/MariaDB 配置
- Web 管理密码初始化

4. 初始化完成后，可通过以下方式验证：

```bash
trojan status
trojan info
trojan web
```

5. 浏览器访问：

```text
https://你的域名
```

部署完成后的常用检查：

- `trojan status`：查看 Trojan 服务状态
- `systemctl status trojan-web`：查看 Web 面板状态
- `trojan log`：查看 Trojan 日志
- `trojan info`：查看用户和流量信息

建议上线后立即做的事情：

- 修改默认管理员密码
- 确认域名证书有效
- 检查 `443` 端口对外可访问
- 做一次 `export` 导出备份
- 开启 BBR 等网络优化

### 单机/多节点一键脚本入口

当前仓库现在额外提供了一个多节点一键部署脚本：

```bash
source <(curl -sL https://raw.githubusercontent.com/shafishcn/trojan/master/install.sh) --help
```

它支持两种模式：

- `--control`：一键部署控制中心二进制、`systemd` 服务和环境变量文件
- `--agent`：一键部署节点 Agent 二进制、`systemd` 服务和环境变量文件

控制中心示例：

```bash
source <(curl -sL https://raw.githubusercontent.com/shafishcn/trojan/master/install.sh) --control \
  --version v2.15.5 \
  --dsn 'root:pass@tcp(127.0.0.1:3306)/trojan_control?parseTime=true&charset=utf8mb4' \
  --admin-pass 'change-me' \
  --jwt-secret 'jwt-secret' \
  --agent-token 'agent-token' \
  --metrics-token 'metrics-token'
```

节点 Agent 示例：

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

脚本执行后会自动完成：

- 下载当前 release 对应架构的二进制
- 写入 `/etc/trojan-control.env` 或 `/etc/trojan-agent.env`
- 写入对应的 `systemd` 服务文件
- 执行 `systemctl daemon-reload`
- 默认 `enable --now` 启动服务

如果你只想生成文件、不想立即启动，可以加：

```bash
--no-start
```

### 2. Docker 部署

适用场景：

- 你更倾向把运行环境封在容器里
- 希望快速拉起 MariaDB 和 Trojan

推荐步骤：

1. 启动 MariaDB：

```bash
docker run --name trojan-mariadb --restart=always -p 3306:3306 \
  -v /home/mariadb:/var/lib/mysql \
  -e MYSQL_ROOT_PASSWORD=trojan \
  -e MYSQL_ROOT_HOST=% \
  -e MYSQL_DATABASE=trojan \
  -d mariadb:10.2
```

2. 启动 Trojan 容器：

```bash
docker run -it -d --name trojan --net=host --restart=always --privileged jrohy/trojan init
```

3. 进入容器初始化：

```bash
docker exec -it trojan bash
trojan
```

4. 初始化完成后启动 Web 服务：

```bash
systemctl start trojan-web
systemctl enable trojan-web
```

Docker 部署注意点：

- `--net=host` 代表容器直接使用宿主机网络
- MariaDB 数据目录一定要做持久化
- 证书、配置和数据库文件建议都映射到宿主机持久目录

### 3. 源码构建部署

适用场景：

- 你在自己维护 fork
- 你需要自定义二进制或二次开发

构建命令：

```bash
go build -ldflags "-w -s" -o trojan .
```

交叉编译示例：

```bash
GOOS=linux GOARCH=amd64 go build -ldflags "-w -s" -o trojan-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -ldflags "-w -s" -o trojan-linux-arm64 .
```

本地调试入口：

```bash
go run .
go run . web
go run . control
go run . agent
```

## 多节点控制中心部署

如果你要把项目用成“控制中心 + 多台 Agent”的统一管理平台，建议直接按下面这条路线部署。

### 推荐拓扑

- 1 台控制中心主机，运行 `trojan control`
- 1 个独立 MySQL 实例，作为控制中心事实库
- 多台 VPS 节点，运行 `trojan agent`
- 1 个 HTTPS 反向代理入口，例如 `nginx` 或 `caddy`
- 1 套 Prometheus 或其他监控系统

推荐原则：

- 控制中心只暴露 `443`
- MySQL 不暴露公网
- Agent 主动连接控制中心，不需要额外开放公网入口

### 部署前准备

控制中心最少需要准备这些敏感配置：

- `--admin-user`
- `--admin-pass`
- `--jwt-secret`
- `--agent-token`
- `--metrics-token`
- 每个节点单独的 `--node-secret`

建议直接启用这些安全和保留参数：

- `--login-rate-limit 30`
- `--agent-rate-limit 600`
- `--audit-retention-days 90`
- `--task-retention-days 30`
- `--usage-retention-days 30`
- `--node-stale-minutes 10`
- `--failed-task-alert-threshold 1`
- `--pending-task-alert-threshold 20`

### MySQL 准备

控制中心建议使用带 `parseTime=true` 的 DSN：

```text
root:strong-pass@tcp(127.0.0.1:3306)/trojan_control?parseTime=true&charset=utf8mb4
```

控制中心启动时会自动执行 schema migration，不需要手工导表。

### 控制中心启动示例

内存版原型：

```bash
go run . control \
  --host 0.0.0.0 \
  --port 8081 \
  --admin-user admin \
  --admin-pass change-me \
  --agent-token agent-secret \
  --metrics-token metrics-secret \
  --login-rate-limit 30 \
  --agent-rate-limit 600 \
  --audit-retention-days 90 \
  --task-retention-days 30 \
  --usage-retention-days 30
```

MySQL 版：

```bash
go run . control \
  --store mysql \
  --dsn 'root:pass@tcp(127.0.0.1:3306)/trojan_control?parseTime=true' \
  --admin-user admin \
  --admin-pass change-me \
  --jwt-secret replace-me \
  --agent-token agent-secret \
  --metrics-token metrics-secret \
  --login-rate-limit 30 \
  --agent-rate-limit 600 \
  --audit-retention-days 90 \
  --task-retention-days 30 \
  --usage-retention-days 30
```

启动后建议优先验证：

- `GET /readyz`
- `GET /metrics`
- `GET /api/control/runtime/status`
- `GET /api/control/alerts/summary`

### 节点 Agent 启动示例

```bash
go run . agent \
  --control-url https://control.example.com \
  --token agent-secret \
  --node-secret node-secret-001 \
  --node-key node-01 \
  --name tokyo-01 \
  --domain hk.example.com \
  --port 443
```

每个节点至少保证这些值唯一：

- `--node-key`
- `--name`
- `--node-secret`

### 生产环境推荐做法

控制中心建议直接跑在 `systemd` 下，示例文件见：

- [控制中心 systemd 样例](docs/examples/control.service)
- [节点 Agent systemd 样例](docs/examples/agent.service)
- [多节点一键部署脚本](deploy-multi-node.sh)

反向代理建议至少配置 HTTPS，并限制 `/metrics` 暴露范围，示例见：

- [nginx 反向代理样例](docs/examples/nginx-control.conf)
- [Caddy 反向代理样例](docs/examples/Caddyfile)

监控抓取示例见：

- [Prometheus 抓取样例](docs/examples/prometheus-scrape.yml)

### 备份、恢复与升级

控制中心已经内置：

- `GET /api/control/backup/export`
- `POST /api/control/backup/import`
- `POST /api/control/maintenance/cleanup`

推荐每次升级前至少做一次：

1. 导出控制中心备份
2. 备份 MySQL
3. 在测试环境做导入恢复演练
4. 再升级控制中心和 Agent

仓库里已经提供了这些运维材料：

- [控制中心生产部署指南](docs/production-deployment.md)
- [控制中心备份脚本模板](docs/examples/backup-control-plane.sh)
- [控制中心恢复脚本模板](docs/examples/restore-control-plane.sh)
- [控制中心备份 cron 样例](docs/examples/backup-control-plane.cron)
- [控制中心恢复演练清单](docs/disaster-recovery-drill.md)
- [控制中心发布升级 SOP](docs/release-upgrade-sop.md)
- [控制中心回滚手册](docs/rollback-runbook.md)

### 上线检查清单

单机模式建议至少确认：

- 域名解析正确
- 证书签发成功
- `trojan status` 正常
- Web 面板可以登录
- 用户创建、删除、流量限制功能正常

多节点模式建议至少确认：

- 控制中心前面已有 HTTPS
- MySQL 不暴露公网
- `/readyz`、`/metrics`、告警摘要已经接入监控
- 每个节点都已经启用独立 `node-secret`
- 备份导出和导入至少演练过一次

## 运行截图
![avatar](asset/1.png)
![avatar](asset/2.png)

## 命令行
```
Usage:
  trojan [flags]
  trojan [command]

Available Commands:
  add           添加用户
  agent         启动多节点节点代理
  clean         清空指定用户流量
  completion    自动命令补全(支持bash和zsh)
  control       启动多节点控制中心原型
  del           删除用户
  help          Help about any command
  info          用户信息列表
  log           查看trojan日志
  port          修改trojan端口
  restart       重启trojan
  start         启动trojan
  status        查看trojan状态
  stop          停止trojan
  tls           证书安装
  update        更新trojan
  updateWeb     更新trojan管理程序
  version       显示版本号
  import [path] 导入sql文件
  export [path] 导出sql文件
  web           以web方式启动

Flags:
  -h, --help   help for trojan
```

## 注意
安装完trojan后强烈建议开启BBR等加速: [one_click_script](https://github.com/jinwyp/one_click_script)  

## 多节点规划

如果要把当前项目扩展为多 VPS 的统一控制中心，可参考以下设计草案：

- [多节点统一管理设计草案](docs/multi-node-control-plane.md)
- [多节点控制中心 MySQL 初版表结构](docs/multi-node-schema.sql)
- [多节点控制中心生产部署指南](docs/production-deployment.md)
- [控制中心 systemd 样例](docs/examples/control.service)
- [节点 Agent systemd 样例](docs/examples/agent.service)
- [nginx 反向代理样例](docs/examples/nginx-control.conf)
- [Caddy 反向代理样例](docs/examples/Caddyfile)
- [Prometheus 抓取样例](docs/examples/prometheus-scrape.yml)
- [控制中心备份脚本模板](docs/examples/backup-control-plane.sh)
- [控制中心恢复脚本模板](docs/examples/restore-control-plane.sh)
- [控制中心备份 cron 样例](docs/examples/backup-control-plane.cron)
- [控制中心恢复演练清单](docs/disaster-recovery-drill.md)
- [控制中心发布升级 SOP](docs/release-upgrade-sop.md)
- [控制中心回滚手册](docs/rollback-runbook.md)

当前仓库也已经提供一个最小控制中心原型：

```bash
# 内存版
go run . control --host 0.0.0.0 --port 8081 --admin-user admin --admin-pass change-me --agent-token agent-secret --metrics-token metrics-secret --login-rate-limit 30 --agent-rate-limit 600 --audit-retention-days 90 --task-retention-days 30 --usage-retention-days 30

# MySQL版
go run . control --store mysql --dsn 'root:pass@tcp(127.0.0.1:3306)/trojan_control?parseTime=true' --admin-user admin --admin-pass change-me --agent-token agent-secret --metrics-token metrics-secret --login-rate-limit 30 --agent-rate-limit 600 --audit-retention-days 90 --task-retention-days 30 --usage-retention-days 30

# 节点 agent
go run . agent --control-url http://127.0.0.1:8081 --token agent-secret --node-secret node-secret-001 --node-key node-01 --name tokyo-01 --domain hk.example.com --port 443
```

控制中心新增的多节点用户接口包括：

- `GET /readyz`
- `GET /metrics`
- `GET /api/control/overview`
- `GET /api/control/runtime/status`
- `GET /api/control/alerts/summary`
- `GET/POST /api/control/users`
- `GET/DELETE /api/control/users/:username`
- `POST /api/control/users/:username/bindings`
- `DELETE /api/control/users/:username/bindings/:nodeKey`
- `GET /api/control/users/:username/nodes`
- `GET /api/control/users/:username/usage`
- `GET /api/control/users/:username/subscription/clash`
- `GET /api/control/users/:username/subscription/links`
- `GET /api/control/tasks?nodeKey=&taskType=&status=&limit=`
- `GET /api/control/tasks?nodeKey=&taskType=&status=&limit=&offset=`
- `GET /api/control/tasks/:id`
- `GET /api/control/audit?actor=&action=&resourceType=&limit=&offset=`
- `POST /api/control/nodes/:nodeKey/agent-secret/rotate`
- `POST /api/control/maintenance/cleanup`
- `GET /api/control/backup/export`
- `POST /api/control/backup/import`
- `POST /api/control/auth/login`
- `GET /api/control/auth/me`
- `GET /api/control/admins`
- `POST /api/control/admins`
- `PATCH /api/control/admins/:username`
- `POST /api/control/nodes/:nodeKey/sync`
- `POST /api/control/tasks/:id/retry`

控制中心管理台根路径 `/` 现在也内置了最小运维界面，支持：

- 管理员账号登录和 JWT 会话
- `viewer / admin / super_admin` 三档角色
- MySQL 启动时自动执行版本化 schema migration，记录在 `control_schema_migrations`
- 节点支持独立 `node-secret` HMAC 签名鉴权，可由控制中心轮换
- 登录接口和 Agent API 支持按分钟限流，可通过 `--login-rate-limit` / `--agent-rate-limit` 调整，设为负数可关闭
- 任务列表和审计日志支持 `offset + limit` 分页
- 启动后支持按保留天数自动清理审计日志、已完成任务/事件和 usage 快照，也支持手动触发清理
- 支持 `super_admin` 导出和导入控制中心备份，覆盖管理员、节点、用户、绑定、任务、审计和 usage 状态
- 提供 `/readyz` 就绪探针和运行状态接口，便于接入反向代理、容器健康检查和监控
- 提供 Prometheus 风格 `/metrics` 指标接口，可用独立 `metrics token` 保护
- 提供告警摘要接口，可基于节点失联、失败任务和待处理积压做最小告警判定
- 节点、用户、任务总览
- 任务状态筛选
- 单任务详情和审计事件时间线
- 控制中心操作审计查询
- 管理员列表、管理员创建和角色/状态调整
- 用户绑定、解绑、同步和订阅下载

## Thanks
感谢JetBrains提供的免费GoLand  
[![avatar](asset/jetbrains.svg)](https://jb.gg/OpenSource)
