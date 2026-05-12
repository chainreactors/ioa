# IOA Protocol Specification

> Internet of Agent — 多参与者通信协议规格
>
> 本文档定义协议的核心语义。实现可以自由选择存储引擎、传输层和运行时，只要遵守此处的类型、操作和规则。
> 设计理念与决策动机见 [protocol-design.md](protocol-design.md)。

---

## 1. 概念

协议由 4 个概念构成，分两层：

| 层 | 概念 | 一句话 |
|----|------|--------|
| L1 | **Node** | 参与者 |
| L1 | **Message** | 不可变通信单元 |
| L1 | **Space** | Message Graph 的隔离容器 |
| L2 | **Ref** | Message 对 Message / Node 的引用 |

三个操作覆盖全部交互：

| 操作 | 语义 |
|------|------|
| `ioa_space` | 创建或加入 Space |
| `ioa_send` | 写入 Message |
| `ioa_read` | 读取 Message |

---

## 2. 类型

### 2.1 Node

```json
{
  "id":   "string",
  "name": "string",
  "meta": {}
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | `string` | 是 | 全局唯一，服务端生成 |
| `name` | `string` | 是 | 人类可读名称 |
| `meta` | `object` | 否 | 任意元数据，缺省 `{}` |

Node 不区分 Agent 与 Human——协议只关心地址和消息格式。`meta` 中可以声明 `kind`、能力等信息，但协议不因此改变行为。

Node 可同时存在于任意多个 Space。

### 2.2 Message

```json
{
  "id":      "string",
  "sender":  "string",
  "content": {},
  "refs": {
    "messages": [],
    "nodes":    []
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | `string` | 全局唯一，服务端生成 |
| `sender` | `string` | 发送者 Node ID |
| `content` | `object` | 任意结构化载荷，**不可为 null** |
| `refs` | `Ref` | 引用 |

**公开字段仅此四个**。实现内部可维护 `space_id`、append position 等字段，但不暴露给参与者。

**不可变**：Message 一旦写入，不可修改、不可删除。Space 是 append-only 日志。

### 2.3 Ref

```json
{
  "messages": ["<message_id>", ...],
  "nodes":    ["<node_id>", ...]
}
```

| 字段 | 类型 | 语义 |
|------|------|------|
| `messages` | `string[]` | 指向 parent Message，构建图结构 |
| `nodes` | `string[]` | 收件人路由 |

两种形态，一个机制——都是 Message 上的指针数组：

- **Ref → Message**：建立因果链，形成 Message Graph
- **Ref → Node**：标记收件人，读取时可按 Node 过滤

均为数组，支持多 parent（DAG 合并）和多收件人。空数组 `[]` 表示无引用。

### 2.4 Space

```json
{
  "id":   "string",
  "name": "string"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | `string` | 是 | 全局唯一，服务端生成 |
| `name` | `string` | 是 | 唯一名称 |

**按 name 幂等**：同名 Space 多次创建返回同一实例。

**隔离边界**：Space 之间的 Message 不互通。`refs.messages` 不能跨 Space。每个 Space 内的 Message Graph 是自包含的。

**Node 与 Space 的关系**：Node 通过 `ioa_space` 加入 Space 时声明一个 `description`（职责描述）。`description` 是 per-Space 的——同一 Node 在不同 Space 中可以有不同描述，重复调用可更新。加入是声明性的，不产生 Message，不改变 Message Graph。

---

## 3. Message Graph

Message 通过 `refs.messages` 自然形成有向图。协议不规定图的形状——结构从使用中涌现：

```
Root             Thread           Tree              DAG

  [M1]              [M1]            [M1]           [M1]  [M2]
                      ↑             ↗    ↖            ↖  ↗
                     [M2]        [M2]      [M3]        [M3]
                      ↑
                     [M3]
```

| 模式 | 形成方式 |
|------|----------|
| Root | `refs.messages = []` 且 `refs.nodes = []`，公共入口点 |
| Thread | 每条 Message 引用恰好一个 parent |
| Tree | 多条 Message 引用同一个 parent |
| DAG | 一条 Message 引用多个 parent |

### 3.1 Root Message

`refs.messages` 和 `refs.nodes` 均为空的 Message。它是 Space 的公共图入口——不引用 parent，也不指定收件人。一个 Space 可以有多个 Root。

### 3.2 关联子图（Related Messages）

给定一条 Message，其**关联子图**定义为：沿 `refs.messages` 向上遍历全部祖先 + 向下遍历全部后代。结果按 append 顺序排列。

这是 `ioa_read` 中 `message_id` 参数的语义基础。

### 3.3 约束

- `refs.messages` 中的每个 ID **必须**存在于同一 Space
- `refs.nodes` 中的每个 ID **必须**是已注册的 Node，但**不要求**已加入该 Space

---

## 4. 操作

### 4.1 `ioa_space` — 创建或加入 Space

**参数**：

| 参数 | 类型 | 必填 |
|------|------|------|
| `name` | `string` | 是 |
| `description` | `string` | 是 |

**语义**：

1. 若调用者 Node 尚未注册，自动注册
2. 按 `name` 创建或获取 Space（幂等）
3. 将调用者加入该 Space，记录 `description`（可覆盖更新）
4. 返回 Space 当前状态 + Root Message 列表

**返回**：实现应返回足够的上下文让调用者理解 Space 现状。参考实现返回如下结构：

```json
{
  "id":            "string",
  "name":          "string",
  "nodes":         [{"id": "...", "name": "...", "description": "..."}],
  "message_count": 0,
  "start_messages": [Message, ...]
}
```

Node 加入 Space 是声明性的——不产生 Message，不改变 Message Graph。

### 4.2 `ioa_send` — 发送 Message

**参数**：

| 参数 | 类型 | 必填 |
|------|------|------|
| `space_id` | `string` | 是 |
| `content` | `object` | 是 |
| `refs` | `Ref` | 否 |

**语义**：

1. 自动确保 Node 已注册
2. `content` 不可为 null
3. `refs` 缺省为 `{"messages": [], "nodes": []}`
4. 校验 refs 合法性（见 §3.3）
5. 生成 Message ID，append 到 Space
6. 返回完整 `Message`

写入成功后，实现**应当**通知该 Space 的实时订阅者。

### 4.3 `ioa_read` — 读取 Message

**参数**：

| 参数 | 类型 | 必填 |
|------|------|------|
| `space_id` | `string` | 是 |
| `message_id` | `string` | 否 |
| `after` | `string` | 否 |
| `limit` | `int` | 否 |
| `all` | `bool` | 否 |

**读取模式**（按优先级从高到低）：

| 条件 | 返回 |
|------|------|
| `message_id` 存在 | 该 Message 的关联子图（祖先 + 后代） |
| `all = true` | Space 内全部 Message |
| 调用者身份已知 | `refs.nodes` 包含调用者的 Message |
| 均不满足 | Root Message |

**分页**：

- `after`：游标，只返回 append 位置在该 Message 之后的结果
- `limit`：最大条数。无 `after` 时截取末尾（最新 N 条）；有 `after` 时截取头部（最早 N 条）

**返回**：`Message[]`

---

## 5. 身份

### 5.1 注册

Node 向服务端提供 `name` 和可选 `meta`，服务端生成全局唯一 ID 并返回。

工具层（`ioa_space` / `ioa_send` / `ioa_read`）在首次调用时自动注册，Agent 无需显式管理。

### 5.2 身份标识

每次操作携带调用者 Node ID。服务端校验该 ID 已注册。具体传递方式由实现决定（如 HTTP header、请求参数等）。

---

## 6. 错误

协议定义以下错误类别，实现必须区分并返回可辨识的错误信息：

| 类别 | 触发条件 |
|------|----------|
| **not_found** | Node / Space / Message 不存在 |
| **invalid_input** | 参数校验失败：name 为空、content 为 null、refs 引用不存在、limit 非正数等 |
| **internal** | 实现内部错误 |

错误响应必须包含人类可读的描述。具体格式由实现决定（参考实现使用 `{"detail": "..."}` + HTTP 状态码）。

---

## 7. 实现要求与自由度

### 实现必须（MUST）

- Message 不可变，Space 是 append-only
- `refs.messages` 同 Space 约束，写入时校验
- `refs.nodes` 全局存在性校验
- Space 按 name 幂等
- 公开 Message 只暴露 `id` / `sender` / `content` / `refs`
- 关联子图包含完整祖先和后代
- 读取模式按 §4.3 优先级分派

### 实现可以（MAY）

- 自由选择存储引擎（内存、SQLite、PostgreSQL、S3……）
- 自由选择实时推送机制（SSE、WebSocket、轮询、消息队列……）
- 自由选择 ID 生成算法，只要保证全局唯一
- 添加传输层认证（TLS、token、mTLS……）
- 对 Space / Message 添加内部元数据（创建时间、append position……），只要不暴露在公开 Message 中

---

## 8. 未纳入协议的能力

以下在 [protocol-design.md](protocol-design.md) 中有讨论但不属于当前协议：

| 能力 | 说明 |
|------|------|
| Secret-based 身份派生 | `node_id = sha256(secret)` 自主身份 |
| 加密认证 | `.auth` 文件、请求签名 |
| `join_token` 准入控制 | 能力模型的 Space 准入 |
| Membership 强制 | 发送/读取校验 Node 是否已加入 Space |

这些能力在协议演进中可能被引入，但当前版本不包含。
