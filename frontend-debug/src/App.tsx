import { useEffect, useMemo, useRef, useState } from "react";

type CreateJobRequest = {
  article: string;
  options: {
    voice_id: string;
    image_style: string;
    aspect_ratio: "16:9" | "9:16";
    video_count: number;
  };
};

type ApiEnvelope<T> = {
  code: number;
  data: T;
  message?: string;
};

type JobResponse = {
  job_id: string;
  status: string;
  created_at: string;
  estimated_seconds?: number;
};

type JobStatusResponse = {
  job_id: string;
  status: string;
  progress: number;
  created_at: string;
  updated_at: string;
  tasks: Record<string, number>;
  task_state?: {
    ready_keys: string[];
    running_keys: string[];
    failed_keys: string[];
  };
  runtime_hint?: string;
  warnings: string[];
  error: { code: string; message: string } | null;
  result: { video_url: string; duration: number; file_size: number } | null;
};

type RawTaskDetail = {
  id: number;
  key: string;
  type: string;
  status: string;
  resource_key: string;
  depends_on: string[] | null;
  attempt: number;
  max_attempts: number;
  payload: Record<string, unknown> | null;
  output_ref: Record<string, unknown> | null;
  error: { code: string; message: string } | null;
  created_at: string;
  updated_at: string;
};

type TaskListResponse = {
  job_id: string;
  tasks: RawTaskDetail[];
};

type TaskDetail = {
  id: number;
  key: string;
  type: string;
  status: string;
  resource_key: string;
  depends_on: string[];
  attempt: number;
  max_attempts: number;
  payload: Record<string, unknown>;
  output_ref: Record<string, unknown>;
  error: { code: string; message: string } | null;
  created_at: string;
  updated_at: string;
};

type DispatchResponse = {
  job_id: string;
  status: string;
  progress: number;
  dispatched: boolean;
  executed_task_id: number;
  executed_task_key: string;
};

type VoicePreset = {
  id: string;
  name: string;
  reference_audio?: string;
  preview_url?: string;
};

type VoiceListResponse = {
  default_voice_id?: string;
  voices: VoicePreset[];
};

type LogEntry = {
  id: string;
  tone: "info" | "success" | "error";
  message: string;
  at: string;
};

type WorkflowNodeLayout = {
  task: TaskDetail;
  layer: number;
  row: number;
  x: number;
  y: number;
};

type WorkflowEdgeLayout = {
  from: WorkflowNodeLayout;
  to: WorkflowNodeLayout;
  tone: "idle" | "running" | "ready" | "success" | "failed";
};

type WorkflowGraph = {
  nodes: WorkflowNodeLayout[];
  edges: WorkflowEdgeLayout[];
  width: number;
  height: number;
};

const fallbackVoicePresets: VoicePreset[] = [
  {
    id: "male_calm",
    name: "男_沉稳青年音",
    reference_audio:
      "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E6%B2%89%E7%A8%B3%E9%9D%92%E5%B9%B4%E9%9F%B3.MP3",
  },
  {
    id: "male_strong",
    name: "男_王明军",
    reference_audio:
      "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E7%8E%8B%E6%98%8E%E5%86%9B.MP3",
  },
  {
    id: "female_explainer",
    name: "女_解说小美",
    reference_audio:
      "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E5%A5%B3_%E8%A7%A3%E8%AF%B4%E5%B0%8F%E7%BE%8E.MP3",
  },
  {
    id: "female_documentary",
    name: "女_专题片配音",
    reference_audio:
      "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E5%A5%B3_%E4%B8%93%E9%A2%98%E7%89%87%E9%85%8D%E9%9F%B3.MP3",
  },
  {
    id: "boy",
    name: "正太",
    reference_audio:
      "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E6%AD%A3%E5%A4%AA.wav",
  },
];

const defaultVoiceID = "male_calm";
const imageStylePresets = [
  { id: "realistic", name: "写实风格" },
  { id: "现代工笔人物画风", name: "现代工笔人物画风" },
] as const;

