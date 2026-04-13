import type {JobStatus, NodeStatus} from './workflow';

export interface ApiSuccessResponse<T> {
  code: 0;
  data: T;
}

export interface ApiErrorResponse {
  code: number;
  message: string;
  request_id?: string;
}

export interface TaskProgressSnapshot {
  phase?: string;
  message?: string;
  current?: number;
  total?: number;
  unit?: string;
}

export interface JobTaskCounts {
  total: number;
  pending: number;
  ready: number;
  running: number;
  succeeded: number;
  failed: number;
  cancelled: number;
  skipped: number;
}

export interface JobTaskStateSnapshot {
  ready_keys: string[];
  running_keys: string[];
  failed_keys: string[];
}

export interface JobResult {
  video_url: string;
  duration: number;
  file_size: number;
}

export interface JobError {
  code?: string;
  message: string;
}

export interface JobSummaryResponse {
  job_id: string;
  name: string;
  status: JobStatus;
  progress: number;
  created_at: string;
  updated_at: string;
  tasks: JobTaskCounts;
  task_state?: JobTaskStateSnapshot;
  runtime_hint?: string;
  warnings: string[];
  error: JobError | null;
  result: JobResult | null;
}

export interface TaskError {
  code?: string;
  message: string;
}

export interface TaskOutputRef {
  artifact_path?: string;
  progress?: TaskProgressSnapshot;
  [key: string]: unknown;
}

export interface JobTaskResponse {
  id: number;
  key: string;
  type: string;
  status: NodeStatus;
  resource_key: string;
  depends_on: string[];
  attempt: number;
  max_attempts: number;
  payload: Record<string, unknown>;
  output_ref: TaskOutputRef;
  error: TaskError | null;
  created_at?: string;
  updated_at?: string;
}

export interface JobTasksResponse {
  job_id: string;
  tasks: JobTaskResponse[];
}

export interface CreateJobOptions {
  voice_id?: string;
  image_style?: string;
  aspect_ratio?: '9:16' | '16:9';
  video_count?: number;
}

export interface CreateJobRequest {
  name?: string;
  article: string;
  options?: CreateJobOptions;
}

export interface CreateJobResponse {
  job_id: string;
  name: string;
  status: JobStatus;
  created_at: string;
  estimated_seconds: number;
}

export interface JobListItemResponse {
  job_id: string;
  name: string;
  status: JobStatus;
  progress: number;
  created_at: string;
  updated_at: string;
}

export interface JobListResponse {
  jobs: JobListItemResponse[];
}

export interface DeleteJobResponse {
  cancelled: boolean;
  deleted: boolean;
  status: JobStatus;
}

export interface ArtifactEntryResponse {
  name: string;
  path: string;
  kind: 'directory' | 'json' | 'text';
}

export interface JobArtifactDirectoryResponse {
  path: string;
  kind: 'directory';
  entries: ArtifactEntryResponse[];
}

export interface JobArtifactJSONResponse {
  path: string;
  kind: 'json';
  content_type: string;
  json: unknown;
}

export interface JobArtifactTextResponse {
  path: string;
  kind: 'text';
  content_type: string;
  text: string;
}

export type JobArtifactResponse =
  | JobArtifactDirectoryResponse
  | JobArtifactJSONResponse
  | JobArtifactTextResponse;

export interface VoicePresetResponse {
  id: string;
  name: string;
  reference_audio: string;
  preview_url?: string;
}

export interface VoicesResponse {
  default_voice_id: string;
  voices: VoicePresetResponse[];
}
