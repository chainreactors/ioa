# 多参与者通信协议设计

> 一个极简的通用协议，实现任意 Agent 与 Human 之间的通信、协调与移交。
> 设计目标：用最少的概念获得最大的能力。
> 规格定义见 [spec.md](spec.md)。

---

## 1. 问题本质：AI 时代缺少一个语义优先的通信协议

### 1.1 为什么需要这个协议

多 Agent 协作和人机交互的根本瓶颈，不是 Agent 的能力不够，而是**没有一个原生支持语义通信的协议**。

现有的协作系统（JIRA、Slack、MCP、A2A）都是前 AI 时代的产物。它们的共同假设是：参与者是人类，通信的接收方需要预定义的结构才能理解消息。所以这些系统用字段、状态机、工单模板、API schema 来约束信息的格式——这是在帮人类降低认知负担。

但 AI Agent 不需要这些脚手架。Agent 能直接理解自然语言，能从非结构化的语义中提取任意信息。**预定义结构对 AI 来说不是帮助，是束缚**——每一个硬编码的字段、每一种固定的状态枚举，都是在说"你只能用这种方式思考这个问题"。

### 1.2 语义优先

本协议的核心理念是**语义优先**（Semantic-First）：

**状态通过语义传达，而非通过预定义结构声明。**

传统方式：协议预定义一组状态（pending / running / complete），参与者从中选择一个填入固定字段。这意味着协议的设计者必须在设计时预见所有可能的状态——这在现实中做不到。每一种预料之外的状态都变成了对协议的"补丁"。

语义优先的方式：参与者在 `content` 中用自然语言或自由结构化数据描述当前的状态、上下文和意图。接收方（AI 或人类）通过理解语义来获取信息。协议不预定义任何业务语义，只提供传输语义的机制。

这不是"偷懒不定义 schema"——这是对 AI 能力的正确利用。当接收方有语义理解能力时，预定义结构是冗余的约束；当接收方没有语义理解能力时，可以通过 L2 应用层的 `content_schema` 提供结构化约束。协议同时支持两种模式，但默认为语义优先。

**语义优先的具体体现**：

| 传统协议 | 本协议 |
|---------|--------|
| 预定义 Task 状态机（5种/10种/20种状态） | 状态是 Message `content` 中的自然语义，无限种 |
| 预定义工单字段（优先级、截止时间、指派人...） | 字段由参与者在 `content` 中自由定义 |
| 固定的消息类型（request/response/notification） | 消息类型由 `content` 语义和图结构涌现 |
| 结构化元数据承载业务语义（metadata 混用） | `content` 承载一切语义，系统字段严格非语义 |

### 1.3 协议的定位

本协议不解决 Agent 的能力问题——你的 Agent 能不能做代码审计、能不能修漏洞，这不是协议的事。

本协议解决的是：**当你的 Agent 做完了代码审计，它怎么把结果告诉另一个 Agent 或人类，怎么发起协作，怎么跟踪协作过程，怎么把工作移交出去**。这些都是通信问题，需要一个通信协议。

现有的 IM 工具（钉钉、Slack）可以作为传输信道，但它们不是通信协议——它们的消息模型是为人类聊天设计的，不支持因果链、图结构、语义自由的 content、跨 Agent 的自动路由。本协议提供这些能力，同时可以通过任何信道传输。

---

## 2. 设计原则

### 2.1 参与者等价

协议不区分 Agent 和 Human。两者都是 **Node**——平等的参与者。

传统系统将 Human 视为"审批者"、Agent 视为"执行者"，这种角色不对称硬编码在协议层，导致拓扑僵化。本协议的立场是：**协议只关心对方的地址和消息格式，不关心对方是碳基还是硅基**。

一个 Human 可以像 Agent 一样发起任务；一个 Agent 可以像 Human 一样审批工作。能力差异（响应速度、并发度、专业领域）通过元数据声明，不在协议层强制。

### 2.2 极简主义

协议的全部概念：

