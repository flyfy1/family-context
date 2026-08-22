# Family Daily — Backend API 与成员 Space V1

> 日期：2026-08-22
> 状态：本地/NAS PoC 已实现；公网 OAuth 与自动同步待后续

## 权限模型

后端现在有三种权限：

| 权限 | 凭证 | 能力 |
|---|---|---|
| 管理员 | `X-Admin-Token` | 创建和修改成员、签发成员令牌、查看家庭数据、修改 Update 可见范围 |
| 家庭网页 PoC | `X-Family-Token` | 使用当前家庭网页和旧版问答接口 |
| 家庭成员 | `Authorization: Bearer <member-token>` | 访问自己的 Space、上传内容、配置分享策略、连接自己的 MCP |

生产环境要求 `ADMIN_API_TOKEN` 与 `FAMILY_API_TOKEN` 不同。成员令牌在数据库中只保存 SHA-256 哈希；明文只会在管理员创建成员或轮换令牌时返回一次。

## 管理员 API

```text
GET  /api/v1/admin/members
POST /api/v1/admin/members
PUT  /api/v1/admin/members/{member-id}
POST /api/v1/admin/members/{member-id}/token

GET  /api/v1/admin/updates
PUT  /api/v1/admin/updates/{update-id}/visibility
```

创建成员会同时创建：

```text
spaces/members/<member-id>/
├── profile.json
├── share-policy.json       # 首次保存策略后出现
├── private/
├── updates/
├── media/
├── imports/                # 手机媒体导入元数据与 AI 分析结果
├── summaries/
├── jobs/
└── context/                # MCP 文件工具只能读写这里
```

管理员修改可见范围不会物理删除成员的权威记录。将 Update 改为 `private` 时，只移除家庭共享投影，成员 Space 中的原始记录和审计事件继续保留。

V1 暂不提供管理员永久删除接口。永久删除必须同时覆盖 SQLite、成员文件、共享投影、媒体、历史版本和敏感审计载荷，不能用简单的单表删除代替。

## 成员 API

```text
GET  /api/v1/me
GET  /api/v1/me/updates
POST /api/v1/me/updates/text
POST /api/v1/me/updates/image
POST /api/v1/me/media-imports
GET  /api/v1/me/media-imports
GET  /api/v1/me/media-imports/{import-id}
POST /api/v1/me/media-imports/{import-id}/decision

POST /api/v1/bedtime-stories
GET  /api/v1/bedtime-stories
GET  /api/v1/bedtime-stories/{story-id}
GET  /api/v1/bedtime-stories/{story-id}/audio
POST /api/v1/bedtime-stories/{story-id}/audio
GET  /api/v1/me/share-policy
PUT  /api/v1/me/share-policy
GET  /api/v1/me/mcp-sessions
POST /api/v1/me/mcp-sessions
DELETE /api/v1/me/mcp-sessions/{session-id}
```

图片上传使用 `multipart/form-data`：

- `image`：JPEG、PNG、WebP 或 GIF，最大 25MB；
- `text`：可选说明；
- `visibility`：`private` 或 `family`。

媒体先写入成员的 `media/`，然后写成员 Markdown 记录和 SQLite 索引。家庭可见内容另外生成 `spaces/shared/updates/<update-id>.json` 投影。

手机照片/视频同步使用独立的私密导入流，不会在上传时默认公开。完整请求字段、幂等规则、AI 状态和审核流程见 [手机照片/视频同步 API V1](./mobile-media-sync-api-v1.md)。

儿童睡前故事只使用家庭可见 Update，先持久化故事文本与来源，再通过 Gemini TTS 生成本地 WAV。完整契约见 [儿童家庭睡前故事 API V1](./child-bedtime-story-api-v1.md)。

## 分享策略

每位成员可以配置：

```json
{
  "shareMode": "manual | review | auto",
  "sharePrompt": "成员定义的分享判断规则"
}
```

- `manual`：其他 AI 只能写私人 Update，由成员手动分享；
- `review`：预留给“AI 建议、成员确认”的审核流，V1 不自动发布；
- `auto`：MCP 可以请求创建家庭可见 Update，但必须同时配置非空 Prompt。每次请求都会由 Gemini 根据该 Prompt 返回允许或拒绝；拒绝、无法判断或 AI 调用失败时一律不分享。

Prompt 是策略输入，不是权限本身。即使 Prompt 要求跨成员读取，MCP 也不能读取其他成员的私密内容、目录或令牌边界之外的数据。

## MCP 接口

每位成员的端点：

```text
POST /mcp/members/{member-id}
GET  /mcp/members/{member-id}      # V1 返回 405，不提供 SSE 长连接
DELETE /mcp/members/{member-id}    # 结束会话
```

