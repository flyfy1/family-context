# Family Daily — 手机照片/视频同步 API V1

> 日期：2026-08-22  
> 状态：后端与 Android 照片自动同步 MVP 已实现；iOS 尚未实现

## 1. 这版打通的用户闭环

```text
手机照片/视频
    ↓ 成员 Bearer Token
先保存到该成员的私人 Space
    ↓
Gemini 内容分析 + 分享对象建议
    ↓
review/manual：等待成员确认
auto：仅在成员明确配置 Prompt 且 AI 建议安全分享时发布
    ↓
生成家庭可见 Update
```

上传不是“直接发动态”。原始媒体始终先成为 `MediaImport`，默认 `shareDecision=pending`。AI 失败不会回滚或删除本地原件。

V1 的实际访问范围仍然只有：

- `private`：仅成员自己；
- `family`：整个家庭。

Gemini 可以从服务端提供的真实家庭成员名单中给出 `suggestedRecipients`，用于回答“建议分享给谁”。这只是审核建议，不会授予访问权限。因为成员级 ACL 尚未实现，“只分享给爸爸和妈妈”不能直接发布；如果建议对象少于整个家庭，即使 `shareMode=auto` 也会保持 `pending`，等待成员处理。

## 2. 鉴权

所有接口都使用成员令牌：

```http
Authorization: Bearer <member-token>
```

成员令牌由管理员创建成员或轮换令牌时签发。服务端数据库只保存令牌哈希。客户端不能提交 `memberId`；服务端只根据 Bearer Token 决定写入哪个成员 Space，避免越权。

## 3. 接口一览

```text
POST /api/v1/me/media-imports
GET  /api/v1/me/media-imports
GET  /api/v1/me/media-imports/{import-id}
POST /api/v1/me/media-imports/{import-id}/decision
```

读取原文件继续使用响应中的鉴权 URL：

```text
GET /space-files/members/{member-id}/media/{file-name}
Authorization: Bearer <member-token>
```

其他成员的令牌不能读取该文件或 `MediaImport`。

## 4. 上传照片或视频

### 请求

```http
POST /api/v1/me/media-imports
Authorization: Bearer <member-token>
Content-Type: multipart/form-data
```

| 字段 | 必填 | 说明 |
|---|---:|---|
| `media` | 是 | 原始照片或视频，最大 100MB |
| `capturedAt` | 否 | 媒体拍摄时间，RFC3339，例如 `2026-08-22T12:00:00+08:00` |
| `deviceId` | 建议 | App 安装/设备的稳定随机 ID，最长 200 字符 |
| `clientMediaId` | 建议 | 客户端为这项媒体生成的稳定 ID，最长 200 字符 |

支持格式：

- 图片：JPEG、PNG、WebP、GIF；
- 视频：MP4、QuickTime/MOV、WebM。

不要把设备序列号、手机号或广告 ID 直接用作 `deviceId`。建议首次安装时生成随机 UUID，存入 App 私有存储。

`deviceId + clientMediaId` 在同一成员下是幂等键。首次创建返回 `201`；相同键的重试返回已有记录和 `200`，不会再次保存、分析或分享。客户端必须在重试时复用同一个 `clientMediaId`。

示例：

```bash
curl -X POST "$API_BASE/api/v1/me/media-imports" \
  -H "Authorization: Bearer $MEMBER_TOKEN" \
  -F "media=@/path/to/photo.jpg;type=image/jpeg" \
  -F "capturedAt=2026-08-22T12:00:00+08:00" \
  -F "deviceId=8ef68f4d-32ce-4aa2-a2c8-2a299795bf89" \
  -F "clientMediaId=media-store-id-10482"
```

### 响应

```json
{
  "id": "...",
  "familyId": "family-default",
  "memberId": "...",
  "mediaType": "image",
  "mimeType": "image/jpeg",
  "originalName": "photo.jpg",
  "mediaUrl": "/space-files/members/.../media/....jpg",
  "capturedAt": "2026-08-22T04:00:00Z",
  "deviceId": "8ef68f4d-32ce-4aa2-a2c8-2a299795bf89",
  "clientMediaId": "media-store-id-10482",
  "sha256": "...",
  "analysisStatus": "ready",
  "analysis": {
    "summary": "一个孩子正在公园骑自行车。",
    "suggestedCaption": "今天在公园练习骑车",
    "people": "一位孩子",
    "activities": ["骑自行车"],
    "containsSensitive": false,
    "suggestedVisibility": "family",
    "suggestedRecipients": [
      {"memberId": "...", "name": "爷爷"},
      {"memberId": "...", "name": "奶奶"}
    ],
    "recipientReason": "祖辈可能会关心孩子学会的新活动",
    "reason": "符合成员配置的普通家庭活动分享规则"
  },
  "shareDecision": "pending",
  "createdAt": "2026-08-22T04:10:00Z",
  "updatedAt": "2026-08-22T04:10:03Z"
}
```