| 层级 | 概念 | 数量 |
|------|------|------|
| L0（基础设施） | Space | 1 |
| L1（协议层） | Node, Message, Ref | 3 |
| L2（应用层） | Checkpoint, Team, Handoff, Swarm... | 由 L0+L1 组合涌现 |

对 Agent/Human 暴露的工具：

| 工具 | 说明 |
|------|------|
| `ioa_space` | 声明进入协作域 |
| `ioa_send` | 写入消息 |
| `ioa_read` | 读取消息 |

参与者只需理解三个核心概念（Node、Message、Ref）和三个工具。Space 是 server 管理的基础设施，对参与者透明。所有复杂的交互模式（审批流、并行分发、DAG 汇聚、任务移交）都由基础元素组合而成，不需要额外的协议级概念。

### 2.3 机制与策略分离

协议定义**机制**（mechanism），不定义**策略**（policy）。

- **机制**：Message 通过 Ref 引用其他 Message 和 Node，形成图结构；Space 提供隔离与认证边界
- **策略**：接收者是否接受、超时如何处理、失败如何补偿——由应用层决定

### 2.4 AI-First

协议为 AI Agent 的工具调用（tool-use）范式优化：

- **小而完整的工具闭包**：AI 只需要 `ioa_space`、`ioa_send`、`ioa_read` 就能完整参与
- **无隐式会话状态**：消息上下文存在于 Space 的 Message Graph 中，不依赖连接状态
- **组合优于记忆**：复杂模式通过组合简单操作实现，AI 不需要记住特殊 API



---

## 3. 核心机制

### 3.1 分层总览

```
L0（基础设施层）: Space                          ← server 管理，参与者透明
L1（协议层）:     Node    Message    Ref          ← 参与者、通信单元、关联
L2（应用层）:     Checkpoint  Team  Handoff  ...  ← L0+L1 组合涌现的协作模式
```

- **L0** 是基础设施——隔离、认证、Message Graph 的容器。由 server 管理，参与者无需理解其内部机制。
- **L1** 是协议核心——参与者（Node）、通信单元（Message）和它们之间的引用关系（Ref）。参与者直接与 L1 交互。
- **L2** 是协作模式——由 L0+L1 机制组合涌现，不属于协议本身。

### 3.2 L0：Space — 基础设施

**设计初衷**：没有隔离边界，所有协作的消息会混在一起——安全扫描的消息和架构评审的消息出现在同一个流中，agent 无法区分上下文。Space 解决的是**协作上下文的隔离问题**。之所以放在 L0 而非 L1，是因为 Space 是一切交互的前提条件——没有 Space，Message 无处存放，Ref 无处指向。它是基础设施，不是参与者交互的对象。之所以对参与者透明，是因为隔离和准入是系统管理关注的事，不应该泄漏到 agent 的推理过程中。

```
Space {
    id:   string    // 全局唯一标识
    name: string    // 人类可读名称，按 name 幂等
}
```

**语义**：

| 维度 | 说明 |
|------|------|
| **是什么** | Message Graph 的隔离容器、认证边界 |
| **做什么** | 提供 Message 的存储域；隔离不同协作上下文；管理 Node 的加入与职责声明 |
| **不做什么** | 不暴露管理 API 给参与者；不允许跨 Space 的 `refs.messages`；不定义 Space 之间的关系 |

**关键约束**：

- **隔离不可破**：`refs.messages` 不能跨 Space——这不是限制，而是 Space 的核心机制。协议在内部绝不破坏这个隔离。每个 Space 的 Message Graph 是自包含的，这保证了上下文的完整性和可信性
- **幂等**：同名 Space 多次创建返回同一实例
- **声明式加入**：`ioa_space(name, description)` 同时完成进入和职责声明，不产生 Message，不改变 Message Graph
- **Space 间无直接关系**：协议不定义 Space-to-Space 的关系（父子、桥接、嵌套），Space 间的信息传递由 Node 主动完成

**隔离即语义隔离**：

