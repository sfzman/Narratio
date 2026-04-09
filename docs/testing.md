# docs/testing.md — 测试策略

## 原则

- 每个 scheduler / store / executor 子模块必须有单元测试，覆盖率 > 70%
- 外部 API 调用（DashScope 文本、DashScope 图像、TTS）一律使用 mock HTTP server
- FFmpeg 调用使用 fixture 文件（不实际执行 ffmpeg）
- 不允许测试依赖外网或本地服务
- retry / polling 逻辑必须可注入时钟，测试中不得真实等待

## 测试文件位置

```
internal/pipeline/script/
  script.go
  script_test.go       # 单元测试
  prompt.go
  client.go
  client_test.go       # HTTP client 测试（mock server）
```

## Mock HTTP Server 写法

使用 `net/http/httptest`：

```go
func TestScriptRun(t *testing.T) {
    // 创建 mock DashScope 文本 API server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 验证请求
        assert.Equal(t, "POST", r.Method)
        assert.Equal(t, "/v1/messages", r.URL.Path)

        // 返回 mock 响应
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]any{
            "content": []map[string]any{
                {"type": "text", "text": mockScriptJSON},
            },
        })
    }))
    defer server.Close()

    // 用 mock server URL 初始化 client
    client := NewClient(server.URL, "test-key")
    runner := New(client)

    output, err := runner.Run(context.Background(), ScriptInput{
        ArticleText: "测试文章内容",
    })

    assert.NoError(t, err)
    assert.Len(t, output.Segments, 3)
    assert.NotEmpty(t, output.Segments[0].Script)
}
```

## 必须覆盖的测试场景

### pipeline/script
- [ ] 正常请求，正确解析 segments
- [ ] DashScope 文本接口返回 500，触发重试
- [ ] DashScope 文本接口返回 429，触发退避重试
- [ ] 返回非法 JSON，达到重试上限后返回 error
- [ ] 文章超过 10000 字，自动截断

### pipeline/tts
- [ ] 正常请求，音频文件写入成功
- [ ] 并发 3 个 segment 同时合成
- [ ] TTS 服务超时（60s），返回 error
- [ ] 返回空文件（size=0），视为失败
- [ ] 重试 3 次后仍失败

### pipeline/image
- [ ] 正常流程：提交 → 轮询 → 下载图片
- [ ] task_status == FAILED，跳过该段，返回 warning
- [ ] 轮询超时（120s），跳过并降级为纯色背景
- [ ] 并发控制：同时只有 2 个请求

### pipeline/video
- [ ] 检查 FFmpeg 命令参数是否正确构建
- [ ] FFmpeg 执行超时（300s）
- [ ] 输出文件不存在时返回 error
- 注：使用 `exec.Command` 的函数通过依赖注入替换（见下方）

### handler
- [ ] POST /jobs 参数校验（article 为空、超长）
- [ ] GET /jobs/:id 任务不存在返回 1002
- [ ] GET /jobs/:id/download 任务未完成返回 1003
- [ ] DELETE /jobs/:id 对 queued 任务直接返回 cancelled
- [ ] DELETE /jobs/:id 对 running 任务先返回 cancelling，再轮询到 cancelled

### store
- [ ] 创建 job 后可正确读取
- [ ] 创建 task 后能按 job_id 查询
- [ ] SQLite 下 job + task DAG 初始化事务行为正确
- [ ] task 初始化失败时，job 与已插入 task 会整体回滚
- [ ] 并发更新 task 状态不会破坏数据一致性
- [ ] 相同测试集在 SQLite 和 MySQL 方言下语义一致

### app/jobs
- [ ] CreateJob 会规范化默认选项并写入 `JobSpec`
- [ ] CreateJob 会同步创建 workflow 对应的 task DAG
- [ ] 取消 queued job 会将未开始 task 标记为 cancelled

### scheduler
- [ ] 所有依赖满足后，task 从 pending 进入 ready
- [ ] 共享同一 `ResourceKey` 的 task 会共同受并发上限约束
- [ ] 无依赖冲突的 task 可并行启动
- [ ] 上游 task 失败后，下游 task 正确变为 skipped 或保持 pending
- [ ] 运行中 task 的 panic 被 recover 后状态置为 failed

### executor
- [ ] script executor 正确消费 segmentation / outline / character_sheet 输出，按 segment 调用文本生成，并产出每段 10 个 shot 的 script 结构
- [x] script executor 会逐段落盘 `script/segment_{index}.json`，便于运行中观察和中断后续跑
- [x] script executor 在同一 job 重试时，会优先复用已存在且可解析的 segment artifact
- [x] script executor 会为每个 shot 产出 `involved_characters / image_to_image_prompt / text_to_image_prompt`，并在出现主要人物时确保 prompt 带上准确人物名
- [ ] tts executor 正确消费 segmentation 输出并落盘 tts manifest / subtitles.srt / 占位 WAV
- [x] character_image executor 正确消费 character_sheet 输出并落盘 artifact / fallback JPG
- [x] character_image executor 在注入真实 image client 时，会写出真实参考图并回填最小追踪字段
- [ ] image executor 正确消费 script / character_image artifact，并按“优先 matched、否则 candidates”把角色参考 prompt 拼进 manifest
- [x] image executor 优先从 script 的 shot 级 prompt 消费出图输入，不再依赖 summary/script/text 兼容字段
- [x] image executor 当前优先消费 `image_to_image_prompt / text_to_image_prompt`，并把它们作为单图选 shot 的基础语义
- [x] image executor 在 manifest 中记录 prompt source trace，便于联调时确认每段图片实际消费了哪些 shot 文本
- [x] image executor 在生成失败时回退到 fallback 图片，并真实写出本地占位图文件
- [x] image executor 单图模式会从 10 个 shots 中收紧挑选少量代表性 shot，避免把整段 10 个镜头全部拼进单张图片 prompt
- [ ] video executor 校验 `tts.segment_count` 与 `audio_segment_paths` / image 数量对齐；在 workspace 模式下再校验 tts/image artifact、WAV / SRT / JPG 依赖文件存在性，并在输入缺失时返回明确错误

## FFmpeg 测试策略

将 `exec.Command` 封装为可注入的接口：

```go
type CommandRunner interface {
    Run(name string, args ...string) ([]byte, error)
}

// 生产实现
type RealRunner struct{}
func (r *RealRunner) Run(name string, args ...string) ([]byte, error) {
    return exec.Command(name, args...).CombinedOutput()
}

// 测试 mock
type MockRunner struct {
    ShouldFail bool
    Output     []byte
}
func (m *MockRunner) Run(name string, args ...string) ([]byte, error) {
    if m.ShouldFail {
        return nil, errors.New("ffmpeg: exit status 1")
    }
    // 创建一个假的输出文件
    return m.Output, nil
}
```

## 运行测试

```bash
# 运行所有测试
go test ./...

# 带覆盖率
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# 运行特定模块
go test ./internal/pipeline/script/...

# 详细输出
go test -v ./internal/pipeline/tts/...
```

## CI 要求

每次 PR 必须通过：
1. `go test ./...`（所有测试通过）
2. `go vet ./...`（无静态检查问题）
3. `golangci-lint run`（lint 检查）
4. 覆盖率不低于上次 PR（不允许覆盖率下降）
