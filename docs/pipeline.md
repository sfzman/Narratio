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

### Task Type: segmentation（原文分段）

**模块路径**：`internal/pipeline/script/`

**职责**

- 对原始文章做 deterministic 分段
- 作为后续 script / storyboard 类 task 的稳定上游输入
- 不调用外部 AI 服务

**推荐资源池**：`local_cpu`

**输入**
```go
type SegmentationInput struct {
    JobID       string
    ArticleText string
    Language    string
}
```

**输出**
```go
type SegmentationOutput struct {
    Segments []TextSegment
}

type TextSegment struct {
    Index     int
    Text      string
    CharCount int
}
```

**Artifact 文件**：`jobs/{job_id}/segments.json`（相对 `WORKSPACE_DIR`）

**当前代码状态**
- 已接入真实 artifact writer，成功后会把 `SegmentationOutput` 落盘到 workspace
- 当前实现参考原 Gradio app，先按句号/换行切句，再按约 250 非标点字数聚合成段
- `task.output_ref.artifact_path` 现在指向真实存在的 JSON 文件

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
    StoryPosition             OutlineStoryPosition
    Mainline                  string
    PlotStages                []OutlineStage
    RelationshipStateChanges  []string
    ContinuityNotes           []string
    SegmentReadingNotes       []string
}

type OutlineStoryPosition struct {
    Genre         string
    EraBackground string
    CoreConflict  string
    EmotionalTone string
    EndingType    string
}

type OutlineStage struct {
    Name     string
    Happened string
    Goal     string
    Obstacle string
    Outcome  string
}
```

**Artifact 文件**：`jobs/{job_id}/outline.json`（相对 `WORKSPACE_DIR`）

**当前代码状态**
- 已接入真实 artifact writer：无论走本地 stub 还是启用真实文本生成，成功后都会把 `OutlineOutput` 落盘到 workspace
- `outline` 的内容语义已对齐原 Gradio app：虽然产物仍统一为 JSON，但字段表达的是“供后续分镜使用的完整上下文大纲”，而不是简短章节摘要
- `task.output_ref.artifact_path` 现在指向真实存在的 JSON 文件，而不是仅有占位路径

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
    Name                      string
    Role                      string
    Age                       string
    Gender                    string
    Appearance                string
    Temperament               string
    PersonalityTraits         []string
    Identity                  string
    RelationshipToProtagonist string
    VisualSignature           string
    ReferenceSubjectType      string
    ImagePromptFocus          string
}
```

**Artifact 文件**：`jobs/{job_id}/character_sheet.json`（相对 `WORKSPACE_DIR`）

**当前代码状态**
- 已接入真实 artifact writer：无论走本地 stub 还是启用真实文本生成，成功后都会把 `CharacterSheetOutput` 落盘到 workspace
- `character_sheet` 的内容语义已对齐原 Gradio app：虽然产物仍统一为 JSON，但字段表达的是“服务人物参考图与分镜生成的主要人物表”，而不是简化描述
- `image_prompt_focus` 现在只表达参考图的构图与出图约束，避免和 `appearance` / `visual_signature` 重复
- `task.output_ref.artifact_path` 现在指向真实存在的 JSON 文件，而不是仅有占位路径

---

### Task Type: character_image（人物参考图）

**模块路径**：`internal/pipeline/image/`

**职责**

- 根据 `character_sheet` 产出独立的人物参考图 manifest
- 将“人物参考图”与普通 `image` 段落配图拆分成两个 task 契约
- 当前阶段只落盘结构化 artifact，不接真实出图

**推荐资源池**：`image_gen`

**输入**
```go
type CharacterImageInput struct {
    CharacterSheet CharacterSheetOutput
}
```

**输出**
```go
type CharacterImageOutput struct {
    Images []CharacterReferenceImage
}

type CharacterReferenceImage struct {
    CharacterIndex       int
    CharacterName        string
    ReferenceSubjectType string
    FilePath             string
    Prompt               string
    MatchTerms           []string
    IsFallback           bool
}
```

**Artifact 文件**：`jobs/{job_id}/character_images/manifest.json`（相对 `WORKSPACE_DIR`）

**当前代码状态**
- 已接入独立 skeleton executor：读取真实 `character_sheet.json`，为每个角色生成参考图 manifest
- 当前每个角色会额外生成 `match_terms`，供下游 `image` 做轻量命中；默认至少包含角色名本身，并会按 `/ | 、 ， , ; ；` 这类分隔符拆出简单别名
- 当前只会落盘 JSON artifact，并预留未来参考图文件路径；对应 JPG 文件此阶段不会真实生成
- `task.output_ref.character_sheet_ref` 会回填上游 `character_sheet` artifact 引用，便于后续 image / storyboard 继续消费

---

### Task Type: script（脚本优化）

**模块路径**：`internal/pipeline/script/`

**职责**

- 基于已完成分段的原文、大纲和人物表生成朗诵脚本

**推荐资源池**：`llm_text`

