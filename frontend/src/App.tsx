import { useEffect, useState } from "react";

type CreateJobRequest = {
  article: string;
  language: string;
  options: {
    voice_id: string;
    image_style: string;
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

type LogEntry = {
  id: string;
  tone: "info" | "success" | "error";
  message: string;
  at: string;
};

const defaultRequest: CreateJobRequest = {
  article:
    "暮色落进旧城的屋檐，巷口卖糖画的摊子还亮着一盏小灯。少年提着书箱，在雨后的石板路上慢慢往家走。",
  language: "zh",
  options: {
    voice_id: "default",
    image_style: "realistic",
  },
};

const apiBaseUrl = (
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/api/v1"
).replace(/\/$/, "");

function App() {
  const [request, setRequest] = useState<CreateJobRequest>(defaultRequest);
  const [jobId, setJobId] = useState("");
  const [job, setJob] = useState<JobStatusResponse | null>(null);
  const [tasks, setTasks] = useState<TaskDetail[]>([]);
  const [health, setHealth] = useState<Record<string, string>>({});
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    void refreshHealth();
  }, []);

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
      await refreshJob(data.job_id);
    } catch (error) {
      appendLog("error", `创建任务失败：${formatError(error)}`);
    } finally {
      setBusy(null);
    }
  }

  async function refreshJob(id = jobId) {
    if (!id) {
      appendLog("error", "请先创建或输入 job_id");
      return;
    }

    setBusy("refresh");
    try {
      const data = await requestJSON<JobStatusResponse>(`/jobs/${id}`);
      setJob(data);
      setJobId(data.job_id);
      appendLog("info", `已刷新 job ${data.job_id}，状态 ${data.status}`);
      if (data.runtime_hint) {
        appendLog("info", data.runtime_hint);
      }
    } catch (error) {
      appendLog("error", `刷新任务失败：${formatError(error)}`);
    } finally {
      setBusy(null);
    }
  }

  async function refreshTasks() {
    if (!jobId) {
      appendLog("error", "请先创建或输入 job_id");
      return;
    }

    setBusy("tasks");
    try {
      const data = await requestJSON<TaskListResponse>(`/jobs/${jobId}/tasks`);
      setTasks(data.tasks.map(normalizeTaskDetail));
      appendLog("info", `已读取 ${data.tasks.length} 个 task`);
    } catch (error) {
      appendLog("error", `读取 task 失败：${formatError(error)}`);
    } finally {
      setBusy(null);
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
      await refreshJob(jobId);
      await refreshTasks();
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
              <span>语言</span>
              <input
                value={request.language}
                onChange={(event) =>
                  setRequest((current) => ({ ...current, language: event.target.value }))
                }
              />
            </label>
            <label className="field">
              <span>Voice ID</span>
              <input
                value={request.options.voice_id}
                onChange={(event) =>
                  setRequest((current) => ({
                    ...current,
                    options: { ...current.options, voice_id: event.target.value },
                  }))
                }
              />
            </label>
            <label className="field">
              <span>Image Style</span>
              <input
                value={request.options.image_style}
                onChange={(event) =>
                  setRequest((current) => ({
                    ...current,
                    options: { ...current.options, image_style: event.target.value },
                  }))
                }
              />
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
              <pre className="json-block">{JSON.stringify(job.tasks, null, 2)}</pre>
            </div>
          ) : (
            <div className="empty-state">创建任务后可在这里看到聚合状态。</div>
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
              tasks.map((task) => (
                <article className="task-card" key={task.id}>
                  <div className="task-topline">
                    <strong>{task.key}</strong>
                    <span className={`status-chip status-${task.status}`}>{task.status}</span>
                  </div>
                  <p className="task-meta">
                    {task.type} · {task.resource_key} · attempt {task.attempt}/{task.max_attempts}
                  </p>
                  <p className="task-deps">
                    depends_on: {task.depends_on.length > 0 ? task.depends_on.join(", ") : "none"}
                  </p>
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

function normalizeTaskDetail(task: RawTaskDetail): TaskDetail {
  return {
    ...task,
    depends_on: Array.isArray(task.depends_on) ? task.depends_on : [],
    payload: task.payload ?? {},
    output_ref: task.output_ref ?? {},
  };
}

export default App;