Space 的隔离不仅是数据隔离，更是**语义隔离**。以 Team 模式为例：当多个 Agent 组成一个 Team 讨论某个问题时，Team 会创建一个独立的 Space。Team 内部的讨论（试错、争论、中间结论）不会污染主 Space 的消息图——这是对协作语义的保护。主 Space 只需要看到 Team 的最终结论，不需要看到达成结论的过程。

当 Team 需要将某个结论传递到外部 Space 时，Node 通过指定目标 Space ID 主动发送一条 Message。这个行为发生在协议的 L1 层面（Node 调用 `ioa_send` 指定另一个 `space_id`），不需要任何特殊的"跨 Space 引用"机制。**隔离是默认的，打破隔离是显式的、由参与者主动决定的。**

这恰恰印证了抽象的完备性：协议没有为"跨 Space 通信"设计专门的机制，但通过 Node 同时存在于多个 Space、在不同 Space 中发送 Message 这两个已有能力，任何形式的跨 Space 交互都自然实现：

```
Space A（主协作域）           Space B（Team 讨论域）
    │                            │
    │                        Agent X ── send(讨论消息) ──► Space B
    │                        Agent Y ── send(讨论消息) ──► Space B
    │                            │
    │  ◄── Agent X 将结论         │   （Team 内部讨论对 Space A 不可见）
    │      send 到 Space A       │
    │                            │
```

**最小抽象，最大表达力**：Space 只提供一个机制（隔离），但通过与 Node 和 Message 的组合，实现了语义隔离、上下文保护、选择性信息传递等丰富的协作模式——不需要任何额外的协议级概念。

### 3.3 L1：Node — 参与者

**设计初衷**：协作需要身份——不知道"谁"，就没有协作主体。传统协议把 Human 和 Agent 分成不同角色（MCP 的 Host/Client/Server、A2A 的 Client/Remote Agent），导致角色不对称硬编码在协议层。Node 解决的是**参与者身份的统一抽象问题**——协议只关心"有一个参与者在通信"，不关心它是人还是 AI。之所以不区分 Agent 和 Human，是因为能力差异是动态的、场景相关的，不应该被协议固化。

```
Node {
    id:   string    // 全局唯一标识
    name: string    // 人类可读名称
    meta: object    // 可选：静态能力声明（类型、能力...）
}
```

**语义**：

| 维度 | 说明 |
|------|------|
| **是什么** | 协议中的参与者身份 |
| **做什么** | 发送和接收 Message；通过 `meta` 声明自身能力（静态属性）；在不同 Space 中声明不同职责 |
| **不做什么** | 不区分 Agent 与 Human（`meta.kind` 是元数据，不影响协议行为）；不暴露 Presence/Membership 类型 |

**关键约束**：

- Node 可同时存在于任意多个 Space
- `description` 是 Node 与 Space 的关系属性（per-Space），不是 Node 的全局属性
- `refs.nodes` 是收件人路由，不代表被引用的 Node 已加入该 Space
- 身份机制由实现决定（见 spec.md §5）

### 3.4 L1：Message — 通信单元

**设计初衷**：协作的本质是信息交换——没有信息在参与者之间流动，就没有协作发生。Message 解决的是**通信的原子单元问题**。之所以设计为不可变（append-only），是因为协作历史必须可信——如果消息可以被篡改或删除，任何基于消息历史的推理都不可靠。之所以 `content` 是完全自由的 `object`，是因为协议不应该限制参与者谈论什么——Task 状态、自然语言讨论、结构化数据都可以放在 `content` 中，由应用层自行约定。

```
Message {
    id:         string      // 全局唯一标识
    sender:     string      // 发送者 Node ID
    created_at: string      // 服务端写入时间，RFC3339 UTC
    content:    object      // 任意结构化载荷
    refs: {
        messages: string[]  // → Message IDs（图结构：parent 链接）
        nodes:    string[]  // → Node IDs（收件人路由）
    }
}
```

**语义**：

