import React, { memo } from 'react';
import { Handle, Position } from 'reactflow';
import { CheckCircle2, Circle, Clock, AlertCircle, Play, SkipForward, XCircle } from 'lucide-react';
import { cn } from '../../lib/utils';
import { NodeStatus } from '../../types/workflow';
import { motion } from 'motion/react';

interface NodeCardProps {
  data: {
    id: string;
    label: string;
    chineseLabel: string;
    status: NodeStatus;
    summary: string;
    progress?: number;
    onView?: () => void;
    onEdit?: () => void;
    onRetry?: () => void;
    retrying?: boolean;
  };
  selected?: boolean;
}

const StatusIcon = ({ status }: { status: NodeStatus }) => {
  switch (status) {
    case 'succeeded':
      return <CheckCircle2 className="w-4 h-4 text-emerald-400" />;
    case 'running':
      return (
        <motion.div
          animate={{ rotate: 360 }}
          transition={{ duration: 2, repeat: Infinity, ease: "linear" }}
        >
          <Clock className="w-4 h-4 text-brand-cyan" />
        </motion.div>
      );
    case 'failed':
      return <AlertCircle className="w-4 h-4 text-rose-400" />;
    case 'ready':
      return <Play className="w-4 h-4 text-blue-400" />;
    case 'skipped':
      return <SkipForward className="w-4 h-4 text-slate-500" />;
    case 'cancelled':
      return <XCircle className="w-4 h-4 text-slate-500" />;
    default:
      return <Circle className="w-4 h-4 text-slate-600" />;
  }
};

const NodeCard = ({ data, selected }: NodeCardProps) => {
  const isRunning = data.status === 'running';
  const isRetrying = data.retrying === true;

  return (
    <div className={cn(
      "group relative min-w-[240px] rounded-xl border bg-slate-900/90 p-4 transition-all duration-200 backdrop-blur-md",
      selected ? "border-brand-cyan ring-1 ring-brand-cyan shadow-[0_0_20px_rgba(103,232,249,0.15)]" : "border-slate-800 hover:border-slate-700 shadow-xl",
      data.status === 'failed' && "border-rose-500/50",
      data.status === 'succeeded' && "border-emerald-500/30"
    )}>
      <Handle type="target" position={Position.Left} className="!bg-slate-600 !w-2 !h-2" />
      
      <div className="flex items-start justify-between mb-3">
        <div>
          <h3 className="text-xs font-medium text-slate-400 uppercase tracking-wider">{data.label}</h3>
          <p className="text-sm font-semibold text-slate-100">{data.chineseLabel}</p>
        </div>
        <StatusIcon status={data.status} />
      </div>

      <div className="space-y-3">
        <p className="text-xs text-slate-400 line-clamp-1">{data.summary}</p>
        
        {isRunning && typeof data.progress === 'number' && (
          <div className="space-y-1.5">
            <div className="flex justify-between text-[10px] text-slate-500">
              <span>Progress</span>
              <span>{data.progress}%</span>
            </div>
            <div className="h-1 w-full bg-slate-800 rounded-full overflow-hidden">
              <motion.div 
                className="h-full bg-brand-cyan"
                initial={{ width: 0 }}
                animate={{ width: `${data.progress}%` }}
                transition={{ duration: 0.5 }}
              />
            </div>
          </div>
        )}
      </div>

      <div className="mt-4 flex items-center gap-2 pt-3 border-t border-slate-800 opacity-0 group-hover:opacity-100 transition-opacity">
        <button
          onClick={(event) => {
            event.stopPropagation();
            data.onView?.();
          }}
          className="text-[10px] px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
        >
          View
        </button>
        <button
          onClick={(event) => {
            event.stopPropagation();
            data.onEdit?.();
          }}
          className="text-[10px] px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
        >
          Edit
        </button>
        {data.status === 'failed' && (
          <button
            onClick={(event) => {
              event.stopPropagation();
              if (!isRetrying) {
                data.onRetry?.();
              }
            }}
            disabled={isRetrying}
            className="text-[10px] px-2 py-1 rounded bg-rose-900/30 hover:bg-rose-900/50 text-rose-300 transition-colors disabled:cursor-not-allowed disabled:opacity-60"
          >
            {isRetrying ? 'Retrying...' : 'Retry'}
          </button>
        )}
      </div>

      <Handle type="source" position={Position.Right} className="!bg-slate-600 !w-2 !h-2" />
    </div>
  );
};

export default memo(NodeCard);
