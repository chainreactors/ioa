# 多参与者通信协议设计

> 一个极简的通用协议，实现任意 Agent 与 Human 之间的通信、协调与移交。
> 设计目标：用最少的概念获得最大的能力。
> 实现/API/场景见 [protocol-implement.md](protocol-implement.md)。

---

## 1. 设计理念

### 1.1 参与者等价

协议不区分 Agent 和 Human。两者都是 **Node**——平等的参与者。

传统系统将 Human 视为"审批者"、Agent 视为"执行者"，这种角色不对称硬编码在协议层，导致拓扑僵化。本协议的立场是：**协议只关心对方的地址和消息格式，不关心对方是碳基还是硅基**。

一个 Human 可以像 Agent 一样发起任务；一个 Agent 可以像 Human 一样审批工作。能力差异（响应速度、并发度、专业领域）通过元数据声明，不在协议层强制。

这与 Carl Hewitt 1973 年提出的 Actor 模型哲学完全一致："一切皆 Actor"——你无法从外部区分一个 Actor 是简单的计数器还是复杂的决策系统。

### 1.2 极简主义

协议的全部概念：

| 层级 | 概念 | 数量 |
|------|------|------|
| L0（基础设施） | Space | 1 |
| L1（协议层） | Node, Message, Ref（2 种形态：→Message, →Node） | 3 |
| L2（应用层） | Checkpoint, Team, Handoff, Swarm... | 由 L0+L1 组合 |

对 Agent/Human 暴露的最小工具面：

| 类型 | 工具 | 说明 |
|------|------|------|
| 核心工具 | `space`, `send`, `read` | 创建/进入域、写入消息、读取消息 |

一个基础设施、三个核心概念、三个核心工具——这是整个协议。所有复杂的交互模式（审批流、并行分发、DAG 汇聚、任务移交）都由这些基础元素组合而成，不需要额外的协议级概念。L2 应用层的模式（Checkpoint、Handoff 等）完全由 L0+L1 的机制组合涌现，不侵入协议本身。

### 1.3 机制与策略分离

协议定义**机制**（mechanism），不定义**策略**（policy）。

- 机制：Message 通过 Ref 引用其他 Message 和 Node，形成图结构；Space 提供隔离与认证边界
- 策略：接收者是否接受、超时如何处理、失败如何补偿——这些由应用层决定

这与 Unix 的设计哲学一脉相承：内核提供进程、文件描述符、信号等机制，策略由用户空间程序定义。

### 1.4 AI-First

协议为 AI Agent 的工具调用（tool-use）范式优化：

- **小而完整的工具闭包**：AI 只需要 `space`、`send`、`read` 就能完整参与
- **无隐式会话状态**：消息上下文存在于 Space 的 Message 图中；参与者身份在 `space(name, description)` 中显式声明
- **组合优于记忆**：复杂模式通过组合简单操作实现，AI 不需要记住特殊 API

### 1.5 Message Graph 是涌现结构

Message 通过 `refs.messages` 自然形成有向图——**协议提供机制（引用），结构从使用中涌现**。

- 线性链 = Thread（每条消息引用一个 parent）
- 分叉 = Tree（多条消息引用同一个 parent）
- 合并 = DAG（一条消息引用多个 parent）

协议不规定用哪种结构——它们都是 `refs.messages` 的自然组合。应用层可以根据场景自由选择：对话用 Thread，任务分解用 Tree，结果汇聚用 DAG。

---

## 2. 理论基础

本协议的设计建立在三个经过数十年验证的理论基础之上。完整的调研分析见 [通信范式调研报告](../internal/research-communication-paradigms-before-agents.md)。

### 2.1 π-calculus — 通道可以传递通道

Robin Milner 1992 年提出的 π-calculus 对经典进程代数做了一个看似微小却意义深远的扩展：**允许通道名作为消息在通道上传递**。

```
传统 CSP:   A --[data]--> B          通道是静态的，拓扑在编译时确定
π-calculus: A --[channel_name]--> B   通道本身可以被传递，拓扑可以动态重组
```

**对应关系**：

| π-calculus | 本协议 |
|------------|--------|
| 通道（channel） | Space |
| (ν x) 创建新通道 | `space()` 创建新 Space |
| 在通道上发送通道名 | Message 的 content 中包含 Space ID + join_token = 传递通达性 |
| 进程 | Node |

在本协议中，Space 的通达性通过 Message 的 `content` 字段传递 Space ID 与 join_token 实现——效果完全等价于 π-calculus 中的通道传递。持有 join_token 即拥有加入 Space 的能力，如同知道通道名即可通信。

### 2.2 Actor Model — 一切皆参与者

Carl Hewitt 1973 年提出的 Actor 模型规定：每个 Actor 收到消息后可以且仅可以做三件事：

