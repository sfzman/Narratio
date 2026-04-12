import React from 'react';
import { 
  Folder, 
  ChevronDown, 
  ChevronRight, 
  Plus, 
  Settings, 
  LayoutGrid, 
  FileText, 
  Image as ImageIcon, 
  Music, 
  Video,
  Layers,
  Search,
  Trash2,
  PencilLine
} from 'lucide-react';
import { cn } from '../../lib/utils';
import type {JobListItemResponse} from '@/types/api';
import type {WorkflowNode} from '@/types/workflow';

interface SidebarProps {
  jobs: JobListItemResponse[];
  activeJobId: string | null;
  activeTaskId: string | null;
  jobsLoading?: boolean;
  jobsError?: string | null;
  actionError?: string | null;
  deletingJobId?: string | null;
  workflowNodes?: WorkflowNode[];
  onJobSelect: (jobId: string) => void;
  onTaskSelect: (taskId: string) => void;
  onCreateTask: () => void;
  onDeleteJob: (jobId: string) => void;
  onRenameJob: (jobId: string) => void;
}

const TASK_ICONS: Record<string, React.ComponentType<{className?: string}>> = {
  segmentation: FileText,
  outline: LayoutGrid,
  character_sheet: Layers,
  script: FileText,
  character_image: ImageIcon,
  tts: Music,
  image: ImageIcon,
  shot_video: Video,
  video: Video,
};

function statusBadgeClass(status: JobListItemResponse['status']): string {
  switch (status) {
    case 'running':
      return 'bg-cyan-400';
    case 'completed':
      return 'bg-emerald-400';
    case 'failed':
      return 'bg-rose-400';
    case 'cancelled':
      return 'bg-slate-500';
    case 'cancelling':
      return 'bg-amber-400';
    default:
      return 'bg-slate-600';
  }
}

