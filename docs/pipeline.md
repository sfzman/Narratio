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
- 若最后一段明显过短（当前阈值约为 `< 80` 非标点字）且并回上一段后不会让上一段过长，当前会自动把短尾段回并，避免为几十字单独生成完整下游资产
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
- executor 运行中会按角色上报瞬时 progress，便于前端观察当前推进到第几个角色参考图
- 当前人物参考图固定走“单人、正面、全身像”构图，不跟 job 的 `aspect_ratio` 联动；当前固定比例为 `2:3`，默认尺寸为 `832x1248`
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
    Shots []Shot // 按 segment 长度动态生成，当前约为 3~10 个分镜
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
- `script` 当前仍按 segment 逐段生成 shot，但每段 shot 数已改为随 segment 长度动态收敛；常见范围约为 3~10 个
- 已接入真实 artifact writer：无论走本地 stub 还是启用真实文本生成，成功后都会把 `ScriptOutput` 落盘到 workspace
- `script` 现在会读取上游 `segments.json`、`outline.json` 和 `character_sheet.json` 的真实内容来构建 prompt，而不是只传 artifact 路径
- `script` 不再负责分段，只负责沿用上游 `segmentation` 的段落边界
- 启用真实文本生成时，`script` 当前会按 segment 逐段调用文本接口，并在每段成功后立即写出对应的 `segment_{index}.json`，最后再汇总写回同一个 `script.json`
- 若同一 job 的 `script/segment_{index}.json` 已存在且可解析，当前 executor 会直接复用该段结果，便于 retry / 中断后续跑
- `script` prompt 当前会显式告知“当前 segment 目标分镜数”；`outline` 与 `character_sheet` 只作为剧情连续性和人物一致性的约束上下文
- 当前 `script` artifact 已进一步收敛到 shot 级结构：每段只保留 `index + shots`
- 原先临时保留的 `text / script / summary` 已从 `script` artifact 移除；原文继续由 `segments.json` 提供，TTS 直接消费 segmentation 结果
- 当前每个 shot 的核心字段已经收敛到 `involved_characters / image_to_image_prompt / text_to_image_prompt`
- 若某个 shot 出现主要人物，当前归一化逻辑会确保 `image_to_image_prompt` 里包含人物表中的准确名称，避免下游 image 无法命中对应参考图
- `prompt` 现在只保留在程序内部作为兼容派生值，不再写回 `script` artifact

---

### Task Type: tts（语音合成）

**模块路径**：`internal/pipeline/tts/`

**职责**

- 根据 segmentation task 的原文分段结果生成音频和 segment 级字幕时间轴
- 每个 segment 内部先按中文句号 `。` 切句，再逐句串行调用 TTS 服务，最后合并成 segment 级音频

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
**并发**：当前以串行生成为主；由于 TTS 服务性能限制，segment 内必须逐句串行请求  
**超时**：单句请求超时由具体 TTS client 控制  
**重试**：最多 3 次  
**输出格式**：WAV，采样率 24000Hz

**文件命名规范**：`{job_id}/audio/segment_{index:03d}.wav`

**当前代码状态**
- 已接入最小 skeleton executor：读取真实 `segments.json`，并把结构化 `tts_manifest.json` 真正落盘到 workspace
- 当前默认仍会按分段数生成 skeleton `audio_segments` / `subtitle_items`，`audio_segment_paths` 与字幕文本都来自 segmentation 结果
- 当前会逐个 segment 增量写出 `audio/segment_{index}.wav`，并在每段完成后刷新一次 `tts_manifest.json`，便于运行中观察与中断后保留中间结果
- 当前不再额外写出 `subtitles.srt`；下游若需要字幕时间轴，统一从 `tts_manifest.json.subtitle_items` 读取
- 若注入真实 TTS client，当前 executor 已会对每个 segment 的 `text` 按 `。` 切句，并逐句串行调用，再把句子级 WAV 合并成 segment 级音频
- 当前字幕仍保持 segment 级；尚未升级到句级时间轴

---

### Task Type: image（配图生成）

**模块路径**：`internal/pipeline/image/`

**职责**

- 根据 script task 的分镜结果生成配图
- 消费 `character_image` artifact，和人物参考图任务保持解耦

**推荐资源池**：`image_gen`

**输入**：`[]Segment`（当前优先消费每段 `Shots[*].ImageToImagePrompt / TextToImagePrompt`）+ `character_image` manifest + `image_style` + `aspect_ratio`