| Actor 原语 | 含义 | 本协议对应 |
|-----------|------|-----------|
| **Create** | 创建新 Actor | `space()` 创建新域 |
| **Send** | 向其他 Actor 发消息 | `send()` 发送消息 |
| **Become** | 改变自己处理下一条消息的行为 | `read(...)` 观察 → 改变行为 |

Hewitt 的核心洞察是：你不需要知道收到消息的对方是简单的数据容器还是复杂的决策系统——你只需要知道它的地址和它接受的消息协议。

这正是本协议的参与者等价性：Node 不区分 Agent 和 Human，只需要知道对方在哪个 Space、能收什么 Message。

### 2.3 Event Sourcing — 事件序列是唯一真相

Event Sourcing 的核心原则：**不存储当前状态，只存储导致当前状态的所有事件序列**。当前状态通过回放事件重建。

在本协议中，Space 内的 Message 图就是这个"事件日志"：

- 新加入的 Node 通过回放 Message 历史获取完整上下文
- 任何时刻的"状态"都是 Message 图的投影（projection）
- Ref 的完整链条可追溯——DAG 中每个节点的完整因果链可通过遍历 `refs.messages` 重建
- 不同 Node 可以对同一个 Space 维护不同的投影/视图

---

## 3. 基本抽象

### 3.1 分层模型

```
L0（基础设施层）: Space                          ← 一切发生的域
L1（协议层）:     Node    Message    Ref          ← 参与者、通信单元、关联
L2（应用层）:     Checkpoint  Team  Handoff  Swarm  ...  ← L0+L1 组合涌现的协作模式
```

L0 是土地——隔离、认证、消息图的容器。L1 是建筑——参与者、通信单元和它们之间的引用关系。L2 是活动——由 L0+L1 机制组合涌现的协作模式，不属于协议本身。

**为什么 Space 独立为 L0**：

Space 不是与 Node、Message 平级的"又一种实体"。它是一切交互的**前提条件**——没有 Space，Node 无处发送 Message，Ref 无处指向。Space 提供隔离边界和认证边界，是协议的基础设施层。将它提升到 L0，明确了这种先决关系。

### 3.2 L0：Space

Space 是隔离与认证边界——Message Graph 存在的域。

```
Space {
    id:         string      // 全局唯一标识
    name:       string?     // 可选：人类可读名称
    join_token: string      // 创建时生成，持有者可加入
}
```

读取 Space 初始化信息时返回一个只读投影：

```
SpaceInfo {
    id:            string
    name:          string
    nodes:         { id: string, name: string, description: string }[]
    message_count: number
}
```

`SpaceInfo` 不是新的协议实体，只是 Space 当前状态的读取视图。它不返回 Message 列表；外部观察者需要查看消息时使用 `GET /spaces/{id}/messages`。

**设计决策**：

Space 是 Message Graph 的容器、隔离边界和认证边界。一个 Space 内的所有 Message 构成一个自包含的有向图。

- **隔离**：Space 之间的 Message 不互通。`refs.messages` 不能跨 Space。这保证了每个 Space 的图结构是完整的、自包含的
- **初始化摘要**：Node 调用 `space(name, description)` 后会得到 `SpaceInfo`，看到已加入的 Node 和 Message 总数
- **起点消息**：每个 `refs.messages = []` 且 `refs.nodes = []` 的 Message 都是公共图入口；协议不为它单独建模
- **显式加入内置在 space 中**：`space(name, description)` 只更新 Space 内的 Node 描述，不产生 Message，也不改变 Message Graph
- **准入控制**：Space 创建时生成 `join_token`，creator 获取后分发。持有 token 的 Node 可以加入 Space；加入后通过 membership 校验，不再依赖 token。token 可放入 Message 的 content 中传递——与 Space ID 共享机制一致
- **任意 Node 可创建 Space**：Space 由参与者按需创造，不需要中心授权。对应 π-calculus 的 `(ν x)` 操作

### 3.3 L1：Node

Node 是协议中的参与者。Agent 和 Human 都是 Node。

```
Node {
    id:   string    // sha256(secret) — 由持有者的 secret 派生
    name: string    // 人类可读名称
    meta: object    // 可选全局元数据（类型、能力...）
}
```

**身份派生**：

Node 的身份不由 server 分配，而是从 `secret` 确定性派生：

```
secret（持有者生成） → node_id = sha256(secret).hex()
```

- Node 持有 `secret`，存储在本地 `.auth` 文件中
- `node_id` 是 `secret` 的 SHA-256 哈希——公开、确定性、不可逆
- Server 只存储 `node_id`，从不接触 `secret` 本身
- 认证：请求携带 `secret`，server 哈希后与 `node_id` 比对

身份是自主的：Node 自己生成 secret，自己派生 id，向 server 宣告存在。Server 是身份的登记处，不是颁发者。

