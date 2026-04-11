# docs/job-lifecycle.md — Job / Task 生命周期

## 概述

Narratio 不再把一次生成请求建模成一条固定顺序流水线，而是：

- 一个 `job` 表示一次用户请求
- 一个 `task` 表示 job 内的一个可执行节点
- `task` 之间通过依赖关系组成 DAG
- `scheduler` 根据 DAG 和资源上限调度 task

这套模型用于支持：

- 不同工作流模板
- 无依赖 task 并行执行
- 按资源类型限流
- 后续扩展更多中间产物，如大纲、人物表、分镜

## 分层约束

```
handler -> app/jobs -> scheduler -> pipeline executors
                  \-> store
```

- `handler` 只负责 HTTP
- `app/jobs` 负责创建 job、构建 task DAG、取消 job
- `scheduler` 负责挑选 ready task 并派发执行
- `pipeline/*` 负责执行具体 task
- `store` 保存 job / task 元数据，不保存运行时对象

## 核心数据结构

```go
type JobSpec struct {
    Article string
    Options RenderOptions
}

type RenderOptions struct {
    VoiceID     string
    ImageStyle  string
    AspectRatio string
    VideoCount  *int
}

type Job struct {
    ID        string
    Token     string
    Status    JobStatus
    Progress  int
    Spec      JobSpec
    Warnings  []string
    Error     *JobError
    Result    *JobResult
    CreatedAt time.Time
    UpdatedAt time.Time
}

type Task struct {
    ID          string
    JobID       string
    Type        TaskType
    Status      TaskStatus
    ResourceKey ResourceKey
    DependsOn   []string
    Attempt     int
    MaxAttempts int
    Payload     map[string]any
    OutputRef   map[string]any
    Error       *TaskError
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

约束：

- `Task.Type` 表示业务语义，如 `segmentation`、`outline`、`character_sheet`、`character_image`、`tts`、`shot_video`
- `Task.ResourceKey` 表示资源语义，如 `local_cpu`、`llm_text`、`image_gen`、`video_gen`
- 不允许把资源并发控制硬编码到 task type

## Job 状态

顶层 `job.status` 只表示生命周期：

- `queued`
- `running`
- `cancelling`
- `completed`
- `failed`
- `cancelled`

推导规则：

- job 创建完成但尚未有 task 执行时：`queued`
- 任一 task `running` 时：`running`
- 收到取消请求且仍有 task 未结束：`cancelling`
- 全部必需 task 成功：`completed`
- 任一关键 task 失败且 workflow 无法继续：`failed`
- 剩余未完成 task 均被取消：`cancelled`

## Task 状态

`task.status` 使用更细粒度枚举：

- `pending`
- `ready`
- `running`
- `succeeded`
- `failed`
- `cancelled`
- `skipped`

状态迁移：

```
pending -> ready -> running -> succeeded
                       \----> failed
