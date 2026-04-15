import React from 'react';
import {AnimatePresence, motion} from 'motion/react';
import {X} from 'lucide-react';
import {cn} from '@/lib/utils';

interface DialogShellProps {
  isOpen: boolean;
  title: string;
  description?: string;
  labelledBy: string;
  submitting?: boolean;
  onClose: () => void;
  headerIcon?: React.ReactNode;
  children: React.ReactNode;
  widthClassName?: string;
  panelClassName?: string;
}

const DialogShell = ({
  isOpen,
  title,
  description,
  labelledBy,
  submitting = false,
  onClose,
  headerIcon,
  children,
  widthClassName = 'max-w-lg',
  panelClassName,
}: DialogShellProps) => {
  if (!isOpen) {
    return null;
  }

  return (
    <AnimatePresence>
      <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
        <motion.div
          initial={{opacity: 0}}
          animate={{opacity: 1}}
          exit={{opacity: 0}}
          onClick={submitting ? undefined : onClose}
          className="absolute inset-0 bg-slate-950/80 backdrop-blur-sm"
        />

        <motion.div
          initial={{opacity: 0, scale: 0.95, y: 20}}
          animate={{opacity: 1, scale: 1, y: 0}}
          exit={{opacity: 0, scale: 0.95, y: 20}}
          role="dialog"
          aria-modal="true"
          aria-labelledby={labelledBy}
          className={cn(
            'relative w-full overflow-hidden rounded-2xl border border-slate-800 bg-slate-900 shadow-2xl',
            widthClassName,
            panelClassName,
          )}
        >
          <div className="flex items-center justify-between border-b border-slate-800 p-6">
            <div className="flex items-center gap-3">
              {headerIcon}
              <div>
                <h2 id={labelledBy} className="text-xl font-bold text-slate-100">{title}</h2>
                {description && (
                  <p className="mt-1 text-xs uppercase tracking-widest text-slate-500">{description}</p>
                )}
              </div>
            </div>
            <button
              onClick={onClose}
              disabled={submitting}
              className="rounded-full p-2 text-slate-500 transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:text-slate-700"
            >
              <X className="h-5 w-5" />
            </button>
          </div>

          {children}
        </motion.div>
      </div>
    </AnimatePresence>
  );
};

export default DialogShell;
