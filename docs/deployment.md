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

### .env.example

```bash
PORT=8080
GIN_MODE=debug

DATABASE_DRIVER=sqlite
DATABASE_DSN=./narratio.db

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

- `cmd/server` 目前已完成配置读取、SQLite store 初始化、DashScope 文本 client 组装、script executor registry 初始化，以及 `app/jobs` / `scheduler.Service` 组装
- 已启动最小 Gin HTTP server，并开放 `GET /api/v1/health` 与 `POST /api/v1/jobs`
- SQLite 模式会在启动时自动执行首个 migration
- 还没有启动 scheduler loop

## 项目结构（完整）

```
narratio/
  backend/
    cmd/server/main.go
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
    Makefile
  frontend/
    src/
    public/
    package.json
    vite.config.ts
  docs/                  ← 当前目录
  AGENTS.md
  docker-compose.yml
  README.md
```

## Makefile 常用命令

```makefile
# backend/Makefile

.PHONY: dev test lint build

dev:
	go run cmd/server/main.go

test:
	go test ./... -coverprofile=coverage.out

test-verbose:
	go test -v ./...

lint:
	golangci-lint run

build:
	go build -o bin/narratio cmd/server/main.go

coverage:
	go tool cover -html=coverage.out
```

## 健康检查

服务启动时自动检查：
1. 所有必填环境变量是否存在
2. 数据库连接是否可用
3. migrations 是否已应用
4. `ffmpeg` 是否在 PATH 中
5. Workspace 目录是否可写
6. TTS 服务 `/health` 是否可达
7. DashScope 文本 / 图像 / 视频配置是否完整

任意检查失败，服务拒绝启动并打印明确错误信息。

## 数据库策略

- 本地开发：SQLite，单文件数据库即可
- 线上部署：MySQL
- schema 通过 migrations 管理，不允许手工修改线上表结构

## Docker（后续补充）

> Docker 化在 MVP 验证后再做，避免过早复杂化。
