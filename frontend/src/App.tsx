/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import Sidebar from './components/layout/Sidebar';
import WorkflowCanvas from './components/workflow/WorkflowCanvas';
import NodeInspector from './components/workflow/NodeInspector';
import CreateTaskModal from './components/modals/CreateTaskModal';
import DeleteProjectModal from './components/modals/DeleteProjectModal';
import RenameProjectModal from './components/modals/RenameProjectModal';
import {CreateTaskParams, WorkflowNode} from './types/workflow';
import {Bell, Settings} from 'lucide-react';
import {AnimatePresence} from 'motion/react';
import {ApiError} from './lib/api';
import {createJob, deleteJob, getJob, getJobTasks, listJobs, renameJob, retryTask} from './lib/jobs';
import type {JobListItemResponse, JobSummaryResponse} from './types/api';
import {mapJobToProject} from './lib/workflow-mapper';

function formatJobStatus(status: JobListItemResponse['status'] | null): string {
  if (!status) {
    return 'IDLE';
  }
  return status.replace(/_/g, ' ').toUpperCase();
}

function isTerminalJobStatus(status: JobListItemResponse['status'] | null | undefined): boolean {
  return status === 'completed' || status === 'failed' || status === 'cancelled';
}

function getWorkflowPollDelayMs(elapsedMs: number): number {
  if (elapsedMs < 30_000) {
    return 3_000;
  }
  if (elapsedMs < 120_000) {
    return 5_000;
  }
  return 10_000;
}

function getWorkflowPollingHint(status: JobListItemResponse['status'] | null | undefined, elapsedMs: number): string | null {
  if (!status || isTerminalJobStatus(status) || elapsedMs < 600_000) {
    return null;
  }
  return '该任务运行已超过 10 分钟；如果进度长时间不变化，请检查后端日志。';
}

function sortJobsByUpdatedAt(jobs: JobListItemResponse[]): JobListItemResponse[] {
  return [...jobs].sort((left, right) => {
    const timeDiff = new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime();
    if (timeDiff !== 0) {
      return timeDiff;
    }
    return right.job_id.localeCompare(left.job_id);
  });
}

