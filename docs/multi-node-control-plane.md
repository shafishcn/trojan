# 多节点统一管理设计草案

## 目标

当前项目适合单台 VPS 上的 Trojan 部署、用户管理和 Web 运维。  
多节点改造的目标是把它扩展为一套 `控制中心 + 节点 Agent` 架构，实现：

- 统一管理多台 VPS 节点
- 用户只创建一次，按策略下发到多个节点
- 节点统一上报状态、版本、流量和告警
- 后台统一执行重启、升级、同步配置、拉取日志等操作
- 统一生成订阅和节点分组配置

## 总体架构

```text
管理员 / 前端
      |
      v
控制中心 (Gin + MySQL)
      |
      +-- 用户管理
      +-- 节点管理
      +-- 策略编排
      +-- 任务调度
      +-- 订阅生成
      |
      v
  Agent@VPS-1      Agent@VPS-2      Agent@VPS-3
      |                |                |
      v                v                v
  trojan 本机服务   trojan 本机服务   trojan 本机服务
```

## 角色划分

### 1. 控制中心

控制中心是新的主控服务，负责：

- 管理节点列表、标签、地域和在线状态
- 管理用户、套餐、过期时间、总流量额度
- 管理用户与节点的绑定关系
- 生成待执行任务并等待节点回报结果
- 汇总所有节点的流量、心跳和监控数据
- 统一生成 Trojan / Clash 订阅

### 2. 节点 Agent

Agent 部署在每台 VPS 上，负责：

- 主动向控制中心注册和发送心跳
- 拉取待执行任务并在本机执行
- 读写本机 Trojan 配置
- 调用 systemctl 启停 Trojan 服务
- 上报本机状态、用户流量、版本和任务结果

Agent 侧可以最大化复用当前仓库中的以下能力：

- `trojan/`：安装、启停、更新、切换类型、证书处理
- `core/server.go`：Trojan 配置读写
- `web/controller/common.go`：主机信息采集
- `util/`：系统命令、日志、端口等基础能力

## 当前代码与未来模块映射

| 当前目录 | 现有职责 | 多节点改造后的建议职责 |
| --- | --- | --- |
| `cmd/` | CLI 命令入口 | 保留本地运维 CLI，新增 agent 子命令 |
| `web/` | 单机 Web 面板 | 逐步拆分为控制中心 API 和 agent 本地 API |
| `trojan/` | 本机 Trojan 运维逻辑 | 作为 agent 的本机执行层 |
| `core/mysql.go` | 单机用户库 | 节点本地缓存或执行结果库 |
| `core/leveldb.go` | 单机状态存储 | agent 本地状态、token、任务游标 |

## 核心数据模型

控制中心以 MySQL 为事实源，建议最少包含这些对象：

- `users`：用户基础信息
- `plans`：套餐定义
- `nodes`：节点信息
- `node_tokens`：节点认证信息
- `user_node_bindings`：用户与节点绑定关系
- `tasks`：控制中心下发的任务
- `task_results`：节点执行结果
- `node_heartbeats`：节点最近状态
- `usage_reports`：用户流量上报
- `subscriptions`：订阅模板和策略

对应的 SQL 初版已放在：

- [docs/multi-node-schema.sql](/Users/shafish/Project/Go/trojan/docs/multi-node-schema.sql)

## 建议 API

### 控制中心对外管理接口

#### 节点管理

- `GET /api/control/nodes`
- `POST /api/control/nodes`
- `GET /api/control/nodes/:id`
- `PATCH /api/control/nodes/:id`
- `POST /api/control/nodes/:id/disable`
- `POST /api/control/nodes/:id/enable`

#### 用户管理

- `GET /api/control/users`
- `POST /api/control/users`
- `GET /api/control/users/:id`
- `PATCH /api/control/users/:id`
- `DELETE /api/control/users/:id`
- `POST /api/control/users/:id/bindings`
- `DELETE /api/control/users/:id/bindings/:bindingId`

#### 任务调度

- `POST /api/control/tasks`
- `GET /api/control/tasks`
- `GET /api/control/tasks/:id`

#### 监控与报表

- `GET /api/control/overview`
- `GET /api/control/nodes/:id/usage`
- `GET /api/control/users/:id/usage`

#### 订阅管理

- `GET /api/control/subscriptions/:id`
- `POST /api/control/subscriptions`
- `GET /api/control/users/:id/clash`

### Agent 与控制中心通信接口

#### 注册与认证

- `POST /api/agent/register`
- `POST /api/agent/renew-token`

#### 心跳与状态上报

- `POST /api/agent/heartbeat`
- `POST /api/agent/usage`
- `POST /api/agent/events`

#### 任务拉取与回报

- `GET /api/agent/tasks/pending`
- `POST /api/agent/tasks/:id/start`
- `POST /api/agent/tasks/:id/result`

## 任务模型建议

控制中心不直接远程执行任意命令，而是只下发白名单任务。  
推荐的 `tasks.type`：

- `sync_users`
- `restart_trojan`
- `stop_trojan`
- `start_trojan`
- `rotate_cert`
- `switch_trojan_type`
- `update_binary`
- `refresh_subscription`

每个任务带结构化 payload，例如：

```json
{
  "type": "sync_users",
  "payload": {
    "users": [
      {
        "username": "alice",
        "password": "base64-password",
        "quota": 10737418240,
        "expiryDate": "2026-05-10"
      }
    ]
  }
}
```

## 关键流程

### 1. 节点注册

1. Agent 启动后读取本地标识和预置 token
2. Agent 调用 `/api/agent/register`
3. 控制中心记录节点基础信息和标签
4. 控制中心返回长期 token 或短期访问 token

### 2. 用户下发

1. 管理员在控制中心创建用户
2. 控制中心根据绑定关系生成 `sync_users` 任务
3. 目标节点轮询到任务
4. Agent 在本机同步用户和配置
5. Agent 回报执行状态

### 3. 流量汇总

1. Agent 定时读取本机用户流量
2. 通过 `/api/agent/usage` 上报增量或快照
3. 控制中心按用户和节点聚合展示

## 安全边界

多节点模式下，必须把安全策略前置，而不是沿用单机模式的信任假设：

- 控制中心和 agent 之间使用独立 token 或 mTLS
- 所有执行任务只允许白名单类型，不接受任意 shell
- 所有节点身份都可吊销和轮换
- 任务、心跳、错误日志都要落审计
- 管理员初始化与密码重置接口彻底分离
- 节点侧仅暴露最小接口，尽量使用 agent 主动轮询

## 分阶段实施建议

### Phase 1：抽离本机执行层

目标：

- 把当前 `trojan/` 和 `core/` 中的本机运维逻辑整理成 service 层
- 让 CLI / Web / 未来 agent 都能复用同一套接口

输出：

- `service/node` 或等价目录
- 标准化的执行结果结构体

### Phase 2：最小控制中心

目标：

- 新增节点表、用户表、绑定表
- 新增节点注册、心跳、用户下发

输出：

- 最小控制中心 API
- 节点 Agent 轮询能力

### Phase 3：任务系统

目标：

- 支持重启、升级、同步配置、切换 trojan 类型
- 支持任务状态跟踪、重试和审计

### Phase 4：统一订阅与流量聚合

目标：

- 控制中心统一生成多节点订阅
- 按用户/节点做流量汇总和报表展示

## 下一步建议

如果要继续往代码落地，建议优先做这两项：

1. 新增控制中心 MySQL 表和对应 Go 结构体
2. 抽出节点注册、心跳、任务拉取的最小接口定义
