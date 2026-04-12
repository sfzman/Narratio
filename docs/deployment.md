# docs/deployment.md — 部署与运行

## 本地开发环境

### 前置依赖

```bash
# Go 1.22+
go version

# FFmpeg 5.0+
ffmpeg -version

# Node.js 20+ (前端)
node -v
```

### 启动步骤

```bash
# 1. 克隆项目
git clone https://github.com/your-org/narratio.git
cd narratio

# 2. 配置环境变量
cp backend/.env.example backend/.env
# 编辑 .env 填入各 API Key

# 3. 启动后端
cd backend
go run cmd/server/main.go

# 4. 启动前端（新终端）
cd frontend
npm install
npm run dev
```

后端默认端口：`8080`  
前端默认端口：`5173`（Vite）

前端环境变量约定：

- `VITE_API_BASE_URL`：后端 API 基地址，默认 `/api/v1`
- 开发态默认通过 Vite 代理把 `/api/v1` 转发到 `http://localhost:8080`
- 若本地后端不在默认 `8080`，可通过 `VITE_API_PROXY_TARGET=http://127.0.0.1:8081` 这类方式覆盖代理目标
- 若前后端端口、域名或反向代理路径发生变化，再通过 `.env.local` 覆盖

后端配置加载规则：

- 先读取当前进程已有环境变量
- 若当前目录存在 `.env`，自动补充读取
- 若从仓库根目录启动，也会尝试读取 `backend/.env`
- `.env` 中的值不会覆盖已经存在的系统环境变量

当前开发态 CORS 策略：

- 后端对前端调试接口开启宽松 CORS
- 允许 `http://localhost:5173` 这类本地 Vite 页面直接访问 `http://localhost:8080`
- 当前实现对所有 origin 返回 `Access-Control-Allow-Origin: *`

### .env.example

```bash
PORT=8080
GIN_MODE=debug

DATABASE_DRIVER=sqlite
DATABASE_DSN=./narratio.db
ENABLE_LIVE_TEXT_GENERATION=false
ENABLE_LIVE_IMAGE_GENERATION=false
ENABLE_LIVE_VIDEO_GENERATION=false

DASHSCOPE_TEXT_API_KEY=your-dashscope-text-key-here
DASHSCOPE_TEXT_BASE_URL=https://coding.dashscope.aliyuncs.com/v1
DASHSCOPE_TEXT_MODEL=qwen-max
DASHSCOPE_TEXT_REQUEST_TIMEOUT_SECONDS=600
DASHSCOPE_TEXT_MAX_RETRIES=2
DASHSCOPE_TEXT_RETRY_BACKOFF_SECONDS=2

DASHSCOPE_IMAGE_API_KEY=your-dashscope-image-key-here
DASHSCOPE_IMAGE_BASE_URL=https://dashscope.aliyuncs.com/api/v1
DASHSCOPE_IMAGE_MODEL=qwen-image-2.0

DASHSCOPE_VIDEO_API_KEY=your-dashscope-video-key-here
DASHSCOPE_VIDEO_BASE_URL=https://dashscope.aliyuncs.com
DASHSCOPE_VIDEO_MODEL=wan2.6-i2v-flash
DASHSCOPE_VIDEO_SUBMIT_TIMEOUT_SECONDS=60
DASHSCOPE_VIDEO_POLL_INTERVAL_SECONDS=10
DASHSCOPE_VIDEO_MAX_WAIT_SECONDS=900
DASHSCOPE_VIDEO_MAX_REQUEST_BYTES=6291456
DASHSCOPE_VIDEO_RESOLUTION=720P
DASHSCOPE_VIDEO_NEGATIVE_PROMPT=
DASHSCOPE_VIDEO_IMAGE_JPEG_QUALITY=80
DASHSCOPE_VIDEO_IMAGE_MIN_JPEG_QUALITY=45

TTS_API_BASE_URL=https://your-tts-service.com
TTS_JWT_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"
TTS_JWT_EXPIRE_SECONDS=300
TTS_REQUEST_TIMEOUT_SECONDS=300
TTS_DEFAULT_VOICE_ID=male_calm
TTS_EMOTION_PROMPT=https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/male-read-emo.wav

WORKSPACE_DIR=./workspace
BACKGROUND_RUNNER_WORKER_COUNT=4
RESOURCE_LOCAL_CPU_CONCURRENCY=4
RESOURCE_LLM_TEXT_CONCURRENCY=2
RESOURCE_TTS_CONCURRENCY=3
RESOURCE_IMAGE_GEN_CONCURRENCY=2
RESOURCE_VIDEO_GEN_CONCURRENCY=1
RESOURCE_VIDEO_RENDER_CONCURRENCY=1
SCRIPT_TIMEOUT_PER_SEGMENT_SECONDS=200
SHOT_VIDEO_TIMEOUT_PER_SHOT_SECONDS=200
VIDEO_RENDER_TIMEOUT_SECONDS=1800
FFMPEG_STARTUP_CHECK_TIMEOUT_SECONDS=10
SHOT_VIDEO_DEFAULT_DURATION_SECONDS=3
```