| 维度 | 说明 |
|------|------|
| **是什么** | 不可变的通信单元，append-only 日志中的一条记录 |
| **做什么** | 承载任意结构化载荷（`content`）；通过 `refs` 建立与其他 Message 和 Node 的关联 |
| **不做什么** | 不可修改、不可删除；不暴露系统内部字段（`space_id`、append position） |

**关键约束**：

- **公开字段仅五个**：`id`、`sender`、`created_at`、`content`、`refs`。系统字段下沉到内部存储
- **`content` 完全自由**：协议不限制 `content` 的 schema，应用层自行约定结构
- **同 Space 约束**：`refs.messages` 只能引用同一 Space 内的 Message

### 3.5 L1：Ref — 连接机制

**设计初衷**：孤立的消息没有上下文——"修好了"这条消息，如果不知道它是对哪个漏洞报告的回应、发给了谁，就毫无意义。Ref 解决的是**消息之间的关联问题**。之所以嵌入在 Message 中而不是独立实体，是因为引用与消息不可分离——每条消息天然携带"我在回应什么"和"我发给谁"的信息。之所以只有两种形态（→Message 和 →Node），是因为这两种关联覆盖了所有协作需求：因果链（谁导致了什么）和路由（发给谁）。

**两种形态**：

| Ref 形态 | 指向 | 语义 |
|---------|------|------|
| `refs.messages` | Message ID（同 Space） | 构建图结构（Thread / Tree / DAG） |
| `refs.nodes` | Node ID | 收件人路由 |

**语义**：

| 维度 | 说明 |
|------|------|
| **是什么** | Message 上的指针数组，建立实体间的关联 |
| **做什么** | `refs.messages` 形成因果链和图结构；`refs.nodes` 形成收件人路由和 inbox |
| **不做什么** | 不指向 Space（Space 通达性通过 `content` 传递）；不独立于 Message 存在 |

**为什么只有两种形态**：Ref → Space 不需要作为协议级字段。Space 的通达性通过 `content` 传递（如包含 Space ID），避免协议被迫定义"引用一个 Space 意味着什么"。

**数组化**：两种 Ref 都是数组，支持 DAG 合并（一条 Message 引用多个 parent）和多收件人。

**可组合**：一条 Message 可以同时携带两种 Ref：

```python
# 回复某条消息并定向给某人
ioa_send(space_id="s1",
         content={"task": "请接手审查"},
         refs={"messages": [msg_42.id], "nodes": [agent_b.id]})
```

### 3.6 Message Graph — 涌现结构

**设计初衷**：预定义的交互结构（如"任务有5种状态"、"对话必须是线性的"）无法覆盖现实中所有的协作模式。Message Graph 解决的是**交互结构的通用性问题**——协议不规定结构的形状，只提供一个机制（`refs.messages`），让线性对话（Thread）、任务分解（Tree）、结果汇聚（DAG）从使用中自然涌现。之所以不定义"Thread 类型"或"DAG 类型"，是因为每增加一种类型就多一个协议级概念，而涌现模式只需要一个机制就产生所有结构。

Message 通过 `refs.messages` 自然形成有向图。**协议提供机制（引用），结构从使用中涌现**。

```
四种基本模式：

1. Root（根）        2. Thread（线性链）     3. Tree（分叉）        4. DAG（合并）

   [M1]                [M1]                   [M1]                  [M1]  [M2]
                         ↑                    ↗    ↖                  ↖  ↗
                        [M2]              [M2]      [M3]              [M3]
                         ↑
                        [M3]
```

| 模式 | 形成方式 | 典型场景 |
|------|----------|----------|
| Root | `refs.messages = []`，公共图入口 | 任务发起、话题开始 |
| Thread | 每条 Message 引用一个 parent | 线性对话 |
| Tree | 多条 Message 引用同一个 parent | 任务分解、并行分支 |
| DAG | 一条 Message 引用多个 parent | 结果汇聚、多源合并 |