**`.auth` 文件**：

```json
{"secret": "a1b2c3d4..."}
```

Client 初始化时读取 `.auth`，自动在每个请求中携带 secret。Agent 不需要理解认证机制——`.auth` 存在即可通信，不存在则无法参与。

**设计决策**：

- `meta` 中可以包含 `kind: "agent" | "human"`，但这是元数据而非协议约束——协议不会因为 kind 不同而改变行为
- Node 可以同时存在于任意多个 Space 中
- Node 通过 `space(name, description)` 创建或进入某个 Space，并为该 Space 声明自己的职责描述
- `description` 属于 Node 和 Space 的关系，不属于全局 Node；同一个 Node 可以在不同 Space 中使用不同 description
- `refs.nodes` 是收件人路由，不代表被引用的 Node 已经加入该 Space
- 协议不暴露单独的 Presence/Membership 类型；加入关系是 Space 的内部状态，通过 `space` 返回的 `SpaceInfo` 以最小字段读取

### 3.4 L1：Message

Message 是不可变的通信单元，也是 Message Graph 中的节点。

```
Message {
    id:      string      // 全局唯一标识
    sender:  string      // 发送者 Node ID
    content: object      // 任意结构化载荷
    refs: {              // L1：对其他实体的引用
        messages: string[]  //   → Message IDs（图结构：parent 链接）
        nodes:    string[]  //   → Node IDs（收件人路由）
    }
}
```

服务端内部可以维护 `space_id`、append position 等系统字段，但这些不是暴露给 Node 的 Message 协议字段。

**设计决策**：

- **不可变**：Message 一旦发送不可修改、不可删除。Space 是 append-only 日志
- **最小暴露**：公开 Message 只有 `id`、`sender`、`content`、`refs`。`space_id` 来自请求路径，append position 属于系统内部元数据
- **创建者清晰**：`sender` 就是这条 Message 的创建者。需要追责或继续沟通时，找 `sender` 对应的 Node
- **`refs.messages` 构建图结构**：通过引用 parent 消息形成有向图。线性链（Thread）、分叉（Tree）、合并（DAG）都是自然涌现的模式
- **`refs.nodes` 是收件人路由**：设置 `refs.nodes` 表示"这条消息发送给这些 Node"，接收方可以通过 `read(space_id)` 直接读取发给自己的消息
- **`content` 完全自由**：协议不限制 content 的 schema。应用层如何利用 content 构建协作模式（如 Checkpoint、Handoff）属于 L2
- **同 Space 约束**：`refs.messages` 只能引用同一 Space 内的 Message，保证每个 Space 内的图是自包含的
- **数组化**：`refs.messages` 和 `refs.nodes` 都是数组，支持一条 Message 引用多个 parent（DAG 合并）或发送给多个 Node

### 3.5 L1：Ref

Ref 是 Message 对其他 L0/L1 实体的引用——**连接的机制**。

L0 定义了 Space，L1 定义了 Node 和 Message，但 Ref 只有两种形态：

| Ref 形态 | 指向 | 语义 |
|---------|------|------|
| Ref → Message | Message ID（同 Space） | 构建图结构（Thread / Tree / DAG） |
| Ref → Node | Node ID | 收件人路由 |

**两种形态，一个机制**：

在协议层面，两种 Ref 的机制完全相同——都是 Message 上的指针数组。语义差异来自目标实体的类型，而非 Ref 本身：

```
Ref → Message:  构建图结构，形成因果链
Ref → Node:     收件人路由，形成 Node 在 Space 内的 inbox
```

**数组化**：

每种 Ref 都是数组，一条 Message 可以同时引用多个 parent 或多个目标 Node：

```python
# DAG 合并：汇总三个子任务的结果
send(space_id="s1",
     content={"summary": "综合三个子任务的结果"},
     refs={"messages": [result_1.id, result_2.id, result_3.id]})

# 发送给多个 Node
send(space_id="s1",
     content={"alert": "发现关键漏洞"},
     refs={"nodes": [agent_a.id, agent_b.id, human.id]})
```

**可组合**：

一条 Message 可以同时携带两种 Ref：

```python
# 回复某条消息并定向给某人
send(space_id="s1",
     content={"task": "请接手审查"},
     refs={"messages": [msg_42.id], "nodes": [agent_b.id]})
```

### 3.6 Message Graph

Message 通过 `refs.messages` 构成有向图（Directed Graph）。这不是一个额外的协议概念——它是 Message + Ref 自然涌现的结构。

```
四种基本模式：

1. Root（根）        2. Thread（线性链）     3. Tree（分叉）        4. DAG（合并）

   [M1]                [M1]                   [M1]                  [M1]  [M2]
                         ↑                    ↗    ↖                  ↖  ↗
                        [M2]              [M2]      [M3]              [M3]
                         ↑
                        [M3]
```