当前代码状态：

- `cmd/server` 目前已完成配置读取、SQLite store 初始化、`segmentation / outline / character_sheet / script / character_image / tts / image / shot_video / video` executor registry 初始化，以及 `app/jobs` / `scheduler.Service` 组装
- 已启动最小 Gin HTTP server，并开放 `GET /api/v1/health`、`GET /api/v1/voices`、`POST /api/v1/jobs`、`GET /api/v1/jobs/:job_id`、`GET /api/v1/jobs/:job_id/tasks` 与开发态 `POST /api/v1/jobs/:job_id/dispatch-once`
- SQLite 模式会在启动时自动执行首个 migration，当前首版 schema 初始化是幂等的，可重复启动
- 当前已接入最小后台 scheduler runner，`POST /jobs` 后会自动持续推进 job
- 后台 runner 当前支持跨 job 并发 worker 池；并发 worker 数由 `BACKGROUND_RUNNER_WORKER_COUNT` 控制，默认 `4`
- `segmentation / outline / character_sheet / script / tts / character_image / image / shot_video` 成功后会把结构化结果写入 `WORKSPACE_DIR/jobs/{job_id}/...`
- `video` 在 runtime 中已切到真实 FFmpeg 渲染路径；成功后会把最终成片写到 `WORKSPACE_DIR/jobs/{job_id}/output/final.mp4`
- 资源池并发上限当前已支持通过环境变量配置：`RESOURCE_LOCAL_CPU_CONCURRENCY`、`RESOURCE_LLM_TEXT_CONCURRENCY`、`RESOURCE_TTS_CONCURRENCY`、`RESOURCE_IMAGE_GEN_CONCURRENCY`、`RESOURCE_VIDEO_GEN_CONCURRENCY`、`RESOURCE_VIDEO_RENDER_CONCURRENCY`
- 服务启动时会先执行一次 FFmpeg 启动检查；若本机 `ffmpeg` 不可用，runtime 会直接启动失败
- `image` / `character_image` 已支持注入真实 DashScope client；只有显式打开 `ENABLE_LIVE_IMAGE_GENERATION=true` 且配置了 `DASHSCOPE_IMAGE_API_KEY` 时，才会尝试真实图片请求

注意：

- 当前默认是 skeleton 模式，`ENABLE_LIVE_TEXT_GENERATION=false`
- `DASHSCOPE_TEXT_REQUEST_TIMEOUT_SECONDS` 用来控制 DashScope 文本 HTTP client timeout，默认 `600` 秒；若日志里出现 `Client.Timeout exceeded while awaiting headers`，优先检查这里
- `DASHSCOPE_TEXT_MAX_RETRIES` / `DASHSCOPE_TEXT_RETRY_BACKOFF_SECONDS` 用来控制文本请求最小 retry/backoff；默认按 `429`、`5xx`、timeout 重试 2 次，退避 2s / 4s
- 图片默认也仍是 skeleton 模式，`ENABLE_LIVE_IMAGE_GENERATION=false`
- `shot_video` 当前已支持真实图生视频；但默认仍受 `ENABLE_LIVE_VIDEO_GENERATION=false` 保护，关闭时会稳定回退到 `image_fallback`
- `SHOT_VIDEO_TIMEOUT_PER_SHOT_SECONDS` 用来控制 `shot_video` task 的整体 execution deadline 预算；当前会按“前 `video_count` 个真正参与图生视频的 shot 数量 * 每 shot 超时”动态计算，默认 `200` 秒/shot
- `SHOT_VIDEO_DEFAULT_DURATION_SECONDS` 用来控制 `shot_video` manifest 中每个 clip 的默认时长，默认 `3` 秒
- `VIDEO_RENDER_TIMEOUT_SECONDS` 用来控制 `video` task 的执行超时，默认 `1800` 秒
- `FFMPEG_STARTUP_CHECK_TIMEOUT_SECONDS` 用来控制服务启动时 `ffmpeg -version` 检查超时，默认 `10` 秒
- 当前代码已支持按 `ENABLE_LIVE_VIDEO_GENERATION=true` 条件组装真实 DashScope 视频 client；即使该开关关闭，`video` 仍会基于 fallback 图 + TTS 真实合成最终 MP4
- 默认资源池并发上限分别为：`local_cpu=4`、`llm_text=2`、`tts=3`、`image_gen=2`、`video_gen=1`、`video_render=1`
- `BACKGROUND_RUNNER_WORKER_COUNT` 控制“同时可被后台主动推进的 job 数”；它不直接替代资源池限流，真实 task 并发上限仍由各 `RESOURCE_*_CONCURRENCY` 决定
- 即使配置了 `DASHSCOPE_TEXT_API_KEY`，只要不显式打开该开关，`outline / character_sheet / script` 也不会调用真实 DashScope 文本接口；`segmentation` 始终走本地 deterministic 路径
- 即使配置了 `DASHSCOPE_IMAGE_API_KEY`，只要不显式打开该开关，`image` / `character_image` 也不会调用真实 DashScope 图像接口
- 即使已经配置了 `DASHSCOPE_VIDEO_*` 这组参数，若不显式打开 `ENABLE_LIVE_VIDEO_GENERATION=true`，当前版本也不会启用真实图生视频
- 即使显式打开了 `ENABLE_LIVE_VIDEO_GENERATION=true`，也建议先确认 `character_image` / `image` 已经跑出稳定的真实 shot 图片；`shot_video` 只消费 `images/image_manifest.json.shot_images[*]`，不会直接消费人物参考图

