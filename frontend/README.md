# Narratio Frontend

`frontend/` 是 Narratio 的产品前端 skeleton，技术栈为 React + TypeScript + Vite + Tailwind CSS + React Flow。

当前状态：

- 已有 DAG 画布、任务创建弹窗、节点详情侧栏的静态 UI 骨架
- 尚未接入真实 Narratio 后端 API
- 不依赖 AI Studio / Gemini runtime

本地运行：

```bash
cd frontend
npm install
npm run dev
```

默认与本仓库 `backend/` 一起联调，后端接口契约见：

- `docs/frontend.md`
- `docs/api-spec.md`
- `docs/deployment.md`