const defaultRequest: CreateJobRequest = {
  article:
    "暮色落进旧城的屋檐，巷口卖糖画的摊子还亮着一盏小灯。少年提着书箱，在雨后的石板路上慢慢往家走。",
  options: {
    voice_id: defaultVoiceID,
    image_style: "realistic",
    aspect_ratio: "9:16",
    video_count: 12,
  },
};

const apiBaseUrl = (
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1"
).replace(/\/$/, "");

const taskOrder: Record<string, number> = {
  segmentation: 1,
  outline: 2,
  character_sheet: 3,
  script: 4,
  tts: 5,
  character_image: 6,
  image: 7,
  shot_video: 8,
  video: 9,
};

const workflowNodeWidth = 250;
const workflowNodeHeight = 196;
const workflowColumnGap = 84;
const workflowRowGap = 28;
const workflowCanvasPadding = 36;

function App() {
  const [request, setRequest] = useState<CreateJobRequest>(defaultRequest);
  const [jobId, setJobId] = useState("");
  const [job, setJob] = useState<JobStatusResponse | null>(null);
  const [tasks, setTasks] = useState<TaskDetail[]>([]);
  const [health, setHealth] = useState<Record<string, string>>({});
  const [voicePresets, setVoicePresets] = useState<VoicePreset[]>(fallbackVoicePresets);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [autoPollingEnabled, setAutoPollingEnabled] = useState(true);
  const [autoPollingActive, setAutoPollingActive] = useState(false);
  const lastTerminalStatusRef = useRef<string | null>(null);
  const sortedTasks = [...tasks].sort(compareTaskDetail);
  const workflowGraph = useMemo(() => buildWorkflowGraph(sortedTasks), [sortedTasks]);
  const workflowSpotlight = buildWorkflowSpotlight(job);
  const selectedVoicePreset =
    voicePresets.find((preset) => preset.id === request.options.voice_id) ?? null;

  useEffect(() => {
    void refreshVoices();
    void refreshHealth();
  }, []);

  useEffect(() => {
    if (!jobId || !autoPollingEnabled) {
      setAutoPollingActive(false);
      return;
    }

    if (job && isTerminalStatus(job.status)) {
      setAutoPollingActive(false);
      return;
    }

    let cancelled = false;
    const poll = async () => {
      if (cancelled) {
        return;
      }

      setAutoPollingActive(true);
      await refreshJob(jobId, { silent: true, syncTasks: true });
    };

    void poll();
    const timer = window.setInterval(() => {
      void poll();
    }, 1500);

    return () => {
      cancelled = true;
      window.clearInterval(timer);
      setAutoPollingActive(false);
    };
  }, [jobId, autoPollingEnabled, job?.status]);

  useEffect(() => {
    if (!job || !isTerminalStatus(job.status)) {
      lastTerminalStatusRef.current = null;
      return;
    }
    if (lastTerminalStatusRef.current === job.status) {
      return;
    }

    lastTerminalStatusRef.current = job.status;
    appendLog("success", `job ${job.job_id} 已进入终态：${job.status}`);
  }, [job]);

  function appendLog(tone: LogEntry["tone"], message: string) {
    const timestamp = new Date().toLocaleTimeString("zh-CN", {
      hour12: false,
    });

    setLogs((current) => [
      {
        id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
        tone,
        message,
        at: timestamp,
      },
      ...current,
    ]);
  }

  async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
    const response = await fetch(`${apiBaseUrl}${path}`, {
      headers: {
        "Content-Type": "application/json",
      },
      ...init,
    });

    const data = (await response.json()) as ApiEnvelope<T>;
    if (!response.ok || data.code !== 0) {
      throw new Error(data.message ?? `Request failed: ${response.status}`);
    }

    return data.data;
  }

  async function refreshHealth() {
    try {
      const response = await fetch(`${apiBaseUrl}/health`);
      const data = (await response.json()) as {
        services?: Record<string, string>;
      };
      setHealth(data.services ?? {});
    } catch (error) {
      appendLog("error", `健康检查失败：${formatError(error)}`);
    }
  }

  async function refreshVoices() {
    try {
      const data = await requestJSON<VoiceListResponse>("/voices");
      if (data.voices.length > 0) {
        setVoicePresets(data.voices);
      }

      const nextDefaultVoiceID = data.default_voice_id ?? defaultVoiceID;
      setRequest((current) => ({
        ...current,
        options: {
          ...current.options,
          voice_id: current.options.voice_id || nextDefaultVoiceID,
        },
      }));
    } catch {
      setVoicePresets(fallbackVoicePresets);
    }
  }

  async function createJob() {
    setBusy("create");
    try {
      const data = await requestJSON<JobResponse>("/jobs", {
        method: "POST",
        body: JSON.stringify(request),
      });
      setJobId(data.job_id);
      setTasks([]);
      setJob(null);
      appendLog("success", `已创建 job ${data.job_id}`);
      await refreshJob(data.job_id, { syncTasks: true });
    } catch (error) {
      appendLog("error", `创建任务失败：${formatError(error)}`);
    } finally {
      setBusy(null);
    }
  }

  async function refreshJob(
    id = jobId,
    options?: { silent?: boolean; syncTasks?: boolean },
  ) {
    if (!id) {
      if (!options?.silent) {
        appendLog("error", "请先创建或输入 job_id");
      }
      return;
    }

    if (!options?.silent) {
      setBusy("refresh");
    }
    try {
      const data = await requestJSON<JobStatusResponse>(`/jobs/${id}`);
      setJob(data);
      setJobId(data.job_id);
      if (!options?.silent) {
        appendLog("info", `已刷新 job ${data.job_id}，状态 ${data.status}`);
      }
      if (data.runtime_hint && !options?.silent) {
        appendLog("info", data.runtime_hint);
      }
      if (options?.syncTasks) {
        await refreshTasks(data.job_id, { silent: true });
      }
    } catch (error) {
      if (!options?.silent) {
        appendLog("error", `刷新任务失败：${formatError(error)}`);
      }
    } finally {
      if (!options?.silent) {
        setBusy(null);
      }
    }
  }

  async function refreshTasks(id = jobId, options?: { silent?: boolean }) {
    if (!id) {
      if (!options?.silent) {
        appendLog("error", "请先创建或输入 job_id");
      }
      return;
    }

    if (!options?.silent) {
      setBusy("tasks");
    }
    try {
      const data = await requestJSON<TaskListResponse>(`/jobs/${id}/tasks`);
      setTasks(data.tasks.map(normalizeTaskDetail));
      if (!options?.silent) {
        appendLog("info", `已读取 ${data.tasks.length} 个 task`);
      }
    } catch (error) {
      if (!options?.silent) {
        appendLog("error", `读取 task 失败：${formatError(error)}`);
      }
    } finally {
      if (!options?.silent) {
        setBusy(null);
      }
    }
  }

  async function dispatchOnce() {
    if (!jobId) {
      appendLog("error", "请先创建或输入 job_id");
      return;
    }

    setBusy("dispatch");
    try {
      const data = await requestJSON<DispatchResponse>(`/jobs/${jobId}/dispatch-once`, {
        method: "POST",
      });
      appendLog(
        data.dispatched ? "success" : "info",
        data.dispatched
          ? `已推进 task ${data.executed_task_key}`
          : "当前没有可推进的 ready task",
      );
      await refreshJob(jobId, { syncTasks: true });
    } catch (error) {
      appendLog("error", `推进任务失败：${formatError(error)}`);
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="app-shell">
      <header className="hero">
        <div>
          <p className="eyebrow">Narratio Skeleton Console</p>
          <h1>前端调试台</h1>
          <p className="hero-copy">
            这不是正式产品页，而是把后端 skeleton 链路跑顺的操作台。
          </p>
        </div>
        <div className="health-strip">
          <span className="health-label">API</span>
          <code>{apiBaseUrl}</code>
          <span className={`polling-pill ${autoPollingActive ? "active" : "idle"}`}>
            {autoPollingEnabled
              ? autoPollingActive
                ? "自动轮询中"
                : "自动轮询待命"
              : "自动轮询关闭"}
          </span>
          <button onClick={() => void refreshHealth()} type="button">
            刷新健康状态
          </button>
        </div>
      </header>

      <main className="grid">
        <section className="panel panel-form">
          <div className="panel-head">
            <h2>创建 Job</h2>
            <span>{busy === "create" ? "提交中" : "就绪"}</span>
          </div>
          <label className="field">
            <span>文章内容</span>
            <textarea
              rows={10}
              value={request.article}
              onChange={(event) =>
                setRequest((current) => ({ ...current, article: event.target.value }))
              }
            />
          </label>
          <div className="field-row">
            <label className="field">
              <span>Voice Preset</span>
              <select
                value={request.options.voice_id}
                onChange={(event) =>
                  setRequest((current) => ({
                    ...current,
                    options: { ...current.options, voice_id: event.target.value },
                  }))
                }
              >
                {voicePresets.map((preset) => (
                  <option key={preset.id} value={preset.id}>
                    {preset.name}
                  </option>
                ))}
              </select>
              <small className="field-hint">
                当前默认预设为 `male_calm`
                {selectedVoicePreset?.preview_url || selectedVoicePreset?.reference_audio ? (
                  <>
                    {" · "}
                    <a
                      href={selectedVoicePreset.preview_url ?? selectedVoicePreset.reference_audio}
                      rel="noreferrer"
                      target="_blank"
                    >
                      试听参考音频
                    </a>
                  </>
                ) : null}
              </small>
            </label>
            <label className="field">
              <span>Image Style</span>
              <select
                value={request.options.image_style}
                onChange={(event) =>
                  setRequest((current) => ({
                    ...current,
                    options: { ...current.options, image_style: event.target.value },
                  }))
                }
              >
                {imageStylePresets.map((preset) => (
                  <option key={preset.id} value={preset.id}>
                    {preset.name}
                  </option>
                ))}
              </select>
              <small className="field-hint">当前默认画风为写实风格。</small>
            </label>
            <label className="field">
              <span>Video Count</span>
              <input
                min={0}
                step={1}
                type="number"
                value={request.options.video_count}
                onChange={(event) => {
                  const nextValue = Number.parseInt(event.target.value, 10);
                  setRequest((current) => ({
                    ...current,
                    options: {
                      ...current.options,
                      video_count: Number.isNaN(nextValue) ? 0 : Math.max(0, nextValue),
                    },
                  }));
                }}
              />
              <small className="field-hint">
                默认 12，只对前 n 个分镜尝试图生视频，其余分镜直接回退为静态图。
              </small>
            </label>
            <label className="field">
              <span>Aspect Ratio</span>
              <select
                value={request.options.aspect_ratio}
                onChange={(event) =>
                  setRequest((current) => ({
                    ...current,
                    options: {
                      ...current.options,
                      aspect_ratio: event.target.value as "16:9" | "9:16",
                    },
                  }))
                }
              >
                <option value="9:16">竖屏 9:16</option>
                <option value="16:9">横屏 16:9</option>
              </select>
            </label>
          </div>
          <button className="primary-button" onClick={() => void createJob()} type="button">
            创建任务
          </button>
        </section>

        <section className="panel panel-actions">
          <div className="panel-head">
            <h2>调度操作</h2>
            <span>{jobId || "未选择任务"}</span>
          </div>
          <label className="field">
            <span>Job ID</span>
            <input value={jobId} onChange={(event) => setJobId(event.target.value)} />
          </label>
          <label className="toggle-row">
            <input
              checked={autoPollingEnabled}
              onChange={(event) => setAutoPollingEnabled(event.target.checked)}
              type="checkbox"
            />
            <span>自动轮询 job / tasks</span>
          </label>
          <div className="button-row">
            <button onClick={() => void refreshJob()} type="button">
              刷新状态
            </button>
            <button onClick={() => void refreshTasks()} type="button">
              拉取 Tasks
            </button>
            <button className="accent-button" onClick={() => void dispatchOnce()} type="button">
              Dispatch Once
            </button>
          </div>
          <div className="health-grid">
            {Object.entries(health).map(([key, value]) => (
              <div className="health-card" key={key}>
                <span>{key}</span>
                <strong>{value}</strong>
              </div>
            ))}
          </div>
        </section>

        <section className="panel panel-job">
          <div className="panel-head">
            <h2>Job 状态</h2>
            <span>{job?.status ?? "暂无"}</span>
          </div>
          {job ? (
            <div className="job-card">
              {job.runtime_hint ? <div className="runtime-hint">{job.runtime_hint}</div> : null}
              {job.error ? (
                <div className="error-card">
                  <div className="error-head">
                    <h3>任务失败</h3>
                    <span>{job.error.code}</span>
                  </div>
                  <p>{job.error.message}</p>
                </div>
              ) : null}
              <div className="progress-line">
                <div className="progress-bar" style={{ width: `${job.progress}%` }} />
              </div>
              <div className="job-meta">
                <div>
                  <span>进度</span>
                  <strong>{job.progress}%</strong>
                </div>
                <div>
                  <span>创建时间</span>
                  <strong>{formatDate(job.created_at)}</strong>
                </div>
                <div>
                  <span>更新时间</span>
                  <strong>{formatDate(job.updated_at)}</strong>
                </div>
              </div>
              {job.task_state ? (
                <div className="task-state-grid">
                  <div>
                    <span>ready</span>
                    <strong>{job.task_state.ready_keys.join(", ") || "none"}</strong>
                  </div>
                  <div>
                    <span>running</span>
                    <strong>{job.task_state.running_keys.join(", ") || "none"}</strong>
                  </div>
                  <div>
                    <span>failed</span>
                    <strong>{job.task_state.failed_keys.join(", ") || "none"}</strong>
                  </div>
                </div>
              ) : null}
              {workflowSpotlight ? (
                <div className={`spotlight-card spotlight-${workflowSpotlight.tone}`}>
                  <div className="spotlight-head">
                    <h3>{workflowSpotlight.title}</h3>
                    <span>{workflowSpotlight.label}</span>
                  </div>
                  <p>{workflowSpotlight.description}</p>
                </div>
              ) : null}
              {job.result ? (
                <div className="result-card">
                  <div className="result-head">
                    <h3>最终产物</h3>
                    <span>completed</span>
                  </div>
                  <div className="result-grid">
                    <div>
                      <span>视频地址</span>
                      <strong>{job.result.video_url}</strong>
                    </div>
                    <div>
                      <span>时长</span>
                      <strong>{formatDuration(job.result.duration)}</strong>
                    </div>
                    <div>
                      <span>文件大小</span>
                      <strong>{formatFileSize(job.result.file_size)}</strong>
                    </div>
                  </div>
                </div>
              ) : null}
              <pre className="json-block">{JSON.stringify(job.tasks, null, 2)}</pre>
            </div>
          ) : (
            <div className="empty-state">创建任务后可在这里看到聚合状态。</div>
          )}
        </section>

        <section className="panel panel-workflow">
          <div className="panel-head">
            <h2>任务画布</h2>
            <span>{tasks.length > 0 ? `${tasks.length} 个节点` : "等待任务"}</span>
          </div>
          {workflowGraph ? (
            <div className="workflow-scroll">
              <div
                className="workflow-canvas"
                style={{
                  width: `${workflowGraph.width}px`,
                  height: `${workflowGraph.height}px`,
                }}
              >
                <svg
                  className="workflow-edges"
                  width={workflowGraph.width}
                  height={workflowGraph.height}
                  viewBox={`0 0 ${workflowGraph.width} ${workflowGraph.height}`}
                  aria-hidden="true"
                >
                  {workflowGraph.edges.map((edge) => (
                    <path
                      key={`${edge.from.task.id}-${edge.to.task.id}`}
                      className={`workflow-edge workflow-edge-${edge.tone}`}
                      d={buildWorkflowEdgePath(edge.from, edge.to)}
                    />
                  ))}
                </svg>
                {workflowGraph.nodes.map((node) => (
                  <article
                    className={`workflow-node workflow-node-${node.task.status}`}
                    key={node.task.id}
                    style={{
                      width: `${workflowNodeWidth}px`,
                      minHeight: `${workflowNodeHeight}px`,
                      transform: `translate(${node.x}px, ${node.y}px)`,
                    }}
                  >
                    <div className="workflow-node-head">
                      <div>
                        <p className="workflow-node-layer">L{node.layer + 1}</p>
                        <strong>{node.task.key}</strong>
                      </div>
                      <span className={`status-chip status-${node.task.status}`}>
                        {node.task.status}
                      </span>
                    </div>
                    <p className="workflow-node-meta">
                      {node.task.type} · {node.task.resource_key}
                    </p>
                    {renderTaskProgress(taskToProgressTask(node.task), "workflow-node-progress")}
                    <div className="workflow-node-grid">
                      <div>
                        <span>depends_on</span>
                        <strong>
                          {node.task.depends_on.length > 0 ? node.task.depends_on.join(", ") : "none"}
                        </strong>
                      </div>
                      <div>
                        <span>attempt</span>
                        <strong>
                          {node.task.attempt}/{node.task.max_attempts}
                        </strong>
                      </div>
                    </div>
                    {node.task.status === "skipped" ? (
                      <div className="workflow-node-note workflow-node-note-skipped">
                        上游失败或被跳过，本节点已 fail-fast 标记为 skipped。
                      </div>
                    ) : null}
                    {node.task.error ? (
                      <div className="workflow-node-note workflow-node-note-error">
                        <strong>{node.task.error.code}</strong>
                        <p>{node.task.error.message}</p>
                      </div>
                    ) : null}
                    <div className="workflow-node-actions">
                      <button disabled type="button">
                        编辑
                      </button>
                      <button disabled type="button">
                        重试
                      </button>
                    </div>
                  </article>
                ))}
              </div>
            </div>
          ) : (
            <div className="empty-state">
              还没有 task 明细。先创建任务并点击“拉取 Tasks”，再查看 DAG 画布。
            </div>
          )}
        </section>

        <section className="panel panel-tasks">
          <div className="panel-head">
            <h2>Task 明细</h2>
            <span>{tasks.length} 条</span>
          </div>
          <div className="task-list">
            {tasks.length === 0 ? (
              <div className="empty-state">还没有 task 明细。先创建任务并点击 “拉取 Tasks”。</div>
            ) : (
              sortedTasks.map((task) => (
                <article className="task-card" key={task.id}>
                  <div className="task-topline">
                    <strong>{task.key}</strong>
                    <span className={`status-chip status-${task.status}`}>{task.status}</span>
                  </div>
                  <p className="task-meta">
                    {task.type} · {task.resource_key} · attempt {task.attempt}/{task.max_attempts}
                  </p>
                  {renderTaskProgress(task)}
                  <p className="task-deps">
                    depends_on: {task.depends_on.length > 0 ? task.depends_on.join(", ") : "none"}
                  </p>
                  {task.error ? (
                    <div className="task-error">
                      <strong>{task.error.code}</strong>
                      <p>{task.error.message}</p>
                    </div>
                  ) : null}
                  <details>
                    <summary>payload</summary>
                    <pre className="json-block">{JSON.stringify(task.payload, null, 2)}</pre>
                  </details>
                  <details>
                    <summary>output_ref</summary>
                    <pre className="json-block">{JSON.stringify(task.output_ref, null, 2)}</pre>
                  </details>
                </article>
              ))
            )}
          </div>
        </section>

        <section className="panel panel-logs">
          <div className="panel-head">
            <h2>操作日志</h2>
            <span>{busy ? `busy: ${busy}` : "idle"}</span>
          </div>
          <div className="log-list">
            {logs.length === 0 ? (
              <div className="empty-state">还没有日志。创建一个 job 试试。</div>
            ) : (
              logs.map((log) => (
                <div className={`log-entry log-${log.tone}`} key={log.id}>
                  <span>{log.at}</span>
                  <p>{log.message}</p>
                </div>
              ))
            )}
          </div>
        </section>
      </main>
    </div>
  );
}