### 最小真实联调：只打开 live image

当需要验证 `segmentation -> ... -> image -> video` 这条链路是否能在真实出图下跑通时，建议只打开 `ENABLE_LIVE_IMAGE_GENERATION=true`，其余模块继续保持默认。这样可以把联调成本控制在最低，也避免污染日常开发数据库和 workspace；此时最终 `video` 仍会用 fallback shot clip + TTS 真正产出 MP4。

下面这组命令会：

- 继续复用 `backend/.env` 里的 DashScope 图像配置
- 临时覆盖 `PORT`、`DATABASE_DSN`、`WORKSPACE_DIR`
- 显式保持 `ENABLE_LIVE_TEXT_GENERATION=false`
- 让真实联调写入临时 SQLite 和临时 workspace

若只是想复用仓库内的现成脚本，也可以直接运行：

```bash
./backend/scripts/live_image_smoke.sh
```

当前这份 smoke 脚本除了拉起临时服务、提交最小 job 之外，还会在 job 完成后直接核对 3 个检查点：

- `character_images/manifest.json` 已落盘，且 manifest 里声明的参考图文件都真实存在
- `images/image_manifest.json.shot_images[*]` 至少包含 `segment_index / shot_index / file_path / prompt`
- 若本轮真实出图有远端下载源，summary 会统计 `character_image` / `shot_images` 上的 `source_image_url` 回填数量

如果想单独验证“`image_to_image` 是否真的把人物参考图带进了请求”，可以直接运行：

```bash
./backend/scripts/live_image_reference_smoke.sh
```

这条脚本不会走整条 job 流水线，而是直接用固定 fixture 调 `character_image` / `image` executor，重点核对：

- 至少有 1 个 `image_to_image` shot
- 至少有 1 次真实图片请求带了 `reference_images`
- 请求 prompt 已把人物名替换成 `图1中的人物`

如果想继续验证“`shot_video` 是否真的把 shot image 变成了真实视频片段”，可以直接运行：

```bash
./backend/scripts/live_shot_video_smoke.sh
```

这条脚本同样使用固定 fixture，重点核对：

- 至少有 1 个 clip 的 `status = generated_video`
- `shot_videos/manifest.json` 已落盘
- `video_path / source_image_path / source_video_url` 已稳定回填

如果想继续验证“最终 `video` 是否真的把 `tts + shot_video` 渲染成成片”，可以直接运行：

```bash
./backend/scripts/live_video_render_smoke.sh
```

这条脚本同样使用固定 fixture，重点核对：

- `output/final.mp4` 已真实落盘
- 最终文件大小大于 0，且 `duration_seconds` 可读
- 真实 `generated_video` clip 与 TTS 音频能够一起完成最终 mux

如果想继续验证“服务级下载接口是否真的能把最终成片流出来”，可以直接运行：

```bash
./backend/scripts/e2e_video_download_smoke.sh
```

这条脚本会真实拉起后端服务、创建最小 job、等待完成，然后检查：

- `GET /api/v1/jobs/:job_id/download` 能返回完整 `video/mp4`
- `Content-Disposition` 为附件下载
- `Range` 请求能返回 `206 Partial Content`

