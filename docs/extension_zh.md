# IOA 扩展指南

[English](extension.md)

L2 协作模式**不是协议扩展**——它们是对 `content` 结构和 `refs` 组合方式的约定。新增模式无需修改 IOA 服务端或协议。

## L2 的工作原理

每个 L2 模式最终都调用同一个 L1 操作：

```
ioa_send(space_id, content, refs)
```

- `content` — 任意 dict，结构由 L2 自行定义
- `refs.messages` — 用于因果链（Checkpoint 用到，Handoff 不用）
- `refs.nodes` — 用于路由（Handoff 用到，Team 不用）

每个 L2 模式只是对 `content` 结构和 `refs` 组合方式的一种约定。

## 内置模式

```
                    refs.messages    refs.nodes    content_schema
Checkpoint              ✓               ✗              可选
Team                    ✗               ✗              ✗
Handoff                 ✗               ✓              ✓
Swarm                   ✓               ✓              ✓
```

| 模式 | 核心机制 | 典型流程 |
|------|---------|---------|
| Checkpoint | `refs.messages` 构建消息对 | 提交 → 反馈 |
| Team | 共享 Space，广播 | 发送 → 所有成员读取 |
| Handoff | `refs.nodes` 路由 | 发送 → 目标接收 |
| Swarm | 图结构 + 路由 | 广播 → 自组织 → 汇报 |

## 扩展机制：Skill + 子命令

IOA 提供两种机制扩展 L2：

### 1. Skill（面向 AI Agent）

Skill 是 `SKILL.md` + `schema.json` 的组合，教会 AI Agent 如何使用协作模式。存放在 `skills/<name>/` 目录。

**SKILL.md** — 自然语言指令：

```markdown
---
name: handoff
description: 即发即忘式任务移交。
---

# Handoff

将工作委托给另一个 Agent。发送上下文后继续工作。

## 消息格式
使用 `content_type: "handoff"` 发送。
...
```

**schema.json** — content 结构定义：

```json
{
  "type": "object",
  "properties": {
    "title": {"type": "string"},
    "message": {"type": "string"}
  },
  "required": ["title"]
}
```

Skill 内嵌在二进制中，通过 `ioa init` 导出：

```bash
ioa init                    # 导出所有 skill 到 .agent/skills/
ioa init handoff swarm      # 导出指定 skill
```

### 2. 子命令（面向 CLI）

协议子命令通过特定参数和逻辑扩展 `ioa send` 和 `ioa read`。它们是 Go 包，通过 `protocols.Register()` 注册。

## 添加新的 L2 模式

### 第一步：定义协议包

创建 `protocols/<name>/<name>.go`：

```go
package vote

import (
    "context"
    "fmt"
    "github.com/chainreactors/ioa/protocols"
)

func init() {
    protocols.Register(&protocols.Protocol{
        Name:        "vote",
        Description: "简单投票协议",
        Send: &protocols.Handler{
            Description: "投票",
            Flags:       &SendFlags{},
            Execute:     execSend,
        },
        Read: &protocols.Handler{
            Description: "读取投票",
            Flags:       &ReadFlags{},
            Execute:     execRead,
        },
    })
}

type SendFlags struct {
    PollID string `long:"poll-id" json:"poll_id" description:"投票标识"`
    Choice string `long:"choice" json:"choice" description:"你的选择"`
}

type ReadFlags struct {
    PollID string `long:"poll-id" json:"poll_id" description:"按投票过滤"`
}

func execSend(ctx context.Context, env *protocols.Env, args interface{}) (string, error) {
    var flags SendFlags
    protocols.ParseArgs(args, &flags)
    if flags.PollID == "" || flags.Choice == "" {
        return "", fmt.Errorf("vote: --poll-id and --choice are required")
    }

    content := map[string]interface{}{
        "poll_id": flags.PollID,
        "choice":  flags.Choice,
    }

    msg, err := env.Client.Send(ctx, env.SpaceID, protocols.SendMessage{
        ContentType: "vote",
        Content:     content,
    })
    if err != nil {
        return "", err
    }
    data, _ := json.MarshalIndent(msg, "", "  ")
    return string(data), nil
}

func execRead(ctx context.Context, env *protocols.Env, args interface{}) (string, error) {
    var flags ReadFlags
    protocols.ParseArgs(args, &flags)

    messages, err := env.Client.Read(ctx, env.SpaceID, protocols.ReadOptions{All: true})
    if err != nil {
        return "", err
    }

    var votes []protocols.Message
    for _, m := range messages {
        if protocols.MessageContentType(m) == "vote" {
            if flags.PollID == "" || m.Content["poll_id"] == flags.PollID {
                votes = append(votes, m)
            }
        }
    }
    data, _ := json.MarshalIndent(votes, "", "  ")
    return string(data), nil
}
```

### 第二步：注册导入

在 `cmd/ioa/main.go` 中添加空白导入：

```go
import (
    _ "github.com/chainreactors/ioa/protocols/vote"
)
```

### 第三步：添加 Skill（可选）

创建 `skills/vote/SKILL.md`：

```markdown
---
name: vote
description: 用于群组决策的简单投票协议。
---

# Vote

在 Space 内投票和计票。

## 消息格式
使用 `content_type: "vote"` 发送。
Content: `{"poll_id": "...", "choice": "..."}`
```

创建 `skills/vote/schema.json`：

```json
{
  "type": "object",
  "properties": {
    "poll_id": {"type": "string", "description": "投票标识"},
    "choice": {"type": "string", "description": "你的选择"}
  },
  "required": ["poll_id", "choice"]
}
```

### 结果

完成以上步骤后：

```bash
# CLI 子命令自动可用
ioa send vote --space <id> --poll-id p1 --choice approve
ioa read vote --space <id> --poll-id p1

# Skill 导出可用
ioa init vote
```

无需修改 IOA 服务端、协议或任何现有代码。

## 协议注册 API

```go
type Protocol struct {
    Name        string
    Description string
    Send        *Handler
    Read        *Handler
}

type Handler struct {
    Description string
    Flags       interface{}    // 带 go-flags tag 的结构体
    Execute     func(ctx context.Context, env *Env, args interface{}) (string, error)
}

type Env struct {
    Client   ClientAPI
    SpaceID  string
    NodeName string
}
```

- `Flags` — 带 `long` tag 的结构体，用于 go-flags 解析。CLI 用户以 `--flag value` 形式传入
- `Execute` — 通过 `args` 接收解析后的参数。使用 `protocols.ParseArgs(args, &flags)` 反序列化
- `Env` — 提供 IOA 客户端、当前 Space 和节点名的访问

## 嵌入 Skill

Skill 通过 `skills/embed.go` 使用 Go 的 `embed` 包嵌入到二进制中。添加新 skill 只需在 `skills/<name>/` 中放入 `SKILL.md` 和 `schema.json`——会自动包含。
