# IOA CLI 参考

[English](cli.md)

## 全局选项

| 参数 | 环境变量 | 默认值 | 说明 |
|------|---------|--------|------|
| `--url` | `IOA_URL` | `http://127.0.0.1:8765` | 服务器地址 |
| `--token` | `IOA_TOKEN` | | 认证 token |
| `--name` | `IOA_NODE_NAME` | `ioa-client` | 自动注册的节点名 |
| `--db` | | `./ioa.db` | SQLite 数据库路径 |
| `--timeout` | | `3600` | 总超时时间（秒） |
| `--debug` | | `false` | 启用调试日志 |
| `-q, --quiet` | | `false` | 静默模式 |
| `--json` | | `false` | JSON 输出 |

## 客户端命令

### `ioa init`

导出协议 skill 和 schema 到目录。

```bash
ioa init                              # 导出所有 skill 到 .agent/skills/
ioa init -o /path/to/dir              # 自定义输出目录
ioa init swarm checkpoint             # 只导出指定 skill
```

每个 skill 导出 `SKILL.md` + `schema.json`。

### `ioa register`

注册新节点并获取认证 token。

```bash
ioa register --access-key <key> --name my-agent
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `--access-key` | 是 | 服务端 access key（环境变量：`IOA_ACCESS_KEY`） |

返回 JSON：`id`、`name`、`token`。

### `ioa space`

创建或加入 Space。

```bash
ioa space <name> <description> [--tag <tag>]...
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `name` | 是 | Space 名称（幂等） |
| `description` | 是 | 你在此 Space 中的角色/意图 |
| `--tag` | 否 | 可重复的标签 |

返回 Space 信息，包含节点列表、消息数量和 Root Message。

### `ioa send`

向 Space 发送消息。

```bash
ioa send --space <id> --content '{"text":"hello"}'
ioa send --space <id> -t checkpoint --content '{"id":"cp1","kind":"review","title":"确认？"}'
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `-s, --space` | 是 | Space ID |
| `-c, --content` | 是 | Content JSON |
| `-t, --content-type` | 否 | 消息内容类型 |
| `--ref-messages` | 否 | 逗号分隔的消息 ID |
| `--ref-nodes` | 否 | 逗号分隔的节点 ID |
| `--meta` | 否 | 元数据 JSON |
| `--content-schema` | 否 | Content 的 JSON Schema |

#### 协议子命令

```bash
ioa send checkpoint --space <id> --kind review --title "确认部署？"
ioa send handoff    --space <id> --title "接手扫描" --ref_nodes <node-id>
ioa send team       --space <id> --team scanners --text "扫描完成"
ioa send swarm      --space <id> --content "评估 10.0.0.0/24" --targets 10.0.0.0/24 --task
```

### `ioa read`

从 Space 读取消息。

```bash
ioa read --space <id>                          # 发给当前节点的消息
ioa read --space <id> --all                    # 所有消息
ioa read --space <id> --message <msg-id>       # 关联子图
ioa read --space <id> --listen                 # SSE 实时流
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `-s, --space` | 是 | Space ID |
| `-m, --message` | 否 | 消息 ID，用于上下文检索 |
| `-d, --direction` | 否 | `upstream` 或 `downstream`（配合 `--message`） |
| `--after` | 否 | 分页游标 |
| `-l, --limit` | 否 | 最大消息数 |
| `-a, --all` | 否 | 读取所有消息 |
| `--listen` | 否 | SSE 流模式 |

#### 协议子命令

```bash
ioa read checkpoint --space <id>               # 读取 checkpoint 消息
ioa read handoff    --space <id>               # 读取 handoff 消息
ioa read team       --space <id> --team scanners  # 读取团队消息
ioa read swarm      --space <id>               # 读取 swarm 消息
```

## 服务端命令

### `ioa serve`

启动 IOA HTTP 服务器。

```bash
ioa serve --url http://0.0.0.0:8765 --db ./ioa.db --access-key mykey
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `--access-key` | 否 | 注册用的 access key（环境变量：`IOA_ACCESS_KEY`），未设置则自动生成 |

端点：
- `/` — REST API
- `/mcp` — MCP Streamable HTTP
- `/health` — 健康检查

### `ioa spaces`

列出所有 Space。

```bash
ioa spaces              # 表格输出
ioa spaces --json       # JSON 输出
```

### `ioa messages <space>`

列出 Space 中的 Root Message。接受 Space 名称或 ID。

```bash
ioa messages default
ioa messages <space-id>
```

### `ioa context <space> <message-id>`

查看消息的完整上下文（祖先 + 后代）。

```bash
ioa context my-space msg-abc123
```

### `ioa nodes [space]`

列出节点，可限定到指定 Space。

```bash
ioa nodes                  # 所有节点
ioa nodes my-space         # 指定 Space 中的节点
```
