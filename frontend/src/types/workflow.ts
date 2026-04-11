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
  label: string;
  chineseLabel: string;
  status: NodeStatus;
  progress?: number;
  summary?: string;
  updatedAt: string;
  dependencies: string[];
  artifacts?: any;
}

export interface Project {
  id: string;
  name: string;
  status: JobStatus;
  createdAt: string;
  nodes: WorkflowNode[];
}

export interface CreateTaskParams {
  article: string;
  voicePreset: string;
  imageStyle: string;
  aspectRatio: '9:16' | '16:9';
  videoCount: number;
}