- **Root**：`refs.messages = []` 且 `refs.nodes = []`。它不引用 parent，也不发送给特定 Node，是公共图入口点
- **Thread**：每条 Message 引用恰好一个 parent，形成线性对话链
- **Tree**：多条 Message 引用同一个 parent，形成分支结构（如任务分解）
- **DAG**：一条 Message 引用多个 parent（`refs.messages = [m1, m2, ...]`），形成合并点（如结果汇聚）

**Context（上下文）** = 从任意 Message 沿 `refs.messages` 向上遍历到所有祖先。对于 Thread 是线性链，对于 DAG 是拓扑排序后的全部祖先节点。

协议定义机制（refs 形成图），模式从使用中涌现。应用层不需要声明"这是一个 Thread"或"这是一个 DAG"——结构自然呈现。

### 3.7 L2：应用层

L2 是在 L0（Space）和 L1（Node、Message、Ref）之上，由这些机制组合涌现出的协作模式。

**L2 不是协议的一部分**。它不引入新的实体、新的 Ref 类型、也不约定 `content` 中必须存在某个字段。L2 模式完全建立在协议已有的三个能力之上：

1. **content 自由**：`content` 是任意 object，应用层自由决定其内部结构
2. **content_schema 可选校验**：Space 可以关联 JSON Schema，约束 content 格式——但这是应用层的选择，不是协议的要求
3. **Ref 组合**：`refs.messages`（因果链）和 `refs.nodes`（路由）可以自由组合，构建任意交互流程

```
协议保证:  content 是任意 object，refs 支持图结构和路由
L2 利用:   content 承载语义，refs 组织交互流程
协议不知道: "checkpoint" 或 "handoff" 意味着什么
```

L2 模式对协议是透明的——协议只看到 Message、Ref、Space，不知道也不关心 content 内部的语义。新的 L2 模式可以随时创建，不需要修改协议。

以下以现有的三个 L2 模式为例，展示 L2 模式如何从 L0+L1 的组合中涌现。

#### 3.7.1 Checkpoint — 审批门

Checkpoint 是可审阅的决策点：工作流中需要显式反馈才能继续的边界。

**组合方式**：

| L0+L1 机制 | Checkpoint 如何使用 |
|-----------|-------------------|
| Space | 每个 Workspace 一个独立 Space（`aide.checkpoints.{ws}`），隔离不同工作流 |
| content | 承载 checkpoint 的语义载荷（type、title、content、options、feedback） |
| refs.messages | feedback 引用对应的 checkpoint，形成 submitted → feedback 消息对 |
| refs.nodes | 不使用——审阅者通过 `read(space_id, all=true)` 主动发现 |
| content_schema | 可选：约束 Space 内的 content 格式一致性 |

**交互流程**：

```
Action Node                    Space                        Eval Node / Human
    │                            │                               │
    ├─ send(提交 checkpoint) ────►│                               │
    │   refs.messages=[parent]   │                               │
    │                            │◄── read() ─────────────────────┤
    │                            │                               │
    │                            │◄── send(提供 feedback) ────────┤
    │                            │    refs.messages=[cp_msg_id]   │
    │◄── read() ─────────────────┤                               │
    │   获取 feedback，继续工作    │                               │
```

**核心模式**：提交 → 反馈的消息对，通过 `refs.messages` 关联。checkpoint 的"当前状态"（是否已审阅、反馈内容）通过回放消息对重建——体现 Event Sourcing 原则。

审阅者可以是 AI（自动评估）或 Human（工作流暂停，等待 Human 通过 UI 反馈后继续），这是应用层的策略选择，不影响协议层的消息结构。

#### 3.7.2 Team — 多方通信

Team 是 Workspace 内多个 Agent/Human 之间的广播通信模式。

**组合方式**：

| L0+L1 机制 | Team 如何使用 |
|-----------|--------------|
| Space | 每个 Team 一个独立 Space（`aide.team.{ws}.{team}`），成员通过 `space()` 加入 |
| content | 承载消息载荷（text、sender workspace/task 上下文） |
| refs.messages | 不使用——消息按 append 顺序形成时间线 |
| refs.nodes | 不使用——所有 Space 成员均可见（广播语义） |

**交互流程**：

```
Agent A                        Space                            Agent B
    │                            │                               │
    ├─ space(join) ──────────────►│◄── space(join) ───────────────┤
    │                            │                               │
    ├─ send(消息) ───────────────►│                               │
    │                            │◄── read() ─────────────────────┤
    │                            │   看到 Agent A 的消息           │
    │                            │                               │
    │                            │◄── send(回复) ─────────────────┤
    │◄── read() ─────────────────┤                               │
```

**核心模式**：共享 Space + 广播消息。Team 是最简单的 L2 模式——不使用任何 Ref 机制，仅利用 Space 的隔离和 `read(all=true)` 的全量读取。

