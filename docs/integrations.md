# docs/integrations.md — 外部服务集成规范

## 通用原则

- 所有外部服务调用必须封装在 `internal/pipeline/` 对应子模块中
- 超时、重试、API Key 均通过 config 传入，不得硬编码
- 所有 HTTP client 必须设置超时，不得使用 `http.DefaultClient`
- 调用前后打印结构化日志（request_id、耗时、状态码）

---

## 1. DashScope 文本生成 API（OpenAI-compatible mode，文本生成）

**用途**：提炼大纲、生成人物表，并基于既定分段逐段生成分镜化 `script`（每段 shots 数会随 segment 长度动态收敛，当前常见范围约为 3~10 个；核心字段为 `involved_characters / image_to_image_prompt / text_to_image_prompt`）

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

**超时**：当前代码里 DashScope 文本 HTTP client timeout 由 `DASHSCOPE_TEXT_REQUEST_TIMEOUT_SECONDS` 配置，默认 `600s`；其中 `script` task 的执行 deadline 现按 `segment_count * SCRIPT_TIMEOUT_PER_SEGMENT_SECONDS` 动态计算，默认每段 200 秒，其他文本 task 仍默认使用 12 分钟执行 deadline。后台 runner / 开发态手动 dispatch 的外层超时已放宽，不再比 `script` task 更早截断。  
**重试**：当前代码已接入最小 retry/backoff，配置项为 `DASHSCOPE_TEXT_MAX_RETRIES` 与 `DASHSCOPE_TEXT_RETRY_BACKOFF_SECONDS`；默认仅对 timeout、`429`、`5xx` 生效，重试 2 次，退避 `2s / 4s`  
**限流**：无需客户端限流，依赖 API 本身的速率限制返回 429 时退避
**当前调用形态**：`outline` / `character_sheet` 当前各调用 1 次；`script` 启用真实文本生成时会按 segment 逐段调用，再汇总写回同一个 `script.json`
**当前输出预算**：通用文本请求默认 `max_tokens=4096`；`script` executor 当前会把自身最小预算提升到 `8192`

**当前实现状态**：当前仓库的 `script/client.go` 已接入 OpenAI-compatible 文本请求；发送前会记录请求 URL / model / message_count / HTTP timeout，收到响应后会记录状态码与耗时；当命中可重试错误时，会额外打印下一次 attempt 与 backoff 时长

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

**当前 Go 代码状态**：配置了 `TTS_API_BASE_URL` 和 `TTS_JWT_PRIVATE_KEY` 后，后端会自动使用下面这套 live 接口形态；否则继续走 placeholder。  
**Endpoint**：`${TTS_API_BASE_URL}/api/v1/tts`（参考原 Python `narration_tools.py`）

**认证**：`Authorization: Bearer <jwt>`，JWT 由后端使用 `TTS_JWT_PRIVATE_KEY` 按 RS256 签名生成，payload 至少包含 `iat` 和 `exp`

**请求格式**
```json
{
  "text": "要合成的单句文本",
  "reference_audio": "https://.../voice.mp3",
  "emotion_prompt": "https://.../emotion.wav"
}
```

**响应格式**：二进制音频数据流（`Content-Type: audio/wav`）

**超时**：按单句请求控制，当前由 `TTS_REQUEST_TIMEOUT_SECONDS` 配置 HTTP client timeout  
**并发**：当前目标实现要求句子级串行调用  
**重试**：待补

**健康检查 Endpoint**：`${TTS_API_BASE_URL}/health`（GET，用于启动时检查）

**注意事项**
- TTS executor 消费的是 segmentation 的 `segments[*].text`
- 每个 segment 会先按中文句号 `。` 切句，再一句一句调用 TTS 服务
- `voice_id` 会先在后端映射到内置 narration voice preset；当前内置 preset 为 `male_calm / male_strong / female_explainer / female_documentary / boy`
- `voice_id` 为空或传 `default` 时，后端会统一规范化到 `TTS_DEFAULT_VOICE_ID`；当前默认值为 `male_calm`
- `emotion_prompt` 当前由 `TTS_EMOTION_PROMPT` 配置统一注入
- 句子级音频最终会被合并成 segment 级 WAV，并由 TTS 模块计算 segment 级字幕时间轴

---

## 3. DashScope 图像生成 API（原生接口）

**用途**：根据 segment 分镜生成配图，或根据人物表生成 `character_image` 参考图

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

**降级策略**：若某段图像生成失败，使用与 `aspect_ratio` 对齐的纯色背景图（`16:9 -> 1280x720`，`9:16 -> 720x1280`，固定颜色 `#1a1a2e`）替代，不阻断视频合成。

