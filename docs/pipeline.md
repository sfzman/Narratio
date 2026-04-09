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
- 作为后续 script 类 task 的稳定上游输入
- 不调用外部 AI 服务

**推荐资源池**：`local_cpu`

**输入**
```go
type SegmentationInput struct {
    JobID       string
    ArticleText string
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
- 在保持独立 task 契约的前提下，逐步补齐真实参考图生成能力

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
    GenerationRequestID  string
    GenerationModel      string
    SourceImageURL       string
}
```

**Artifact 文件**：`jobs/{job_id}/character_images/manifest.json`（相对 `WORKSPACE_DIR`）

**当前代码状态**
- 已接入独立 executor：读取真实 `character_sheet.json`，为每个角色生成参考图 manifest
- 当前每个角色会额外生成 `match_terms`，供下游 `image` 做轻量命中；默认至少包含角色名本身，并会按 `/ | 、 ， , ; ；` 这类分隔符拆出简单别名
- 默认仍是 skeleton 模式：会把 JSON artifact 和本地 fallback JPG 一起落盘
- 若显式开启 `ENABLE_LIVE_IMAGE_GENERATION=true` 且注入 DashScope 图像 client，`character_image` 也会按角色逐个请求真实参考图，并把返回图片写入 `character_{index}.jpg`
- 当前若真实出图成功，会在 manifest 对应角色上补回最小追踪信息：`generation_request_id`、`generation_model`、`source_image_url`
- `task.output_ref.character_sheet_ref` 会回填上游 `character_sheet` artifact 引用，便于后续 image / script 继续消费

---

### Task Type: script（脚本优化）

**模块路径**：`internal/pipeline/script/`

**职责**

- 基于已完成分段的原文、大纲和人物表生成分镜化 script
- 这里的 `script` 就是之前口头提到的 storyboard；后续统一只保留 `script` 这个步骤名

**推荐资源池**：`llm_text`

**输入**
```go
type ScriptInput struct {
    JobID       string
    VoiceID     string // 透传给后续 TTS，用于 prompt 调整语气时可选使用
    Segments    SegmentationOutput
    Outline     *OutlineOutput
    Characters  *CharacterSheetOutput
}
```

**输出**
```go
type ScriptOutput struct {
    Segments []Segment // 分段 script 列表，按顺序
}

type Segment struct {
    Index int    // 从 0 开始
    Shots []Shot // 固定 10 个分镜
}

type Shot struct {
    Index              int
    VisualContent      string
    CameraDesign       string
    InvolvedCharacters []string
    ImageToImagePrompt string // 有主要人物时使用，必须包含 character_sheet 里的准确人物名
    TextToImagePrompt  string // 空镜或群演镜头时使用
}
```

**调用服务**：DashScope 文本生成 API（OpenAI-compatible mode，承载 Qwen 文本模型）  
**超时**：当前代码里文本 HTTP client timeout 为 600s；`script` task 的执行 deadline 现已按 `segment_count * SCRIPT_TIMEOUT_PER_SEGMENT_SECONDS` 动态计算，默认每段 200 秒，其他 task 仍默认使用 12 分钟执行 deadline。后台 runner / 开发态手动 dispatch 的外层超时已放宽，不再比 `script` task 更早截断。  
**重试**：文档原先约定为最多 2 次，指数退避；当前代码尚未接入真实 retry/backoff  
**Prompt 模板**：见 `internal/pipeline/script/prompt.go`

**错误处理**
- 文章超长（>10000字）→ 截断并在 job warning 中记录
- API 返回非法 JSON → 重试，超过次数后 job 状态置为 failed

**Artifact 文件**
- 汇总文件：`jobs/{job_id}/script.json`（相对 `WORKSPACE_DIR`）
- 分段中间产物：`jobs/{job_id}/script/segment_{index:03d}.json`

**当前代码状态**
- 目标契约应与原 Gradio app 对齐：每个 segment 生成 10 个 shot；后续统一把这层结构称为 `script`
- 已接入真实 artifact writer：无论走本地 stub 还是启用真实文本生成，成功后都会把 `ScriptOutput` 落盘到 workspace
- `script` 现在会读取上游 `segments.json`、`outline.json` 和 `character_sheet.json` 的真实内容来构建 prompt，而不是只传 artifact 路径
- `script` 不再负责分段，只负责沿用上游 `segmentation` 的段落边界
- 启用真实文本生成时，`script` 当前会按 segment 逐段调用文本接口，并在每段成功后立即写出对应的 `segment_{index}.json`，最后再汇总写回同一个 `script.json`
- 若同一 job 的 `script/segment_{index}.json` 已存在且可解析，当前 executor 会直接复用该段结果，便于 retry / 中断后续跑
- `script` prompt 当前已对齐为“当前 segment 的 10 个分镜生成”；`outline` 与 `character_sheet` 只作为剧情连续性和人物一致性的约束上下文
- 当前 `script` artifact 已进一步收敛到 shot 级结构：每段只保留 `index + 10 shots`
- 原先临时保留的 `text / script / summary` 已从 `script` artifact 移除；原文继续由 `segments.json` 提供，TTS 直接消费 segmentation 结果
- 当前每个 shot 的核心字段已经收敛到 `involved_characters / image_to_image_prompt / text_to_image_prompt`
- 若某个 shot 出现主要人物，当前归一化逻辑会确保 `image_to_image_prompt` 里包含人物表中的准确名称，避免下游 image 无法命中对应参考图
- `prompt` 现在只保留在程序内部作为兼容派生值，不再写回 `script` artifact

