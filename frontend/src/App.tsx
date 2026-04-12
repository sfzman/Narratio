/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import Sidebar from './components/layout/Sidebar';
import WorkflowCanvas from './components/workflow/WorkflowCanvas';
import NodeInspector from './components/workflow/NodeInspector';
import CreateTaskModal from './components/modals/CreateTaskModal';
import {CreateTaskParams, WorkflowNode} from './types/workflow';
import {Bell, Settings} from 'lucide-react';
import {AnimatePresence} from 'motion/react';
import {ApiError} from './lib/api';
import {createJob, deleteJob, getJob, getJobTasks, listJobs} from './lib/jobs';
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

export default function App() {
  const [selectedNode, setSelectedNode] = useState<WorkflowNode | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [jobs, setJobs] = useState<JobListItemResponse[]>([]);
  const [jobsLoading, setJobsLoading] = useState(true);
  const [jobsError, setJobsError] = useState<string | null>(null);
  const [jobActionError, setJobActionError] = useState<string | null>(null);
  const [deletingJobId, setDeletingJobId] = useState<string | null>(null);
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
      setJobs(response.jobs);
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
        current.map((item) =>
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
        ),
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

  const handleDeleteJob = useCallback(async (jobId: string) => {
    if (!window.confirm('Delete this job and its artifacts?')) {
      return;
    }

    setJobActionError(null);
    setDeletingJobId(jobId);
    try {
      await deleteJob(jobId);
      await loadJobs();
      setSelectedNode(null);
      setWorkflowNodes([]);
    } catch (error) {
      const message = error instanceof ApiError ? error.message : 'Failed to delete job.';
      setJobActionError(message);
    } finally {
      setDeletingJobId(null);
    }
  }, [loadJobs]);

  const handleRenameJob = useCallback(() => {
    setJobActionError('Rename is not implemented yet.');
  }, []);

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
            selectedNodeId={selectedNode?.id || null} 
            onNodeSelect={setSelectedNode} 
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
    </div>
  );
}
