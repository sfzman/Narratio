# AGENTS.md — Narratio

## 项目概述

**Narratio** 是一个将文章自动转换为朗诵短片的 Web App。
用户输入文章，系统自动生成带配音、配图、字幕的短视频。

- 后端：Go (Gin framework)
- 前端：React + TypeScript (Vite)
- 核心 AI 服务：Qwen 系列（文本/图像）、自部署 TTS
- 视频合成：FFmpeg

## 文档地图（先读这里，再读对应文档）

| 你要做的事 | 去读 |
|---|---|
| 了解完整流水线流程 | `docs/pipeline.md` |
| 调用外部 AI 服务 | `docs/integrations.md` |
| 新增或修改 API 接口 | `docs/api-spec.md` |
| 实现任务生命周期 / Task DAG / 调度 | `docs/job-lifecycle.md` |
| 编写 Go 代码 | `docs/coding-standards.md` |
| 编写测试 | `docs/testing.md` |
| 部署 / 运行项目 | `docs/deployment.md` |

## 分层架构（严格执行，不得违反）

```
model → store → pipeline/* → scheduler → app/jobs → handler
```

- `model/`：纯数据结构，不得 import 其他内部包
- `store/`：只依赖 `model/`，负责 job / task 状态持久化
- `pipeline/*`：每个子模块只依赖 `model/`，实现某类 task 的 executor，不得互相调用
- `scheduler/`：负责 task 依赖解析、资源限流、任务派发，不实现具体 AI 调用
- `app/jobs/`：负责任务创建、workflow 构建、取消、运行态管理，只调用 `store` 和 `scheduler`
- `handler/`：只调用 `app/jobs` 和 `store`，不得直接调用 pipeline 子模块、scheduler 或外部 API

## 禁止事项

- ❌ handler 层直接调用外部 AI API
- ❌ handler 层直接启动后台 goroutine
- ❌ pipeline 子模块之间互相 import
- ❌ scheduler 层实现具体 AI / FFmpeg 调用
- ❌ 在 goroutine 中直接 panic（必须 recover + 写入 job 错误状态）
- ❌ 硬编码 API Key、URL、超时时间（统一在 config 层读取）
- ❌ 修改已有接口的 response 结构（向后兼容，只新增字段）
- ❌ 单个函数超过 80 行（拆分）

## Agent 工作规范

1. **先读文档再写代码**：每次任务开始，先确认相关 docs/ 文件已读
2. **步子要小**：优先补一层文档和一层最小骨架，不跨多层同时开工
3. **测试先行**：每个模块必须有对应测试，外部 API 调用一律使用 mock
4. **遇到不确定的设计决策**：写入 `docs/design-decisions.md`，标注 `[OPEN]`，不要自行假设
5. **先补契约再写实现**：新增用户可选项时，必须同时更新 API、`model.JobSpec`、对应 pipeline 输入
6. **发现文档与代码不一致**：优先修复文档，并在 PR 描述中说明

## 当前开发状态

> 本文件由 agent 维护，每次 sprint 结束后更新。

- [x] 项目骨架初始化
- [x] model/job + model/task
- [x] store/job + store/task
- [x] scheduler 最小骨架
- [ ] pipeline/script executor
- [ ] pipeline/tts executor
- [ ] pipeline/image executor
- [ ] pipeline/video executor
- [x] app/jobs + workflow builder
- [ ] handler 层
- [ ] React 前端基础页面
- [ ] 端到端集成测试