---

### Task Type: tts（语音合成）

**模块路径**：`internal/pipeline/tts/`

**职责**

- 根据 segmentation task 的原文分段结果生成音频和 segment 级字幕时间轴

**推荐资源池**：`tts`

**输入**：`[]TextSegment`（来自 segmentation task 输出）+ `voice_id`

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
- 已接入最小 skeleton executor：读取真实 `segments.json`，并把结构化 `tts_manifest.json` 真正落盘到 workspace
- 当前 manifest 会按分段数生成 skeleton `audio_segments` / `subtitle_items`，`audio_segment_paths` 与字幕文本都来自 segmentation 结果
- 当前还会把 `subtitle_items` 同步写成真实 `subtitles.srt`
- 当前还会按 `audio_segment_paths` 真实写出占位静音 WAV 文件，便于继续闭合下游 video 输入契约
- 尚未接入真实 TTS API、segment 内并发合成与音频文件落盘

---

### Task Type: image（配图生成）

**模块路径**：`internal/pipeline/image/`

**职责**

- 根据 script task 的分镜结果生成配图
- 消费 `character_image` artifact，和人物参考图任务保持解耦

**推荐资源池**：`image_gen`

**输入**：`[]Segment`（当前优先消费每段 `Shots[*].ImageToImagePrompt / TextToImagePrompt`）+ `character_image` manifest + `image_style`

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
    PromptSourceType  string   // 本次 prompt 的基础语义来源：shots / empty
    PromptSourceText  string   // 实际参与 prompt 组装的基础文本
    PromptSourceShots []string // 当来源是 shots 时，记录本次聚合使用的 shot 文本
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
- 目标形态：应由 `script` 每个 segment 下的 10 个 shot 分别驱动出图；若该镜头出现主要人物，则优先消费 `image_to_image_prompt`，否则消费 `text_to_image_prompt`
- 当前临时实现：仍保持“每个 segment 产 1 张图”，但只会从 10 个 shots 中挑选一个更收敛的小集合来组装 prompt
- 单图选 shot 规则：优先保留命中 `matched_characters` 的 shot；若不足，再补当前 segment 的开头 / 中段 / 结尾代表性 shot，最多取 3 条
- 角色参考：优先使用 `matched_characters`，否则回退到 `character_references` candidates，并把所选角色的参考 prompt 一并拼入
- 风格参数：来自 `image_style`
- 固定后缀：`，电影级构图，高质量，16:9`
- 禁止包含人物面部（避免版权/肖像问题）：追加 `，无人物面部特写`

**文件命名规范**：`{job_id}/images/segment_{index:03d}.jpg`

**当前代码状态**
- 已接入最小 skeleton executor：会读取真实 `script.json` 与 `character_images/manifest.json`，并把结果落盘到 `jobs/{job_id}/images/image_manifest.json`
- 当前 manifest 已包含每段的 skeleton prompt、prompt source trace、`character_references` 与 `matched_characters`
- 当前单张配图仍保持“每个 segment 产 1 张图”，但 prompt 基础语义已固定取收紧后的 `shots` 子集，并优先使用 `image_to_image_prompt / text_to_image_prompt`
- `matched_characters` 当前使用轻量规则：遍历 `match_terms`，在 segment `shots` 的 prompt 与 `involved_characters` 中做字符串命中
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
- 当前会先校验 `tts` / `image` 依赖里的 `artifact_path` 必须存在且非空
- 当前对 `tts` 先做 `output_ref` 级最小结构校验（如 `segment_count`、`audio_segment_paths` 数量、总时长）
- 若 executor 注入了 `workspaceDir`，当前还会额外校验 `tts_manifest.json`、`subtitles.srt` 与 `audio_segment_paths` 引用的 WAV 文件是否存在
- 若 executor 注入了 `workspaceDir`，当前还会对 `image` artifact 做最小文件/结构校验（如 manifest 存在、`images` 非空、数量与 `tts.segment_count` 一致，且每条 `file_path` 引用的 JPG 文件存在）
- `scheduler` 会在 `video` 成功后将上述产物提升为 `job.Result`
- 尚未接入真实 FFmpeg 调用、更细粒度输入内容解析与最终文件落盘

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
      script/
        segment_000.json
        segment_001.json
        ...
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