**当前实现状态**：当前仓库已接入最小 HTTP client 与 executor 注入；只有显式开启 `ENABLE_LIVE_IMAGE_GENERATION=true` 时才会尝试真实请求。当前真实 smoke 已验证该接口需要使用 `input.messages` + `parameters` 的请求形状。当前真实出图成功后，会把 `request_id`、使用模型和下载源图 URL 回填到 `image` / `character_image` manifest；若仍走 skeleton 或单个 shot 请求失败，则优先复用最近一次成功图补位，否则才落本地纯色 fallback JPEG。普通 `image` executor 现已支持把 `character_image` 参考图作为多图输入一并送入请求，并按 job 的 `aspect_ratio` 动态切到 `1280*720` 或 `720*1280`；但 `character_image` 自身固定为单人正面全身参考图，当前尺寸固定 `832*1248`（`2:3`）。

---

## 4. DashScope 图生视频 API（原生接口）

**用途**：基于单张 `shot image` 生成单个 shot 的短视频片段，供 `shot_video` task 落盘中间 manifest 使用

**当前代码状态**：当前仓库已经补了真实 HTTP client、纯 client 测试和 executor 注入；runtime 在 `ENABLE_LIVE_VIDEO_GENERATION=true` 时会真正逐 shot 调该接口，默认仍关闭，因此默认执行路径仍会回退到 `image_fallback`。  
**Submit Endpoint**：`${DASHSCOPE_VIDEO_BASE_URL}/api/v1/services/aigc/video-generation/video-synthesis`  
**Query Endpoint**：`${DASHSCOPE_VIDEO_BASE_URL}/api/v1/tasks/{task_id}`

**认证**：`Authorization: Bearer ${DASHSCOPE_VIDEO_API_KEY}`

**最小请求语义**
- 一次请求只生成一个 shot clip
- 真实联调前，建议先确认 `character_image` / `image` 已经跑出稳定产物；`shot_video` 不直接消费人物参考图，它只消费 `image_manifest.json` 里的 `shot_images`
- 优先使用上游 `image.shot_images[*].source_image_url` 作为 `img_url`
- 若远端 URL 提交失败，则回退读取本地 `source_image_path`，编码为 data URL 再提交；这一步会参考原 gradio app 做请求体大小控制
- `prompt` 当前按原 gradio app 语义，来自 shot 侧视频提示词；`shot_video` 后续真实接入时再从现有 shot 上下文派生
- 片段时长由 `shot_video` 决定；当前默认仍使用 `SHOT_VIDEO_DEFAULT_DURATION_SECONDS`

**上游最小依赖字段**
- `segment_index`：定位该 clip 属于哪个 segment
- `shot_index`：定位该 clip 属于哪个 shot
- `file_path`：本地首帧图路径；这是本地回退与最终稳定依赖
- `prompt`：当前 shot 的图生视频提示词来源
- `source_image_url`：可选优化字段；若存在则优先用远端 URL 直传，不存在时回退读取本地 `file_path`

**当前最小可配置项**
- `DASHSCOPE_VIDEO_SUBMIT_TIMEOUT_SECONDS`：当前 Go client 预留的 HTTP 超时配置，后续 runtime 接 live client 时会作为请求超时使用；默认 `60`
- `DASHSCOPE_VIDEO_POLL_INTERVAL_SECONDS`：轮询间隔；默认 `10`
- `DASHSCOPE_VIDEO_MAX_WAIT_SECONDS`：单个 shot 最长轮询等待时间；默认 `900`
- `SHOT_VIDEO_TIMEOUT_PER_SHOT_SECONDS`：scheduler 侧的 task deadline 预算；当前 `shot_video` 会按前 `video_count` 个真正参与图生视频的 shot 数量 * 该值来计算整体 task 超时，默认 `200`
- `DASHSCOPE_VIDEO_MAX_REQUEST_BYTES`：本地图回退为 data URL 时允许的最大请求体大小；默认 `6291456`
- `DASHSCOPE_VIDEO_RESOLUTION`：默认分辨率；默认 `720P`
- `DASHSCOPE_VIDEO_NEGATIVE_PROMPT`：额外负向提示词；默认空，client 内部仍会自动补文字/字幕类负向约束
- `DASHSCOPE_VIDEO_IMAGE_JPEG_QUALITY`：本地图片压缩起始质量；默认 `80`
- `DASHSCOPE_VIDEO_IMAGE_MIN_JPEG_QUALITY`：本地图片压缩最低质量；默认 `45`

**请求示例**
```json
{
  "model": "wan2.6-i2v-flash",
  "input": {
    "prompt": "首帧静态图已提供，请严格保持首帧中的人物外观、服装、场景、构图和光线连续一致。",
    "negative_prompt": "字幕，台词字幕，屏幕文字，logo，水印",
    "img_url": "https://example.com/shot.jpg"
  },
  "parameters": {
    "resolution": "720P",
    "duration": 6,
    "prompt_extend": true,
    "shot_type": "multi",
    "watermark": false
  }
}
```

说明：
- submit 请求需要额外带 `X-DashScope-Async: enable`
- 当前 client 默认按原 gradio app 的异步任务流：submit -> poll -> download
- 当前轮询会读取 `output.task_status / output.video_url / output.actual_prompt / output.orig_prompt`