function formatError(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }

  return String(error);
}

function formatDate(value: string) {
  try {
    return new Date(value).toLocaleString("zh-CN", { hour12: false });
  } catch {
    return value;
  }
}

function formatDuration(seconds: number) {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return "0s";
  }

  if (seconds < 60) {
    return `${seconds.toFixed(1)}s`;
  }

  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return `${minutes}m ${remainingSeconds.toFixed(1)}s`;
}

function formatFileSize(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "0 B";
  }

  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }

  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

function normalizeTaskDetail(task: RawTaskDetail): TaskDetail {
  return {
    ...task,
    depends_on: Array.isArray(task.depends_on) ? task.depends_on : [],
    payload: task.payload ?? {},
    output_ref: task.output_ref ?? {},
  };
}

function isTerminalStatus(status: string) {
  return status === "completed" || status === "failed" || status === "cancelled";
}

function compareTaskDetail(a: TaskDetail, b: TaskDetail) {
  const orderA = taskOrder[a.key] ?? Number.MAX_SAFE_INTEGER;
  const orderB = taskOrder[b.key] ?? Number.MAX_SAFE_INTEGER;
  if (orderA !== orderB) {
    return orderA - orderB;
  }

  return a.id - b.id;
}

function taskToProgressTask(task: TaskDetail): TaskDetail {
  return task;
}

