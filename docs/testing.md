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
- [x] 检查 FFmpeg 命令参数是否正确构建
- [x] FFmpeg 执行超时当前沿用 scheduler task context，`video` 额外支持独立 `VIDEO_RENDER_TIMEOUT_SECONDS` 配置
- [x] 输出文件不存在或空文件时返回 error
- 注：使用 `exec.Command` 的函数通过依赖注入替换（见下方）

### handler
- [ ] POST /jobs 参数校验（article 为空、超长）
- [x] GET /api/v1/voices 返回内置 narration voice preset 列表
- [ ] GET /jobs/:id 任务不存在返回 1002
- [x] GET /jobs/:id/download 任务未完成返回 1003
- [x] DELETE /jobs/:id 对 queued 任务直接返回 cancelled
- [ ] DELETE /jobs/:id 对 running 任务先返回 cancelling，再轮询到 cancelled

### store
- [ ] 创建 job 后可正确读取
- [ ] 创建 task 后能按 job_id 查询
- [ ] SQLite 下 job + task DAG 初始化事务行为正确
- [ ] task 初始化失败时，job 与已插入 task 会整体回滚
- [ ] 并发更新 task 状态不会破坏数据一致性
- [ ] 相同测试集在 SQLite 和 MySQL 方言下语义一致

### app/jobs
- [ ] CreateJob 会规范化默认选项（含 `aspect_ratio=9:16`、`video_count=12`）并写入 `JobSpec`
- [ ] CreateJob 会同步创建 workflow 对应的 task DAG
- [x] 取消 queued job 会将未开始 task 标记为 cancelled
- [x] background runner 当前支持跨 job 并发 worker，不同 job 不再被单个长 job 串行阻塞

### scheduler
- [ ] 所有依赖满足后，task 从 pending 进入 ready
- [ ] 共享同一 `ResourceKey` 的 task 会共同受并发上限约束
- [x] 无依赖冲突且资源配额允许的 task 可在同一轮 `DispatchOnce` 中并行启动
- [x] executor 运行中通过 `model.ReportTaskProgress(...)` 上报的进度会被持久化到 `task.output_ref.progress`，并在 task 进入终态后清理
- [x] 上游 task 失败后，下游 task 会按 fail-fast 语义递归标记为 `skipped`
- [ ] 运行中 task 的 panic 被 recover 后状态置为 failed

### executor
- [ ] script executor 正确消费 segmentation / outline / character_sheet 输出，按 segment 调用文本生成，并按 segment 长度产出动态 shot 数的 script 结构
- [x] script executor 会逐段落盘 `script/segment_{index}.json`，便于运行中观察和中断后续跑
- [x] script executor 在同一 job 重试时，会优先复用已存在且可解析的 segment artifact
- [x] script executor 会为每个 shot 产出 `involved_characters / image_to_image_prompt / text_to_image_prompt`，并在出现主要人物时确保 prompt 带上准确人物名
- [x] tts executor 正确消费 segmentation 输出并落盘 tts manifest / 占位 WAV
- [x] tts executor 在注入真实 client 时，会按 `。` 切句并逐句串行调用，再合并成 segment 级 WAV
- [x] tts executor 会在每个 segment 完成后立即写出对应 WAV，并增量刷新 `tts_manifest.json`
- [x] character_image executor 正确消费 character_sheet 输出并落盘 artifact / fallback JPG
- [x] character_image executor 在注入真实 image client 时，会写出真实参考图并回填最小追踪字段
- [x] image executor 正确消费 script / character_image artifact，并按 shot 逐张出图；优先使用 matched 角色参考图，失败后用最近一次成功图补位
- [x] shot_video executor 正确消费 `image.shot_images` 并落盘 `shot_videos/manifest.json`
- [x] shot_video executor 在未注入 live client 时，会把每个 shot 登记成 `image_fallback` clip，供最终 video 阶段选择回退
- [x] shot_video executor 当前 manifest 已预留 `status / source_image_path / duration_seconds / generation_request_id / generation_model / source_video_url` 等真实图生视频字段
- [x] shot_video / video 当前会校验 `status` 只能是正式枚举 `generated_video / image_fallback`
- [x] shot_video executor 当前已支持注入 mock client，并在成功时写出 `generated_video` clip、失败时回退到 `image_fallback`
- [x] shot_video executor 当前支持 `video_count`，只对排序后的前 `n` 个 shot 调图生视频，其余 shot 直接登记为 `image_fallback`
- [x] shot_video executor 当前会按 shot 上报运行中 progress，并在写 manifest 前切到 `writing_artifact`
- [x] shot_video HTTP client 当前已覆盖 submit -> poll -> download 的纯 mock 测试，并对齐原 gradio app 的异步任务流
- [x] 文档层已明确：真实 `shot_video` 联调的前置条件应先确认 `character_image` / `image` 已跑出稳定 `shot_images`
- [x] image executor 优先从 script 的 shot 级 prompt 消费出图输入，不再依赖 summary/script/text 兼容字段
- [x] image executor 当前优先消费 `image_to_image_prompt / text_to_image_prompt`，并把它们作为单图选 shot 的基础语义
- [x] image executor 不再兼容旧的 shot 级 `prompt` 字段，缺少这两个正式字段的 shot 会被直接忽略
- [x] image executor 在 manifest 中记录 prompt source trace，便于联调时确认每段图片实际消费了哪些 shot 文本
- [x] image executor 在生成失败时回退到 fallback 图片，并真实写出本地占位图文件
- [x] image executor 单图模式会从 segment 的 shots 中收紧挑选少量代表性 shot，避免把整段所有镜头都拼进单张图片 prompt
- [x] image executor 当前会额外输出 `shot_images` manifest，并把它作为真实出图主路径；segment 级 `images` 只保留兼容摘要
- [x] 文档层已明确：`shot_video` 当前真正稳定消费的是 `image_manifest.json.shot_images[*].segment_index / shot_index / file_path / prompt`，`source_image_url` 只是可选优化字段
- [x] 已补一条 fixture 驱动的 `live_image_reference_smoke`，专门验证 `image_to_image` 请求会带上人物参考图，并把人物名替换成 `图1中的人物`
- [x] 已补一条 fixture 驱动的 `live_shot_video_smoke`，专门验证至少一个 `generated_video` clip 能真实落盘，并回填 `video_path / source_image_path / source_video_url`
- [x] video executor 已改为消费 `shot_video` artifact，并在 workspace 模式下校验 clip coverage 与 `video_path` / `image_path` 依赖文件存在性
- [x] video executor 当前会校验 `shot_video.clips` 的非零时长、排序与去重语义，并把 `duration_seconds` 汇总为视觉拼接总时长
- [x] 已补一条 fixture 驱动的 `live_video_render_smoke`，专门验证 `tts + shot_video` 能经 FFmpeg 真实落盘 `output/final.mp4`
- [x] video executor 已覆盖 `ffprobe` 失败、最终 mux 失败、最终输出缺失/空文件等关键失败分支
- [x] video executor 当前会按关键阶段上报运行中 progress，至少覆盖依赖校验、音频拼接、segment 渲染、分段拼接、最终 mux 和产物整理
- [ ] video executor 校验 `tts.segment_count` 与 `shot_video.clips` 的时长/排序/拼接语义，并在真实 FFmpeg 模式下返回明确错误

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