协议不规定用哪种结构——它们都是 `refs.messages` 的自然组合。涌现模式只需要一个机制，所有结构自然呈现。

**Context（上下文）** = 从任意 Message 沿 `refs.messages` 向上遍历到所有祖先。新加入的 Node 通过回放 Message 历史获取完整上下文——任何时刻的"状态"都是 Message Graph 的投影。

---

## 4. L2：协作模式

### 4.1 L2 的性质

L2 不是协议的一部分。它不引入新的实体、新的 Ref 类型、也不约定 `content` 中必须存在某个字段。L2 模式完全建立在协议已有的三个能力之上：

1. **content 自由**：`content` 是任意 object，应用层自由决定其内部结构
2. **content_schema 可选校验**：Space 可以关联 JSON Schema 约束 content 格式——这是应用层的选择，不是协议的要求
3. **Ref 组合**：`refs.messages`（因果链）和 `refs.nodes`（路由）自由组合，构建任意交互流程

新的 L2 模式可以随时创建，不需要修改协议。

### 4.2 协作模式

#### Checkpoint — 审批门

可审阅的决策点：工作流中需要显式反馈才能继续的边界。

| L0+L1 机制 | 使用方式 |
|-----------|---------|
| Space | 独立 Space 隔离不同工作流 |
| content | 承载 checkpoint 的语义载荷（type、title、options、feedback） |
| refs.messages | feedback 引用对应的 checkpoint，形成提交→反馈消息对 |
| refs.nodes | 不使用——审阅者主动发现 |

**核心模式**：提交→反馈的消息对，通过 `refs.messages` 关联。状态通过回放消息对重建。

#### Team — 多方通信

多个 Agent/Human 之间的广播通信。

| L0+L1 机制 | 使用方式 |
|-----------|---------|
| Space | 每个 Team 一个独立 Space，成员通过 `ioa_space` 加入 |
| content | 承载消息载荷 |
| refs.messages | 不使用——消息按 append 顺序形成时间线 |
| refs.nodes | 不使用——广播语义，所有成员可见 |

**核心模式**：共享 Space + 广播消息。最简单的 L2 模式，不使用任何 Ref 机制。

#### Handoff — 任务移交

纯语义的跨 Node 任务转移：发送方将工作上下文传递给接收方，发送即遗忘。

| L0+L1 机制 | 使用方式 |
|-----------|---------|
| Space | 共享 Space |
| content | 纯语义载荷（title、brief、source 等） |
| refs.messages | 不使用——独立消息，不形成因果链 |
| refs.nodes | **核心路由**——决定谁收到移交消息 |

**核心模式**：fire-and-forget。通过 `refs.nodes` 实现三种路由：定向（`[target_id]`）、广播（`[]`）、多候选（`[a, b, c]`）。

#### Swarm — 协同作战

1:N 分布式协同作战：一个 coordinator 指挥 N 个异构 agent 在同一 Space 中协作。

| L0+L1 机制 | 使用方式 |
|-----------|---------|
| Space | 一个 Space = 一个作战域 |
| content | 自然语言为主（content + targets + meta） |
| refs.messages | Report 引用 Task，形成因果链 |
| refs.nodes | coordinator 用于定向分发 |

**核心模式**：root message 是指令，`refs.messages` 引用指令的是回报。消息语义由图结构决定，不需要 type 字段。

---

## 5. 关键设计决策

### 5.1 L2 是组合模式而非协议扩展

| | L0+L1 机制组合 | 协议扩展 |
|---|---|---|
| 添加新模式 | 自由组合 content + refs，无需修改协议 | 需要新的协议实体或 Ref 类型 |
| 向后兼容 | 旧实现正常处理（只是不理解 content 语义） | 旧实现必须升级 |
| 灵活性 | content 完全自由 | 受协议定义约束 |

选择组合模式保证协议稳定性：L0+L1 保持不变，新的协作模式通过 L2 自由创建。

### 5.2 Ref 只有两种形态

Ref 只取 →Message 和 →Node 两种形态，不包含 →Space。

