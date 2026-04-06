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
  "language": "zh",
  "options": {
    "voice_id": "default",
    "image_style": "realistic"
  }
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| article | string | ✅ | 文章内容，1~10000 字 |
| language | string | ❌ | 语言，默认 `zh` |
| options.voice_id | string | ❌ | TTS 音色 ID，默认 `default` |
| options.image_style | string | ❌ | 图像风格，默认 `realistic` |

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

---

### GET /api/v1/jobs/:job_id — 查询任务状态

**Response 200**
```json
{
  "code": 0,
  "data": {
    "job_id": "job_abc123",
    "status": "running",
    "progress": 60,
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:31:30Z",
    "tasks": {
      "total": 6,
      "pending": 1,
      "ready": 0,
      "running": 1,
      "succeeded": 4,
      "failed": 0,
      "cancelled": 0,
      "skipped": 0
    },
    "task_state": {
      "ready_keys": ["character_sheet"],
      "running_keys": [],
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
        "key": "outline",
        "type": "outline",
        "status": "succeeded",
        "resource_key": "llm_text",
        "depends_on": [],
        "attempt": 1,
        "max_attempts": 1,
        "payload": {
          "article": "..."
        },
        "output_ref": {
          "artifact_path": "jobs/job_abc123/outline.json"
        },
        "error": null
      }
    ]
  }
}
```

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
- 已完成任务可复用该接口删除产物文件；此时 `cancelled=false`，`deleted=true`

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
    "voices": [
      { "id": "default", "name": "标准女声", "preview_url": "/..." },
      { "id": "male_1", "name": "标准男声", "preview_url": "/..." }
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
    "dashscope_text": "configured",
    "tts": "not_configured"
  }
}
```

当前实现说明：

- 当前 health 接口反映的是服务 bootstrap 结果和关键配置是否存在
- 还没有对 DashScope、TTS、FFmpeg 做真实联通性探测

当前实现说明：

- `POST /api/v1/jobs` 已实现
- `GET /api/v1/jobs/:job_id` 已实现，返回 job 状态和 task 聚合统计
- `GET /api/v1/jobs/:job_id/tasks` 已实现，返回 task 明细
- `POST /api/v1/jobs/:job_id/dispatch-once` 已实现，仅用于开发态手动推进 task
- `DELETE /api/v1/jobs/:job_id`、下载接口、音色列表接口尚未实现

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
