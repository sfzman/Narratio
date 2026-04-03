# docs/integrations.md — 外部服务集成规范

## 通用原则

- 所有外部服务调用必须封装在 `internal/pipeline/` 对应子模块中
- 超时、重试、API Key 均通过 config 传入，不得硬编码
- 所有 HTTP client 必须设置超时，不得使用 `http.DefaultClient`
- 调用前后打印结构化日志（request_id、耗时、状态码）

---

## 1. Qwen 文本生成 API（脚本优化）

**用途**：将原始文章分段，生成朗诵脚本和图像摘要

**Endpoint**：由 `QWEN_TEXT_API_BASE_URL` 配置，不在代码中写死

**认证**：`Authorization: Bearer ${QWEN_API_KEY}`

**Model**：通过配置传入，MVP 固定单一模型，不允许 handler 覆盖

**请求示例**
```go
// 详见 internal/pipeline/script/client.go
payload := map[string]any{
    "model":      cfg.QwenTextModel,
    "max_tokens": 4096,
    "input": map[string]any{"prompt": buildPrompt(input)},
}
```

**响应解析**：取 `content[0].text`，期望返回 JSON 格式（见 pipeline.md Stage 1）

**超时**：30s  
**重试**：2 次，退避 2s、4s  
**限流**：无需客户端限流，依赖 API 本身的速率限制返回 429 时退避

**错误码处理**

| HTTP 状态码 | 处理策略 |
|---|---|
| 429 | 退避后重试 |
| 500/502/503/504 | 正常重试 |
| 400 | 不重试，直接 failed（通常是 prompt 问题） |
| 401 | 不重试，报警（Key 失效） |

---

## 2. 自部署 TTS 服务

**用途**：将文本合成为语音音频

**Endpoint**：`${TTS_API_BASE_URL}/synthesize`（从环境变量读取）

**认证**：`Authorization: Bearer ${TTS_API_KEY}`

**请求格式**
```json
{
  "text": "要合成的文本内容",
  "voice_id": "default",
  "format": "wav",
  "sample_rate": 24000
}
```

**响应格式**：二进制音频数据流（`Content-Type: audio/wav`）

**超时**：60s（单个 segment）  
**并发**：最大 3 个并发请求（通过 semaphore 控制）  
**重试**：3 次，退避 1s、2s、4s

**健康检查 Endpoint**：`${TTS_API_BASE_URL}/health`（GET，用于启动时检查）

**注意事项**
- 响应直接流式写入本地文件，不加载到内存
- 文件写入完成后校验文件大小 > 0
- 若音频时长为 0（静音文件），视为失败
- TTS 模块负责计算 segment 级字幕时间轴，并返回 `SubtitleItems`

---

## 3. Qwen 图像生成 API

**用途**：根据段落摘要生成配图

**Endpoint**：`https://dashscope.aliyuncs.com/api/v1/services/aigc/text2image/image-synthesis`

**认证**：`Authorization: Bearer ${QWEN_API_KEY}`

**请求格式**
```json
{
  "model": "wanx2.1-t2i-turbo",
  "input": {
    "prompt": "图像描述文本",
    "negative_prompt": "人物面部特写, 模糊, 低质量"
  },
  "parameters": {
    "size": "1280*720",
    "n": 1,
    "style": "<from image_style>"
  }
}
```

**响应**：异步任务模式
1. 提交请求 → 获得 `task_id`
2. 轮询 `GET /api/v1/tasks/{task_id}` 直到 `task_status == SUCCEEDED`
3. 从结果中取图片 URL，下载到本地

**轮询间隔**：2s，最多等待 120s  
**超时**：单个图片整体流程 120s  
**并发**：最大 2 个并发（图像生成 QPS 限制较低）

**错误处理**
- `task_status == FAILED`：取 `message` 字段写入 job warning，生成 fallback 图片，不阻断整体流程
- 超时：同上，跳过该段并记录 warning

**降级策略**：若某段图像生成失败，使用纯色背景图（1280x720，固定颜色 `#1a1a2e`）替代，不阻断视频合成。

---

## 4. FFmpeg

**用途**：将音频 + 图片合成为最终视频

**依赖**：系统安装 FFmpeg >= 5.0，通过 `exec.Command("ffmpeg", ...)` 调用

**启动检查**：服务启动时执行 `ffmpeg -version`，失败则拒绝启动

**关键命令模板**（详见 `internal/pipeline/video/ffmpeg.go`）

```bash
# 单段合成（图片 + 音频）
ffmpeg -loop 1 -i image.jpg -i audio.wav \
  -c:v libx264 -tune stillimage -c:a aac \
  -shortest -t {duration} segment.mp4

# 多段拼接 + 字幕
ffmpeg -f concat -safe 0 -i segments.txt \
  -vf "subtitles=subtitles.srt" \
  -c:v libx264 -c:a aac \
  final.mp4
```

**超时**：通过 `context.WithTimeout` 控制，设为 300s

**错误处理**：捕获 stderr 输出，写入 job 错误信息，便于调试

---

## 环境变量汇总

```bash
# Claude
CLAUDE_API_KEY=sk-ant-...

# Qwen
QWEN_API_KEY=sk-...

# 自部署 TTS
TTS_API_BASE_URL=https://your-tts-service.com
TTS_API_KEY=your-key

# 存储
QWEN_TEXT_API_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
QWEN_TEXT_MODEL=qwen-max
WORKSPACE_DIR=/var/narratio/workspace

# 服务
PORT=8080
GIN_MODE=release
```

本地开发使用 `.env` 文件（`.gitignore` 中已排除），生产使用环境变量或 Secret Manager。