MCP 端点既接受成员主令牌，也接受单独签发的 MCP 访问令牌。外部客户端应使用 MCP 访问令牌，不应复制成员主令牌：

```http
Authorization: Bearer <fdmcp-token>
Content-Type: application/json
```

成员可以通过 `/api/v1/me/mcp-sessions` 为 ChatGPT、Claude Code 或其他客户端分别创建具名访问会话。管理员也可以通过 `/api/v1/admin/members/{member-id}/mcp-sessions` 代为创建、查看和撤销。

- 默认有效期为 45 天，满足至少 30 天的接入周期；
- Access Token 只在创建响应中返回一次，SQLite 只保存 SHA-256 哈希；
- 每个 Token 固定绑定一个成员，无法访问另一个成员的 MCP 端点；
- 撤销立即生效；旋转成员主令牌时会撤销该成员的所有 MCP 访问会话；
- Access Token 可跨服务重启继续使用；`Mcp-Session-Id` 只是一次 MCP 协议连接的短期状态，客户端重连时可以重新 `initialize`。

Claude Code 可以直接配置远程 HTTP MCP：

```bash
claude mcp add --transport http family-daily \
  https://family-api.example/mcp/members/<member-id> \
  --header "Authorization: Bearer <fdmcp-token>"
```

不要把 Token 放进 URL、仓库或聊天消息。

ChatGPT 自定义 MCP 应用填写同一个远程端点并选择 OAuth。后端提供：

```text
GET  /.well-known/oauth-protected-resource/mcp/members/{member-id}
GET  /.well-known/oauth-authorization-server
POST /oauth/register
GET  /oauth/authorize
POST /oauth/authorize
POST /oauth/token
```

实现使用 Authorization Code + PKCE S256、Resource Indicator、动态客户端注册、一次性五分钟授权码、一小时 Access Token，以及轮换的 60 天 Refresh Token。授权页面接受该成员的主令牌或尚未撤销/过期的 `fdmcp_` 会话令牌，并再次校验令牌成员必须与 `resource` 中的成员一致。`offline_access` 会返回 Refresh Token；旧 Refresh Token 在轮换后立即失效。旋转成员主令牌时，会同时撤销该成员的 Bearer 会话、短期 Access Token 和所有 OAuth Refresh Token。

生产环境必须设置 HTTPS origin 形式的 `PUBLIC_BASE_URL`，否则后端拒绝启动。它用于生成 MCP Endpoint、Protected Resource Metadata、Issuer 和 OAuth 回调发现信息，不能依赖外部请求传入的 Host 猜测生产地址。

实现基于 MCP `2025-11-25` Streamable HTTP 的 JSON 响应模式：客户端先调用 `initialize`，保存响应中的 `Mcp-Session-Id`，后续请求携带该 Header。

V1 工具：

```text
list_updates
list_family_updates
get_share_policy
list_context_files
read_context_file
write_context_file
create_update
```

文件工具只接受扁平的 `.md`、`.txt` 或 `.json` 文件名，单文件最大 512KB。不能使用绝对路径、`..`、子目录、符号链接、NAS 根路径或执行命令。

`list_updates` 读取当前成员自己的 Space，包括自己的 private 与 family 动态。`list_family_updates` 只读取同一家庭内标记为 family 的动态，并返回作者姓名/角色；支持可选 RFC 3339 `since` 和 1–100 的 `limit`，不会返回其他成员的 private 动态或其他家庭的数据。

Claude Code 使用具名 Bearer 会话；ChatGPT 使用 OAuth 发现、PKCE 和轮换 Refresh Token。两种入口最终都只解析为一个成员身份：可以读取这个成员自己的 Space、全家可见动态和该成员的 `context/`，但不能读取其他成员私密内容。

兼容性依据：[MCP 2025-11-25 Authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization)、[OpenAI MCP 文档](https://developers.openai.com/api/docs/mcp/) 和 [Claude Code MCP 文档](https://code.claude.com/docs/id/mcp)。

## 手机自动同步边界

当前已经实现手机照片/视频的私密导入、客户端幂等键、文件校验值、Gemini 分析建议和确认分享。“自动同步”仍缺少：

- 设备注册与撤销；
- 每设备 Scope；
- 分块和断点续传；
- 网络和充电条件；
- 删除同步语义；
- 后台任务状态。

这些应在网页 PoC 验证后单独实现，不能把相册或监控目录直接暴露给 MCP。

## NAS 配置

NAS 可以作为 `FAMILY_DAILY_STORAGE_DIR` 对应的已挂载目录。生产启动仍要求 `.family-daily-storage` 标记文件；挂载消失时后端必须拒绝启动，避免数据落到系统盘上的同名目录。

NAS 容量充足不代表已经备份。仍需异盘或离线备份、数据库一致性快照、恢复演练、容量告警和挂载健康检查。
