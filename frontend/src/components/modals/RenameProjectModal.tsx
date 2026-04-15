import React from 'react';
import {FolderPen, PencilLine} from 'lucide-react';
import DialogShell from './DialogShell';

interface RenameProjectModalProps {
  isOpen: boolean;
  currentName: string;
  submitting?: boolean;
  error?: string | null;
  onClose: () => void;
  onRename: (name: string) => Promise<void> | void;
}

const RenameProjectModal = ({
  isOpen,
  currentName,
  submitting = false,
  error = null,
  onClose,
  onRename,
}: RenameProjectModalProps) => {
  const [name, setName] = React.useState(currentName);

  React.useEffect(() => {
    if (!isOpen) {
      return;
    }
    setName(currentName);
  }, [currentName, isOpen]);

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

  const trimmedName = name.trim();

  return (
    <DialogShell
      isOpen={isOpen}
      title="Rename Project"
      description="Update the title shown in the sidebar and header"
      labelledBy="rename-project-title"
      submitting={submitting}
      onClose={onClose}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (submitting || trimmedName === '') {
            return;
          }
          void onRename(trimmedName);
        }}
      >
        <div className="space-y-4 p-6">
          <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-slate-500">
            <FolderPen className="h-3.5 w-3.5" />
            <span>Project Name</span>
          </div>
          <label className="block">
            <span className="sr-only">Project name</span>
            <input
              autoFocus
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Enter a project name"
              className="w-full rounded-xl border border-slate-800 bg-slate-950 px-4 py-3 text-sm text-slate-200 transition-colors placeholder:text-slate-700 focus:border-brand-cyan/50 focus:outline-none"
            />
          </label>
          <div className="rounded-xl border border-slate-800 bg-slate-950/60 px-4 py-3 text-xs text-slate-500">
            This only changes the display name. Running tasks and generated artifacts stay untouched.
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
            type="submit"
            disabled={submitting || trimmedName === ''}
            className="flex items-center gap-2 rounded-xl bg-brand-cyan px-6 py-3 text-sm font-bold text-slate-950 shadow-lg shadow-cyan-500/20 transition-all hover:bg-cyan-400 disabled:bg-slate-700 disabled:text-slate-400 disabled:shadow-none"
          >
            <PencilLine className="h-4 w-4" />
            {submitting ? 'RENAMING...' : 'SAVE NAME'}
          </button>
        </div>
      </form>
    </DialogShell>
  );
};

export default RenameProjectModal;
