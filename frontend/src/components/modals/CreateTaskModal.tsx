import React from 'react';
import {X, Play, Info, ChevronDown, Monitor, Smartphone, Type, Image as ImageIcon, Volume2, FolderPen} from 'lucide-react';
import {motion, AnimatePresence} from 'motion/react';
import {VOICE_PRESETS, IMAGE_STYLES} from '../../constants/workflow';
import type {CreateTaskParams} from '../../types/workflow';
import {cn} from '../../lib/utils';

interface CreateTaskModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreate: (params: CreateTaskParams) => Promise<void> | void;
  submitting?: boolean;
  error?: string | null;
}

const DEFAULT_VOICE_ID = VOICE_PRESETS[0]?.id ?? 'male_calm';
const DEFAULT_IMAGE_STYLE = IMAGE_STYLES[0]?.id ?? 'realistic';

const CreateTaskModal = ({
  isOpen,
  onClose,
  onCreate,
  submitting = false,
  error = null,
}: CreateTaskModalProps) => {
  const [name, setName] = React.useState('');
  const [article, setArticle] = React.useState('');
  const [voicePreset, setVoicePreset] = React.useState(DEFAULT_VOICE_ID);
  const [imageStyle, setImageStyle] = React.useState(DEFAULT_IMAGE_STYLE);
  const [aspectRatio, setAspectRatio] = React.useState<'16:9' | '9:16'>('16:9');
  const [videoCount, setVideoCount] = React.useState(12);

  React.useEffect(() => {
    if (!isOpen) {
      return;
    }
    setName('');
    setArticle('');
    setVoicePreset(DEFAULT_VOICE_ID);
    setImageStyle(DEFAULT_IMAGE_STYLE);
    setAspectRatio('16:9');
    setVideoCount(12);
  }, [isOpen]);

  const articleEmpty = article.trim() === '';

  if (!isOpen) return null;

  return (
    <AnimatePresence>
      <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onClick={onClose}
          className="absolute inset-0 bg-slate-950/80 backdrop-blur-sm"
        />
        
        <motion.div
          initial={{ opacity: 0, scale: 0.95, y: 20 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          exit={{ opacity: 0, scale: 0.95, y: 20 }}
          className="relative w-full max-w-2xl bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl overflow-hidden"
        >
          {/* Header */}
          <div className="p-6 border-b border-slate-800 flex items-center justify-between">
            <div>
              <h2 className="text-xl font-bold text-slate-100">Create New Task</h2>
              <p className="text-xs text-slate-500 uppercase tracking-widest mt-1">Configure your generation parameters</p>
            </div>
            <button onClick={onClose} className="p-2 hover:bg-slate-800 rounded-full text-slate-500 transition-colors">
              <X className="w-5 h-5" />
            </button>
          </div>

          <div className="p-8 space-y-8 max-h-[70vh] overflow-y-auto">
            {/* Name Input */}
            <div className="space-y-3">
              <div className="flex items-center gap-2 text-xs font-bold text-slate-500 uppercase tracking-widest">
                <FolderPen className="w-3.5 h-3.5" />
                <span>Project Name</span>
              </div>
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Optional. Defaults to the first 10 characters of the article."
                className="w-full bg-slate-950 border border-slate-800 rounded-xl px-4 py-3 text-sm text-slate-200 placeholder:text-slate-700 focus:outline-none focus:border-brand-cyan/50 transition-colors"
              />
            </div>

            {/* Article Input */}
            <div className="space-y-3">
              <div className="flex items-center gap-2 text-xs font-bold text-slate-500 uppercase tracking-widest">
                <Type className="w-3.5 h-3.5" />
                <span>Story Article / Prompt</span>
              </div>
              <textarea
                value={article}
                onChange={(e) => setArticle(e.target.value)}
                placeholder="Describe the sequence or paste your script segment here..."
                className="w-full h-32 bg-slate-950 border border-slate-800 rounded-xl p-4 text-sm text-slate-200 placeholder:text-slate-700 focus:outline-none focus:border-brand-cyan/50 transition-colors resize-none"
              />
            </div>

            <div className="grid grid-cols-2 gap-8">
              {/* Voice Preset */}
              <div className="space-y-3">
                <div className="flex items-center gap-2 text-xs font-bold text-slate-500 uppercase tracking-widest">
                  <Volume2 className="w-3.5 h-3.5" />
                  <span>Voice Preset</span>
                </div>
                <div className="relative">
                  <select
                    value={voicePreset}
                    onChange={(e) => setVoicePreset(e.target.value)}
                    className="w-full bg-slate-950 border border-slate-800 rounded-xl px-4 py-3 text-sm text-slate-200 appearance-none focus:outline-none focus:border-brand-cyan/50 transition-colors"
                  >
                    {VOICE_PRESETS.map(v => (
                      <option key={v.id} value={v.id}>{v.name}</option>
                    ))}
                  </select>
                  <ChevronDown className="w-4 h-4 absolute right-4 top-1/2 -translate-y-1/2 text-slate-600 pointer-events-none" />
                </div>
              </div>

              {/* Visual Style */}
              <div className="space-y-3">
                <div className="flex items-center gap-2 text-xs font-bold text-slate-500 uppercase tracking-widest">
                  <ImageIcon className="w-3.5 h-3.5" />
                  <span>Visual Style</span>
                </div>
                <div className="relative">
                  <select
                    value={imageStyle}
                    onChange={(e) => setImageStyle(e.target.value)}
                    className="w-full bg-slate-950 border border-slate-800 rounded-xl px-4 py-3 text-sm text-slate-200 appearance-none focus:outline-none focus:border-brand-cyan/50 transition-colors"
                  >
                    {IMAGE_STYLES.map((style) => (
                      <option key={style.id} value={style.id}>{style.name}</option>
                    ))}
                  </select>
                  <ChevronDown className="w-4 h-4 absolute right-4 top-1/2 -translate-y-1/2 text-slate-600 pointer-events-none" />
                </div>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-8">
              {/* Aspect Ratio */}
              <div className="space-y-3">
                <div className="flex items-center gap-2 text-xs font-bold text-slate-500 uppercase tracking-widest">
                  <Monitor className="w-3.5 h-3.5" />
                  <span>Aspect Ratio</span>
                </div>
                <div className="flex gap-4">
                  <button
                    onClick={() => setAspectRatio('16:9')}
                    className={cn(
                      "flex-1 aspect-video rounded-xl border flex flex-col items-center justify-center gap-2 transition-all",
                      aspectRatio === '16:9' 
                        ? "bg-brand-cyan/5 border-brand-cyan text-brand-cyan" 
                        : "bg-slate-950 border-slate-800 text-slate-600 hover:border-slate-700"
                    )}
                  >
                    <div className="w-8 h-4 border-2 border-current rounded-sm" />
                    <span className="text-[10px] font-bold">16:9</span>
                  </button>
                  <button
                    onClick={() => setAspectRatio('9:16')}
                    className={cn(
                      "flex-1 aspect-video rounded-xl border flex flex-col items-center justify-center gap-2 transition-all",
                      aspectRatio === '9:16' 
                        ? "bg-brand-cyan/5 border-brand-cyan text-brand-cyan" 
                        : "bg-slate-950 border-slate-800 text-slate-600 hover:border-slate-700"
                    )}
                  >
                    <div className="w-4 h-8 border-2 border-current rounded-sm" />
                    <span className="text-[10px] font-bold">9:16</span>
                  </button>
                </div>
              </div>

              {/* Segment Count */}
              <div className="space-y-3">
                <div className="flex items-center gap-2 text-xs font-bold text-slate-500 uppercase tracking-widest">
                  <Smartphone className="w-3.5 h-3.5" />
                  <span>Video Count</span>
                </div>
                <div className="flex items-center gap-4 bg-slate-950 border border-slate-800 rounded-xl p-2">
                  <button 
                    onClick={() => setVideoCount(Math.max(1, videoCount - 1))}
                    className="w-10 h-10 flex items-center justify-center hover:bg-slate-900 rounded-lg text-slate-400 transition-colors"
                  >
                    -
                  </button>
                  <span className="flex-1 text-center font-mono font-bold text-slate-100">{videoCount}</span>
                  <button 
                    onClick={() => setVideoCount(videoCount + 1)}
                    className="w-10 h-10 flex items-center justify-center hover:bg-slate-900 rounded-lg text-slate-400 transition-colors"
                  >
                    +
                  </button>
                </div>
                <p className="text-[10px] text-slate-600 flex items-center gap-1">
                  <Info className="w-3 h-3" />
                  Only the first {videoCount} shots will attempt video generation.
                </p>
              </div>
            </div>
          </div>

          {/* Footer */}
          <div className="p-6 border-t border-slate-800 bg-slate-900/50 flex items-center justify-end gap-4">
            {error && (
              <div className="mr-auto max-w-md text-xs text-rose-400">
                {error}
              </div>
            )}
            <button 
              onClick={onClose}
              disabled={submitting}
              className="px-6 py-2 text-xs font-bold tracking-widest text-slate-500 hover:text-slate-300 transition-colors"
            >
              CANCEL
            </button>
            <button
              disabled={submitting || articleEmpty}
              onClick={() => onCreate({
                name: name.trim() || undefined,
                article,
                voicePreset,
                imageStyle,
                aspectRatio,
                videoCount,
              })}
              className="bg-brand-cyan hover:bg-cyan-400 disabled:bg-slate-700 disabled:text-slate-400 disabled:shadow-none text-slate-950 px-8 py-3 rounded-xl font-bold text-sm transition-all shadow-lg shadow-cyan-500/20 flex items-center gap-2"
            >
              {submitting ? 'CREATING...' : 'START TASK'}
              <Play className="w-4 h-4 fill-current" />
            </button>
          </div>
        </motion.div>
      </div>
    </AnimatePresence>
  );
};

export default CreateTaskModal;