**当前稳定 artifact 字段**
- `segment_index / shot_index`：唯一定位 clip
- `source_image_path`：真实图生视频的输入图来源
- `duration_seconds`：该 clip 的目标/结果时长
- `video_path`：真实视频落盘后填写
- `image_path`：失败回退静态图时填写
- `status`：当前正式枚举 `generated_video | image_fallback`
- `generation_request_id / generation_model / source_video_url`：真实接口追踪字段

**降级策略**
- 单个 shot 图生视频失败时，不阻断整个 job
- 当前先回退到 `image_fallback`，由最终 `video` 阶段统一消费

**当前代码边界**
- 当前仓库已经补了 `video.Client` / `video.Request` / `video.Response` 以及 `HTTPClient`
- `shot_video` executor 当前若注入了 client，会按 shot 逐个调用；若未注入或单个 shot 调用失败，则回退到 `image_fallback`
- 当前需要显式开启 `ENABLE_LIVE_VIDEO_GENERATION=true` 且配置 `DASHSCOPE_VIDEO_API_KEY`，runtime 才会组装真实 DashScope 视频 client
- 但仅开启 video live 还不够；更有意义的前提是先让 `character_image` / `image` 产出真实且可接受的 shot 首帧，否则后面的图生视频结果不会有参考价值
- 默认仍是关闭态，因此当前线上/本地默认行为仍是 `image_fallback`

---

## 5. FFmpeg

**用途**：将 `tts` 音频与 `shot_video` 产出的逐 shot 媒体合成为最终视频

**依赖**：系统安装 FFmpeg >= 5.0，通过 `exec.Command("ffmpeg", ...)` 调用

**启动检查**：服务启动时会先执行一次 `ffmpeg -version` 探测；若超时或命令不可用，runtime 直接启动失败

**关键命令模板**（详见 `internal/pipeline/video/ffmpeg.go`）

```bash
# 静态图回退 clip
ffmpeg -loop 1 -framerate 24 -t {duration} -i image.jpg \
  -vf "{cover_scale_filter}" -an \
  -c:v libx264 -pix_fmt yuv420p clip.mp4

# 归一化真实 shot video
ffmpeg -i shot.mp4 -map 0:v:0 \
  -vf "{cover_scale_filter}" -an \
  -c:v libx264 -pix_fmt yuv420p normalized.mp4

# segment 内拼接
ffmpeg -f concat -safe 0 -i clips.txt -an -c:v libx264 segment_base.mp4

# 按 segment 音频时长做整体变速
ffmpeg -i segment_base.mp4 \
  -filter:v "setpts={factor}*PTS" -an \
  -c:v libx264 segment.mp4

# 最终 mux
ffmpeg -i concatenated.mp4 -i merged_audio.wav \
  -map 0:v:0 -map 1:a:0 \
  -c:v libx264 -c:a aac -shortest \
  -movflags +faststart final.mp4
```

说明：
- 当前仓库里的 `tts` 不再额外产出 `subtitles.srt`
- 若后续视频阶段需要烧录字幕，应直接从 `tts_manifest.json.subtitle_items` 派生，而不是依赖独立 SRT 文件

**超时**：
- `FFMPEG_STARTUP_CHECK_TIMEOUT_SECONDS`：服务启动时执行 `ffmpeg -version` 的超时，默认 `10`
- `VIDEO_RENDER_TIMEOUT_SECONDS`：`video` task 的执行超时，默认 `1800`

**错误处理**：捕获 stderr 输出，写入 job 错误信息，便于调试

补充说明：
- `shot_video` 已支持真实图生视频；若 live 关闭或单个 shot 失败，则回退到 `image_fallback`
- `shot_video` 默认会给每个 clip 写入稳定时长；默认 `3` 秒，可通过 `SHOT_VIDEO_DEFAULT_DURATION_SECONDS` 调整
- `shot_video` manifest 会稳定保留 `source_image_path`，便于追踪图生视频输入图来源
- `video` 当前已经会真实消费 `shot_video` manifest / `tts` artifact，并在 workspace 下落盘最终 MP4；落盘后还会校验最终文件非空

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
ENABLE_LIVE_VIDEO_GENERATION=false
DASHSCOPE_VIDEO_API_KEY=sk-...
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

# 自部署 TTS
TTS_API_BASE_URL=https://your-tts-service.com
TTS_JWT_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"
TTS_JWT_EXPIRE_SECONDS=300
TTS_REQUEST_TIMEOUT_SECONDS=300
TTS_DEFAULT_VOICE_ID=male_calm
TTS_EMOTION_PROMPT=https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/male-read-emo.wav

# 存储
WORKSPACE_DIR=/var/narratio/workspace
VIDEO_RENDER_TIMEOUT_SECONDS=1800
FFMPEG_STARTUP_CHECK_TIMEOUT_SECONDS=10
SHOT_VIDEO_DEFAULT_DURATION_SECONDS=3

# 服务
PORT=8080
GIN_MODE=release
```

本地开发使用 `.env` 文件（`.gitignore` 中已排除），生产使用环境变量或 Secret Manager。