pending/ready/running -> cancelled
pending/ready -> skipped
```

说明：

- `pending`：依赖尚未满足
- `ready`：依赖已满足，等待调度
- `running`：已获得资源并交给 executor
- `skipped`：因上游失败或工作流裁剪而不执行

## DAG 与工作流构建

`app/jobs` 在创建 job 后，必须同步构建一组 task。

当前最小实现约束：

- `app/jobs.CreateJob(spec)` 负责规范化默认值
- 当前默认会把 `options.aspect_ratio` 规范化为 `9:16`
- 当前默认会把 `options.video_count` 规范化为 `12`
- 使用固定 workflow builder 生成首版 task DAG
- 通过 store 的原子接口一次性写入 `job + tasks`
- 当前实现会在入库成功后把 `job.ID` 投递给后台 runner
- 后台 runner 串行调用 `scheduler.DispatchOnce(jobID)`，直到 job 进入终态或当前没有 ready task

MVP 先支持一套固定 workflow，但内部表达必须是 DAG，而不是硬编码顺序调用。

示例：

```text
segmentation -----------\
outline -----------------\
character_sheet ----------> script ----------------\
character_sheet ----------> character_image ---> image ---> shot_video ---> video
segmentation -------------> tts -----------------------------------------/
```

在这个例子里：

- `segmentation`、`outline` 与 `character_sheet` 可并行
- `script` 只有在三者都成功后才会进入 `ready`
- `tts` 只依赖 `segmentation`，直接消费原文分段结果
- `character_image` 依赖 `character_sheet`，用于独立产出人物参考图 artifact
- `image` 依赖 `script` 和 `character_image`，普通配图与人物参考图在任务层分离
- `shot_video` 依赖 `image`，用于逐 shot 产出“视频片段或静态图回退”的中间 manifest
- `outline` 与 `character_sheet` 依赖 `llm_text`，`segmentation` 依赖 `local_cpu`
- `character_image` 与 `image` 依赖 `image_gen`
- `shot_video` 依赖 `video_gen`
- `video` 依赖 `tts` 和 `shot_video`

## 资源感知调度

调度器按 `ResourceKey` 做统一限流。

MVP 资源池示例：

```go
const (
    ResourceLocalCPU   ResourceKey = "local_cpu"
    ResourceLLMText    ResourceKey = "llm_text"
    ResourceTTS        ResourceKey = "tts"
    ResourceImageGen   ResourceKey = "image_gen"
    ResourceVideoGen   ResourceKey = "video_gen"
    ResourceVideoRender ResourceKey = "video_render"
)
```

默认并发建议：

- `local_cpu`: 4
- `llm_text`: 2
- `tts`: 3
- `image_gen`: 2
- `video_gen`: 1
- `video_render`: 1

调度规则：

1. 只有所有依赖都 `succeeded` 的 task 才能进入 `ready`
2. `ready` task 只有在对应资源池有空闲配额时才能启动
3. task 结束后必须释放资源配额
4. 调度器不关心具体 API，只关心 task 状态和资源占用

当前最小实现约束：

- scheduler 第一版先只负责 `pending -> ready` 判定
- 再提供内存版 `ResourceManager` 做资源配额检查
- 第二步增加“单次只 dispatch 一个 ready task”的同步执行入口
- 第三步增加 `DispatchOnce(jobID)`，从 store 读取并把 task/job 状态写回数据库
- 第四步开始接真实包内 executor，但仍可先用 stub 产物，不急着调外部 API
- 当前 skeleton 已可让 script task 读取 segmentation / outline / character_sheet 的依赖产物
- 当前 `character_image` 独立 task 已接入：成功后会把人物参考图 manifest 落盘到 workspace，并在非 live 模式下写出 fallback JPG
- 当前 `shot_video` 独立 task 已接入：会读取 `image.shot_images`，默认写入 `image_fallback` clip 清单；若显式开启 `ENABLE_LIVE_VIDEO_GENERATION=true`，则会逐 shot 调真实图生视频并回填 `generated_video` clip
- 当前 `shot_video.payload.video_count` 已接入：执行时会按 `segment_index / shot_index` 排序，只为前 `n` 个 shot 尝试真实图生视频，其余 shot 直接回退为 `image_fallback`
- 当前最小后台 runner 已接入，默认会在 job 创建后自动持续推进
- `POST /jobs/:id/dispatch-once` 仍保留为开发态接口；若同一 job 已在后台运行，该接口返回 no-op

## 取消语义

- `DELETE /jobs/:id` 作用于整个 job
- `app/jobs` 收到取消请求后，向该 job 下所有未结束 task 传播取消信号
- `running` task 通过 `context.Context` 取消
- `pending` / `ready` task 直接置为 `cancelled`
- 已完成 task 保持原状态，不回滚

## Panic 与异常恢复

- 所有后台 goroutine 外层必须 `recover`
- `recover` 后将对应 task 置为 `failed`
- job 状态由 task 聚合结果重新计算

## 清理策略

- `completed` / `failed` / `cancelled` 的 workspace 默认保留 24 小时
- workspace 清理与 job / task 元数据解耦
- 即使产物被清理，短期内仍保留 job / task 元数据供前端显示