成员发现通过 Space 的 `nodes` 列表实现；成员可以通过 `meta` 声明角色和能力，供其他成员检索。

#### 3.7.3 Handoff — 任务移交

Handoff 是纯语义的跨 Node 任务转移：发送方将工作上下文传递给接收方，然后不再关心。

**组合方式**：

| L0+L1 机制 | Handoff 如何使用 |
|-----------|-----------------|
| Space | 移交发生在共享 Space 中（`aide.handoff.{ws}`） |
| content | 纯语义载荷：title、brief（工作摘要）、source（来源上下文）。所有字段可选，支持任意扩展 |
| refs.messages | 不使用——handoff 是独立消息，不形成因果链 |
| refs.nodes | **核心路由机制**——决定谁收到移交消息 |
| content_schema | Pydantic 模型导出 JSON Schema，设置到 Space 上做结构校验 |

**交互流程**：

```
Source Node                    Space                      Target Node
    │                            │                               │
    ├─ send(移交) ───────────────►│                               │
    │   refs.nodes=[target_id]   │                               │
    │   (fire and forget)        │◄── inbox poll ─────────────────┤
    │                            │   系统代码注入到 Agent 对话      │
    │   已不再关心                 │                               │
```

**纯语义设计**：Handoff 没有 accept/complete 生命周期——发送即遗忘。接收方不需要确认、不需要汇报完成。发送方交接后移开，接收方自主决定如何行动。

**接收方式**：接收端由系统代码（HandoffChannel）硬编码实现，通过 inbox 轮询发现消息后注入 Agent 的对话输入管道——Agent 看到的是一条被 `<handoff>` 标签包裹的普通消息。

**三种路由模式**（由 `refs.nodes` 自然涌现）：

| 模式 | refs.nodes | 语义 |
|------|-----------|------|
| 定向移交 | `[target_id]` | 发送给指定 Node |
| 广播移交 | `[]` | Space 内所有 Node 可见 |
| 多候选移交 | `[a_id, b_id, c_id]` | 多个 Node 均可见 |

**上下文传递的两层模型**：

- **Brief（结构化摘要）**：自包含的工作摘要（目标、进展、发现、待办、约束）。接收方可以立即开始工作
- **Source（深度恢复入口）**：指向来源 workspace、task、session。接收方可以通过 `read(source.space_id)` 回溯完整上下文

#### 3.7.4 Swarm — 协同作战

Swarm 是 1:N 分布式 agent 协同作战模式：一个 coordinator（Human/Claude）指挥 N 个异构 agent 在同一个 Space 中协作。

**组合方式**：

| L0+L1 机制 | Swarm 如何使用 |
|-----------|---------------|
| Space | 一个 Space = 一个作战域（`aiscan.swarm.{case}`） |
| content | 自然语言为主，三个字段：`content`（自然语言）、`targets`（结构化目标）、`meta`（发送者元数据） |
| refs.messages | Report 引用 Task，形成 task→report 因果链 |
| refs.nodes | coordinator 用于定向分发 Task 给特定 agent |
| content_schema | 统一 schema 覆盖所有消息（Task 和 Report 共用同一个 schema） |

**Schema（单一、固定）**：

```json
{
  "content": "自然语言指令或回报",
  "targets": ["10.0.0.0/24"],
  "meta": {"ip": "10.0.0.5", "capabilities": ["gogo", "spray"]}
}
```

所有字段除 `content` 外均可选。消息的语义由 IOA 机制决定——root message 是 Task，refs.messages 引用 Task 的是 Report。不需要 `type` 字段。

**交互流程**：

```
Coordinator                    Space                      Agent A / Agent B
    │                            │                               │
    │                            │◄── send(announce + meta) ─────┤ (root, agent 上线)
    │                            │                               │
    ├─ send(task + targets) ─────►│                               │
    │   refs.nodes=[A]           │◄── read() ─────────────────────┤
    │                            │                               │
    │                            │◄── send(report) ──────────────┤
    │                            │    refs.messages=[task_id]     │
    │◄── read() ─────────────────┤                               │
```

**核心特征**：自然语言为主，结构化字段极简（targets + meta）。AI agent 之间的交流本身就是自然语言——扫描参数、发现结果、状态判断都在 `content` 中用自然语言表达。`targets` 提供确定性的目标列表供系统代码路由和分割，`meta` 让 agent 在消息中声明自身上下文（IP、网络位置、能力）。

---

## 4. 设计决策记录

以下记录了设计过程中的关键决策及其理由。

### 4.1 为什么 Space 是 L0

旧设计将 Space 与 Node、Message 并列为 L1 实体。新设计将 Space 提升为 L0 基础设施层。

**Space 是一切交互的前提**：