**输出**
```go
type ImageOutput struct {
    Images     []GeneratedImage     // 当前兼容 video 的 segment 级图片摘要
    ShotImages []GeneratedShotImage // 新增的 shot 级出图 manifest
}

type GeneratedImage struct {
    SegmentIndex int
    FilePath     string // 相对 workspace 路径
    Width        int    // 当前按 aspect_ratio 输出：16:9 -> 1280x720；9:16 -> 720x1280
    Height       int
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

type GeneratedShotImage struct {
    SegmentIndex int
    ShotIndex    int
    FilePath     string // 规划中的 shot 图片路径
    Width        int
    Height       int
    Prompt       string // 直接来自单个 shot 的正式提示词
    PromptType   string // image_to_image / text_to_image
    IsFallback   bool   // true 表示该镜头最终没有成功出图，走了降级
    FilledFromPrevious bool // true 表示该镜头复用了最近一次成功图补位
    GenerationRequestID string // 真实出图成功时回填上游 request id
    GenerationModel     string // 真实出图成功时回填实际使用模型
    SourceImageURL      string // 真实出图成功时回填下载源图片 URL
    InvolvedCharacters  []string
    CharacterReferences []ImageCharacterReference
    MatchedCharacters   []ImageCharacterReference
}

type ImageCharacterReference struct {
    CharacterIndex int
    CharacterName  string
    FilePath       string // 对应 character_image 预留参考图路径
    Prompt         string // 从 character_image 透传的人物参考描述
    MatchTerms     []string
    SourceImageURL string // 真实参考图成功时回填下载源图片 URL
}
```

**调用服务**：DashScope 图像生成 API（原生接口，承载图像模型）  
**并发**：最大并发 2（图像生成耗时较长，避免超限）  
**超时**：单个请求 120s  
**重试**：最多 3 次  
**输出格式**：JPEG，质量 90

**Prompt 构建规则**
- 真实执行路径：由 `script` 每个 segment 下的 shot 分别驱动出图，当前实现是一镜一图
- prompt 选择：若该镜头出现主要人物，则优先消费 `image_to_image_prompt`，否则消费 `text_to_image_prompt`
- 角色参考：若当前 shot 命中主要人物，则把对应 `character_image` 参考图作为输入参考；若未命中，则回退到现有 candidates
- 图生图 prompt：会把人物名替换成 `图1中的人物 / 图2中的人物` 这类占位表达，再和参考图输入一同发送给图像接口
- 风格参数：来自 `image_style`
- 固定后缀：`，电影级构图，高质量，{aspect_ratio}`；当前支持 `16:9 / 9:16`
- 禁止包含人物面部（避免版权/肖像问题）：追加 `，无人物面部特写`

**文件命名规范**：真实出图主路径为 `jobs/{job_id}/images/segment_{segment_index:03d}_shot_{shot_index:03d}.jpg`

兼容摘要 `images[*].file_path` 不再单独生成新文件，而是复用本 segment 某个已生成 shot 图片的路径。

**当前代码状态**
- 已接入最小 skeleton executor：会读取真实 `script.json` 与 `character_images/manifest.json`，并把结果落盘到 `jobs/{job_id}/images/image_manifest.json`
- 当前真实出图路径已经切到 `shot_images`：每个可消费 shot 都会生成自己的 JPG，并记录 `segment_index / shot_index / file_path / prompt / prompt_type`
- executor 运行中会按 shot 上报瞬时 progress，便于前端观察当前推进到第几张分镜图
- 当前 manifest 仍保留 `images` 作为给 `video` 的 segment 级兼容摘要，但不再额外单独请求 segment 图；摘要只复用本 segment 某个已生成 shot 的文件路径
- `matched_characters` 当前使用轻量规则：遍历 `match_terms`，在 segment `shots` 的 prompt 与 `involved_characters` 中做字符串命中
- 当前 prompt 组装顺序与原 Gradio 思路对齐：优先使用 `matched_characters`，若为空则回退 `character_references` 作为 candidates；图生图时会把这些角色的参考图作为输入一并传给模型
- `image` 侧不再兼容旧的 shot 级 `prompt` 字段；若某个 shot 没有 `image_to_image_prompt / text_to_image_prompt`，则视为不可消费
- 当前已支持注入真实 DashScope 图像 client；启用后会按 shot 请求并把返回图片落到 `segment_{segment}_shot_{shot}.jpg`
- 当前若真实出图成功，会在 manifest 对应 shot 上补回最小追踪信息：`generation_request_id`、`generation_model`、`source_image_url`
- 当前单个 shot 会最多重试 3 次；若重试耗尽仍失败，则优先用“最近一次成功生成的 shot 图”补位；若此前还没有成功图，才回退到本地 fallback JPEG
- 对 `shot_video` 而言，真正稳定的上游契约是 `image_manifest.json.shot_images[*]`；其中最小必需字段是 `segment_index / shot_index / file_path / prompt`
- `source_image_url` 对 `shot_video` 不是必填，但若存在，live 图生视频会优先直接用它提交远端请求；若不存在，则回退读取本地 `file_path`
- `character_image` 不会被 `shot_video` 直接消费；它的职责是先帮助 `image` 产出更稳定的 shot 首帧，再由 `shot_video` 只消费最终的 `shot_images`

