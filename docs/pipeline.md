# docs/pipeline.md — 流水线设计

## 概述

Narratio 的内容处理仍然由 script / tts / image / video 等模块完成，但执行方式不再是固定顺序 orchestrator。

当前设计改为：

- `job` 被拆成多个 `task`
- `task` 由 `scheduler` 调度
- `pipeline/*` 实现各类 task executor

因此，本文件定义的是“各类 task 的输入输出契约”，不是一条硬编码流水线。

顶层 job / task 生命周期见 `docs/job-lifecycle.md`。

## 各阶段规范

---

### Task Type: outline（提炼大纲）

**模块路径**：`internal/pipeline/script/`

**职责**

- 从原始文章提炼结构化大纲
- 供后续 script / image 等 task 复用

**推荐资源池**：`llm_text`

**输入**
```go
type OutlineInput struct {
    JobID       string
    ArticleText string
    Language    string
}
```

**输出**
```go
type OutlineOutput struct {
    Sections []OutlineSection
}

type OutlineSection struct {
    Index   int
    Title   string
    Summary string
}
```

---

### Task Type: character_sheet（生成人物表）

**模块路径**：`internal/pipeline/script/`

**职责**

- 抽取人物、身份、关系、外观提示
- 为后续 script / image task 提供稳定设定

**推荐资源池**：`llm_text`

**输入**
```go
type CharacterSheetInput struct {
    JobID       string
    ArticleText string
    Language    string
}
```

**输出**
```go
type CharacterSheetOutput struct {
    Characters []CharacterProfile
}

type CharacterProfile struct {
    Name        string
    Role        string
    Description string
}
```

---

### Task Type: script（脚本优化）

**模块路径**：`internal/pipeline/script/`

**职责**

- 基于原文、可选大纲、可选人物表生成朗诵脚本分段

**推荐资源池**：`llm_text`

**输入**
```go
type ScriptInput struct {
    JobID       string
    ArticleText string // 原始文章文本，最大 10000 字
    Language    string // "zh" | "en"，默认 "zh"
    VoiceID     string // 透传给后续 TTS，用于 prompt 调整语气时可选使用
    Outline     *OutlineOutput
    Characters  *CharacterSheetOutput
}
```

**输出**
```go
type ScriptOutput struct {
    Segments []Segment // 朗诵段落列表，按顺序
}

type Segment struct {
    Index   int    // 从 0 开始
    Text    string // 该段原文
    Script  string // 优化后的朗诵文本（含停顿符号 | 表示短停顿，|| 表示长停顿）
    Summary string // 该段的一句话摘要（用于图像生成 prompt）
}
```

**调用服务**：DashScope 文本生成 API（OpenAI-compatible mode，承载 Qwen 文本模型）  
**超时**：30s  
**重试**：最多 2 次，指数退避  
**Prompt 模板**：见 `internal/pipeline/script/prompt.go`

**错误处理**
- 文章超长（>10000字）→ 截断并在 job warning 中记录
- API 返回非法 JSON → 重试，超过次数后 job 状态置为 failed

---

### Task Type: tts（语音合成）

**模块路径**：`internal/pipeline/tts/`

**职责**

- 根据 script task 结果生成音频和 segment 级字幕时间轴

**推荐资源池**：`tts`

**输入**：`[]Segment`（来自 script task 输出）+ `voice_id`

**输出**
```go
type TTSOutput struct {
    AudioSegments []AudioSegment
    TotalDuration float64 // 总时长，单位秒
    SubtitleItems []SubtitleItem // MVP 仅支持 segment 级字幕
}

type AudioSegment struct {
    SegmentIndex int
    FilePath     string  // 相对于 workspace 目录的路径
    Duration     float64 // 单位秒
}

type SubtitleItem struct {
    SegmentIndex int
    Start        float64 // 单位秒
    End          float64 // 单位秒
    Text         string
}
```

**调用服务**：自部署 TTS API  
**并发**：单个 tts task 内部可按 segment 并行请求，最大并发 3  
**超时**：单个请求 60s  
**重试**：最多 3 次  
**输出格式**：WAV，采样率 24000Hz

**文件命名规范**：`{job_id}/audio/segment_{index:03d}.wav`

---

### Task Type: image（配图生成）

**模块路径**：`internal/pipeline/image/`

**职责**

- 根据 script task 和可选人物表结果生成配图

**推荐资源池**：`image_gen`

**输入**：`[]Segment`（使用每段的 `Summary` 字段作为图像 prompt）+ `image_style`

**输出**
```go
type ImageOutput struct {
    Images []GeneratedImage
}

type GeneratedImage struct {
    SegmentIndex int
    FilePath     string // 相对 workspace 路径
    Width        int    // 1280
    Height       int    // 720
    IsFallback   bool   // true 表示该图片由本地降级生成
}
```

**调用服务**：DashScope 图像生成 API（原生接口，承载图像模型）  
**并发**：最大并发 2（图像生成耗时较长，避免超限）  
**超时**：单个请求 120s  
**重试**：最多 2 次  
**输出格式**：JPEG，质量 90

**Prompt 构建规则**
- 基础 prompt：`{segment.Summary}`
- 风格参数：来自 `image_style`
- 固定后缀：`，电影级构图，高质量，16:9`
- 禁止包含人物面部（避免版权/肖像问题）：追加 `，无人物面部特写`

**文件命名规范**：`{job_id}/images/segment_{index:03d}.jpg`

---

### Task Type: video（视频合成）

**模块路径**：`internal/pipeline/video/`

**职责**

- 汇总上游产物，生成最终 MP4

**推荐资源池**：`video_render`

**输入**：`TTSOutput` + `ImageOutput`

**输出**
```go
type VideoOutput struct {
    FilePath string  // 最终视频文件路径
    Duration float64 // 总时长
    FileSize int64   // 字节数
}
```

**工具**：FFmpeg（必须在系统 PATH 中）  
**合成逻辑**
1. 每个 Segment：将对应图片静态展示，时长 = 音频时长
2. 图片之间添加 0.5s 淡入淡出转场
3. 字幕：MVP 使用 `TTSOutput.SubtitleItems` 生成 segment 级 SRT，不做逐词高亮
4. 背景音乐：如有配置则混入（音量 -15dB）
5. 输出：MP4，H.264，AAC 音频，1280x720

**FFmpeg 命令模板**：见 `internal/pipeline/video/ffmpeg.go`  
**超时**：300s（5分钟）  
**不重试**（FFmpeg 失败通常是输入问题，重试无意义，直接 failed）

**文件命名规范**：`{job_id}/output/final.mp4`

---

## Executor 接口

每类 task 应实现统一 executor 接口，由 `scheduler` 调用：

```go
type Executor interface {
    Type() model.TaskType
    Execute(ctx context.Context, task model.Task, job model.Job, dependencies map[string]model.Task) (model.Task, error)
}
```

要求：

- executor 只能处理自己负责的 task type
- executor 不负责判断依赖是否满足
- executor 不负责资源并发控制
- `dependencies` 中只包含当前 task 声明的直接依赖 task
- executor 通过返回更新后的 `task` 回写 `OutputRef` 等执行结果
- executor 必须可被单元测试 mock

## Workspace 目录结构

```
/var/narratio/workspace/
  {job_id}/
    audio/
      segment_000.wav
      segment_001.wav
      ...
    images/
      segment_000.jpg
      segment_001.jpg
      ...
    subtitles/
      full.srt
    output/
      final.mp4
```

临时文件在 job 完成 24 小时后由定时清理任务删除。