1. **Node 需要 Space 才能通信**——没有 Space，Message 无处存放，Ref 无处指向
2. **Space 是隔离和认证的边界**——它不是与 Node 平等的"参与者"，而是参与者活动的"场所"
3. **对应 π-calculus 的通道**——通道是进程通信的基础设施，不是进程本身

将 Space 下沉到 L0，明确了这种"场所 vs 参与者"的区分。协议的其他一切——Node 的加入、Message 的发送、Ref 的引用——都在 Space 提供的边界内发生。

### 4.2 为什么 Ref 提升到 L1

旧设计将 Ref 独立为 L2 链接层。新设计将 Ref 并入 L1 协议层。

**Ref 不是独立的实体**：

1. **Ref 嵌入在 Message 中**——它不是一个独立存在的对象，而是 Message 的字段（`refs.messages`、`refs.nodes`）
2. **Ref 与 Message 不可分离**——每条 Message 天然携带 Ref，即使是空的 `[]`
3. **简化心智模型**——L1 包含参与者（Node）、通信（Message）和关联（Ref）三个互补概念，形成完整的协议语义

将 Ref 放在 L1，反映了它作为 Message 内在组成部分的真实角色，而非一个独立的"链接层"。

### 4.3 为什么 L2 是组合模式而非协议扩展

L2 模式（Checkpoint、Team、Handoff）可以通过两种方式实现：

| | L0+L1 机制组合 | 协议扩展 |
|---|---|---|
| 添加新模式 | 自由组合 content + refs，无需修改协议 | 需要新的协议实体或 Ref 类型 |
| 向后兼容 | 旧实现正常处理（只是不理解 content 语义） | 旧实现必须升级 |
| 校验 | 可选（content_schema） | 强制 |
| 灵活性 | content 完全自由，应用层自行约定结构 | 受协议定义约束 |

选择组合模式，因为：

1. **协议稳定性**：L0+L1 保持不变，新的协作模式通过 L2 组合自由创建
2. **机制与策略分离**：协议提供 content（机制），应用层决定如何利用 content 构建语义（策略）
3. **渐进式标准化**：一个 L2 模式可以从单个项目的 ad-hoc 实践开始，经过验证后逐步成为推荐模式

### 4.4 为什么 Ref 只有两种形态

L0 定义了 Space，L1 定义了 Node、Message 和 Ref，但 Ref 只取两种形态：→Message 和 →Node。

**Space 不需要协议级引用**。原因：

1. **`refs.messages` 已限制在同一 Space 内**——Space 是 Message Graph 的隔离边界，不需要被其他 Message "引用"
2. **Space 通达性通过 content 传递**——将 `{space_id, join_token}` 放入 `content` 即可传递加入 Space 的能力，不需要协议级 Ref 字段
3. **机制与策略分离**——协议提供隔离与认证机制（Space + join_token），更高层的策略（如 token 是否可转让、是否有有效期）由应用层决定。如果 Ref → Space 是协议级字段，协议就被迫定义"引用一个 Space 意味着什么"

这使得 Ref 保持两种形态，协议更干净。

### 4.5 为什么 Message Graph 是一等概念

旧设计明确声明"协议中不存在 Thread 概念"。新设计认识到：`refs.messages` 已经在创建图结构，我们应该承认并利用它。

承认 Message Graph 的好处：

1. **上下文遍历**：从任意 Message 沿 `refs.messages` 向上遍历，可以获取完整的因果上下文
2. **DAG 合并**：允许一条 Message 引用多个 parent，自然支持"汇总多个子任务结果"的模式
3. **更好的工具支持**：应用层可以基于图结构做可视化、搜索、分析

但 Message Graph 不是一个额外的协议实体——它是 Message + Ref 的自然涌现，不增加概念数量。

### 4.6 为什么 refs 是数组

单值 Ref（`message?: string`）只能形成 Tree（每个消息最多一个 parent）。数组 Ref（`messages: string[]`）支持完整的 DAG：

| 模式 | 单值 Ref | 数组 Ref |
|------|---------|---------|
| Thread（线性链） | ✓ | ✓ |
| Tree（分叉） | ✓ | ✓ |
| DAG（合并） | ✗ | ✓ |
| 多收件人消息 | ✗ | ✓ |

实际场景中，DAG 合并（汇总多个子任务结果）和多收件人消息是常见的协作模式。数组 Ref 用最小的成本支持了这些模式。

### 4.7 为什么用 Space 而不用 Route/Channel/Session

**Route** 暗示路径和方向——但我们的域是双向的、多方的。**Channel** 暗示点对点管道——但我们的域容纳任意数量的参与者。**Session** 暗示时间性和会话状态——但我们的域是持久的 append-only 日志。

**Space**（场/域）准确表达了概念的本质：一个隔离且受认证保护的交互域，没有方向性、没有点对点限制、没有时间起止的暗示。

