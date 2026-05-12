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
| L1（实体） | Node, Message, Space | 3 |
| L2（链接） | Ref（2 种形态：→Message, →Node） | 1 |
| **总计** | | **4** |

对 Agent/Human 暴露的最小工具面：

| 类型 | 工具 | 说明 |
|------|------|------|
| 核心工具 | `space`, `send`, `read` | 创建/进入域、写入消息、读取消息 |

四个概念、三个核心工具——这是整个协议。所有复杂的交互模式（审批流、并行分发、DAG 汇聚、能力发现）都由这些基础元素组合而成，不需要额外的协议级概念。

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
L1（实体层）: Node    Message    Space      ← 世界里有什么
L2（链接层）: Ref                           ← 实体间如何关联
L3（模式层）: Checkpoint, Broadcast...      ← 应用层组合模式
```

L1 是名词——静态实体。L2 是连接——实体间的关联。L3 是句子——由 L1 和 L2 组合出的应用层模式，不属于协议本身。

### 3.2 L1：Node

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

### 3.3 L1：Message

Message 是不可变的通信单元，也是 Message Graph 中的节点。

```
Message {
    id:      string      // 全局唯一标识
    sender:  string      // 发送者 Node ID
    content: object      // 任意结构化载荷
    refs: {              // L2：对 L1 实体的引用
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
- **`content` 完全自由**：协议不限制 content 的 schema。类型约定（如 `content.type = "checkpoint"`）属于 L3 应用层
- **同 Space 约束**：`refs.messages` 只能引用同一 Space 内的 Message，保证每个 Space 内的图是自包含的
- **数组化**：`refs.messages` 和 `refs.nodes` 都是数组，支持一条 Message 引用多个 parent（DAG 合并）或发送给多个 Node

### 3.4 L1：Space

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

### 3.5 Message Graph

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

### 3.6 L2：Ref

Ref 是协议中唯一的 L2 概念——**Message 对 L1 实体的引用**。

L1 定义了三种实体（Node、Message、Space），但 Ref 只有两种形态：

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

---

## 4. 设计决策记录

以下记录了设计过程中的关键决策及其理由。

### 4.1 为什么 Ref 只有两种形态

L1 定义了三种实体（Node、Message、Space），但 Ref 只取两种形态：→Message 和 →Node。

**Space 不需要协议级引用**。原因：

1. **`refs.messages` 已限制在同一 Space 内**——Space 是 Message Graph 的隔离边界，不需要被其他 Message "引用"
2. **Space 通达性通过 content 传递**——将 `{space_id, join_token}` 放入 `content` 即可传递加入 Space 的能力，不需要协议级 Ref 字段
3. **机制与策略分离**——协议提供隔离与认证机制（Space + join_token），更高层的策略（如 token 是否可转让、是否有有效期）由应用层决定。如果 Ref → Space 是协议级字段，协议就被迫定义"引用一个 Space 意味着什么"

这使得 L2 从三种形态简化为两种，协议更干净。

### 4.2 为什么 Message Graph 是一等概念

旧设计明确声明"协议中不存在 Thread 概念"。新设计认识到：`refs.messages` 已经在创建图结构，我们应该承认并利用它。

承认 Message Graph 的好处：

1. **上下文遍历**：从任意 Message 沿 `refs.messages` 向上遍历，可以获取完整的因果上下文
2. **DAG 合并**：允许一条 Message 引用多个 parent，自然支持"汇总多个子任务结果"的模式
3. **更好的工具支持**：应用层可以基于图结构做可视化、搜索、分析

但 Message Graph 不是一个额外的协议实体——它是 Message + Ref 的自然涌现，不增加概念数量。

### 4.3 为什么 refs 是数组

单值 Ref（`message?: string`）只能形成 Tree（每个消息最多一个 parent）。数组 Ref（`messages: string[]`）支持完整的 DAG：

| 模式 | 单值 Ref | 数组 Ref |
|------|---------|---------|
| Thread（线性链） | ✓ | ✓ |
| Tree（分叉） | ✓ | ✓ |
| DAG（合并） | ✗ | ✓ |
| 多收件人消息 | ✗ | ✓ |

实际场景中，DAG 合并（汇总多个子任务结果）和多收件人消息是常见的协作模式。数组 Ref 用最小的成本支持了这些模式。

### 4.4 为什么用 Space 而不用 Route/Channel/Session

**Route** 暗示路径和方向——但我们的域是双向的、多方的。**Channel** 暗示点对点管道——但我们的域容纳任意数量的参与者。**Session** 暗示时间性和会话状态——但我们的域是持久的 append-only 日志。

**Space**（场/域）准确表达了概念的本质：一个隔离且受认证保护的交互域，没有方向性、没有点对点限制、没有时间起止的暗示。

### 4.5 为什么图模式是涌现而非规定

协议不定义"Thread 类型"或"DAG 类型"。图结构完全由 `refs.messages` 的使用方式决定：

- 每条消息引用一个 parent → Thread
- 多条消息引用同一个 parent → Tree
- 一条消息引用多个 parent → DAG

**规定模式 vs 涌现模式**：

如果协议定义了 Thread 类型，那么 DAG 需要另一个类型，Tree 需要另一个类型——每种图结构都需要协议级概念。涌现模式只需要一个机制（`refs.messages`），所有结构自然呈现。

这与 Conway's Game of Life 的哲学类似：简单的规则产生复杂的行为。

### 4.6 为什么 Space 之间没有直接关系

Space-to-Space 的关系（父子、桥接、嵌套）增加了协议复杂度但不增加表达力：

- **Fork**（派生子 Space）= Create Space + MessageCopy（读旧 Space 的 Message，写入新 Space）
- **Bridge**（连接两个 Space）= 一个 Node 同时在两个 Space 中，转发 Message
- **Nesting**（嵌套 Space）= 在外层 Space 中发送包含 Space ID + join_token 的 Message（通过 content）

所有 Space-Space 交互都可以通过 Node 在 Space 中发送 Message 来实现。协议层不需要"知道"Space 之间的关系。

### 4.7 为什么 space 内置加入语义，但不需要 Presence 类型

旧设计把"能读写某个 Space"近似视为在场，但这会让新加入的 Agent 缺少一个明确入口：它不知道当前有哪些参与者、各自负责什么，也不知道当前有多少 Message。

`space(name, description)` 同时解决创建、进入、初始化和身份声明问题：

1. **Node 主动声明自己**：Node 进入 Space 时提供 description，说明自己在这个 Space 中的职责
2. **description 是 per-space 的**：同一个 Node 在设计 Space 和评审 Space 中可以有不同职责
3. **不污染消息图**：space 的加入语义不产生 Message，也不会影响 read 的图结构
4. **不增加 L1 概念**：协议不暴露 Presence、Membership、Participant 等新类型；SpaceInfo 中的 `nodes` 只是 Space 当前状态的最小投影

这保留了协议的极简性，同时让真实协作中的初始化流程足够清晰。

### 4.8 为什么系统字段不暴露给 Node

`space_id` 和 append position 对存储和传输有用，但不是 Node 需要依赖的通信语义：

1. **space_id 来自路径**：Node 调用 `read(space_id, ...)` 或 `send(space_id, ...)` 时已经知道当前 Space，Message 内重复暴露会制造两个来源
2. **append position 是系统元数据**：排序由 append-only 日志顺序表达；内部位置适合窗口读取、调试或持久化索引，不应成为 Agent 推理的必要字段
3. **创建者由 sender 表达**：如果需要知道消息是谁创建的，使用 `sender`。需要沟通或追责时，找该 sender 对应的 Node
4. **最小公共协议更稳定**：公开字段越少，跨运行时实现越容易保持兼容

因此公开 Message 固定为 `id/sender/content/refs`，系统字段下沉到内部存储结构。

### 4.9 为什么身份由 secret 派生而非 server 分配

传统做法：server 生成 `id` + `api_key`，client 存储两者。身份由 server 颁发，server 同时持有秘密。

本协议：client 生成 `secret`，`node_id = sha256(secret)`。Server 验证 `sha256(presented_secret) == node_id`。

1. **Server 不存储秘密**：`node_id` 本身就是 hash，数据库泄露不暴露任何 secret
2. **身份自主**：与"参与者等价"理念一致——Node 的身份不依赖某个特定 server 的恩赐
3. **零概念增量**：不引入 token、credential、session 等新类型；secret 只是 node_id 的原像
4. **对 AI 透明**：`.auth` 文件存在即可通信，Agent 无需理解密码学

不选 keypair/mTLS 的原因：Node 身份验证只需要"证明你知道 secret"，不需要"证明你持有私钥"。Hash 验证比签名验证简单一个量级，且当前场景不需要不暴露 secret 的零知识性质——secret 只在 client 与 server 之间传输，不经过第三方。

### 4.10 为什么 Space 用 join_token 而非 ACL

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
| 概念数量 | ~10（Tool, Resource, Prompt, Sampling...） | ~8（AgentCard, Task, Message, Part...） | **4（Node, Message, Space, Ref）** |

## 附录 B：理论基础对照表

| 理论 | 核心洞察 | 本协议的对应 |
|------|---------|-------------|
| **π-calculus** (Milner 1992) | 通道名可以在通道上传递 | Space ID 可以在 Message 的 content 中传递 |
| **Actor Model** (Hewitt 1973) | 一切皆 Actor；Create/Send/Become 三原语 | 一切皆 Node；space/send/read 对应三原语，space 同时负责身份声明 |
| **Event Sourcing** | 事件序列是唯一真相 | Space 中的 Message Graph 是唯一真相 |
| **Lamport 因果序** (1978) | 分布式系统中只有因果关系是可靠的时序 | refs.messages 建立因果链 |
| **CAP 定理** (Brewer 2000) | C/A/P 不可兼得 | 选择 AP：可用性 + 分区容忍性，最终一致 |
| **Erlang "Let it crash"** | 不防故障，管理故障 | Node 断线是常态，通过 Message 模式处理 |
| **图论** | DAG 拓扑排序保证无环遍历 | Context 遍历 = DAG 祖先的拓扑排序 |