function readTaskProgress(task: TaskDetail) {
  const progress = task.output_ref.progress as {
    phase?: string;
    message?: string;
    current?: number;
    total?: number;
    unit?: string;
  } | undefined;

  if (!progress || typeof progress !== "object") {
    return null;
  }

  const message =
    typeof progress.message === "string" && progress.message.trim() !== ""
      ? progress.message
      : typeof progress.phase === "string"
        ? progress.phase
        : "";
  const hasCounter =
    typeof progress.current === "number" && typeof progress.total === "number" && progress.total > 0;

  if (!message && !hasCounter) {
    return null;
  }

  return { progress, message, hasCounter };
}

function renderTaskProgress(task: TaskDetail, className = "task-progress") {
  const data = readTaskProgress(task);
  if (!data) {
    return null;
  }

  return (
    <p className={className}>
      <span>进度:</span> {data.message || "running"}
      {data.hasCounter
        ? ` (${data.progress.current}/${data.progress.total}${data.progress.unit ? ` ${data.progress.unit}` : ""})`
        : ""}
    </p>
  );
}

function buildWorkflowGraph(tasks: TaskDetail[]): WorkflowGraph | null {
  if (tasks.length === 0) {
    return null;
  }

  const taskMap = new Map(tasks.map((task) => [task.key, task]));
  const layerCache = new Map<string, number>();
  const resolveLayer = (task: TaskDetail): number => {
    const cached = layerCache.get(task.key);
    if (typeof cached === "number") {
      return cached;
    }
    if (task.depends_on.length === 0) {
      layerCache.set(task.key, 0);
      return 0;
    }
    const layer =
      Math.max(
        ...task.depends_on.map((dependencyKey) => {
          const dependency = taskMap.get(dependencyKey);
          return dependency ? resolveLayer(dependency) + 1 : 0;
        }),
      ) || 0;
    layerCache.set(task.key, layer);
    return layer;
  };

  const nodesByLayer = new Map<number, TaskDetail[]>();
  for (const task of tasks) {
    const layer = resolveLayer(task);
    const current = nodesByLayer.get(layer) ?? [];
    current.push(task);
    nodesByLayer.set(layer, current);
  }

  const layers = [...nodesByLayer.keys()].sort((a, b) => a - b);
  const nodes: WorkflowNodeLayout[] = [];
  for (const layer of layers) {
    const layerTasks = [...(nodesByLayer.get(layer) ?? [])].sort(compareTaskDetail);
    layerTasks.forEach((task, row) => {
      nodes.push({
        task,
        layer,
        row,
        x: workflowCanvasPadding + layer * (workflowNodeWidth + workflowColumnGap),
        y: workflowCanvasPadding + row * (workflowNodeHeight + workflowRowGap),
      });
    });
  }

  const nodeMap = new Map(nodes.map((node) => [node.task.key, node]));
  const edges: WorkflowEdgeLayout[] = [];
  for (const node of nodes) {
    for (const dependencyKey of node.task.depends_on) {
      const dependency = nodeMap.get(dependencyKey);
      if (!dependency) {
        continue;
      }
      edges.push({
        from: dependency,
        to: node,
        tone: resolveWorkflowEdgeTone(dependency.task.status, node.task.status),
      });
    }
  }

  const maxLayer = Math.max(...nodes.map((node) => node.layer));
  const maxRow = Math.max(...nodes.map((node) => node.row));
  return {
    nodes,
    edges,
    width:
      workflowCanvasPadding * 2 +
      (maxLayer + 1) * workflowNodeWidth +
      maxLayer * workflowColumnGap,
    height:
      workflowCanvasPadding * 2 +
      (maxRow + 1) * workflowNodeHeight +
      maxRow * workflowRowGap,
  };
}