```bash
cd backend

tmp_root=$(mktemp -d /tmp/narratio-live-smoke.XXXXXX)
PORT=18080 \
DATABASE_DRIVER=sqlite \
DATABASE_DSN="$tmp_root/smoke.db" \
WORKSPACE_DIR="$tmp_root/workspace" \
ENABLE_LIVE_IMAGE_GENERATION=true \
ENABLE_LIVE_TEXT_GENERATION=false \
go run cmd/server/main.go
```

新开一个终端后，可先检查健康状态：

```bash
curl -sS http://127.0.0.1:18080/api/v1/health
```

若返回里包含 `dashscope_image: configured`，说明服务已读取到图像配置，接下来可提交一个最小 job：

同时还可以直接核对返回里的 `resources` 字段，确认当前服务实际采用的资源池并发上限是否符合你的 `.env` 配置。

```bash
curl -sS http://127.0.0.1:18080/api/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "article": "暴雨后的旧城巷口，林夏抱着一把黑伞站在路灯下，雨水沿着伞骨缓缓滴落。她想起和父亲失散前的最后一句叮嘱，于是深吸一口气，转身走进巷子尽头那家仍亮着暖黄灯光的小书店。",
    "options": {
      "voice_id": "male_calm",
      "image_style": "realistic"
    }
  }'
```

拿到返回的 `job_id` 后，可继续查看 job 和 task 明细：

```bash
curl -sS http://127.0.0.1:18080/api/v1/jobs/<job_id>
curl -sS http://127.0.0.1:18080/api/v1/jobs/<job_id>/tasks
```

当前代码状态下，一轮成功的最小 live image 联调可重点确认：

- `jobs/<job_id>/character_images/manifest.json` 已落盘，且 `character_images[*].file_path` 对应文件真实存在
- `job.status = completed`
- `image.output_ref.generated_image_count > 0`
- `image.output_ref.fallback_image_count = 0`（若真实出图成功）
- `jobs/<job_id>/images/image_manifest.json` 已落盘，且包含 `generation_request_id`、`generation_model`、`source_image_url`
- `jobs/<job_id>/images/segment_000_shot_000.jpg` 这类 shot 级图片已真实落盘
- `image.output_ref.image_count` 只是 segment 级兼容摘要数；真实出图主路径看 `shot_image_count`
- 如果后续要继续做 `shot_video` 真实联调，至少再确认 `image_manifest.json.shot_images[*]` 里的 `segment_index / shot_index / file_path / prompt` 都稳定存在；`source_image_url` 有则更好，但不是硬前置
- 若未同时配置 `TTS_API_BASE_URL` 与 `TTS_JWT_PRIVATE_KEY`，`tts` 仍然只会生成 `tts_manifest.json` 和占位 WAV；这是当前预期行为
- 若同时配置了 `TTS_API_BASE_URL` 与 `TTS_JWT_PRIVATE_KEY`，runtime 会自动接入真实 TTS client；执行时会按 segment 文本内的句号逐句串行合成，再合并成 segment 级 WAV

## 项目结构（完整）

```
narratio/
  frontend/
    src/
    package.json
  backend/
    cmd/server/main.go
    scripts/
      live_image_smoke.sh
    internal/
      config/
      model/
      store/
        sql/
        migrations/
      pipeline/
        script/
        tts/
        image/
        video/
      scheduler/
      handler/
      middleware/
      util/
    .env.example
    go.mod
    go.sum
  docs/                  ← 当前目录
  AGENTS.md
  docker-compose.yml
  README.md
```

## 前端运行

```bash
cd frontend
npm install
npm run dev
```

前端默认读取：

- `VITE_API_BASE_URL`

未设置时默认使用 `http://localhost:8080/api/v1`。

## 常用后端命令

```bash
cd backend

# 启动服务
go run cmd/server/main.go

# 运行测试
go test ./...

# 详细输出
go test -v ./...

# 构建
go build -o bin/narratio cmd/server/main.go

# 最小 live image 联调
./scripts/live_image_smoke.sh
```

## 健康检查

当前实现的启动期检查主要包括：
1. 配置是否能成功加载
2. 数据库连接是否可用
3. SQLite migration 是否已应用

当前尚未在启动时执行以下真实探测：
- `ffmpeg` PATH 检查
- Workspace 可写性探测
- TTS `/health` 联通性检查
- DashScope 文本 / 图像 / 视频的真实可用性探测

## 数据库策略

- 本地开发：SQLite，单文件数据库即可
- 线上部署：MySQL
- schema 通过 migrations 管理，不允许手工修改线上表结构

## Docker（后续补充）

> Docker 化在 MVP 验证后再做，避免过早复杂化。