Space 通达性通过 `content` 传递（如将 Space ID 放入消息内容），不需要协议级 Ref 字段。如果 Ref → Space 是协议级字段，协议就被迫定义"引用一个 Space 意味着什么"——违反机制与策略分离原则。

### 5.3 Space 间无直接关系

Space-to-Space 的关系（父子、桥接、嵌套）增加协议复杂度但不增加表达力：

- **Fork** = 创建新 Space + 复制 Message
- **Bridge** = 一个 Node 同时在两个 Space 中转发 Message
- **Nesting** = 在外层 Space 的 Message `content` 中包含内层 Space ID

所有 Space-Space 交互通过 Node 在 Space 中发送 Message 实现。协议层不需要"知道" Space 之间的关系。

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
| 概念数量 | ~10 | ~8 | **L0: 1 + L1: 3 = 4** |

## 附录 B：理论基础对照表

| 理论 | 核心洞察 | 本协议的对应 |
|------|---------|-------------|
| **π-calculus** (Milner 1992) | 通道名可以在通道上传递 | Space ID 可以在 Message 的 content 中传递 |
| **Actor Model** (Hewitt 1973) | 一切皆 Actor；Create/Send/Become 三原语 | 一切皆 Node；ioa_space/ioa_send/ioa_read 对应三原语 |
| **Event Sourcing** | 事件序列是唯一真相 | Space 中的 Message Graph 是唯一真相；状态通过回放重建 |
| **Lamport 因果序** (1978) | 分布式系统中只有因果关系是可靠的时序 | refs.messages 建立因果链 |
| **CAP 定理** (Brewer 2000) | C/A/P 不可兼得 | 选择 AP：可用性 + 分区容忍性，最终一致 |
| **图论** | DAG 拓扑排序保证无环遍历 | Context 遍历 = DAG 祖先的拓扑排序 |

## 附录 C：L2 模式对比

| 维度 | Checkpoint | Team | Handoff | Swarm |
|------|-----------|------|---------|-------|
| 场景 | 审批门 | 多方通信 | 任务移交 | 协同作战 |
| 参与方 | 提交者→审阅者 | 多个对等 Node | 任意 Node→任意 Node | Coordinator→N Agent |
| 方向 | 单向提交+同步反馈 | 多向广播 | 单向（fire-and-forget） | 指令→回报（异步） |
| refs.messages | 因果链 | 不使用 | 不使用 | 因果链 |
| refs.nodes | 不使用 | 不使用（广播） | 核心路由 | 可选路由 |

**机制利用谱**：

```
                    refs.messages    refs.nodes    content_schema
Checkpoint              ✓               ✗              可选
Team                    ✗               ✗              ✗
Handoff                 ✗               ✓              ✓
Swarm                   ✓               ✓              ✓
```

同样的 L0+L1 机制，通过不同组合产生不同协作模式——印证 L2 的涌现性。



## 附录 E：L2 扩展的便捷性——Aide 实际代码

以下是 Aide 中实现各个 L2 模式的真实代码。每个模式的核心逻辑都极其简短——**因为协议层已经提供了完整的通信机制，L2 只需要定义 content 结构 + 选择 refs 组合方式，然后调用 `client.send()`**。

### Handoff：完整实现只有 10 行

定义 content 结构（`handoff.py`）：

```python
class Handoff(BaseModel):
    """Routing and sender identity live in IOA envelope."""
    title: str = ""
    message: str = ""
```

发送一条 Handoff（`handoff.py`）：

```python
async def _send_one(ctx, route, content: Handoff, target_node_id: str) -> str:
    refs = Ref(
        messages=[ctx.reply_to_message_id] if ctx.reply_to_message_id else [],
        nodes=[target_node_id, route.receiver_node_id],
    )
    message = await ctx.client.send(
        route.space_id,
        {"title": content.title, "message": content.message},
        refs=refs,
        content_type="handoff",
    )
    return message.id
```