### 4.8 为什么图模式是涌现而非规定

协议不定义"Thread 类型"或"DAG 类型"。图结构完全由 `refs.messages` 的使用方式决定：

- 每条消息引用一个 parent → Thread
- 多条消息引用同一个 parent → Tree
- 一条消息引用多个 parent → DAG

**规定模式 vs 涌现模式**：

如果协议定义了 Thread 类型，那么 DAG 需要另一个类型，Tree 需要另一个类型——每种图结构都需要协议级概念。涌现模式只需要一个机制（`refs.messages`），所有结构自然呈现。

这与 Conway's Game of Life 的哲学类似：简单的规则产生复杂的行为。

### 4.9 为什么 Space 之间没有直接关系

Space-to-Space 的关系（父子、桥接、嵌套）增加了协议复杂度但不增加表达力：

- **Fork**（派生子 Space）= Create Space + MessageCopy（读旧 Space 的 Message，写入新 Space）
- **Bridge**（连接两个 Space）= 一个 Node 同时在两个 Space 中，转发 Message
- **Nesting**（嵌套 Space）= 在外层 Space 中发送包含 Space ID + join_token 的 Message（通过 content）

所有 Space-Space 交互都可以通过 Node 在 Space 中发送 Message 来实现。协议层不需要"知道"Space 之间的关系。

### 4.10 为什么 space 内置加入语义，但不需要 Presence 类型

旧设计把"能读写某个 Space"近似视为在场，但这会让新加入的 Agent 缺少一个明确入口：它不知道当前有哪些参与者、各自负责什么，也不知道当前有多少 Message。

`space(name, description)` 同时解决创建、进入、初始化和身份声明问题：

1. **Node 主动声明自己**：Node 进入 Space 时提供 description，说明自己在这个 Space 中的职责
2. **description 是 per-space 的**：同一个 Node 在设计 Space 和评审 Space 中可以有不同职责
3. **不污染消息图**：space 的加入语义不产生 Message，也不会影响 read 的图结构
4. **不增加 L1 概念**：协议不暴露 Presence、Membership、Participant 等新类型；SpaceInfo 中的 `nodes` 只是 Space 当前状态的最小投影

这保留了协议的极简性，同时让真实协作中的初始化流程足够清晰。

### 4.11 为什么系统字段不暴露给 Node

`space_id` 和 append position 对存储和传输有用，但不是 Node 需要依赖的通信语义：

1. **space_id 来自路径**：Node 调用 `read(space_id, ...)` 或 `send(space_id, ...)` 时已经知道当前 Space，Message 内重复暴露会制造两个来源
2. **append position 是系统元数据**：排序由 append-only 日志顺序表达；内部位置适合窗口读取、调试或持久化索引，不应成为 Agent 推理的必要字段
3. **创建者由 sender 表达**：如果需要知道消息是谁创建的，使用 `sender`。需要沟通或追责时，找该 sender 对应的 Node
4. **最小公共协议更稳定**：公开字段越少，跨运行时实现越容易保持兼容

因此公开 Message 固定为 `id/sender/content/refs`，系统字段下沉到内部存储结构。

### 4.12 为什么身份由 secret 派生而非 server 分配

传统做法：server 生成 `id` + `api_key`，client 存储两者。身份由 server 颁发，server 同时持有秘密。

本协议：client 生成 `secret`，`node_id = sha256(secret)`。Server 验证 `sha256(presented_secret) == node_id`。

1. **Server 不存储秘密**：`node_id` 本身就是 hash，数据库泄露不暴露任何 secret
2. **身份自主**：与"参与者等价"理念一致——Node 的身份不依赖某个特定 server 的恩赐
3. **零概念增量**：不引入 token、credential、session 等新类型；secret 只是 node_id 的原像
4. **对 AI 透明**：`.auth` 文件存在即可通信，Agent 无需理解密码学

不选 keypair/mTLS 的原因：Node 身份验证只需要"证明你知道 secret"，不需要"证明你持有私钥"。Hash 验证比签名验证简单一个量级，且当前场景不需要不暴露 secret 的零知识性质——secret 只在 client 与 server 之间传输，不经过第三方。

### 4.13 为什么 Space 用 join_token 而非 ACL

两种准入模型：

| | join_token（能力模型） | ACL（身份模型） |
|---|---|---|
| 授权方式 | 持有 token 即可加入 | creator 逐个添加 node_id |
| 传播 | 可在 Message content 中流转 | 需要 creator 主动操作 |
| 与协议一致性 | 像 Space ID 一样可传递 | 需要额外的管理 API |

join_token 是能力（capability）——**持有它就拥有权利**，不需要询问任何人。这与协议的去中心倾向一致：Space creator 生成 token 后，准入权可以像 Message 一样在网络中自由流动。

---