export default function App() {
  const [selectedNode, setSelectedNode] = useState<WorkflowNode | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [jobs, setJobs] = useState<JobListItemResponse[]>([]);
  const [jobsLoading, setJobsLoading] = useState(true);
  const [jobsError, setJobsError] = useState<string | null>(null);
  const [jobActionError, setJobActionError] = useState<string | null>(null);
  const [deletingJobId, setDeletingJobId] = useState<string | null>(null);
  const [deleteTargetJobId, setDeleteTargetJobId] = useState<string | null>(null);
  const [deleteJobError, setDeleteJobError] = useState<string | null>(null);
  const [renamingJobId, setRenamingJobId] = useState<string | null>(null);
  const [renameTargetJobId, setRenameTargetJobId] = useState<string | null>(null);
  const [renameJobError, setRenameJobError] = useState<string | null>(null);
  const [retryingTaskKey, setRetryingTaskKey] = useState<string | null>(null);
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null);
  const [workflowNodes, setWorkflowNodes] = useState<WorkflowNode[]>([]);
  const [workflowLoading, setWorkflowLoading] = useState(false);
  const [workflowError, setWorkflowError] = useState<string | null>(null);
  const [workflowHint, setWorkflowHint] = useState<string | null>(null);
  const [selectedJobSummary, setSelectedJobSummary] = useState<JobSummaryResponse | null>(null);
  const [createJobSubmitting, setCreateJobSubmitting] = useState(false);
  const [createJobError, setCreateJobError] = useState<string | null>(null);
  const workflowRequestVersion = useRef(0);

  const loadJobs = useCallback(async () => {
    setJobsLoading(true);
    setJobsError(null);
    try {
      const response = await listJobs();
      setJobs(sortJobsByUpdatedAt(response.jobs));
      setSelectedJobId((current) => {
        if (current && response.jobs.some((job) => job.job_id === current)) {
          return current;
        }
        return response.jobs[0]?.job_id ?? null;
      });
    } catch (error) {
      const message = error instanceof ApiError ? error.message : 'Failed to load jobs.';
      setJobsError(message);
    } finally {
      setJobsLoading(false);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function run() {
      if (cancelled) {
        return;
      }
      await loadJobs();
    }

    run();
    return () => {
      cancelled = true;
    };
  }, [loadJobs]);

  const handleTaskSelect = useCallback((taskId: string) => {
    const node = workflowNodes.find((item) => item.id === taskId);
    if (node) {
      setSelectedNode(node);
    }
  }, [workflowNodes]);

  const selectedJob = useMemo(
    () => jobs.find((job) => job.job_id === selectedJobId) ?? null,
    [jobs, selectedJobId],
  );
  const renameTargetJob = useMemo(
    () => jobs.find((job) => job.job_id === renameTargetJobId) ?? null,
    [jobs, renameTargetJobId],
  );
  const deleteTargetJob = useMemo(
    () => jobs.find((job) => job.job_id === deleteTargetJobId) ?? null,
    [jobs, deleteTargetJobId],
  );

  const loadWorkflow = useCallback(async (jobId: string, options?: {silent?: boolean}) => {
    const silent = options?.silent === true;
    const requestVersion = workflowRequestVersion.current + 1;
    workflowRequestVersion.current = requestVersion;

    if (!silent) {
      setWorkflowLoading(true);
      setWorkflowError(null);
    }

    try {
      const [job, tasks] = await Promise.all([
        getJob(jobId),
        getJobTasks(jobId),
      ]);

      if (workflowRequestVersion.current !== requestVersion) {
        return null;
      }

      const project = mapJobToProject(job, tasks.tasks);
      setSelectedJobSummary(job);
      setWorkflowNodes(project.nodes);
      setSelectedNode((current) => {
        if (!current) {
          return null;
        }
        return project.nodes.find((node) => node.id === current.id) ?? null;
      });
      setJobs((current) =>
        sortJobsByUpdatedAt(current.map((item) =>
          item.job_id === job.job_id
            ? {
                ...item,
                name: job.name,
                status: job.status,
                progress: job.progress,
                created_at: job.created_at,
                updated_at: job.updated_at,
              }
            : item,
        )),
      );
      setWorkflowError(null);
      return job;
    } catch (error) {
      if (workflowRequestVersion.current !== requestVersion) {
        return null;
      }

      if (!silent) {
        const message = error instanceof ApiError ? error.message : 'Failed to load workflow.';
        setWorkflowError(message);
        setSelectedJobSummary(null);
        setWorkflowNodes([]);
        setSelectedNode(null);
      }
      return null;
    } finally {
      if (!silent && workflowRequestVersion.current === requestVersion) {
        setWorkflowLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    let active = true;
    let timerId: number | null = null;

    if (!selectedJobId) {
      workflowRequestVersion.current += 1;
      setWorkflowNodes([]);
      setSelectedNode(null);
      setWorkflowError(null);
      setWorkflowHint(null);
      setSelectedJobSummary(null);
      setWorkflowLoading(false);
      return () => {
        active = false;
      };
    }

    const pollStartedAt = Date.now();

    const run = async (silent: boolean) => {
      const job = await loadWorkflow(selectedJobId, {silent});
      if (!active) {
        return;
      }

      const elapsedMs = Date.now() - pollStartedAt;
      setWorkflowHint(getWorkflowPollingHint(job?.status, elapsedMs));

      if (job && isTerminalJobStatus(job.status)) {
        return;
      }

      timerId = window.setTimeout(() => {
        void run(true);
      }, getWorkflowPollDelayMs(elapsedMs));
    };

    void run(false);
    return () => {
      active = false;
      workflowRequestVersion.current += 1;
      if (timerId !== null) {
        window.clearTimeout(timerId);
      }
    };
  }, [loadWorkflow, selectedJobId]);

  const handleDeleteJob = useCallback((jobId: string) => {
    const job = jobs.find((item) => item.job_id === jobId);
    if (!job) {
      setJobActionError('Project not found.');
      return;
    }
    setJobActionError(null);
    setDeleteJobError(null);
    setDeleteTargetJobId(jobId);
  }, [jobs]);

  const handleCloseDeleteModal = useCallback(() => {
    if (deletingJobId) {
      return;
    }
    setDeleteTargetJobId(null);
    setDeleteJobError(null);
  }, [deletingJobId]);

  const handleConfirmDelete = useCallback(async () => {
    if (!deleteTargetJob) {
      setDeleteJobError('Project not found.');
      return;
    }

    setJobActionError(null);
    setDeleteJobError(null);
    setDeletingJobId(deleteTargetJob.job_id);
    try {
      await deleteJob(deleteTargetJob.job_id);
      await loadJobs();
      setSelectedNode(null);
      setWorkflowNodes([]);
      setDeleteTargetJobId(null);
    } catch (error) {
      const message = error instanceof ApiError ? error.message : 'Failed to delete job.';
      setDeleteJobError(message);
    } finally {
      setDeletingJobId(null);
    }
  }, [deleteTargetJob, loadJobs]);

  const handleRenameJob = useCallback((jobId: string) => {
    const job = jobs.find((item) => item.job_id === jobId);
    if (!job) {
      setJobActionError('Project not found.');
      return;
    }
    setJobActionError(null);
    setRenameJobError(null);
    setRenameTargetJobId(jobId);
  }, [jobs]);

  const handleCloseRenameModal = useCallback(() => {
    if (renamingJobId) {
      return;
    }
    setRenameTargetJobId(null);
    setRenameJobError(null);
  }, [renamingJobId]);

  const handleConfirmRename = useCallback(async (nextName: string) => {
    if (!renameTargetJob) {
      setRenameJobError('Project not found.');
      return;
    }

    const trimmedName = nextName.trim();
    if (trimmedName === '') {
      setRenameJobError('Project name cannot be empty.');
      return;
    }
    if (trimmedName === renameTargetJob.name) {
      setRenameJobError(null);
      setRenameTargetJobId(null);
      return;
    }

    setJobActionError(null);
    setRenameJobError(null);
    setRenamingJobId(renameTargetJob.job_id);
    try {
      const renamed = await renameJob(renameTargetJob.job_id, trimmedName);
      setJobs((current) =>
        sortJobsByUpdatedAt(current.map((item) =>
          item.job_id === renamed.job_id
            ? {
                ...item,
                name: renamed.name,
                status: renamed.status,
                progress: renamed.progress,
                created_at: renamed.created_at,
                updated_at: renamed.updated_at,
              }
            : item,
        )),
      );
      setSelectedJobSummary((current) => (
        current && current.job_id === renamed.job_id
          ? {
              ...current,
              name: renamed.name,
              status: renamed.status,
              progress: renamed.progress,
              created_at: renamed.created_at,
              updated_at: renamed.updated_at,
            }
          : current
      ));
      setRenameTargetJobId(null);
    } catch (error) {
      const message = error instanceof ApiError ? error.message : 'Failed to rename project.';
      setRenameJobError(message);
    } finally {
      setRenamingJobId(null);
    }
  }, [renameTargetJob]);

  const handleRetryTask = useCallback(async (node: WorkflowNode) => {
    if (!selectedJobId || retryingTaskKey) {
      return;
    }

    setJobActionError(null);
    setRetryingTaskKey(node.id);
    try {
      await retryTask(selectedJobId, node.id);
      await loadWorkflow(selectedJobId);
    } catch (error) {
      const message = error instanceof ApiError ? error.message : 'Failed to retry task.';
      setJobActionError(message);
    } finally {
      setRetryingTaskKey(null);
    }
  }, [loadWorkflow, retryingTaskKey, selectedJobId]);

  const handleOpenCreateModal = useCallback(() => {
    setCreateJobError(null);
    setIsCreateModalOpen(true);
  }, []);

  const handleCloseCreateModal = useCallback(() => {
    if (createJobSubmitting) {
      return;
    }
    setCreateJobError(null);
    setIsCreateModalOpen(false);
  }, [createJobSubmitting]);

  const handleCreateJob = useCallback(async (params: CreateTaskParams) => {
    setCreateJobSubmitting(true);
    setCreateJobError(null);
    try {
      const job = await createJob(params);
      await loadJobs();
      setSelectedJobId(job.job_id);
      setSelectedNode(null);
      setIsCreateModalOpen(false);
    } catch (error) {
      const message = error instanceof ApiError ? error.message : 'Failed to create job.';
      setCreateJobError(message);
    } finally {
      setCreateJobSubmitting(false);
    }
  }, [loadJobs]);

  return (
    <div className="flex h-screen w-full bg-slate-950 overflow-hidden font-sans">
      {/* Sidebar */}
      <Sidebar 
        jobs={jobs}
        activeJobId={selectedJobId}
        activeTaskId={selectedNode?.id || null} 
        jobsLoading={jobsLoading}
        jobsError={jobsError}
        actionError={jobActionError}
        deletingJobId={deletingJobId}
        renamingJobId={renamingJobId}
        workflowNodes={workflowNodes}
        onJobSelect={setSelectedJobId}
        onTaskSelect={handleTaskSelect} 
        onCreateTask={handleOpenCreateModal}
        onDeleteJob={handleDeleteJob}
        onRenameJob={handleRenameJob}
      />

      {/* Main Content Area */}
      <div className="flex-1 flex flex-col relative min-w-0">
        {/* Header */}
        <header className="h-16 border-b border-slate-900 flex items-center justify-between px-8 bg-slate-950/50 backdrop-blur-md z-20">
          <div className="flex min-w-0 flex-col gap-1">
            <div className="flex items-center gap-4 text-xs font-medium">
              <span className="text-slate-500 uppercase tracking-widest">NARRATIO</span>
              <span className="text-slate-700">/</span>
              <span className="text-slate-500 uppercase tracking-widest">PROJECTS</span>
              <span className="text-slate-700">/</span>
              <span className="truncate text-slate-100 font-bold tracking-widest">
                {selectedJob?.name || 'NO_JOB_SELECTED'}
              </span>
              {selectedJob && (
                <div className="flex items-center gap-2 ml-4 px-2 py-1 bg-brand-cyan/10 border border-brand-cyan/20 rounded-md">
                  <div className="w-1.5 h-1.5 rounded-full bg-brand-cyan animate-pulse" />
                  <span className="text-[10px] font-bold text-brand-cyan uppercase tracking-widest">
                    {formatJobStatus(selectedJob.status)}
                  </span>
                </div>
              )}
            </div>
            {workflowHint && (
              <div className="truncate text-[11px] text-amber-300/80">
                {workflowHint}
              </div>
            )}
          </div>

          <div className="flex items-center gap-6">
            <div className="flex items-center gap-4">
            <button className="text-slate-500 hover:text-slate-300 transition-colors">
              <Bell className="w-5 h-5" />
            </button>
            <button className="text-slate-500 hover:text-slate-300 transition-colors">
              <Settings className="w-5 h-5" />
            </button>
              <div className="w-8 h-8 rounded-full bg-slate-800 border border-slate-700 flex items-center justify-center overflow-hidden ml-2">
                <span className="text-xs font-bold text-slate-300">N</span>
              </div>
            </div>
          </div>
        </header>

        {/* Canvas Area */}
        <main className="flex-1 relative overflow-hidden">
          <WorkflowCanvas 
            workflowNodes={workflowNodes}
            loading={workflowLoading}
            error={workflowError}
            retryingNodeId={retryingTaskKey}
            selectedNodeId={selectedNode?.id || null} 
            onNodeSelect={setSelectedNode}
            onNodeRetry={handleRetryTask}
          />
        </main>

        {/* Node Inspector Overlay */}
        <AnimatePresence>
          {selectedNode && (
            <div className="absolute inset-y-0 right-0 z-40">
              <NodeInspector
                node={selectedNode}
                jobSummary={selectedJobSummary}
                onClose={() => setSelectedNode(null)}
              />
            </div>
          )}
        </AnimatePresence>
      </div>

      {/* Modals */}
      <CreateTaskModal 
        isOpen={isCreateModalOpen} 
        onClose={handleCloseCreateModal} 
        onCreate={handleCreateJob}
        submitting={createJobSubmitting}
        error={createJobError}
      />
      <DeleteProjectModal
        isOpen={deleteTargetJob != null}
        projectName={deleteTargetJob?.name ?? ''}
        onClose={handleCloseDeleteModal}
        onDelete={handleConfirmDelete}
        submitting={deletingJobId != null}
        error={deleteJobError}
      />
      <RenameProjectModal
        isOpen={renameTargetJob != null}
        currentName={renameTargetJob?.name ?? ''}
        onClose={handleCloseRenameModal}
        onRename={handleConfirmRename}
        submitting={renamingJobId != null}
        error={renameJobError}
      />
    </div>
  );
}
