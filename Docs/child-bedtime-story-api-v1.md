# Family Daily — 儿童家庭睡前故事 API V1

> 日期：2026-08-22  
> 状态：后端和 Android 端已实现

## 产品闭环

```text
家长选择孩子、年龄和最近天数
        ↓
后端只读取 visibility=family 的 Update
        ↓
Gemini 生成适龄故事和实际采用的来源 ID
        ↓
故事文本、来源和状态先保存到本地
        ↓
Gemini TTS 按故事语言生成旁白
        ↓
WAV 音频保存到家庭本地存储并可播放
```

这个功能不会读取成员私人 Space、`visibility=private` 的 Update、监控录像或未主动分享的手机媒体。

## API

```text
POST /api/v1/bedtime-stories
GET  /api/v1/bedtime-stories
GET  /api/v1/bedtime-stories/{story-id}
GET  /api/v1/bedtime-stories/{story-id}/audio
POST /api/v1/bedtime-stories/{story-id}/audio
```

V1 由家庭应用中已登录的家长/照护者发起，使用其个人登录会话：

```http
Authorization: Bearer <member-session>
```

## 生成故事

```http
POST /api/v1/bedtime-stories
Content-Type: application/json
Authorization: Bearer <member-session>

{
  "familyId": "our-family",
  "childId": "<member-id>",
  "audienceAge": 6,
  "days": 7,
  "language": "en"
}
```

字段：

| 字段 | 必填 | 说明 |
|---|---:|---|
| `familyId` | 否 | 默认 `our-family` |
| `childId` | 是 | 家庭成员 ID，且该成员的 `role` 必须是 `child` |
| `audienceAge` | 否 | 3–12，默认 6，只用于本次故事的语言难度 |
| `days` | 否 | 1–30，默认 7 |
| `language` | 否 | `en` 或 `zh`，默认 `en`；同时控制故事文本与 TTS 语言 |

后端最多把时间范围内最近 40 条家庭可见 Update 提供给 Gemini。没有家庭可见内容时返回 `409`，不会用私人内容补足故事。

成功响应：

```json
{
  "id": "...",
  "familyId": "our-family",
  "childId": "...",
  "childName": "瓜瓜",
  "audienceAge": 6,
  "language": "en",
  "title": "Guagua and the Starlight by the Window",
  "content": "…",
  "sourceUpdateIds": ["update-1", "update-2"],
  "voice": "Kore",
  "audioUrl": "/api/v1/bedtime-stories/.../audio",
  "status": "ready",
  "createdAt": "2026-08-22T04:20:00Z",
  "updatedAt": "2026-08-22T04:21:00Z"
}
```

`sourceUpdateIds` 由 Gemini 声明，但后端会再次验证：每个 ID 必须属于本次提供的家庭可见 Context，否则整个生成请求失败，避免模型伪造来源。

## 故事和音频状态

| `status` | 含义 |
|---|---|
| `text_ready` | 文本与来源已持久化，TTS 调用还未完成或服务在调用期间中断 |
| `ready` | 文本和 WAV 音频均已保存在本地 |
| `audio_failed` | 文本和来源已保存，但 Gemini TTS 或本地音频写入失败 |

TTS 失败仍返回 `201` 和完整故事文本，而不是丢弃已经生成的故事。客户端可调用下面的幂等接口重试音频；已经是 `ready` 时直接返回现有故事，不会重复生成：

```http
POST /api/v1/bedtime-stories/{story-id}/audio
Authorization: Bearer <member-session>
```

## 查询和播放

```http
GET /api/v1/bedtime-stories?familyId=our-family&childId=<member-id>&language=en
Authorization: Bearer <member-session>
```

返回最近 50 个故事：

```json
{
  "bedtimeStories": []
}
```

读取单个故事：

```http
GET /api/v1/bedtime-stories/{story-id}
Authorization: Bearer <member-session>
```

播放音频：

```http
GET /api/v1/bedtime-stories/{story-id}/audio
Authorization: Bearer <member-session>
```

音频响应为 `audio/wav`。前端如果通过 `fetch` 请求，需要把响应读取成 Blob 后交给 `<audio>` 或 Web Audio API。

## 儿童安全和事实边界

故事 Prompt 要求：

- 只把家庭可见 Update 当作真实事件来源；
- 不添加对话、旅行、健康结果、人物关系等事实；
- 可以用月光、星星、小动物作为明确的童话连接；
- 不制造危险、羞耻、比较或焦虑；
- 不提供健康或医疗建议；
- 可以省略不适合睡前讲的细节；
- 不在人名之外推断身份和关系。

这仍然是生成式内容。面向孩子播放前，第一版产品界面应显示故事全文和来源，让家长可以先阅读；“完全无人审核地每天自动播放”不在 V1 范围内。

## Gemini TTS 配置

默认配置：

```text
GEMINI_TTS_MODEL=gemini-3.1-flash-tts-preview
GEMINI_TTS_VOICE=Kore
```

两项都可以通过环境变量覆盖。TTS 请求使用 `store=false`，输出的原始 PCM 会在后端包装为标准 24kHz、单声道、16-bit WAV（如果 Gemini 已返回 WAV，则直接保留）。

Gemini TTS 当前是 Preview；官方接口通过 `response_format: {"type":"audio"}` 和 `generation_config.speech_config` 选择音色。参考：[Gemini TTS 文档](https://ai.google.dev/gemini-api/docs/speech-generation)。

## 本地数据

```text
data/
├── family-daily.db
└── spaces/shared/stories/
    ├── <story-id>.json
    └── <story-id>.wav
```

SQLite 的 `bedtime_stories` 表保存故事文本、语言、孩子、年龄、来源 Update ID、音色、音频状态和审计信息。JSON/WAV 文件用于本地检查、备份和播放。升级前生成的故事会迁移为 `zh`，避免它们出现在默认英文列表。

## Android V1

- 普通成员和孩子模式都可选择家庭中的孩子并生成故事；
- 故事列表按孩子和当前 App 语言读取；
- 生成完成后可在 App 内阅读并播放后端保存的 WAV 音频；
- 老人模式保持简化，不在首屏显示故事生成选项。

## Later

- Web 端的家长预览、播放和“重新生成”页面；
- 定时生成与睡前提醒；
- 家长编辑文本后再合成音频；
- 细粒度家庭成员收件人权限；
- 多人配音和第三种以上语言；
- 按孩子保存长期年龄与故事偏好。
