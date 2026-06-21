# IOA 协议设计

[English](design.md)

## 设计理念

### 语义优先

核心理念：**状态通过语义传达，而非通过预定义结构声明。**

传统方式：协议预定义一组状态（pending/running/complete），参与者从中选择。这要求协议设计者在设计时预见所有可能的状态——现实中做不到。每一种预料之外的状态都变成对协议的「补丁」。

IOA 的方式：参与者在 `content` 中用自然语言或自由结构化数据描述状态、上下文和意图。接收方（AI 或人类）通过理解语义获取信息。协议不预定义任何业务语义，只提供传输语义的机制。

这不是「偷懒不定义 schema」——这是对 AI 能力的正确利用。当接收方有语义理解能力时，预定义结构是冗余约束；当接收方没有语义理解能力时，可通过 `content_schema` 在应用层提供结构化约束。

| 传统协议 | IOA |
|---------|-----|
| 预定义任务状态（5/10/20 种） | 状态是 `content` 中的自然语义，无限种 |
| 预定义字段（优先级、截止时间、指派人...） | 参与者在 `content` 中自由定义 |
| 固定消息类型（request/response/notification） | 消息类型由 `content` 语义和图结构涌现 |
| 结构化元数据承载业务语义 | `content` 承载一切语义，系统字段严格非语义 |

### 参与者等价

协议不区分 Agent 和 Human，两者都是 **Node**——平等的参与者。

传统系统将 Human 视为「审批者」、Agent 视为「执行者」，角色不对称硬编码在协议层。IOA 的立场：**协议只关心地址和消息格式，不关心对方是碳基还是硅基。**

Human 可以像 Agent 一样发起任务；Agent 可以像 Human 一样审批工作。能力差异（响应速度、并发度、专业领域）通过元数据声明，不在协议层强制。

### 极简主义

协议全貌：

| 层级 | 概念 | 数量 |
|------|------|------|
| L0（基础设施） | Space | 1 |
| L1（协议层） | Node, Message, Ref | 3 |
| L2（应用层） | Checkpoint, Team, Handoff, Swarm... | L0+L1 组合涌现 |

暴露给参与者的工具：

| 工具 | 说明 |
|------|------|
| `ioa_space` | 声明进入协作域 |
| `ioa_send` | 写入消息 |
| `ioa_read` | 读取消息 |

参与者只需理解 3 个核心概念和 3 个工具。所有复杂交互模式（审批流、并行分发、DAG 汇聚、任务移交）都由基础元素组合而成。

### 机制与策略分离

协议定义**机制**（mechanism），不定义**策略**（policy）。

- **机制**：Message 通过 Ref 引用其他 Message 和 Node，形成图结构；Space 提供隔离边界
- **策略**：接收者是否接受、超时如何处理、失败如何补偿——由应用层决定

### AI-First

为 AI Agent 的工具调用（tool-use）范式优化：

- **小而完整的工具闭包**：`ioa_space`、`ioa_send`、`ioa_read` 足以完整参与
- **无隐式会话状态**：消息上下文存在于 Space 的 Message Graph 中，不依赖连接状态
- **组合优于记忆**：复杂模式通过组合简单操作实现，AI 不需要记住特殊 API

## 架构

```
L0（基础设施层）  Space                                    — 隔离边界，server 管理
L1（协议层）      Node    Message    Ref                    — 参与者、通信单元、关联
L2（应用层）      Checkpoint  Team  Handoff  Swarm ...      — L0+L1 组合涌现的协作模式
```

- **L0** 是基础设施——隔离、认证、Message Graph 容器。由 server 管理，参与者透明。
- **L1** 是协议核心——参与者（Node）、通信单元（Message）和引用关系（Ref）。参与者直接与 L1 交互。
- **L2** 是协作模式——由 L0+L1 组合涌现，不属于协议本身。

## 核心概念

### Space（L0）— 隔离边界

Space 是 Message Graph 的容器和隔离边界。

```
Space { id: string, name: string }
```

**为什么是 L0**：Space 是一切交互的前提——没有 Space，Message 无处存放，Ref 无处指向。它是基础设施，不是参与者交互的对象。

**关键属性**：

- **按 name 幂等**：同名 Space 多次创建返回同一实例
- **隔离不可破**：`refs.messages` 不能跨 Space。每个 Space 的 Message Graph 自包含
- **声明式加入**：`ioa_space(name, description)` 加入并声明职责，不产生 Message
- **Space 间无直接关系**：协议不定义 Space 之间的父子、桥接、嵌套。跨 Space 通信由 Node 在不同 Space 中发送消息实现

**语义隔离**：Team 在 Space B 中的内部讨论不会污染 Space A 的消息图。Space A 只在 Node 显式发送时才看到结论。

### Node（L1）— 参与者

```
Node { id: string, name: string, meta: object }
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 全局唯一，服务端生成 |
| `name` | string | 是 | 人类可读名称 |
| `meta` | object | 否 | 任意元数据，缺省 `{}` |

- 不区分 Agent 与 Human——`meta.kind` 是元数据，不影响协议行为
- 可同时存在于多个 Space
- `description` 是 per-Space 的（通过 `ioa_space` 声明），不是 Node 的全局属性

### Message（L1）— 通信单元

```
Message { id, sender, created_at, content, refs }
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 全局唯一，服务端生成 |
| `sender` | string | 发送者 Node ID |
| `created_at` | string | 服务端写入时间，RFC3339 UTC |
| `content` | object | 任意结构化载荷，**不可为 null** |
| `refs` | Ref | 引用 |

**公开字段仅此 5 个**。实现可维护内部字段（`space_id`、append position），但不暴露给参与者。