`sha256` 是服务端收到的原始字节的 SHA-256。客户端可以与本地值比较，确认上传内容一致。

## 5. 分析状态

| `analysisStatus` | 含义 | 客户端行为 |
|---|---|---|
| `processing` | 原件已经落盘，但服务在同步分析期间中断 | V1 不会后台恢复；仍可手动决定是否分享 |
| `ready` | 分析完成 | 展示摘要与分享建议 |
| `failed` | Gemini 或本地策略读取失败 | 保留私密；允许用户手动决定 |
| `skipped_too_large` | 文件已保存，但超过当前 14MB 自动分析上限 | 保留私密；允许用户手动决定 |

当前请求是同步处理：小媒体在 `POST` 响应前完成 Gemini 分析。V1 不增加任务队列和轮询任务资源。

14MB 是服务端对原始文件的保守限制，因为媒体会进行 Base64 编码，Gemini 内嵌图片/短视频请求的总大小限制还包括 Prompt。大文件仍允许同步到 NAS/本地 Space，但不会上传到 Gemini Files API。升级版可增加本地后台任务，并在分析后主动删除 Gemini 临时文件。

参考：[Gemini 图片理解](https://ai.google.dev/gemini-api/docs/image-understanding)、[Gemini 视频理解](https://ai.google.dev/gemini-api/docs/video-understanding)。

## 6. 分享策略如何生效

成员继续通过以下接口配置策略：

```http
PUT /api/v1/me/share-policy
Content-Type: application/json

{
  "shareMode": "review",
  "sharePrompt": "孩子的新活动可以建议分享给爷爷奶奶；工作截图、证件、医疗资料和精确地址不要分享。"
}
```

| 模式 | 上传后的行为 |
|---|---|
| `manual` | AI 可以根据已保存的 Prompt 给建议，但绝不自动发布；成员手动分享 |
| `review` | AI 根据 Prompt 给建议，但始终等待成员确认 |
| `auto` | 仅当 Prompt 非空、内容不敏感、AI 建议 `family`，并且建议对象覆盖除上传者外的全部家庭成员时，才自动生成家庭 Update；部分成员建议保持待确认 |

无论 Prompt 写了什么，它都不能跨越成员令牌和文件目录权限。AI 只能返回候选名单中的成员 ID；后端会删除不存在、重复或跨家庭的 ID，并用本地成员记录覆盖模型返回的姓名。AI 也被要求不做人脸身份识别、不猜测姓名或敏感属性。敏感内容、没有有效收件人或无法判断时，结果会降级为 `private` 和空收件人列表。

## 7. 确认保留或分享

### 保持私密

```http
POST /api/v1/me/media-imports/{import-id}/decision
Authorization: Bearer <member-token>
Content-Type: application/json

{
  "visibility": "private"
}
```

### 分享给家庭

```http
POST /api/v1/me/media-imports/{import-id}/decision
Authorization: Bearer <member-token>
Content-Type: application/json

{
  "visibility": "family",
  "caption": "今天在公园第一次学会骑车。"
}
```

`caption` 可选，最长 2000 字。为空时优先使用 AI 的 `suggestedCaption`，否则使用通用说明。

家庭分享会创建一个 `source=mobile_media_import` 的 `Update`，并在响应中设置：

```json
{
  "shareDecision": "family",
  "updateId": "..."
}
```

重复提交 `family` 是幂等的，返回同一个 `updateId`。已经分享的记录不能再通过此接口改回私密；管理员可以用既有的 Update 可见性管理接口处理撤回，后续版本应提供成员自己的撤回流程。

即使 `analysisStatus=failed` 或 `skipped_too_large`，成员仍可以明确选择分享。

## 8. 查询记录

```http
GET /api/v1/me/media-imports
GET /api/v1/me/media-imports/{import-id}
Authorization: Bearer <member-token>
```

列表按服务端接收时间倒序返回：

```json
{
  "mediaImports": []
}
```

V1 列表暂未分页。手机端 PoC 可用于验证闭环；相册全量同步前必须加入 cursor 分页。

## 9. 本地持久化

```text
data/
├── family-daily.db
└── spaces/
    ├── members/<member-id>/
    │   ├── media/<import-id>.<ext>       # 原始字节
    │   ├── imports/<import-id>.json      # 可回溯元数据与 AI 结果
    │   └── updates/<update-id>.md        # 分享后生成
    └── shared/updates/<update-id>.json   # 家庭共享投影
```

SQLite 保存 `MediaImport`、分析结果、分享状态和审计事件，是权威索引；成员目录保存原件和可检查的 JSON/Markdown 文件。Gemini 请求设置 `store=false`，不会替代本地持久化。

## 10. 手机端建议调用顺序

1. 从系统相册拿到媒体，并为它保存稳定的 `clientMediaId`。
2. 计算本地 SHA-256（可选但建议）。
3. 调用上传接口；网络失败时用相同幂等键重试。
4. 比较响应 `sha256`。
5. 根据 `analysisStatus` 和 `analysis` 展示审核卡片。
6. 用户选择后调用 `/decision`；`auto` 已分享时直接显示 `updateId`。
7. App 本地记录已完成同步的 `clientMediaId`，避免重复扫描上传。

V1 不定义手机删除本地照片后是否删除服务器原件。客户端不得推断删除语义，也不要自动调用管理员接口。

## 11. Android 客户端实现

Android App 当前实现照片同步，不同步视频：

- 用户在 App 内保存服务地址和成员令牌，令牌位于 App 私有 `SharedPreferences`，不进入构建配置或日志；
- 首次生成随机 `deviceId`，每项使用 `mediastore-image-<MediaStore ID>` 作为稳定 `clientMediaId`；
- 打开或回到 App 时触发一次联网同步；WorkManager 在有网络时安排最短 15 分钟周期的补同步；
- 每次最多处理 50 项，并保存 `(DATE_ADDED, _ID)` 游标；网络/服务端错误不会越过失败项，重试复用同一个幂等键；
- Android 14+ 同时支持“全部照片”和系统的“选定照片”授权；权限变化后暂停并提示重新授权；
- 本地大于 100MB、MIME 不在服务端白名单中的照片会跳过，避免单项永久阻塞整个游标；
- 手机删除不传播到服务器，NAS/本地 Space 中的原件仍保留。

增量窗口默认是最近 3 天，可在 App 连接设置中配置为 1–3650 天：

- 窗口以 `DATE_TAKEN` 拍摄时间判断；媒体没有拍摄时间时退回 `DATE_ADDED`；
- MediaStore 查询同时应用滚动时间窗和 `(DATE_ADDED, _ID)` 增量游标；
- 改变回溯天数会重置手机端游标，从新窗口起点补扫，`deviceId + clientMediaId` 继续保证服务端幂等；
- 时间窗只限制未来扫描范围，不代表保留期限，也不会删除已保存到 NAS 的文件。

这里的“定期”不是严格实时。Android WorkManager 的周期任务最短间隔为 15 分钟，并可能被系统省电策略延迟；App 回到前台时会立即补同步。

## 12. 为大规模相册同步建议的接口升级

现有接口足以验证小规模照片自动同步，但有三个明确上限：

1. `POST /media-imports` 会等待 Gemini 分析结束。初次同步大量历史照片时，后台任务可能超过 Android 的执行窗口。建议增加 `analysisMode=deferred`，原件与 SQLite 索引成功落盘后立即返回 `201`，再由服务器本地任务完成分析。
2. 现代手机可能产生 `image/heic` / `image/heif`。建议后端先原样保存这两种 MIME；如果 Gemini 或网页预览不支持，再生成本地派生预览图，但保留原件为权威数据。
3. `GET /media-imports` 未分页。建议增加 `cursor` 和 `limit`，并允许按 `deviceId + clientMediaId` 批量查询，便于 App 重装后核对服务器已有项，而不是重新发送所有文件。

如果只先验证家庭真实使用，当前接口不需要立刻修改；当首次历史照片超过几十张、出现 HEIC，或一次同步耗时接近 10 分钟时，再优先实现第 1、2 项。