---

### Task Type: shot_video（分镜视频片段）

**模块路径**：`internal/pipeline/video/`

**职责**

- 消费 `image.shot_images`，为后续“逐 shot 图生视频”保留独立任务边界
- 产出 shot 级视频片段 manifest，让最终 `video` 只负责拼接与回退选择

**推荐资源池**：`video_gen`

**输入**：`ImageOutput.ShotImages` + `aspect_ratio` + `video_count`（默认 `12`，表示只为排序后的前 `n` 个 shot 尝试生成视频）

**输出**
```go
type ShotVideoOutput struct {
    Clips []GeneratedShotVideo
}

type GeneratedShotVideo struct {
    SegmentIndex int
    ShotIndex    int
    Status       string  // 当前正式枚举：generated_video / image_fallback
    DurationSeconds float64 // 默认 3 秒，可通过配置覆盖；live / fallback 都会稳定写入
    VideoPath    string // 真实图生视频成功后填写
    ImagePath    string // 回退静态图路径；未生成真实视频时填写
    SourceImagePath string // 上游 shot image 路径；即使未来生成了真实视频也保留，便于追踪图生视频输入
    SourceType   string // generated_video / image_fallback
    IsFallback   bool
    GenerationRequestID string // 真实图生视频成功后回填 request id
    GenerationModel     string // 真实图生视频成功后回填实际模型
    SourceVideoURL      string // 真实图生视频成功后回填下载源视频 URL
}
```

**Artifact 文件**：`jobs/{job_id}/shot_videos/manifest.json`（相对 `WORKSPACE_DIR`）

**当前代码状态**
- 已接入独立 executor：会读取真实 `images/image_manifest.json` 里的 `shot_images`
- runtime 在 `ENABLE_LIVE_VIDEO_GENERATION=true` 时会注入真实 DashScope video client；关闭时仍会稳定产出 `image_fallback` clip manifest
- 当前 live 路径会先按 `segment_index / shot_index` 排序，再只对前 `video_count` 个 shot 调图生视频；单个 shot 成功时写入 `generated_video` clip，并回填 `video_path / generation_request_id / generation_model / source_video_url`
- 单个 shot 图生视频失败时不会阻断整 job，而是回退到 `image_fallback`；`image_path / source_image_path / duration_seconds` 仍会稳定写入
- `image_fallback` 在最终 `video` 阶段不是纯静止图，而是会渲染成带轻微上下/左右平移的静态视频片段，再参与 segment 内拼接
- 超出 `video_count` 的 shot 不会调用视频接口，而是直接登记为 `image_fallback`，供最终 `video` 阶段统一拼接
- 默认 clip 时长为 `3` 秒，也可通过 `SHOT_VIDEO_DEFAULT_DURATION_SECONDS` 覆盖
- 每条 clip 都会保留 `source_image_path`，用于稳定表达“这段视频片段是由哪一张 shot image 驱动出来的”；即使成功生成真实视频，这个字段也不丢
- 当前 `status` 的正式枚举先收口为 `generated_video | image_fallback`；还不单独引入 `failed` 状态，失败场景先通过回退到 `image_fallback` 表达
- executor 运行中会按 shot 上报瞬时 progress，便于前端观察当前推进到第几个分镜视频片段
- `task.output_ref.image_source_type` 当前会透传底层仍消费的是 `shot_images`；`generation_mode` 现已稳定收口为 `generated_video | image_fallback | mixed`
- `task.output_ref.aspect_ratio` 当前会透传本次 job 的画幅比例，供最终 `video` 阶段与调试面板复用
- `task.output_ref.requested_video_count / selected_video_count` 当前分别表示请求的前 `n` 个、以及实际参与图生视频的前 `n` 个
- 若要做真实 `shot_video` 联调，前置条件不是只看 `ENABLE_LIVE_VIDEO_GENERATION`，而是先确认 `character_image` / `image` 已经产出了你真正想拿来做首帧的视频输入；否则 `shot_video` 只是继续消费上游 fallback 图
- 当前 live `shot_video` 对上游 `image.shot_images[*]` 的最小依赖字段是：`segment_index / shot_index / file_path / prompt`；其中 `source_image_url` 只是直传优化项，不存在时也不阻塞联调

