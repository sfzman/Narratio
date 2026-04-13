import type {CreateTaskParams} from '@/types/workflow';
import type {
  CreateJobRequest,
  CreateJobResponse,
  DeleteJobResponse,
  JobArtifactResponse,
  JobListResponse,
  JobSummaryResponse,
  JobTasksResponse,
} from '@/types/api';
import {apiRequest, getApiBaseUrl} from './api';

export function toCreateJobRequest(params: CreateTaskParams): CreateJobRequest {
  const name = params.name?.trim();
  return {
    ...(name ? {name} : {}),
    article: params.article,
    options: {
      voice_id: params.voicePreset,
      image_style: params.imageStyle,
      aspect_ratio: params.aspectRatio,
      video_count: params.videoCount,
    },
  };
}

export async function createJob(request: CreateJobRequest | CreateTaskParams): Promise<CreateJobResponse> {
  const payload = 'voicePreset' in request ? toCreateJobRequest(request) : request;
  return apiRequest<CreateJobResponse>('/jobs', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function getJob(jobId: string): Promise<JobSummaryResponse> {
  return apiRequest<JobSummaryResponse>(`/jobs/${jobId}`);
}

export async function listJobs(): Promise<JobListResponse> {
  return apiRequest<JobListResponse>('/jobs');
}

export async function getJobTasks(jobId: string): Promise<JobTasksResponse> {
  return apiRequest<JobTasksResponse>(`/jobs/${jobId}/tasks`);
}

export async function getJobArtifact(jobId: string, path: string): Promise<JobArtifactResponse> {
  const query = new URLSearchParams({path});
  return apiRequest<JobArtifactResponse>(`/jobs/${jobId}/artifact?${query.toString()}`);
}

export async function deleteJob(jobId: string): Promise<DeleteJobResponse> {
  return apiRequest<DeleteJobResponse>(`/jobs/${jobId}`, {
    method: 'DELETE',
  });
}

export function getJobDownloadUrl(jobId: string): string {
  return `${getApiBaseUrl()}/jobs/${jobId}/download`;
}