**不可变**：写入后不可修改、不可删除。Space 是 append-only 日志。

### Ref（L1）— 连接机制

```
Ref { messages: string[], nodes: string[] }
```

两种形态，一个机制——Message 上的指针数组：

| 形态 | 指向 | 语义 |
|------|------|------|
| `refs.messages` | Message ID（同 Space） | 因果链，构建图结构 |
| `refs.nodes` | Node ID | 收件人路由 |

均为数组：支持多 parent（DAG 合并）和多收件人。空 `[]` 表示无引用。

**为什么只有两种形态**：Ref → Space 不需要。Space 的通达性通过 `content` 传递（如在消息体中包含 Space ID）。如果 Ref → Space 是协议级字段，协议就被迫定义「引用一个 Space 意味着什么」——违反机制与策略分离。

### Message Graph — 涌现结构

Message 通过 `refs.messages` 自然形成有向图。协议不规定形状，结构从使用中涌现：

```
Root              Thread            Tree               DAG

  [M1]              [M1]             [M1]            [M1]  [M2]
                      ↑              ↗    ↖             ↖  ↗
                     [M2]         [M2]      [M3]         [M3]
                      ↑
                     [M3]
```

| 模式 | 形成方式 | 典型场景 |
|------|---------|---------|
| Root | `refs.messages = []` 且 `refs.nodes = []` | 公共入口点 |
| Thread | 每条消息引用一个 parent | 线性对话 |
| Tree | 多条消息引用同一个 parent | 任务分解 |
| DAG | 一条消息引用多个 parent | 结果汇聚 |

**上下文** = 从任意 Message 沿 `refs.messages` 向上遍历到所有祖先。新加入的 Node 通过回放 Message 历史获取完整上下文——任何时刻的「状态」都是 Message Graph 的投影。

## 操作

### `ioa_space` — 创建或加入

| 参数 | 类型 | 必填 |
|------|------|------|
| `name` | string | 是 |
| `description` | string | 是 |

1. 若调用者 Node 尚未注册，自动注册
2. 按 `name` 创建或获取 Space（幂等）
3. 将调用者加入 Space，记录 `description`（可覆盖更新）
4. 返回 Space 当前状态 + Root Message 列表

### `ioa_send` — 发送消息

| 参数 | 类型 | 必填 |
|------|------|------|
| `space_id` | string | 是 |
| `content` | object | 是 |
| `refs` | Ref | 否 |
| `content_schema` | object | 否 |

1. 自动确保 Node 已注册
2. `content` 不可为 null
3. 若 `content_schema` 存在：验证其为合法 JSON Schema，作为声明式元数据存储（**不校验**任何 Message 的 content）
4. `refs` 缺省为 `{messages:[], nodes:[]}`
5. 校验 refs 合法性（messages 必须存在于同 Space；nodes 必须已注册）
6. 生成 Message ID，append 到 Space
7. 返回完整 Message

### `ioa_read` — 读取消息

| 参数 | 类型 | 必填 |
|------|------|------|
| `space_id` | string | 是 |
| `message_id` | string | 否 |
| `direction` | string | 否 |
| `after` | string | 否 |
| `limit` | int | 否 |
| `all` | bool | 否 |
| `listen` | bool | 否 |

**读取模式**（按优先级）：

| 条件 | 返回 |
|------|------|
| 指定 `message_id` | 关联子图（祖先 + 后代） |
| `all = true` | Space 内全部消息 |
| 调用者身份已知 | `refs.nodes` 包含调用者的消息 |
| 均不满足 | Root Message |

**方向过滤**（配合 `message_id`）：`upstream`（仅祖先）、`downstream`（仅后代），缺省双向。

**SSE 实时监听**：`listen = true` 开启长连接，新消息逐条推送。

**分页**：`after`（游标）+ `limit`（最大条数）。

## 错误

| 类别 | 触发条件 |
|------|---------|
| **not_found** | Node / Space / Message 不存在 |
| **invalid_input** | 参数校验失败：name 为空、content 为 null、refs 引用不存在、limit 非正数、content_schema 非法 |
| **internal** | 实现内部错误 |

## 与现有协议对比

| 维度 | MCP | A2A | IOA |
|------|-----|-----|-----|
| 核心关系 | Model ↔ Tool | Agent ↔ Agent | Node ↔ Space ↔ Node |
| 参与者类型 | 不对称（Host/Client/Server） | 不对称（Client/Remote） | **完全对等** |
| Human 参与 | 不在范围内 | 不在范围内 | **一等公民** |
| 多方协作 | 不支持 | 双方 | **任意多方** |
| 图结构 | 无 | 无 | **Thread / Tree / DAG** |
| 状态模型 | 上下文窗口 | Task 对象 | **Append-only Message Graph** |
| 概念数量 | ~10 | ~8 | **4** |

## 理论基础

| 理论 | 核心洞察 | IOA 对应 |
|------|---------|---------|
| **π-calculus**（Milner 1992） | 通道名可以在通道上传递 | Space ID 可以在 Message 的 content 中传递 |
| **Actor Model**（Hewitt 1973） | 一切皆 Actor；Create/Send/Become | 一切皆 Node；ioa_space/ioa_send/ioa_read |
| **Event Sourcing** | 事件序列是唯一真相 | Message Graph 是唯一真相；状态通过回放重建 |
| **Lamport 因果序**（1978） | 分布式系统中只有因果关系是可靠的时序 | `refs.messages` 建立因果链 |
| **图论** | DAG 拓扑排序保证无环遍历 | 上下文遍历 = DAG 祖先的拓扑排序 |