function buildWorkflowEdgePath(from: WorkflowNodeLayout, to: WorkflowNodeLayout) {
  const startX = from.x + workflowNodeWidth;
  const startY = from.y + workflowNodeHeight / 2;
  const endX = to.x;
  const endY = to.y + workflowNodeHeight / 2;
  const controlOffset = Math.max(36, (endX - startX) / 2);

  return `M ${startX} ${startY} C ${startX + controlOffset} ${startY}, ${endX - controlOffset} ${endY}, ${endX} ${endY}`;
}

function resolveWorkflowEdgeTone(
  fromStatus: string,
  toStatus: string,
): WorkflowEdgeLayout["tone"] {
  if (fromStatus === "failed" || toStatus === "failed" || toStatus === "skipped") {
    return "failed";
  }
  if (toStatus === "running" || fromStatus === "running") {
    return "running";
  }
  if (toStatus === "ready") {
    return "ready";
  }
  if (fromStatus === "succeeded" && (toStatus === "succeeded" || toStatus === "completed")) {
    return "success";
  }

  return "idle";
}

function buildWorkflowSpotlight(job: JobStatusResponse | null) {
  if (!job?.task_state) {
    return null;
  }

  if (job.task_state.failed_keys.length > 0) {
    return {
      tone: "failed",
      title: "失败节点",
      label: "failed",
      description: job.task_state.failed_keys.join(", "),
    };
  }
  if (job.task_state.running_keys.length > 0) {
    return {
      tone: "running",
      title: "当前运行中",
      label: "running",
      description: job.task_state.running_keys.join(", "),
    };
  }
  if (job.task_state.ready_keys.length > 0) {
    return {
      tone: "ready",
      title: "下一步节点",
      label: "ready",
      description: job.task_state.ready_keys.join(", "),
    };
  }

  return null;
}

export default App;
