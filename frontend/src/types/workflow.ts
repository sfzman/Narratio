export type NodeStatus = 
  | 'pending' 
  | 'ready' 
  | 'running' 
  | 'succeeded' 
  | 'failed' 
  | 'cancelled' 
  | 'skipped';

export type JobStatus = 
  | 'queued' 
  | 'running' 
  | 'cancelling' 
  | 'completed' 
  | 'failed' 
  | 'cancelled';

export interface WorkflowNode {
  id: string;
  type: string;
  taskType?: string;
  label: string;
  chineseLabel: string;
  status: NodeStatus;
  progress?: number;
  summary?: string;
  updatedAt: string;
  resourceKey?: string;
  dependencies: string[];
  attempt?: number;
  maxAttempts?: number;
  payload?: Record<string, unknown>;
  artifacts?: Record<string, unknown>;
  error?: {
    code?: string;
    message: string;
  } | null;
}

export interface Project {
  id: string;
  name: string;
  status: JobStatus;
  createdAt: string;
  nodes: WorkflowNode[];
}

export interface CreateTaskParams {
  name?: string;
  article: string;
  voicePreset: string;
  imageStyle: string;
  aspectRatio: '9:16' | '16:9';
  videoCount: number;
}