---

### Task Type: video（视频合成）

**模块路径**：`internal/pipeline/video/`

**职责**

- 汇总上游产物，生成最终 MP4

**推荐资源池**：`video_render`

**输入**：`TTSOutput` + `ShotVideoOutput` + `aspect_ratio`

**输出**
```go
type VideoOutput struct {
    FilePath string  // 最终视频文件路径
    Duration float64 // 当前返回最终视觉成片时长，并同步回填 narration/visual duration 对比字段
    FileSize int64   // 字节数
}
```

**文件命名规范**：`{job_id}/output/final.mp4`

**当前输出画幅**
- `16:9`：1280x720
- `9:16`：720x1280

**当前代码状态**
- runtime 当前默认注入真实 FFmpeg 渲染 executor：消费 `tts` 与 `shot_video` 的 artifact，真正落盘 `jobs/{job_id}/output/final.mp4`
- `video` 本身不直接调用图生视频 API；是否生成真实视频片段，统一由上游 `shot_video` 决定
- 当前会先校验 `tts` / `shot_video` 依赖里的 `artifact_path` 必须存在且非空
- 当前对 `tts` 先做 `output_ref` 级最小结构校验（如 `segment_count`、`audio_segment_paths` 数量、总时长）
- 若 executor 注入了 `workspaceDir`，当前还会额外校验 `tts_manifest.json` 与 `audio_segment_paths` 引用的 WAV 文件是否存在
- 若 executor 注入了 `workspaceDir`，当前会校验 `shot_videos/manifest.json` 的 clip 结构、segment coverage、排序与去重语义，以及每条 `video_path` / `image_path` 引用的文件是否存在
- executor 运行中会按阶段上报瞬时 progress，当前至少覆盖依赖校验、音频拼接、逐 segment 渲染、分段拼接、最终 mux 和产物整理
- 当前真实渲染路径是：先合并所有 TTS 音频，再按 segment 聚合 shot clip；`generated_video` 会先归一化到统一画幅，`image_fallback` 会先渲染成静态 MP4，然后按 segment 级音频时长做整体变速，最后拼接并 mux 成最终 MP4
- 当前 runtime 会在服务启动时先检查本机 `ffmpeg` 是否可用；`video` task 本身则受独立 `VIDEO_RENDER_TIMEOUT_SECONDS` 配置约束
- 当前 `duration_seconds` 已改为基于 `shot_video.clips[*].duration_seconds` 求和得到的视觉拼接总时长；同时会额外回填 `narration_duration_seconds` 与 `visual_duration_seconds` 便于联调对比
- 当前 `video.output_ref.shot_video_artifact_ref` 会回填上游 `shot_video` artifact 引用；`image_source_type` 继续透传底层静态图来源，方便联调
- `scheduler` 会在 `video` 成功后将上述产物提升为 `job.Result`
- 当前最终 `final.mp4` 还会做“存在且非空”校验；若 FFmpeg 命令返回成功但没有真正产出文件，task 会直接失败
- 为了兼容单测，`NewExecutor(...)` 仍保留无 runner 的 skeleton 构造器；真实服务 runtime 使用的是 `NewRealExecutor(...)`

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
      tts_manifest.json
      audio/
        segment_000.wav
        segment_001.wav
        ...
      images/
        image_manifest.json
        segment_000_shot_000.jpg
        segment_000_shot_001.jpg
        ...
        segment_001_shot_000.jpg
        ...
      shot_videos/
        manifest.json
      output/
        final.mp4
```

临时文件在 job 完成 24 小时后由定时清理任务删除。
