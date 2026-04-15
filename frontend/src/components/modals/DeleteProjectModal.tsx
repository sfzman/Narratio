import React from 'react';
import {AlertTriangle, Trash2} from 'lucide-react';
import DialogShell from './DialogShell';

interface DeleteProjectModalProps {
  isOpen: boolean;
  projectName: string;
  submitting?: boolean;
  error?: string | null;
  onClose: () => void;
  onDelete: () => Promise<void> | void;
}

const DeleteProjectModal = ({
  isOpen,
  projectName,
  submitting = false,
  error = null,
  onClose,
  onDelete,
}: DeleteProjectModalProps) => {
  React.useEffect(() => {
    if (!isOpen || submitting) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose, submitting]);

  if (!isOpen) {
    return null;
  }

  return (
    <DialogShell
      isOpen={isOpen}
      title="Delete Project"
      description="This action cannot be undone"
      labelledBy="delete-project-title"
      submitting={submitting}
      onClose={onClose}
      panelClassName="border-rose-950/70"
      headerIcon={(
        <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-rose-500/20 bg-rose-500/10 text-rose-300">
          <AlertTriangle className="h-5 w-5" />
        </div>
      )}
    >
      <div className="space-y-4 p-6">
        <div className="rounded-2xl border border-rose-500/20 bg-rose-950/20 p-4 text-sm text-slate-200">
          <span className="block text-[11px] font-bold uppercase tracking-widest text-rose-300">Selected Project</span>
          <span className="mt-2 block break-words text-base font-semibold text-slate-100">{projectName}</span>
        </div>
        <div className="rounded-xl border border-slate-800 bg-slate-950/60 px-4 py-3 text-xs leading-6 text-slate-500">
          Running jobs will be cancelled first. Completed, failed, or cancelled jobs will also remove saved metadata and workspace artifacts.
        </div>
        {error && (
          <div className="rounded-xl border border-rose-900/60 bg-rose-950/30 px-4 py-3 text-xs text-rose-300">
            {error}
          </div>
        )}
      </div>

      <div className="flex items-center justify-end gap-4 border-t border-slate-800 bg-slate-900/50 p-6">
        <button
          type="button"
          onClick={onClose}
          disabled={submitting}
          className="px-6 py-2 text-xs font-bold tracking-widest text-slate-500 transition-colors hover:text-slate-300 disabled:cursor-not-allowed disabled:text-slate-700"
        >
          CANCEL
        </button>
        <button
          type="button"
          onClick={() => void onDelete()}
          disabled={submitting}
          className="flex items-center gap-2 rounded-xl bg-rose-500 px-6 py-3 text-sm font-bold text-white shadow-lg shadow-rose-900/30 transition-all hover:bg-rose-400 disabled:bg-slate-700 disabled:text-slate-400 disabled:shadow-none"
        >
          <Trash2 className="h-4 w-4" />
          {submitting ? 'DELETING...' : 'DELETE PROJECT'}
        </button>
      </div>
    </DialogShell>
  );
};

export default DeleteProjectModal;