**输入**
```go
type ScriptInput struct {
    JobID       string
    Language    string // "zh" | "en"，默认 "zh"
    VoiceID     string // 透传给后续 TTS，用于 prompt 调整语气时可选使用
    Segments    SegmentationOutput
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

**Artifact 文件**：`jobs/{job_id}/script.json`（相对 `WORKSPACE_DIR`）

**当前代码状态**
- 已接入真实 artifact writer：无论走本地 stub 还是启用真实文本生成，成功后都会把 `ScriptOutput` 落盘到 workspace
- `script` 现在会读取上游 `segments.json`、`outline.json` 和 `character_sheet.json` 的真实内容来构建 prompt，而不是只传 artifact 路径
- `script` 不再负责分段，只负责沿用上游 `segmentation` 的段落边界，为每段生成朗读化旁白和单句配图摘要
- 当前 `script` task 仍保持小步推进：只把结构化结果落盘并回写真实 `artifact_path`，尚未推进下游真实 TTS / Image / Video 接入

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

**当前代码状态**
- 已接入最小 skeleton executor：消费 `script` 输出，产出假的音频 manifest / 字幕引用，便于继续推进 DAG
- 尚未接入真实 TTS API、segment 内并发合成与音频文件落盘

---

### Task Type: image（配图生成）

**模块路径**：`internal/pipeline/image/`

**职责**

- 根据 script task 的分段摘要生成配图
- 消费 `character_image` artifact，和人物参考图任务保持解耦

**推荐资源池**：`image_gen`

**输入**：`[]Segment`（使用每段的 `Summary` 字段作为图像 prompt）+ `character_image` manifest + `image_style`

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
    Prompt       string // 当前 skeleton 生成的最终出图 prompt
    Summary      string // 对应 script segment 的摘要
    GenerationRequestID string // 真实出图成功时回填上游 request id
    GenerationModel     string // 真实出图成功时回填实际使用模型
    SourceImageURL      string // 真实出图成功时回填下载源图片 URL
    CharacterReferences []ImageCharacterReference
    MatchedCharacters   []ImageCharacterReference // 当前段命中的主要角色
}

type ImageCharacterReference struct {
    CharacterIndex int
    CharacterName  string
    FilePath       string // 对应 character_image 预留参考图路径
    Prompt         string // 从 character_image 透传的人物参考描述
    MatchTerms     []string
}
```

**调用服务**：DashScope 图像生成 API（原生接口，承载图像模型）  
**并发**：最大并发 2（图像生成耗时较长，避免超限）  
**超时**：单个请求 120s  
**重试**：最多 2 次  
**输出格式**：JPEG，质量 90

**Prompt 构建规则**
- 基础 prompt：`{segment.Summary}`
- 角色参考：优先使用 `matched_characters`，否则回退到 `character_references` candidates，并把所选角色的参考 prompt 一并拼入
- 风格参数：来自 `image_style`
- 固定后缀：`，电影级构图，高质量，16:9`
- 禁止包含人物面部（避免版权/肖像问题）：追加 `，无人物面部特写`

**文件命名规范**：`{job_id}/images/segment_{index:03d}.jpg`

**当前代码状态**
- 已接入最小 skeleton executor：会读取真实 `script.json` 与 `character_images/manifest.json`，并把结果落盘到 `jobs/{job_id}/images/image_manifest.json`
- 当前 manifest 已包含每段的 skeleton prompt、摘要、`character_references` 与 `matched_characters`
- `matched_characters` 当前使用轻量规则：遍历 `match_terms`，在 segment `summary / script / text` 中做字符串命中，不做指代消解或语义识别
- 当前 prompt 组装顺序与原 Gradio 思路对齐：优先使用 `matched_characters`，若为空则回退 `character_references` 作为 candidates；所选角色的人物参考描述会直接拼进 prompt
- 当前已支持注入真实 DashScope 图像 client；启用后会按段请求并把返回图片落到 `segment_{index}.jpg`
- 当前若真实出图成功，会在 manifest 对应段上补回最小追踪信息：`generation_request_id`、`generation_model`、`source_image_url`
- 默认仍是 skeleton 模式；未启用或单段请求失败时，当前会把本地纯色 fallback JPEG 真实写到 `segment_{index}.jpg`，并将该段标记为 `is_fallback=true`

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

**文件命名规范**：`{job_id}/output/final.mp4`

**当前代码状态**
- 当前仍是最小 skeleton executor：消费 `tts` 与 `image` 的 `artifact_path` / 时长摘要，产出假的 MP4 引用、时长和文件大小
- 当前 skeleton 不解析 `image` manifest 内部的逐段图片明细，因此上游既可以是真实生成图，也可以是 fallback JPG；这一层先统一按 artifact 引用透传
- `scheduler` 会在 `video` 成功后将上述产物提升为 `job.Result`
- 尚未接入真实 FFmpeg 调用、SRT 生成、输入文件校验与最终文件落盘

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
  jobs/
    {job_id}/
      segments.json
      outline.json
      character_sheet.json
      character_images/
        manifest.json
        character_000.jpg
        character_001.jpg
        ...
      script.json
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
