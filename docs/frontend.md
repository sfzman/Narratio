# docs/frontend.md — 前端接入与运行约定

## 目标

本文件定义 Narratio 产品前端的最小接入边界，避免在接入真实后端前把 `frontend/` 继续当成纯展示稿使用。

当前仓库里有两个前端目录：

- `frontend/`：新的产品前端 skeleton，后续真实接入从这里继续
- `frontend-debug/`：旧的调试台，保留做开发辅助，不再作为正式产品 UI 演进目标

## 当前状态

`frontend/` 目前已经具备以下能力：

- React + TypeScript + Vite
- Tailwind CSS v4
- 左侧 job 列表已接入后端 `GET /api/v1/jobs`
- 当前选中 job 的 DAG 画布已接入 `GET /api/v1/jobs/:job_id` 与 `GET /api/v1/jobs/:job_id/tasks`
- 当前选中 job 已按 `docs/api-spec.md` 约定进行前端轮询
- 任务创建弹窗已接入 `POST /api/v1/jobs`
- 任务删除已接入 `DELETE /api/v1/jobs/:job_id`

当前仍然没有完全产品化，主要还缺：

- 节点详情侧栏已接入通用真实 task 信息，但还没有按不同 task 展示专用内容
- 创建弹窗里的 voice/style 选项当前仍来自前端常量，还没有切到 `/api/v1/voices`
- 仍有部分原型期视觉文案，不能把 README / metadata 当成完整产品说明

## 运行约定

Narratio 产品前端是一个普通 Web App，不依赖 AI Studio / Gemini runtime。

本地开发约定：

- 前端：`frontend`，默认 Vite dev server
- 后端：`backend`，默认 `http://localhost:8080`
- 前端只调用 Narratio 后端 HTTP API，不直接调用 DashScope、TTS 或其他外部模型服务

接入过程中若需要前端环境变量，优先使用 Vite 约定：

- `VITE_API_BASE_URL`：后端 API 基地址，例如 `http://localhost:8080/api/v1`

在真正需要前，不要引入更多 frontend-only 配置项。

## 前端与后端契约

前端当前只应依赖以下后端接口：

- `POST /api/v1/jobs`
- `GET /api/v1/jobs/:job_id`
- `GET /api/v1/jobs/:job_id/tasks`
- `GET /api/v1/jobs/:job_id/artifact`
- `GET /api/v1/jobs/:job_id/download`
- `DELETE /api/v1/jobs/:job_id`
- `GET /api/v1/voices`

字段契约以 `docs/api-spec.md` 为准。

工作流画布的节点状态应直接映射后端 `task.status`：

- `pending`
- `ready`
- `running`
- `succeeded`
- `failed`
- `cancelled`
- `skipped`

运行中节点可读取 `task.output_ref.progress` 作为瞬时进度展示；该字段只在 `running` 期间存在，进入终态后会被清理。

## 最小接入拆分

前端接入按以下顺序推进，不跨太多层：

### 第 1 步：去模板化，但不改页面结构

目标：把 `frontend/` 从 AI Studio 展示稿收紧为 Narratio 项目骨架。

最小范围：

- 清理错误 README / 页面标题 / 模板说明
- 保留现有 React Flow、Sidebar、Modal、Inspector 结构
- 不急着引入路由、全局 store、复杂缓存层

### 第 2 步：补最小 API 层

建议新增最小目录：

- `src/lib/api.ts`：fetch 封装、统一解析 `{ code, data, message }`
- `src/lib/jobs.ts`：jobs 相关接口
- `src/lib/voices.ts`：voice presets 接口
- `src/lib/workflow-mapper.ts`：把后端 job/task 响应映射成画布节点数据

要求：

- API 层与 UI 组件分离
- 不在组件里直接散落手写 URL
- 不提前引入与当前需求无关的状态管理库

### 第 3 步：打通“创建任务”

状态：已完成。

最小范围：

- `article`
- `voice_id`
- `image_style`
- `aspect_ratio`
- `video_count`

提交成功后，进入 job 详情画布；失败时展示后端错误信息。

### 第 4 步：打通“任务画布轮询”

状态：已完成。

最小范围：

- 拉取 `GET /api/v1/jobs/:job_id`
- 拉取 `GET /api/v1/jobs/:job_id/tasks`
- 按 `docs/api-spec.md` 的轮询策略刷新
- 节点 summary / progress / error 来自真实 task 数据，而不是本地常量

### 第 5 步：补节点详情与结果消费

状态：进行中。

最小范围：

- 通用信息块已接通：task 基本信息、依赖、尝试次数、错误
- 通用 artifact / payload 摘要已接通
- `video` 节点已补专用结果展示：下载入口、artifact path、时长、文件大小
- `image` 节点已补专用结果展示：manifest path、生成统计、上游 artifact 引用
- `image` 节点已开始消费 artifact API，可预览 shot 列表与单个 shot 的结构化内容
- `shot_video` 节点已补专用结果展示：clip 统计、fallback 统计、generation mode、artifact path
- `shot_video` 节点已开始消费 artifact API，可预览 clip 列表与单个 clip 的结构化内容
- `tts` 节点已补专用结果展示：generation mode、segment 统计、artifact path、segmentation ref
- `tts` 节点已开始消费 artifact API，可预览 audio segments、subtitle items 与总时长
- `script` 节点已补专用结果展示：segment 统计、segment artifact dir、上游 artifact 引用
- 后端已定义正式 artifact 读取接口，后续可基于它把 `script/image/shot_video/tts` 从“摘要展示”升级为“真实内容预览”
- 当前尚未补专用展示的主要是 `segmentation / outline / character_sheet / character_image`

## 当前不急着做的事

以下内容先不要抢跑：

- 单节点编辑 / 单节点重试
- WebSocket 推送
- 多 job 复杂工作台
- 权限体系 / 登录态
- 前端直接预览 workspace 文件内容
- 为 debug console 和 product frontend 做共享组件抽象

## 接入时的判断原则

- 若后端 API 契约不清楚，先修 `docs/api-spec.md`
- 若前端显示语义和后端状态语义冲突，优先以后端 task/job 状态为准
- 若只是为了“好看”而需要引入复杂状态管理或额外架构，先不做
