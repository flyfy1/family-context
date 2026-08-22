# Family Daily V1 — 本地存储与回溯约定

> 日期：2026-08-22  
> 状态：后端约束已实现；实际服务器挂载路径待配置

## 目标

Family Daily 的权威家庭数据保存在服务器本地专用存储盘上。

云端 AI 可以暂时处理一段录音，但不充当数据库、文件存储或历史记录来源。服务端发送 Gemini 请求时使用无状态模式；本地必须保留：

- 问题；
- 原始录音；
- 转写；
- AI 摘要；
- 分享状态；
- 家庭回复；
- 被重新录制替换的旧草稿；
- 每次状态变化的审计事件。

## 存储布局

```text
<FAMILY_DAILY_STORAGE_DIR>/
├── .family-daily-storage
├── family-daily.db
├── family-daily.db-wal
├── family-daily.db-shm
├── media/
│   └── <answer-id>.<audio-extension>
└── backups/
```

`backups/` 只是预留的本地快照目录。V1 还没有把同盘快照当成灾难恢复方案。

## 启动保护

开发环境允许使用仓库下的 `backend/data`。

生产环境必须满足：

1. `APP_ENV=production`；
2. `FAMILY_DAILY_STORAGE_DIR` 是绝对路径；
3. 路径已经存在；
4. 根目录中存在普通文件 `.family-daily-storage`；
5. 目录可写；
6. 存储根目录不能是 `/`。

任一条件失败，后端拒绝启动。标记文件应创建在已经挂载的专用盘里面；盘掉线后标记随之消失，从而避免程序写入同名的系统盘目录。

示例仅用于说明，不代表当前机器的真实路径：

```dotenv
APP_ENV=production
FAMILY_DAILY_STORAGE_DIR=/your-mounted-disk/family-daily
```

## 写入顺序

回答录音的顺序必须是：

```text
录音写入本地临时文件
→ 刷入磁盘并原子改名
→ SQLite 创建 processing 记录和审计事件
→ 调用 Gemini
→ SQLite 保存转写、摘要或失败状态
→ 追加处理结果审计事件
```

这样即使 Gemini 超时，原始录音仍然存在；即使应用在 AI 返回前退出，数据库中也能看到待恢复的 `processing` 记录。

## 回溯语义

V1 的“重新录制”不是永久删除：

- 当前草稿从正常家庭动态中移除；
- 旧回答复制到 `archived_answers`；
- 原始录音继续保存在本地盘；
- 写入 `answer.archived_for_rerecord` 审计事件；
- 同一个问题可以重新提交新回答。

问题历史可通过受家庭令牌保护的接口读取：

```text
GET /api/v1/questions/{question-id}/history
```

永久删除是不同的产品动作。实现时必须同时清除当前数据、历史版本、审计载荷中的敏感内容和对应媒体，并留下不含内容的删除凭证。V1 尚未开放这个入口。

## Gemini 边界

“数据保存在本地”指所有持久化权威副本都在本地，并不表示音频永远不会离开服务器。为了转写和整理，录音会通过加密网络发送给 Gemini 处理。

当前请求设置 `store=false`，不使用 Gemini 的服务端会话历史。正式试用前仍需在用户界面明确告知 AI 处理行为并取得同意。

## 尚未完成

- 实际专用盘挂载路径和服务器本机配置；
- 加密异盘或离线备份；
- 自动快照计划；
- 数据恢复演练；
- 用户发起的永久删除；
- 磁盘容量与健康告警。

专用盘能避免进程重启后丢数据，但单盘损坏仍会造成数据丢失，因此不能把“本地存储”误认为“已有备份”。
