# docs/coding-standards.md — Go 编码规范

## 目录结构

```
backend/
  cmd/
    server/
      main.go          # 入口，只负责初始化和启动
  internal/
    config/            # 配置加载（从环境变量读取）
    model/             # 数据结构定义（Job, Segment, etc.）
    store/             # 任务状态存储接口与 SQL 实现
      sql/             # SQL store（SQLite / MySQL）
      migrations/      # schema migrations
    pipeline/
      script/          # LLM 类 task executor
      tts/             # TTS task executor
      image/           # 图像生成 task executor
      video/           # 视频合成 task executor
    scheduler/         # Task DAG 调度与资源限流
    app/
      jobs/            # 任务应用层：创建 job、构建 workflow、取消任务
    handler/           # HTTP handlers (Gin)
    middleware/        # Gin 中间件（日志、CORS、recover）
    util/              # 通用工具函数
  pkg/                 # 可对外暴露的包（目前暂无）
```

## 命名规范

- 包名：小写单词，不用下划线（`pipeline`，不用 `pipe_line`）
- 接口名：动词/名词 + `er`（`Synthesizer`、`Generator`）
- 错误变量：`Err` 前缀（`ErrJobNotFound`）
- 导出常量：MixedCaps（`MaxConcurrentTTS`）
- 非导出常量：`camelCase`

## 错误处理

**必须使用 `fmt.Errorf` 包装错误，保留调用链**

```go
// ✅ 正确
if err != nil {
    return fmt.Errorf("tts.synthesize segment %d: %w", seg.Index, err)
}

// ❌ 错误
if err != nil {
    return err
}
```

**不得在 goroutine 中 panic**，必须用 recover：

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Error("recovered panic", "error", r)
            markJobFailed(job, fmt.Errorf("panic: %v", r))
        }
    }()
    // ... 业务逻辑
}()
```

## 接口定义（每类 executor 必须实现）

```go
type Executor interface {
    Type() model.TaskType
    Execute(ctx context.Context, task model.Task, job model.Job) (TaskResult, error)
}
```

好处：方便 scheduler 按 task type 派发，也便于测试时 mock。

## HTTP Handler 规范

使用 Gin。
JSON API 统一通过 `c.JSON` 返回；文件下载和 Range 流式响应允许直接写 `c.Writer`，但必须封装为 helper：

```go
// ✅ 成功响应
c.JSON(http.StatusOK, gin.H{
    "code": 0,
    "data": result,
})

// ✅ 错误响应
c.JSON(http.StatusBadRequest, gin.H{
    "code":       1001,
    "message":    "文章内容不能为空",
    "request_id": c.GetString("request_id"),
})
return // handler 里错误后必须 return

// ✅ 文件下载响应
streamer.ServeFile(c, resultPath, "video/mp4")
```

## 日志规范

使用结构化日志（`log/slog` 标准库）：

```go
slog.Info("tts segment done",
    "job_id", job.ID,
    "segment_index", seg.Index,
    "duration_ms", elapsed.Milliseconds(),
)

slog.Error("qwen image failed",
    "job_id", job.ID,
    "segment_index", seg.Index,
    "error", err,
)
```

**不使用 `fmt.Println` 打日志。**

## 并发控制

系统级并发由 `scheduler` 按 `ResourceKey` 控制，不允许散落在 handler 或 app 层：

```go
type ResourceManager interface {
    Acquire(ctx context.Context, key model.ResourceKey) error
    Release(key model.ResourceKey)
}
```

说明：

- 系统级限流放在 `scheduler`
- executor 内部若要做子任务并发，例如 tts 按 segment 并行，仍需使用局部 semaphore
- 不允许在多个 executor 中重复维护同一外部资源的全局并发上限

对 retry / polling 逻辑：

- 生产代码不得直接调用 `time.Sleep`
- 必须注入 `Clock`、`TickerFactory` 或 `Backoff` 接口，方便单元测试

## 配置加载

所有配置从环境变量读取，在 `internal/config/config.go` 统一定义：

```go
type Config struct {
    Port               string
    DatabaseDriver     string
    DatabaseDSN        string
    QwenTextAPIBaseURL string
    QwenTextModel      string
    QwenAPIKey         string
    TTSBaseURL         string
    TTSAPIKey          string
    WorkspaceDir       string
}

func Load() (*Config, error) {
    // 使用 os.Getenv，缺少必填项时返回 error
}
```

数据库约束：

- `DatabaseDriver` 只允许 `sqlite` 或 `mysql`
- 本地开发默认使用 SQLite
- 生产环境默认使用 MySQL
- 业务层不得自行拼接 DSN，统一由 `config` 负责

## Store 设计

- `store` 层只暴露接口，不向上泄漏具体数据库实现
- SQL 实现必须显式处理事务边界，尤其是 job 创建与 task DAG 创建
- 能用单事务完成的 job/task 初始化，不允许拆成多次无保护写入
- `app/jobs` 初始化 workflow 时，优先调用类似 `InitializeJob(job, tasks)` 的原子接口，不要在上层手写事务拼装
- 查询接口优先返回领域对象，不把 `sql.Rows` 或驱动类型暴露到业务层

## 单函数长度限制

**单个函数不超过 80 行**。超过时拆分为更小的私有函数。

## 禁止使用

- `init()` 函数（除非绝对必要）
- 全局可变状态（用依赖注入代替）
- `time.Sleep` 在生产代码中（用可注入 backoff/ticker 代替）
- `ioutil` 包（已废弃，使用 `os` 和 `io`）
