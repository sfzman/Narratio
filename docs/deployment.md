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

DASHSCOPE_TEXT_API_KEY=your-dashscope-text-key-here
DASHSCOPE_TEXT_BASE_URL=https://coding.dashscope.aliyuncs.com/v1
DASHSCOPE_TEXT_MODEL=qwen-max

DASHSCOPE_IMAGE_API_KEY=your-dashscope-image-key-here
DASHSCOPE_IMAGE_BASE_URL=https://dashscope.aliyuncs.com/api/v1
DASHSCOPE_IMAGE_MODEL=qwen-image-2.0

DASHSCOPE_VIDEO_API_KEY=your-dashscope-video-key-here
DASHSCOPE_VIDEO_BASE_URL=https://dashscope.aliyuncs.com
DASHSCOPE_VIDEO_MODEL=wan2.6-i2v-flash

TTS_API_BASE_URL=https://your-tts-service.com
TTS_API_KEY=your-tts-key-here

WORKSPACE_DIR=./workspace
```

当前代码状态：

- `cmd/server` 目前已完成配置读取、SQLite store 初始化、`segmentation / outline / character_sheet / script / character_image / tts / image / video` executor registry 初始化，以及 `app/jobs` / `scheduler.Service` 组装
- 已启动最小 Gin HTTP server，并开放 `GET /api/v1/health`、`POST /api/v1/jobs`、`GET /api/v1/jobs/:job_id`、`GET /api/v1/jobs/:job_id/tasks` 与开发态 `POST /api/v1/jobs/:job_id/dispatch-once`
- SQLite 模式会在启动时自动执行首个 migration，当前首版 schema 初始化是幂等的，可重复启动
- 当前已接入最小后台 scheduler runner，`POST /jobs` 后会自动持续推进 job
- `segmentation / outline / character_sheet / script / tts / character_image / image` 成功后会把结构化结果写入 `WORKSPACE_DIR/jobs/{job_id}/...`
- `image` 已支持注入真实 DashScope client；只有显式打开 `ENABLE_LIVE_IMAGE_GENERATION=true` 且配置了 `DASHSCOPE_IMAGE_API_KEY` 时，才会尝试真实图片请求

注意：

- 当前默认是 skeleton 模式，`ENABLE_LIVE_TEXT_GENERATION=false`
- 图片默认也仍是 skeleton 模式，`ENABLE_LIVE_IMAGE_GENERATION=false`
- 即使配置了 `DASHSCOPE_TEXT_API_KEY`，只要不显式打开该开关，`outline / character_sheet / script` 也不会调用真实 DashScope 文本接口；`segmentation` 始终走本地 deterministic 路径
- 即使配置了 `DASHSCOPE_IMAGE_API_KEY`，只要不显式打开该开关，`image` 也不会调用真实 DashScope 图像接口

### 最小真实联调：只打开 live image

当需要验证 `segmentation -> ... -> image -> video` 这条链路是否能在真实出图下跑通时，建议只打开 `ENABLE_LIVE_IMAGE_GENERATION=true`，其余模块继续保持 skeleton。这样可以把联调成本控制在最低，也避免污染日常开发数据库和 workspace。

下面这组命令会：

- 继续复用 `backend/.env` 里的 DashScope 图像配置
- 临时覆盖 `PORT`、`DATABASE_DSN`、`WORKSPACE_DIR`
- 显式保持 `ENABLE_LIVE_TEXT_GENERATION=false`
- 让真实联调写入临时 SQLite 和临时 workspace

若只是想复用仓库内的现成脚本，也可以直接运行：

```bash
./backend/scripts/live_image_smoke.sh
```

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

```bash
curl -sS http://127.0.0.1:18080/api/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "article": "暴雨后的旧城巷口，林夏抱着一把黑伞站在路灯下，雨水沿着伞骨缓缓滴落。她想起和父亲失散前的最后一句叮嘱，于是深吸一口气，转身走进巷子尽头那家仍亮着暖黄灯光的小书店。",
    "options": {
      "voice_id": "default",
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

- `job.status = completed`
- `image.output_ref.generated_image_count > 0`
- `image.output_ref.fallback_image_count = 0`（若真实出图成功）
- `jobs/<job_id>/images/image_manifest.json` 已落盘，且包含 `generation_request_id`、`generation_model`、`source_image_url`
- `jobs/<job_id>/images/segment_000.jpg` 已真实落盘
- `tts` 仍然只会生成 `tts_manifest.json`、`subtitles.srt` 和占位 WAV；这是当前预期行为

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
