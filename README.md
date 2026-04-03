# Narratio

将文章自动转换为朗诵短片的 Web App。

用户输入文章后，系统异步完成：

1. 创建一个 `job`
2. 将 `job` 拆成多个有依赖关系的 `task`
3. 由 scheduler 按依赖关系和资源上限调度执行
4. 汇总产物并生成最终视频

当前仓库的主要目标不是直接堆代码，而是先把从 POC 到可维护 Web App 的工程边界定义清楚。

优先阅读：

- [AGENTS.md](/Users/fangzhou/Workspace/qufafa/Narratio/AGENTS.md)
- [docs/job-lifecycle.md](/Users/fangzhou/Workspace/qufafa/Narratio/docs/job-lifecycle.md)
- [docs/pipeline.md](/Users/fangzhou/Workspace/qufafa/Narratio/docs/pipeline.md)
- [docs/api-spec.md](/Users/fangzhou/Workspace/qufafa/Narratio/docs/api-spec.md)

当前技术方向：

- Backend: Go + Gin
- Frontend: React + TypeScript + Vite
- AI: Qwen 文本/图像 + 自部署 TTS
- Render: FFmpeg