IoA 看到了什么？一条 Message，envelope `content_type = "handoff"`，`refs.nodes` 指向目标 Node。Handoff 的语义由应用层 skill/schema 解释，协议层只负责承载和路由。

### Team：完整实现只有 6 行

定义 content 结构（`protocols.py`）：

```python
class TeamMessageContent(BaseModel):
    team: str
    text: str
```

发送一条 Team 消息（`team.py`）：

```python
async def do_team_send(team: str, message: str) -> str:
    async with optional_task_ioa_context() as ctx:
        space = await ensure_workspace_ioa_space(ctx.client, ctx.workspace)
        await ctx.client.send(
            space.id,
            {"team": team, "text": message},
            content_type="team",
        )
        return f"Sent to team '{team}'."
```

不需要 refs——Team 是广播模式，Space 内所有 Node 都能 read 到。IoA 看到一条普通 Message，不知道这是"Team 通信"。

### Checkpoint：完整实现只有 8 行

定义 content 结构（`protocols.py`）：

```python
class CheckpointSubmittedContent(BaseModel):
    id: str
    kind: str = "checkpoint"
    title: str = ""
    content: str = ""
    options: list[str] = Field(default_factory=list)
    labels: list[str] = Field(default_factory=list)
```

发送一条 Checkpoint（`checkpoint.py`）：

```python
async def submit_checkpoint(self, cp: Checkpoint, *, task_id: str, parent_message_id: str = "") -> str:
    node_id = await self._ensure_task_node(task_id)
    space_id = await self._ensure_space(node_id)
    refs = Ref(messages=[parent_message_id]) if parent_message_id else None
    msg = await self._client.send(
        space_id,
        checkpoint_submitted_payload(cp),
        content_type="checkpoint",
        **({"refs": refs} if refs is not None else {}),
    )
    return msg.id
```

IoA 看到的就是一条 Message 通过 `refs.messages` 关联到 parent——一个消息对。

### 统一的消息解析器：新增 L2 模式只需加一个 `if`

所有 L2 模式共享同一个解析入口（`protocols.py`）：

```python
def parse_message_content(content: JsonObject, content_type: str = "") -> MessageContent | None:
    if content_type == "checkpoint":
        return CheckpointSubmittedContent.model_validate(content)
    if content_type == "handoff":
        return Handoff.model_validate(content)
    if content_type == "team":
        return TeamMessageContent.model_validate(content)
    if content_type == "swarm":
        return SwarmContent.model_validate(content)
    return None
```

### 扩展一个新 L2 模式需要什么

如果要新增一个"投票"协议，总共需要：

```python
# 1. 定义 content 结构（5 行）
class VoteContent(BaseModel):
    poll_id: str
    choice: str

# 2. 发送（4 行）
async def vote(poll_id: str, choice: str) -> str:
    async with task_ioa_context() as ctx:
        space = await ensure_workspace_ioa_space(ctx.client, ctx.workspace)
        await ctx.client.send(
            space.id,
            {"poll_id": poll_id, "choice": choice},
            content_type="vote",
        )
        return f"Voted {choice}"

# 3. 在 parse_message_content 里加一个 if（2 行）
    if content_type == "vote":
        return VoteContent.model_validate(content)
```

**11 行代码，零协议改动。** 不需要新的 Ref 类型，不需要新的 Space 类型，不需要修改 IoA server 的任何代码。这就是"L2 是组合模式而非协议扩展"的实际效果。

### 为什么这么简单

因为所有 L2 模式最终都调用同一个 L1 操作：

```python
await client.send(space_id, content, refs=refs)
```

- `content`：任意 dict，由 L2 自行定义结构
- `refs.messages`：用于因果链（Checkpoint 用到，Handoff 不用）
- `refs.nodes`：用于路由（Handoff 用到，Team 不用）

**每个 L2 模式只是对 `content` 结构和 `refs` 组合方式的一种约定。** 协议提供机制，L2 选择如何组合——这就是最小抽象产生最大表达力的具体体现。