## 附录 A：与现有协议的对比

| 维度 | MCP | A2A | 本协议 |
|------|-----|-----|--------|
| 核心关系 | Model ↔ Tool | Agent ↔ Agent | Node ↔ Space ↔ Node |
| 参与者类型 | 不对称（Host/Client/Server） | 不对称（Client/Remote Agent） | **完全对等** |
| Human 参与 | 不在协议范围 | 不在协议范围 | **一等公民** |
| 核心操作 | 工具调用 | 任务委派 | **Ref（构建 Message Graph）** |
| 状态模型 | 上下文窗口 | Task 对象 | **Space（append-only Message Graph）** |
| 多方协作 | 不支持 | 双方 | **任意多方** |
| 图结构 | 无 | 无 | **Thread / Tree / DAG（涌现）** |
| 应用层模式 | 无 | 无 | **Checkpoint / Team / Handoff / Swarm（L2 组合模式）** |
| 概念数量 | ~10（Tool, Resource, Prompt, Sampling...） | ~8（AgentCard, Task, Message, Part...） | **L0: 1 + L1: 3 = 4** |

## 附录 B：理论基础对照表

| 理论 | 核心洞察 | 本协议的对应 |
|------|---------|-------------|
| **π-calculus** (Milner 1992) | 通道名可以在通道上传递 | Space ID 可以在 Message 的 content 中传递 |
| **Actor Model** (Hewitt 1973) | 一切皆 Actor；Create/Send/Become 三原语 | 一切皆 Node；space/send/read 对应三原语，space 同时负责身份声明 |
| **Event Sourcing** | 事件序列是唯一真相 | Space 中的 Message Graph 是唯一真相；L2 状态通过回放消息对重建 |
| **Lamport 因果序** (1978) | 分布式系统中只有因果关系是可靠的时序 | refs.messages 建立因果链 |
| **CAP 定理** (Brewer 2000) | C/A/P 不可兼得 | 选择 AP：可用性 + 分区容忍性，最终一致 |
| **Erlang "Let it crash"** | 不防故障，管理故障 | Node 断线是常态，通过 Message 模式处理 |
| **图论** | DAG 拓扑排序保证无环遍历 | Context 遍历 = DAG 祖先的拓扑排序 |

## 附录 C：L2 模式对比

| 维度 | Checkpoint | Team | Handoff | Swarm |
|------|-----------|------|---------|-------|
| 场景 | 工作流内部的审批门 | Workspace 内多方通信 | 跨边界的任务转移 | 1:N 分布式协同作战 |
| 边界 | 同 Session / 同 Space | 同 Workspace | 跨 Session / 跨 Workspace | 同 Space（作战域） |
| 参与方 | 提交者 → 审阅者 | 多个对等 Node | 任意 Node → 任意 Node | Coordinator → N Agent |
| 方向 | 单向提交 + 同步反馈 | 多向广播 | 单向移交（fire-and-forget） | 指令→回报（异步） |
| refs.messages | 因果链（feedback→checkpoint） | 不使用 | 不使用 | 因果链（report→task） |
| refs.nodes | 不使用 | 不使用（广播） | 核心路由机制 | 可选路由（定向/广播） |
| 路由方式 | 隐式（审阅者主动读取） | 全量广播 | 显式（refs.nodes 指定目标） | 混合（定向 + 广播） |
| content 风格 | 结构化（type + 固定字段） | 结构化（text + 上下文） | 纯语义（title + brief + source） | 自然语言优先（content + targets + meta） |
| 生命周期 | submitted → feedback | 无 | 无（fire-and-forget） | task → report（通过 refs 关联） |
| Space 命名 | `aide.checkpoints.{ws}` | `aide.team.{ws}.{team}` | `aide.handoff.{ws}` | `aiscan.swarm.{case}` |

### L2 模式的机制利用谱

四个模式展示了 L0+L1 机制的不同利用组合：

```
                    refs.messages    refs.nodes    content_schema
Checkpoint              ✓               ✗              可选
Team                    ✗               ✗              ✗
Handoff                 ✗               ✓              ✓
Swarm                   ✓               ✓              ✓
```

- **Checkpoint** 重度使用 `refs.messages` 构建因果链（提交→反馈），不使用 `refs.nodes`
- **Team** 不使用任何 Ref 机制——最简单的 L2 模式，仅靠 Space 隔离 + 广播读取
- **Handoff** 重度使用 `refs.nodes` 做路由，不使用 `refs.messages`——因为它是 fire-and-forget，不需要因果链
- **Swarm** 同时使用两种 Ref——`refs.nodes` 路由 Task 到指定 agent，`refs.messages` 关联 Report 到 Task。是机制利用最完整的 L2 模式

这印证了 L2 的涌现性：同样的 L0+L1 机制，通过不同的组合方式产生完全不同的协作模式。
