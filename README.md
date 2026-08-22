# Family Daily

Family Daily V1 正在验证一个最小家庭交流闭环：

```text
家人提问 → 父母语音回答 → Gemini 转写与整理 → 确认分享 → 家人回复
```

当前仓库包含：

- `backend/`：Go + SQLite 后端；
- `backend/web/`：由后端直接提供的响应式网页端；
- `android/`：简单的原生 Android 客户端；
- `Docs/`：产品和开发文档。

## 启动后端

在仓库根目录创建 `.env`：

```dotenv
GEMINI_API_KEY=your-key
```

然后运行：

```bash
cd backend
go run .
```

默认监听 `http://localhost:8080`。本地开发使用 `family-daily-local` 作为临时家庭访问令牌；真实家庭试用前必须替换。

后端启动后，直接在浏览器打开 [http://localhost:8080](http://localhost:8080) 即可使用网页端。网页支持提问、浏览器录音、AI 整理确认、原声播放、重新录制、家庭回复，以及基础家庭称呼配置、状态筛选、转写原文和本地历史浏览。网页配置仅保存在当前浏览器。

所有权威数据都保存在一个本地存储根目录中：SQLite、原始录音、AI 结果和审计历史不会依赖云端数据库。开发环境默认使用 `backend/data`。

生产服务器必须把专用存储盘配置为绝对路径，并在盘内放置标记文件：

```dotenv
APP_ENV=production
FAMILY_DAILY_STORAGE_DIR=/your-mounted-disk/family-daily
```

```bash
touch /your-mounted-disk/family-daily/.family-daily-storage
```

如果专用盘没有挂载、不可写或标记文件消失，生产后端会拒绝启动，避免悄悄把家庭数据写到系统盘。

不调用 Gemini 的本地演示模式：

```bash
cd backend
AI_MODE=stub go run .
```

## 运行 Android App

1. 用 Android Studio 打开 `android/`；
2. 保持后端运行；
3. 启动模拟器或连接 Android 手机；
4. 运行 `app`。

模拟器默认通过 `http://10.0.2.2:8080` 访问电脑上的后端。连接真实手机时，需要在 `android/app/build.gradle.kts` 中把 `API_BASE_URL` 改成手机可访问的局域网或 HTTPS 地址。

## 验证

后端测试：

```bash
cd backend
go test ./...
```

Android 构建：

```bash
cd android
./gradlew :app:assembleDebug
```

产物位于 `android/app/build/outputs/apk/debug/app-debug.apk`。

## 当前边界

这是本地 V1 原型，还没有实现正式账号、家庭邀请、生产部署、Push Notification 和应用商店发布。当前访问令牌只用于阻止完全匿名访问，不能替代正式身份认证。

专用本地盘提供持久化和版本回溯，但它本身不等于备份。正式家庭试用前仍需确定加密备份和永久删除策略。
