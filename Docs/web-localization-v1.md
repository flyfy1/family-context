# Family Daily — 网页多语言 V1

> 日期：2026-08-22
> 状态：已实现

## 范围

- 网页界面支持英文 `en` 与简体中文 `zh`；
- 新浏览器首次打开默认英文；
- 顶部语言选择器即时切换，选择保存在当前浏览器的 `fd.language`；
- 页面 `<html lang>`、日期格式、按钮、提示、空状态、老人模式和设置页面同步切换；
- Family Daily 与儿童睡前故事按所选语言生成、保存和查询；
- 历史中文 AI 内容不自动翻译，数据库迁移后标记为 `zh`。

Android 本地化、自动检测系统语言、第三种语言、历史内容翻译和后端所有错误文案本地化不在本版本范围内。

## 语言契约

| 值 | 含义 | 日期区域 |
|---|---|---|
| `en` | English，默认 | `en-US` |
| `zh` | 简体中文 | `zh-CN` |

不支持的语言值返回 `400`。省略语言时按 `en` 处理。

## Family Daily API

查询指定语言的最新日报：

```http
GET /api/v1/daily-summaries/latest?familyId=our-family&language=en
X-Family-Token: <family-token>
```

生成指定语言的日报：

```http
POST /api/v1/daily-summaries/generate
Content-Type: application/json
X-Family-Token: <family-token>

{
  "familyId": "our-family",
  "date": "2026-08-22",
  "language": "en"
}
```

响应中的 `language` 表明内容语言。英文和中文日报独立保存、独立查询。

## 睡前故事 API

生成请求增加：

```json
{
  "language": "en"
}
```

列表查询增加 `language=en`。故事文本与 TTS 使用同一个语言值，重试音频时沿用故事已保存的语言。完整接口见 [儿童家庭睡前故事 API V1](./child-bedtime-story-api-v1.md)。

## 手机端接入约定

后续 Android 客户端沿用 `en` / `zh` 两个语言代码。向日报或睡前故事生成接口发请求时应明确提交当前 App 语言；不能依赖后端默认值来猜测用户语言。
