import React from 'react';
import { 
  Info, 
  FileText, 
  Play, 
  Settings, 
  ChevronRight, 
  Clock, 
  CheckCircle2, 
  AlertCircle,
  Download,
  ExternalLink,
  History
} from 'lucide-react';
import { WorkflowNode } from '../../types/workflow';
import { cn } from '../../lib/utils';
import { motion, AnimatePresence } from 'motion/react';

interface NodeInspectorProps {
  node: WorkflowNode | null;
  onClose: () => void;
}

const NodeInspector = ({ node, onClose }: NodeInspectorProps) => {
  if (!node) return null;

  const tabs = ['DETAILS', 'ARTIFACTS', 'ACTIONS'];
  const [activeTab, setActiveTab] = React.useState('DETAILS');

  return (
    <motion.div 
      initial={{ x: 400 }}
      animate={{ x: 0 }}
      exit={{ x: 400 }}
      className="w-[400px] h-full bg-slate-900 border-l border-slate-800 flex flex-col shadow-2xl z-10"
    >
      <div className="p-6 border-b border-slate-800">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-slate-100">Node Inspector</h2>
          <button onClick={onClose} className="text-slate-500 hover:text-slate-300">
            <ChevronRight className="w-5 h-5" />
          </button>
        </div>
        <p className="text-xs text-slate-500 uppercase tracking-widest font-medium">Precision Configuration</p>
      </div>

      <div className="flex border-b border-slate-800 px-6">
        {tabs.map(tab => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={cn(
              "py-4 px-4 text-xs font-bold tracking-widest transition-all relative",
              activeTab === tab ? "text-brand-cyan" : "text-slate-500 hover:text-slate-300"
            )}
          >
            {tab}
            {activeTab === tab && (
              <motion.div 
                layoutId="activeTab"
                className="absolute bottom-0 left-0 right-0 h-0.5 bg-brand-cyan"
              />
            )}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto p-6 space-y-8">
        <AnimatePresence mode="wait">
          {activeTab === 'DETAILS' && (
            <motion.div
              key="details"
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -10 }}
              className="space-y-8"
            >
              {/* Selected Node Header */}
              <div className="bg-slate-800/50 rounded-xl p-4 border border-slate-700/50 flex items-center gap-4">
                <div className="w-12 h-12 rounded-lg bg-slate-800 flex items-center justify-center border border-slate-700">
                  <FileText className="w-6 h-6 text-brand-cyan" />
                </div>
                <div>
                  <h3 className="font-bold text-slate-100">{node.chineseLabel}</h3>
                  <p className="text-xs text-slate-500">{node.label} v2.4.0-alpha</p>
                </div>
              </div>

              {/* Final Video Preview (Specific to Video Node) */}
              {node.id === 'video' && (
                <div className="space-y-4">
                  <h4 className="text-xs font-bold text-slate-500 uppercase tracking-widest">Final Output Preview</h4>
                  <div className="bg-slate-950 border border-slate-800 rounded-xl p-1 shadow-inner group overflow-hidden">
                    <div className="relative aspect-video rounded-lg overflow-hidden bg-slate-900">
                      <img 
                        src="https://picsum.photos/seed/cyberpunk/640/360" 
                        alt="Final Video Preview" 
                        className="w-full h-full object-cover opacity-70 group-hover:scale-105 transition-transform duration-700"
                        referrerPolicy="no-referrer"
                      />
                      <div className="absolute inset-0 flex items-center justify-center">
                        <button className="w-10 h-10 bg-brand-cyan rounded-full flex items-center justify-center shadow-lg shadow-cyan-500/40 hover:scale-110 transition-transform">
                          <Play className="w-5 h-5 text-slate-950 fill-current ml-0.5" />
                        </button>
                      </div>
                      <div className="absolute bottom-2 left-2 right-2 flex items-center justify-between">
                        <span className="px-1.5 py-0.5 bg-brand-cyan/20 border border-brand-cyan/30 text-brand-cyan text-[8px] font-bold rounded">MASTER</span>
                        <span className="text-[8px] text-slate-400 font-mono">02:45</span>
                      </div>
                    </div>
                  </div>
                  <button className="w-full py-2.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg font-bold text-[10px] tracking-widest transition-colors flex items-center justify-center gap-2">
                    <ExternalLink className="w-3 h-3" />
                    OPEN IN FULLSCREEN
                  </button>
                </div>
              )}

              {/* Progress Section */}
              {node.status === 'running' && (
                <div className="space-y-4">
                  <h4 className="text-xs font-bold text-slate-500 uppercase tracking-widest">Live Progress</h4>
                  <div className="space-y-2">
                    <div className="flex justify-between text-xs">
                      <span className="text-slate-400">Context Window</span>
                      <span className="text-brand-cyan font-mono">{node.progress}% Full</span>
                    </div>
                    <div className="h-1.5 w-full bg-slate-800 rounded-full overflow-hidden">
                      <div className="h-full bg-brand-cyan w-[82%]" />
                    </div>
                  </div>
                  <div className="flex justify-between text-xs">
                    <span className="text-slate-400">Tokens Generated</span>
                    <span className="text-slate-200 font-mono">14.2k</span>
                  </div>
                </div>
              )}

              {/* Status & Dependencies */}
              <div className="space-y-4">
                <h4 className="text-xs font-bold text-slate-500 uppercase tracking-widest">Node Context</h4>
                <div className="grid grid-cols-2 gap-4">
                  <div className="bg-slate-800/30 p-3 rounded-lg border border-slate-800">
                    <span className="text-[10px] text-slate-500 block mb-1">STATUS</span>
                    <div className="flex items-center gap-2">
                      <div className={cn(
                        "w-2 h-2 rounded-full",
                        node.status === 'succeeded' ? "bg-emerald-400" : 
                        node.status === 'running' ? "bg-brand-cyan" : "bg-slate-600"
                      )} />
                      <span className="text-xs font-medium capitalize">{node.status}</span>
                    </div>
                  </div>
                  <div className="bg-slate-800/30 p-3 rounded-lg border border-slate-800">
                    <span className="text-[10px] text-slate-500 block mb-1">LAST UPDATE</span>
                    <div className="flex items-center gap-2">
                      <Clock className="w-3 h-3 text-slate-500" />
                      <span className="text-xs font-medium">12m ago</span>
                    </div>
                  </div>
                </div>
              </div>

              {/* Logs */}
              <div className="space-y-4">
                <h4 className="text-xs font-bold text-slate-500 uppercase tracking-widest">Execution Log</h4>
                <div className="space-y-3">
                  {[
                    { msg: 'Input validation complete', status: 'success' },
                    { msg: 'Upstream dependencies resolved', status: 'success' },
                    { msg: 'Processing chunk 4/12...', status: 'running' },
                  ].map((log, i) => (
                    <div key={i} className="flex items-start gap-3">
                      {log.status === 'success' ? (
                        <CheckCircle2 className="w-4 h-4 text-emerald-500 mt-0.5" />
                      ) : (
                        <div className="w-4 h-4 rounded-full border-2 border-brand-cyan border-t-transparent animate-spin mt-0.5" />
                      )}
                      <span className="text-xs text-slate-300">{log.msg}</span>
                    </div>
                  ))}
                </div>
              </div>
            </motion.div>
          )}

          {activeTab === 'ARTIFACTS' && (
            <motion.div
              key="artifacts"
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              className="space-y-6"
            >
              <div className="bg-slate-800/50 rounded-xl p-6 border border-slate-700/50 text-center">
                <div className="w-16 h-16 bg-slate-900 rounded-full flex items-center justify-center mx-auto mb-4 border border-slate-800">
                  <FileText className="w-8 h-8 text-slate-600" />
                </div>
                <h3 className="text-slate-200 font-bold mb-2">No Artifacts Yet</h3>
                <p className="text-xs text-slate-500 leading-relaxed">
                  Artifacts will appear here once the node execution completes successfully.
                </p>
              </div>
            </motion.div>
          )}

          {activeTab === 'ACTIONS' && (
            <motion.div
              key="actions"
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              className="space-y-4"
            >
              <button className="w-full flex items-center justify-between p-4 bg-slate-800/50 hover:bg-slate-800 rounded-xl border border-slate-700/50 transition-colors group">
                <div className="flex items-center gap-3">
                  <History className="w-5 h-5 text-slate-400 group-hover:text-brand-cyan" />
                  <div className="text-left">
                    <span className="block text-sm font-bold text-slate-200">View History</span>
                    <span className="text-[10px] text-slate-500">Compare previous runs</span>
                  </div>
                </div>
                <ChevronRight className="w-4 h-4 text-slate-600" />
              </button>

              <button className="w-full flex items-center justify-between p-4 bg-slate-800/50 hover:bg-slate-800 rounded-xl border border-slate-700/50 transition-colors group">
                <div className="flex items-center gap-3">
                  <Download className="w-5 h-5 text-slate-400 group-hover:text-brand-cyan" />
                  <div className="text-left">
                    <span className="block text-sm font-bold text-slate-200">Export Schema</span>
                    <span className="text-[10px] text-slate-500">Download node configuration</span>
                  </div>
                </div>
                <ChevronRight className="w-4 h-4 text-slate-600" />
              </button>

              <div className="pt-4 space-y-3">
                <button className="w-full py-3 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg font-bold text-xs tracking-widest transition-colors">
                  PAUSE PROCESS
                </button>
                <button className="w-full py-3 bg-rose-900/20 hover:bg-rose-900/40 text-rose-400 rounded-lg font-bold text-xs tracking-widest transition-colors border border-rose-900/30">
                  ABORT NODE
                </button>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </motion.div>
  );
};

export default NodeInspector;
