# docs/integrations.md — 外部服务集成规范

## 通用原则

- 所有外部服务调用必须封装在 `internal/pipeline/` 对应子模块中
- 超时、重试、API Key 均通过 config 传入，不得硬编码
- 所有 HTTP client 必须设置超时，不得使用 `http.DefaultClient`
- 调用前后打印结构化日志（request_id、耗时、状态码）

---

## 1. DashScope 文本生成 API（OpenAI-compatible mode，文本生成）

**用途**：提炼大纲、生成人物表，并基于既定分段逐段生成分镜化 `script`（每段 `script + 10 shots`）

**Endpoint**：由 `DASHSCOPE_TEXT_BASE_URL` 配置，client 内部拼接 `/chat/completions`

**认证**：`Authorization: Bearer ${DASHSCOPE_TEXT_API_KEY}`

**Model**：通过配置传入，MVP 固定单一模型，不允许 handler 覆盖

**请求示例**
```go
// 详见 internal/pipeline/script/client.go
payload := map[string]any{
    "model": cfg.DashScopeTextModel,
    "messages": []map[string]string{
        {"role": "system", "content": "Respond with JSON only."},
        {"role": "user", "content": buildPrompt(input)},
    },
    "max_tokens": 4096,
    "response_format": map[string]string{
        "type": "json_object",
    },
}
```

**响应解析**：取 `choices[0].message.content`，期望返回 JSON 格式（见 pipeline.md Stage 1）

**超时**：当前代码里 DashScope 文本 HTTP client timeout 为 600s；默认后台 runner 还会给单次 `DispatchOnce` 套一层 12 分钟外层 deadline，因此自动跑 job 时，文本 task 当前主要受 600s HTTP timeout 约束  
**重试**：文档原先约定为 2 次、退避 2s/4s；当前代码尚未接入真实 retry/backoff  
**限流**：无需客户端限流，依赖 API 本身的速率限制返回 429 时退避
**当前调用形态**：`outline` / `character_sheet` 当前各调用 1 次；`script` 启用真实文本生成时会按 segment 逐段调用，再汇总写回同一个 `script.json`
**当前输出预算**：通用文本请求默认 `max_tokens=4096`；`script` executor 当前会把自身最小预算提升到 `8192`

**当前实现状态**：当前仓库的 `script/client.go` 已接入 OpenAI-compatible 文本请求；retry/backoff 仍未接入真实实现

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

## 3. DashScope 图像生成 API（原生接口）

**用途**：根据段落摘要生成配图

**Endpoint**：由 `DASHSCOPE_IMAGE_BASE_URL` 配置，走 DashScope 原生多模态接口；当前 POC 对应 Python SDK 的 `dashscope.MultiModalConversation.call(...)`

**认证**：`Authorization: Bearer ${DASHSCOPE_IMAGE_API_KEY}`

**请求格式**
```json
{
  "model": "qwen-image",
  "input": {
    "messages": [
      {
        "role": "user",
        "content": [
          {"image": "https://example.com/reference-1.png"},
          {"text": "图像描述文本"}
        ]
      }
    ]
  },
  "parameters": {
    "negative_prompt": "人物面部特写, 模糊, 低质量",
    "size": "1280*720"
  }
}
```

**响应**：同步返回 message 结构，从中提取图片 URL 后下载到本地

**超时**：单个图片请求 120s  
**并发**：最大 2 个并发（图像生成 QPS 限制较低）

**错误处理**
- 接口失败：取 `code/message` 写入 job warning，生成 fallback 图片，不阻断整体流程
- 超时：同上，跳过该段并记录 warning

**降级策略**：若某段图像生成失败，使用纯色背景图（1280x720，固定颜色 `#1a1a2e`）替代，不阻断视频合成。

**当前实现状态**：当前仓库已接入最小 HTTP client 与 executor 注入；只有显式开启 `ENABLE_LIVE_IMAGE_GENERATION=true` 时才会尝试真实请求。当前真实 smoke 已验证该接口需要使用 `input.messages` + `parameters` 的请求形状。当前真实出图成功后，会把 `request_id`、使用模型和下载源图 URL 回填到 image manifest；若仍走 skeleton 或单段请求失败，也会把本地纯色 fallback JPEG 真实落盘。但仍未补齐多图参考输入。

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

# DashScope 文本（OpenAI-compatible mode）
DASHSCOPE_TEXT_API_KEY=sk-...
DASHSCOPE_TEXT_BASE_URL=https://coding.dashscope.aliyuncs.com/v1
DASHSCOPE_TEXT_MODEL=qwen-max

# DashScope 图片（原生接口）
DASHSCOPE_IMAGE_API_KEY=sk-...
DASHSCOPE_IMAGE_BASE_URL=https://dashscope.aliyuncs.com/api/v1
DASHSCOPE_IMAGE_MODEL=qwen-image-2.0

# DashScope 视频（原生接口）
DASHSCOPE_VIDEO_API_KEY=sk-...
DASHSCOPE_VIDEO_BASE_URL=https://dashscope.aliyuncs.com
DASHSCOPE_VIDEO_MODEL=wan2.6-i2v-flash

# 自部署 TTS
TTS_API_BASE_URL=https://your-tts-service.com
TTS_API_KEY=your-key

# 存储
WORKSPACE_DIR=/var/narratio/workspace

# 服务
PORT=8080
GIN_MODE=release
```

本地开发使用 `.env` 文件（`.gitignore` 中已排除），生产使用环境变量或 Secret Manager。