const Sidebar = ({
  jobs,
  activeJobId,
  activeTaskId,
  jobsLoading = false,
  jobsError = null,
  actionError = null,
  deletingJobId = null,
  workflowNodes = [],
  onJobSelect,
  onTaskSelect,
  onCreateTask,
  onDeleteJob,
  onRenameJob,
}: SidebarProps) => {
  const [expandedProjects, setExpandedProjects] = React.useState<string[]>([]);
  const [searchQuery, setSearchQuery] = React.useState('');
  const [contextMenu, setContextMenu] = React.useState<{
    jobId: string;
    x: number;
    y: number;
  } | null>(null);

  React.useEffect(() => {
    if (!activeJobId) {
      return;
    }
    setExpandedProjects(prev => (
      prev.includes(activeJobId) ? prev : [...prev, activeJobId]
    ));
  }, [activeJobId]);

  React.useEffect(() => {
    if (!contextMenu) {
      return;
    }

    const closeMenu = () => setContextMenu(null);
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setContextMenu(null);
      }
    };

    window.addEventListener('click', closeMenu);
    window.addEventListener('keydown', closeOnEscape);

    return () => {
      window.removeEventListener('click', closeMenu);
      window.removeEventListener('keydown', closeOnEscape);
    };
  }, [contextMenu]);

  const toggleProject = (id: string) => {
    setExpandedProjects(prev => 
      prev.includes(id) ? prev.filter(p => p !== id) : [...prev, id]
    );
  };

  const filteredJobs = jobs.filter((job) => {
    const keyword = searchQuery.trim().toLowerCase();
    if (keyword === '') {
      return true;
    }

    return (
      job.name.toLowerCase().includes(keyword) ||
      job.job_id.toLowerCase().includes(keyword)
    );
  });

  return (
    <div className="w-64 h-full bg-slate-950 border-r border-slate-900 flex flex-col">
      <div className="p-6 flex items-center gap-3 border-b border-slate-900">
        <div className="w-8 h-8 bg-brand-cyan rounded flex items-center justify-center">
          <span className="text-slate-950 font-black text-xl">N</span>
        </div>
        <h1 className="font-bold text-lg tracking-tight text-slate-100">NARRATIO</h1>
      </div>

      <div className="p-4">
        <div className="relative">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-600" />
          <input 
            type="text" 
            placeholder="Search projects..." 
            value={searchQuery}
            onChange={(event) => setSearchQuery(event.target.value)}
            className="w-full bg-slate-900/50 border border-slate-800 rounded-lg py-2 pl-10 pr-4 text-xs text-slate-300 focus:outline-none focus:border-brand-cyan/50 transition-colors"
          />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto py-2">
        <div className="px-4 mb-4 flex items-center justify-between">
          <span className="text-[10px] font-bold text-slate-600 uppercase tracking-widest">Projects</span>
          <button 
            onClick={onCreateTask}
            className="p-1 hover:bg-slate-900 rounded text-slate-500 transition-colors"
          >
            <Plus className="w-4 h-4" />
          </button>
        </div>

        <div className="space-y-1">
          {jobsLoading && (
            <div className="px-4 py-3 text-xs text-slate-500">Loading jobs...</div>
          )}

          {!jobsLoading && jobsError && (
            <div className="px-4 py-3 text-xs text-rose-400">{jobsError}</div>
          )}

          {!jobsLoading && !jobsError && actionError && (
            <div className="px-4 py-3 text-xs text-rose-400">{actionError}</div>
          )}

          {!jobsLoading && !jobsError && filteredJobs.length === 0 && (
            <div className="px-4 py-3 text-xs text-slate-500">No jobs yet.</div>
          )}

          {filteredJobs.map(job => {
            const expanded = expandedProjects.includes(job.job_id);
            const isActiveJob = activeJobId === job.job_id;

            return (
            <div key={job.job_id} className="px-2">
              <button 
                onContextMenu={(event) => {
                  event.preventDefault();
                  onJobSelect(job.job_id);
                  setContextMenu({
                    jobId: job.job_id,
                    x: event.clientX,
                    y: event.clientY,
                  });
                }}
                onClick={() => {
                  onJobSelect(job.job_id);
                  toggleProject(job.job_id);
                }}
                className={cn(
                  "w-full flex items-center gap-2 px-3 py-2 rounded-lg transition-colors group",
                  isActiveJob || expanded ? "bg-slate-900/50 text-slate-100" : "text-slate-500 hover:text-slate-300"
                )}
              >
                {expanded ? (
                  <ChevronDown className="w-4 h-4" />
                ) : (
                  <ChevronRight className="w-4 h-4" />
                )}
                <Folder className={cn("w-4 h-4", isActiveJob || expanded ? "text-brand-cyan" : "text-slate-600")} />
                <span className="text-sm font-medium truncate flex-1 text-left">{job.name}</span>
                <span className={cn("w-2 h-2 rounded-full shrink-0", statusBadgeClass(job.status))} />
              </button>

              {expanded && (
                <div className="mt-1 ml-4 space-y-0.5 border-l border-slate-900">
                  <div className="px-4 py-2 text-[10px] uppercase tracking-widest text-slate-600">
                    {job.status} · {job.progress}%
                  </div>
                  {workflowNodes.map(task => {
                    const isActive = activeTaskId === task.id;
                    const Icon = TASK_ICONS[task.id] || FileText;
                    return (
                      <button
                        key={task.id}
                        onClick={() => onTaskSelect(task.id)}
                        className={cn(
                          "w-full flex items-center gap-3 px-4 py-2 text-xs transition-all relative",
                          isActive 
                            ? "text-slate-100 font-semibold bg-slate-900/30" 
                            : "text-slate-500 hover:text-slate-300"
                        )}
                      >
                        {isActive && (
                          <div className="absolute left-0 top-0 bottom-0 w-0.5 bg-brand-cyan" />
                        )}
                        <Icon className={cn("w-3.5 h-3.5", isActive ? "text-brand-cyan" : "text-slate-700")} />
                        <span className="truncate flex-1 text-left">{task.label}</span>
                        <span className={cn("w-2 h-2 rounded-full shrink-0", statusBadgeClass(task.status as JobListItemResponse['status']))} />
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          )})}
        </div>
      </div>

      <div className="p-4 border-t border-slate-900">
        <button className="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-slate-500 hover:text-slate-300 hover:bg-slate-900 transition-all group">
          <Settings className="w-4 h-4 group-hover:rotate-90 transition-transform duration-500" />
          <span className="text-sm font-medium">Workspace Settings</span>
        </button>
      </div>

      {contextMenu && (
        <div
          className="fixed z-50 min-w-40 rounded-xl border border-slate-800 bg-slate-900/95 p-1 shadow-2xl backdrop-blur-md"
          style={{left: contextMenu.x, top: contextMenu.y}}
          onClick={(event) => event.stopPropagation()}
        >
          <button
            type="button"
            onClick={() => {
              onRenameJob(contextMenu.jobId);
              setContextMenu(null);
            }}
            className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-xs text-slate-400"
          >
            <PencilLine className="h-3.5 w-3.5" />
            Rename
            <span className="ml-auto text-[10px] text-slate-600">Soon</span>
          </button>
          <button
            type="button"
            disabled={deletingJobId === contextMenu.jobId}
            onClick={() => {
              onDeleteJob(contextMenu.jobId);
              setContextMenu(null);
            }}
            className={cn(
              "flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-xs transition-colors",
              deletingJobId === contextMenu.jobId
                ? "cursor-not-allowed text-slate-600"
                : "text-rose-400 hover:bg-rose-950/40"
            )}
          >
            <Trash2 className="h-3.5 w-3.5" />
            {deletingJobId === contextMenu.jobId ? 'Deleting...' : 'Delete'}
          </button>
        </div>
      )}
    </div>
  );
};

export default Sidebar;
