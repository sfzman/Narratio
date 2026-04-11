# docs/api-spec.md — API 接口规范

## 通用约定

- Base URL：`/api/v1`
- 请求/响应格式：`application/json`
- 时间格式：RFC3339（`2024-01-15T10:30:00Z`）
- 错误响应统一格式（见下方）
- 所有接口均需 CORS 支持

## 统一响应结构

**成功**
```json
{
  "code": 0,
  "data": { ... }
}
```

**失败**
```json
{
  "code": 1001,
  "message": "文章内容不能为空",
  "request_id": "req_abc123"
}
```

**错误码表**

| code | 含义 |
|---|---|
| 0 | 成功 |
| 1001 | 请求参数错误 |
| 1002 | 任务不存在 |
| 1003 | 任务尚未完成，结果不可下载 |
| 1004 | 任务当前状态不允许该操作 |
| 5001 | 外部 AI 服务调用失败 |
| 5002 | FFmpeg 合成失败 |
| 5003 | 服务内部错误 |

---

## 接口列表

### POST /api/v1/jobs — 创建生成任务

**Request Body**
```json
{
  "article": "这是一篇文章内容...",
  "options": {
    "voice_id": "male_calm",
    "image_style": "realistic",
    "aspect_ratio": "9:16",
    "video_count": 12
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| article | string | ✅ | 文章内容，1~10000 字 |
| options.voice_id | string | ❌ | TTS 音色 preset ID，默认 `male_calm` |
| options.image_style | string | ❌ | 图像风格，默认 `realistic` |
| options.aspect_ratio | string | ❌ | 画幅比例；当前只支持 `16:9`（横屏）和 `9:16`（竖屏），默认 `9:16` |
| options.video_count | integer | ❌ | 只为按分镜顺序的前 `n` 个 shot 生成视频；默认 `12`，其余 shot 在 `shot_video` 中直接回退为静态图 |

**Response 202**
```json
{
  "code": 0,
  "data": {
    "job_id": "job_abc123",
    "status": "queued",
    "created_at": "2024-01-15T10:30:00Z",
    "estimated_seconds": 120
  }
}
```

**行为说明**

- 当前实现会在创建成功后自动启动后台调度
- 前端不需要再依赖手动点击 `Dispatch Once` 才能推进 job
- 当前接口不再接收 `language` 字段；整个生成链路默认按中文内容处理
- `options.aspect_ratio` 若未传，后端会规范化为 `9:16`
- `options.video_count` 若未传，后端会规范化为 `12`

---

### GET /api/v1/jobs/:job_id — 查询任务状态

**Response 200**
```json
{
  "code": 0,
  "data": {
    "job_id": "job_abc123",
    "status": "running",
    "progress": 62,
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:31:30Z",
    "tasks": {
      "total": 9,
      "pending": 1,
      "ready": 1,
      "running": 1,
      "succeeded": 6,
      "failed": 0,
      "cancelled": 0,
      "skipped": 0
    },
    "task_state": {
      "ready_keys": ["image"],
      "running_keys": ["tts"],
      "failed_keys": []
    },
    "runtime_hint": "当前 job 正由后台 runner 自动推进，可继续刷新查看进展。",
    "warnings": [],
    "error": null,
    "result": null
  }
}
```

**status 枚举值**：`queued` | `running` | `cancelling` | `completed` | `failed` | `cancelled`

**tasks 字段说明**：

- 返回当前 job 下 task 状态聚合结果
- 默认不在 `GET /jobs/:id` 中展开完整 task 列表，避免响应过大
- 如后续前端需要任务明细，可新增 `GET /jobs/:id/tasks`

**task_state / runtime_hint 字段说明**：

- `task_state` 返回当前 ready / running / failed 的 task key 快照，方便前端判断工作流停在哪
- `runtime_hint` 是 skeleton 阶段的人类可读提示，用于解释当前是否由后台 runner 自动推进，或为什么现在没有 running task

**progress**：0~100 的整数，表示整体进度百分比

当 status 为 `completed` 时，result 字段填充：
```json
"result": {
  "video_url": "/api/v1/jobs/job_abc123/download",
  "duration": 87.5,
  "file_size": 15728640
}
```

---

### GET /api/v1/jobs/:job_id/tasks — 查询任务明细

用于开发调试或前端查看 DAG 明细，返回当前 job 下所有 task 的状态和关键产物引用。

**Response 200**
```json
{
  "code": 0,
  "data": {
    "job_id": "job_abc123",
    "tasks": [
      {
        "id": 11,
        "key": "segmentation",
        "type": "segmentation",
        "status": "succeeded",
        "resource_key": "local_cpu",
        "depends_on": [],
        "attempt": 1,
        "max_attempts": 1,
        "payload": {
          "article": "..."
        },
        "output_ref": {
          "artifact_path": "jobs/job_abc123/segments.json",
          "segment_count": 12
        },
        "error": null
      }
    ]
  }
}
```

补充语义：

- `output_ref.artifact_path` 始终是相对 `WORKSPACE_DIR` 的路径
- task 处于 `running` 时，`output_ref.progress` 可能暂时出现，结构为 `{phase, message, current, total, unit}`；该字段仅用于运行态展示，task 进入终态后会被清理
- `script` task 额外会返回 `output_ref.segment_artifact_dir`，指向 `jobs/{job_id}/script`；该目录下会逐段写出 `segment_{index}.json`
- `image` task 当前会额外返回 `output_ref.image_count`（segment 级兼容图片数）与 `output_ref.shot_image_count`（shot 级 manifest 条目数）
- `image.output_ref.generated_image_count / fallback_image_count` 当前都是按 `shot_images` 统计，而不是按 segment 摘要图统计
- `shot_video` task 当前会额外返回 `output_ref.clip_count`、`output_ref.generated_video_count`、`output_ref.fallback_image_count`、`output_ref.generation_mode`
- `shot_video` task 当前会额外返回 `output_ref.requested_video_count / selected_video_count`，分别表示请求的前 `n` 个与实际参与图生视频的前 `n` 个
- `shot_video.output_ref.generation_mode` 当前正式取值为 `generated_video`、`image_fallback` 或 `mixed`
- `shot_video.clips[*].status` 当前正式枚举为 `generated_video` 或 `image_fallback`
- `shot_video.clips[*].source_image_path` 当前会稳定保留上游 shot image 路径，供未来真实图生视频接入时追踪输入来源
- `tts` task 当前会额外返回 `output_ref.generation_mode`，用于标记本次走的是 `placeholder` 还是 `sentence_serial`
- `video` task 当前会额外返回 `output_ref.shot_video_artifact_ref`，并继续透传 `output_ref.image_source_type`
- `video.output_ref.duration_seconds` 当前表示视觉拼接总时长；同时会额外返回 `output_ref.narration_duration_seconds` 与 `output_ref.visual_duration_seconds`
- 对 `segmentation / outline / character_sheet / script / tts / character_image / image / shot_video`，该路径现在应指向已经真实落盘的 JSON artifact
- 默认 DAG 里，`script.depends_on = ["segmentation", "outline", "character_sheet"]`
- 默认 DAG 里，`tts.depends_on = ["segmentation"]`
- 默认 DAG 里，`character_image.depends_on = ["character_sheet"]`
- 默认 DAG 里，`image.depends_on = ["script", "character_image"]`
- 默认 DAG 里，`shot_video.depends_on = ["image"]`
- 默认 DAG 里，`video.depends_on = ["tts", "shot_video"]`
- 默认 DAG 里，`image.payload.aspect_ratio / shot_video.payload.aspect_ratio / video.payload.aspect_ratio` 会透传 `options.aspect_ratio`
- 默认 DAG 里，`shot_video.payload.video_count` 会透传 `options.video_count`；当前执行语义是“只对排序后的前 `n` 个 shot 尝试图生视频，其余直接登记为 `image_fallback`”

---

### GET /api/v1/jobs/:job_id/download — 下载视频

返回视频文件流（`video/mp4`），支持 Range 请求。

**Headers**
```
Content-Type: video/mp4
Content-Disposition: attachment; filename="narratio_job_abc123.mp4"
Content-Length: 15728640
Accept-Ranges: bytes
```

若任务未完成，返回：
```json
{ "code": 1003, "message": "任务尚未完成" }
```

---

### DELETE /api/v1/jobs/:job_id — 取消/删除任务

取消进行中的任务，或删除已完成任务的数据。

**Response 200**
```json
{
  "code": 0,
  "data": {
    "cancelled": true,
    "deleted": false,
    "status": "cancelling"
  }
}
```

补充语义：

- `queued` 任务取消后直接变为 `cancelled`
- `running` 任务取消后先返回 `cancelling`，前端继续轮询，直到状态变为 `cancelled`
- 当前版本尚未实现“已完成任务删除产物文件”；因此当前 `deleted` 固定为 `false`

---

### POST /api/v1/jobs/:job_id/dispatch-once — 开发态手动推进一次调度

仅用于 skeleton / 开发调试阶段。每次请求最多推进一个 ready task。

**Response 200**
```json
{
  "code": 0,
  "data": {
    "job_id": "job_abc123",
    "status": "queued",
    "progress": 33,
    "dispatched": true,
    "executed_task_id": 11,
	"executed_task_key": "outline"
  }
}
```

补充语义：

- 若该 job 当前已被后台 runner 持有，本接口返回 `dispatched=false`

---

### GET /api/v1/voices — 获取可用音色列表

**Response 200**
```json
{
  "code": 0,
  "data": {
    "default_voice_id": "male_calm",
    "voices": [
      {
        "id": "male_calm",
        "name": "男_沉稳青年音",
        "reference_audio": "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E6%B2%89%E7%A8%B3%E9%9D%92%E5%B9%B4%E9%9F%B3.MP3",
        "preview_url": "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E6%B2%89%E7%A8%B3%E9%9D%92%E5%B9%B4%E9%9F%B3.MP3"
      },
      {
        "id": "male_strong",
        "name": "男_王明军",
        "reference_audio": "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E7%8E%8B%E6%98%8E%E5%86%9B.MP3",
        "preview_url": "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E7%8E%8B%E6%98%8E%E5%86%9B.MP3"
      }
    ]
  }
}
```

---

### GET /api/v1/health — 健康检查

**Response 200**
```json
{
  "status": "ok",
  "version": "dev",
  "services": {
    "database": "ok",
    "dashscope_text": "configured_but_disabled",
    "dashscope_image": "configured_but_disabled",
    "tts": "not_configured"
  },
  "resources": {
    "local_cpu": 4,
    "llm_text": 2,
    "tts": 3,
    "image_gen": 2,
    "video_gen": 1,
    "video_render": 1
  }
}
```

当前实现说明：

- 当前 health 接口反映的是服务 bootstrap 结果和关键配置是否存在
- 当前 health 接口同时返回运行中实际采用的资源池并发上限，便于联调时核对调度配置
- 当相关 API Key 已配置但 live 开关未打开时，`dashscope_text` / `dashscope_image` 会返回 `configured_but_disabled`
- 当前 health 接口仍不会主动探测 DashScope / TTS 联通性；但 runtime 启动阶段现在会先检查本机 `ffmpeg` 是否可用

当前已实现接口：

- `POST /api/v1/jobs` 已实现
- `GET /api/v1/voices` 已实现，返回内置 narration voice preset 列表
- `GET /api/v1/jobs/:job_id` 已实现，返回 job 状态和 task 聚合统计
- `GET /api/v1/jobs/:job_id/tasks` 已实现，返回 task 明细
- `GET /api/v1/jobs/:job_id/download` 已实现，返回最终视频文件流并支持 Range
- `DELETE /api/v1/jobs/:job_id` 已实现，支持最小取消语义
- `POST /api/v1/jobs/:job_id/dispatch-once` 已实现，仅用于开发态手动推进 task

---

## 前端轮询策略

```
提交任务后：
  - 前 30s：每 3s 轮询一次
  - 30s~2min：每 5s 轮询一次
  - 2min 以上：每 10s 轮询一次
  - 超过 10min 未完成：提示用户可能出现异常
```

不使用 WebSocket（降低复杂度），使用轮询即可满足需求。
