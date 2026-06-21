# IOA — Internet of Agent

[English](README.md) | [CLI 参考](docs/cli_zh.md) | [扩展指南](docs/extension_zh.md) | [AI 使用指南](docs/ai-usage_zh.md)

极简的语义优先多参与者通信协议，用于 Agent 与 Agent、Agent 与 Human 之间的协作。

## 设计

IOA 基于一个洞察：**AI Agent 能理解语义，因此协议不应预定义语义。**

传统协议将业务逻辑编码到协议层——任务状态、消息类型、工单字段。每一个预定义结构都是赌协议设计者能预见所有用例。没人做得到。

IOA 反转了这个思路。协议只提供**机制**——如何发送、引用和路由消息。所有**语义**——消息意味着什么、存在哪些状态、遵循什么工作流——由参与者在消息 `content` 中自行定义和解读。

### 4 个概念，2 层架构

```
L0  Space                              隔离边界（server 管理，参与者透明）
L1  Node    Message    Ref             参与者、通信单元、关联
```

| 概念 | 是什么 |
|------|-------|
| **Space** | 隔离边界。消息不能跨越。按 name 幂等。 |
| **Node** | 参与者——人或 Agent，协议不做区分。 |
| **Message** | 不可变通信单元。5 个字段：`id`、`sender`、`created_at`、`content`、`refs`。 |
| **Ref** | Message 上的两个指针数组：`refs.messages`（因果链）和 `refs.nodes`（路由）。 |

### 3 个操作

| 操作 | 作用 |
|------|------|
| `ioa_space` | 加入协作域 |
| `ioa_send` | 写入消息 |
| `ioa_read` | 读取消息 |

这就是整个协议。审批流、任务移交、群组通信、多 Agent 协同——都从这些原语的组合中涌现。

### Message Graph

Message 通过 `refs.messages` 形成有向图。结构是涌现的，不是预定义的：

```
Root              Thread            Tree               DAG

  [M1]              [M1]             [M1]            [M1]  [M2]
                      ↑              ↗    ↖             ↖  ↗
                     [M2]         [M2]      [M3]         [M3]
                      ↑
                     [M3]
```

Thread、Tree、DAG——同一个机制，不同的使用方式。

### L2：涌现的协作模式

L2 模式是对 `content` + `refs` 的约定，不是协议扩展。新增模式无需任何服务端改动。

| 模式 | 机制 | 用途 |
|------|------|------|
| **Checkpoint** | `refs.messages` 构建消息对 | 人工审批 |
| **Handoff** | `refs.nodes` 路由 | 即发即忘式移交 |
| **Team** | 共享 Space 广播 | 群组通信 |
| **Swarm** | 图结构 + 路由 | 多 Agent 自组织 |

详见 [扩展指南](docs/extension_zh.md) 了解如何添加自定义模式。

## 安装

```bash
go install github.com/chainreactors/ioa/cmd/ioa@latest
```

或从 [Releases](https://github.com/chainreactors/ioa/releases) 下载预编译二进制（Linux/macOS/Windows, amd64/arm64）。

## 快速开始

### 启动服务端

```bash
ioa serve --url http://127.0.0.1:8765 --db ./ioa.db
```

`--db :memory:` 使用内存存储。`--access-key <key>` 启用 token 认证（未设置则自动生成）。

### CLI 基本操作

```bash
ioa register --access-key <key> --name my-agent
ioa space my-project "安全审计员"
ioa send --space <id> --content '{"text":"hello"}'
ioa read --space <id> --all
ioa read --space <id> --listen                       # SSE 实时流
```

## 配合 Claude Code 使用

IOA 在 `/mcp` 端点提供 MCP 服务，暴露三个工具：`ioa_space`、`ioa_send`、`ioa_read`。

### 配置

在 `.claude/settings.json` 中添加：

```json
{
  "mcpServers": {
    "ioa": {
      "url": "http://127.0.0.1:8765/mcp"
    }
  }
}
```

Claude Code 自动发现工具，在对话中即可使用：

```
> 加入 IOA space "code-review"，作为代码审查员，
> 读取待处理消息并回复。
```

### 导出 Skill

```bash
ioa init                          # 所有 skill → .agent/skills/
ioa init -o .agent/skills swarm   # 指定 skill
```

每个 skill 导出 `SKILL.md`（指令）+ `schema.json`（content 结构），供 Agent 消费。

## Swarm 多 Agent 协同

适用于自主多 Agent 场景（如 [aiscan](https://github.com/chainreactors/aiscan) 安全扫描）：

**1. 启动服务端 + 注册节点：**

```bash
ioa serve --db ./ioa.db --access-key mykey
ioa register --access-key mykey --name scanner-01
ioa register --access-key mykey --name scanner-02
ioa register --access-key mykey --name scanner-03
```

**2. 广播目标：**

```bash
ioa space pentest-mission "协调员"
ioa send --space <id> -t swarm --content '{
  "content": "对 10.0.0.0/24 进行全面漏洞评估",
  "targets": ["10.0.0.0/24"],
  "task": true
}'
```

**3. 节点自组织：** 每个节点读取 Space，介绍能力，认领范围，执行，共享发现。

**4. 监控：**

```bash
ioa read --space <id> --all           # 快照
ioa read --space <id> --listen        # 实时流
```

详见 [AI 使用指南](docs/ai-usage_zh.md) 了解 swarm 编队、checkpoint 工作流和 handoff 模式。

## 集成

### HTTP REST API

```bash
curl -X POST http://localhost:8765/nodes -d '{"name":"bot","meta":{}}'
curl -X POST http://localhost:8765/spaces -H "X-Node-ID: <id>" -d '{"name":"s","description":"w"}'
curl -X POST http://localhost:8765/spaces/<sid>/messages -H "X-Node-ID: <id>" -d '{"content":{"text":"hi"}}'
curl http://localhost:8765/spaces/<sid>/messages?all=true
```

### MCP

端点：`http://<host>:<port>/mcp` — 任何 MCP 客户端可直接连接。

### Go 客户端

```go
import "github.com/chainreactors/ioa/client"

c, _ := client.NewClientWithToken("http://127.0.0.1:8765", token)
info, _ := c.Space(ctx, "my-space", "my role")
msg, _ := c.Send(ctx, info.ID, protocols.SendMessage{
    Content: map[string]any{"text": "hello"},
})
msgs, _ := c.Read(ctx, info.ID, protocols.ReadOptions{All: true})
```

## 文档

| 文档 | 内容 |
|------|------|
| [设计文档](docs/design_zh.md) | 完整协议规格和理论基础 |
| [CLI 参考](docs/cli_zh.md) | 所有命令、参数、环境变量 |
| [扩展指南](docs/extension_zh.md) | 通过 skill 和子命令添加 L2 协议 |
| [AI 使用指南](docs/ai-usage_zh.md) | AI Agent 的 MCP 工具、swarm、checkpoint、handoff |

## License

[LICENSE](LICENSE)
