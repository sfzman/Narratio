import React from 'react';
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Database,
  Download,
  FileCode2,
  FileOutput,
  Link2,
  LoaderCircle,
  Package,
  PlayCircle,
  XCircle,
} from 'lucide-react';
import {AnimatePresence, motion} from 'motion/react';
import {WorkflowNode} from '../../types/workflow';
import {cn} from '../../lib/utils';
import type {ArtifactEntryResponse, JobSummaryResponse} from '../../types/api';
import {getJobArtifact, getJobDownloadUrl} from '../../lib/jobs';

interface NodeInspectorProps {
  node: WorkflowNode | null;
  jobSummary: JobSummaryResponse | null;
  onClose: () => void;
}

function formatStatus(status: WorkflowNode['status']): string {
  return status.replace(/_/g, ' ').toUpperCase();
}

function formatTimestamp(value: string): string {
  if (!value) {
    return '-';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function stringifyValue(value: unknown): string {
  if (value == null) {
    return '-';
  }
  if (typeof value === 'string') {
    return value;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function formatDuration(seconds?: number): string {
  if (typeof seconds !== 'number' || Number.isNaN(seconds)) {
    return '-';
  }
  if (seconds < 60) {
    return `${seconds.toFixed(1)}s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remain = seconds % 60;
  return `${minutes}m ${remain.toFixed(1)}s`;
}

function formatFileSize(bytes?: number): string {
  if (typeof bytes !== 'number' || Number.isNaN(bytes) || bytes < 0) {
    return '-';
  }
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  const units = ['KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unitIndex = -1;
  do {
    value /= 1024;
    unitIndex += 1;
  } while (value >= 1024 && unitIndex < units.length - 1);
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[unitIndex]}`;
}

function buildVideoPreviewUrl(jobSummary: JobSummaryResponse | null): string | null {
  if (!jobSummary || jobSummary.status !== 'completed' || !jobSummary.result) {
    return null;
  }
  return getJobDownloadUrl(jobSummary.job_id);
}

function summarizeArtifacts(artifacts?: Record<string, unknown>): Array<{key: string; value: string}> {
  if (!artifacts) {
    return [];
  }

  return Object.entries(artifacts).map(([key, value]) => ({
    key,
    value: stringifyValue(value),
  }));
}

function statusTone(status: WorkflowNode['status']): string {
  switch (status) {
    case 'succeeded':
      return 'text-emerald-300 border-emerald-500/30 bg-emerald-500/10';
    case 'running':
      return 'text-cyan-300 border-cyan-500/30 bg-cyan-500/10';
    case 'failed':
      return 'text-rose-300 border-rose-500/30 bg-rose-500/10';
    case 'cancelled':
    case 'skipped':
      return 'text-slate-300 border-slate-600/40 bg-slate-700/20';
    case 'ready':
      return 'text-amber-300 border-amber-500/30 bg-amber-500/10';
    default:
      return 'text-slate-300 border-slate-700 bg-slate-800/40';
  }
}

function StatusIcon({status}: {status: WorkflowNode['status']}) {
  switch (status) {
    case 'succeeded':
      return <CheckCircle2 className="h-4 w-4" />;
    case 'running':
      return <LoaderCircle className="h-4 w-4 animate-spin" />;
    case 'failed':
      return <XCircle className="h-4 w-4" />;
    default:
      return <AlertCircle className="h-4 w-4" />;
  }
}

function Section({
  title,
  icon,
  children,
}: {
  title: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className="space-y-3">
      <div className="flex items-center gap-2 text-[11px] font-bold uppercase tracking-widest text-slate-500">
        <span className="text-slate-600">{icon}</span>
        <span>{title}</span>
      </div>
      {children}
    </section>
  );
}

function KeyValueGrid({
  items,
}: {
  items: Array<{label: string; value: string}>;
}) {
  return (
    <div className="grid grid-cols-2 gap-3">
      {items.map((item) => (
        <div key={item.label} className="rounded-xl border border-slate-800 bg-slate-800/30 p-3">
          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">{item.label}</div>
          <div className="break-all text-xs text-slate-200">{item.value}</div>
        </div>
      ))}
    </div>
  );
}

function CodeBlock({value}: {value: string}) {
  return (
    <pre className="max-h-64 overflow-auto rounded-xl border border-slate-800 bg-slate-950 p-3 text-[11px] leading-5 text-slate-300 whitespace-pre-wrap break-all">
      {value}
    </pre>
  );
}

interface ScriptShotArtifact {
  index: number;
  visual_content?: string;
  camera_design?: string;
  involved_characters?: string[];
  image_to_image_prompt?: string;
  text_to_image_prompt?: string;
}

interface ScriptSegmentArtifact {
  index: number;
  shots: ScriptShotArtifact[];
}

function isScriptShotArtifact(value: unknown): value is ScriptShotArtifact {
  return Boolean(value) && typeof value === 'object' && 'index' in value && typeof (value as {index?: unknown}).index === 'number';
}

function isScriptSegmentArtifact(value: unknown): value is ScriptSegmentArtifact {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as {index?: unknown; shots?: unknown};
  return typeof candidate.index === 'number' && Array.isArray(candidate.shots) && candidate.shots.every(isScriptShotArtifact);
}

interface ImageCharacterReferenceArtifact {
  character_index?: number;
  character_name?: string;
  file_path?: string;
  prompt?: string;
  match_terms?: string[];
  source_image_url?: string;
}

interface GeneratedShotImageArtifact {
  segment_index: number;
  shot_index: number;
  file_path?: string;
  width?: number;
  height?: number;
  prompt?: string;
  prompt_type?: string;
  is_fallback?: boolean;
  filled_from_previous?: boolean;
  generation_request_id?: string;
  generation_model?: string;
  source_image_url?: string;
  involved_characters?: string[];
  character_references?: ImageCharacterReferenceArtifact[];
  matched_characters?: ImageCharacterReferenceArtifact[];
}

interface ImageArtifact {
  images?: unknown[];
  shot_images: GeneratedShotImageArtifact[];
}

interface GeneratedShotVideoArtifact {
  segment_index: number;
  shot_index: number;
  status?: string;
  duration_seconds?: number;
  video_path?: string;
  image_path?: string;
  source_image_path?: string;
  source_type?: string;
  is_fallback?: boolean;
  generation_request_id?: string;
  generation_model?: string;
  source_video_url?: string;
}

interface ShotVideoArtifact {
  clips: GeneratedShotVideoArtifact[];
}

interface TTSAudioSegmentArtifact {
  segment_index: number;
  file_path?: string;
  duration?: number;
}

interface TTSSubtitleItemArtifact {
  segment_index: number;
  start?: number;
  end?: number;
  text?: string;
}

interface TTSArtifact {
  audio_segments: TTSAudioSegmentArtifact[];
  subtitle_items: TTSSubtitleItemArtifact[];
  total_duration_seconds?: number;
}

function isImageCharacterReferenceArtifact(value: unknown): value is ImageCharacterReferenceArtifact {
  return Boolean(value) && typeof value === 'object';
}

function isGeneratedShotImageArtifact(value: unknown): value is GeneratedShotImageArtifact {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as {segment_index?: unknown; shot_index?: unknown};
  return typeof candidate.segment_index === 'number' && typeof candidate.shot_index === 'number';
}

function isImageArtifact(value: unknown): value is ImageArtifact {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as {shot_images?: unknown};
  return Array.isArray(candidate.shot_images) && candidate.shot_images.every(isGeneratedShotImageArtifact);
}

function isGeneratedShotVideoArtifact(value: unknown): value is GeneratedShotVideoArtifact {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as {segment_index?: unknown; shot_index?: unknown};
  return typeof candidate.segment_index === 'number' && typeof candidate.shot_index === 'number';
}

function isShotVideoArtifact(value: unknown): value is ShotVideoArtifact {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as {clips?: unknown};
  return Array.isArray(candidate.clips) && candidate.clips.every(isGeneratedShotVideoArtifact);
}

function isTTSAudioSegmentArtifact(value: unknown): value is TTSAudioSegmentArtifact {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as {segment_index?: unknown};
  return typeof candidate.segment_index === 'number';
}

function isTTSSubtitleItemArtifact(value: unknown): value is TTSSubtitleItemArtifact {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as {segment_index?: unknown};
  return typeof candidate.segment_index === 'number';
}

function isTTSArtifact(value: unknown): value is TTSArtifact {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as {audio_segments?: unknown; subtitle_items?: unknown};
  return (
    Array.isArray(candidate.audio_segments) &&
    candidate.audio_segments.every(isTTSAudioSegmentArtifact) &&
    Array.isArray(candidate.subtitle_items) &&
    candidate.subtitle_items.every(isTTSSubtitleItemArtifact)
  );
}

const NodeInspector = ({node, jobSummary, onClose}: NodeInspectorProps) => {
  if (!node) return null;

  const artifactEntries = summarizeArtifacts(node.artifacts);
  const isVideoNode = node.id === 'video';
  const isImageNode = node.id === 'image';
  const isShotVideoNode = node.id === 'shot_video';
  const isTTSNode = node.id === 'tts';
  const isScriptNode = node.id === 'script';
  const videoResult = isVideoNode ? jobSummary?.result ?? null : null;
  const videoArtifactPath = typeof node.artifacts?.artifact_path === 'string' ? node.artifacts.artifact_path : '-';
  const downloadUrl = jobSummary ? getJobDownloadUrl(jobSummary.job_id) : null;
  const videoPreviewUrl = buildVideoPreviewUrl(jobSummary);
  const imageArtifactPath = typeof node.artifacts?.artifact_path === 'string' ? node.artifacts.artifact_path : '-';
  const imageCount = typeof node.artifacts?.image_count === 'number' ? node.artifacts.image_count : null;
  const shotImageCount = typeof node.artifacts?.shot_image_count === 'number' ? node.artifacts.shot_image_count : null;
  const generatedImageCount = typeof node.artifacts?.generated_image_count === 'number' ? node.artifacts.generated_image_count : null;
  const fallbackImageCount = typeof node.artifacts?.fallback_image_count === 'number' ? node.artifacts.fallback_image_count : null;
  const imageScriptRef = typeof node.artifacts?.script_artifact_ref === 'string' ? node.artifacts.script_artifact_ref : null;
  const imageCharacterRef = typeof node.artifacts?.character_image_artifact_ref === 'string' ? node.artifacts.character_image_artifact_ref : null;
  const shotVideoArtifactPath = typeof node.artifacts?.artifact_path === 'string' ? node.artifacts.artifact_path : '-';
  const clipCount = typeof node.artifacts?.clip_count === 'number' ? node.artifacts.clip_count : null;
  const generatedVideoCount = typeof node.artifacts?.generated_video_count === 'number' ? node.artifacts.generated_video_count : null;
  const shotVideoFallbackCount = typeof node.artifacts?.fallback_image_count === 'number' ? node.artifacts.fallback_image_count : null;
  const requestedVideoCount = typeof node.artifacts?.requested_video_count === 'number' ? node.artifacts.requested_video_count : null;
  const selectedVideoCount = typeof node.artifacts?.selected_video_count === 'number' ? node.artifacts.selected_video_count : null;
  const shotVideoGenerationMode = typeof node.artifacts?.generation_mode === 'string' ? node.artifacts.generation_mode : null;
  const ttsArtifactPath = typeof node.artifacts?.artifact_path === 'string' ? node.artifacts.artifact_path : '-';
  const ttsGenerationMode = typeof node.artifacts?.generation_mode === 'string' ? node.artifacts.generation_mode : null;
  const ttsSegmentCount = typeof node.artifacts?.segment_count === 'number' ? node.artifacts.segment_count : null;
  const ttsSegmentationRef = typeof node.artifacts?.segmentation_artifact_ref === 'string' ? node.artifacts.segmentation_artifact_ref : null;
  const scriptArtifactPath = typeof node.artifacts?.artifact_path === 'string' ? node.artifacts.artifact_path : '-';
  const scriptSegmentArtifactDir = typeof node.artifacts?.segment_artifact_dir === 'string' ? node.artifacts.segment_artifact_dir : null;
  const scriptSegmentCount = typeof node.artifacts?.segment_count === 'number' ? node.artifacts.segment_count : null;
  const scriptSegmentationRef = typeof node.artifacts?.segmentation_ref === 'string' ? node.artifacts.segmentation_ref : null;
  const scriptOutlineRef = typeof node.artifacts?.outline_artifact_ref === 'string' ? node.artifacts.outline_artifact_ref : null;
  const scriptCharacterRef = typeof node.artifacts?.character_ref === 'string' ? node.artifacts.character_ref : null;
  const tabs = isVideoNode || isImageNode || isShotVideoNode || isTTSNode || isScriptNode ? ['OUTPUT', 'DETAILS'] : ['DETAILS'];
  const [activeTab, setActiveTab] = React.useState<'OUTPUT' | 'DETAILS'>(
    isVideoNode || isImageNode || isShotVideoNode || isTTSNode || isScriptNode ? 'OUTPUT' : 'DETAILS',
  );
  const [scriptEntries, setScriptEntries] = React.useState<ArtifactEntryResponse[]>([]);
  const [scriptEntriesLoading, setScriptEntriesLoading] = React.useState(false);
  const [scriptEntriesError, setScriptEntriesError] = React.useState<string | null>(null);
  const [selectedScriptPath, setSelectedScriptPath] = React.useState<string | null>(null);
  const [selectedScriptContent, setSelectedScriptContent] = React.useState<string>('');
  const [selectedScriptJSON, setSelectedScriptJSON] = React.useState<unknown>(null);
  const [selectedScriptLoading, setSelectedScriptLoading] = React.useState(false);
  const [selectedScriptError, setSelectedScriptError] = React.useState<string | null>(null);
  const [imageArtifactJSON, setImageArtifactJSON] = React.useState<unknown>(null);
  const [imageArtifactLoading, setImageArtifactLoading] = React.useState(false);
  const [imageArtifactError, setImageArtifactError] = React.useState<string | null>(null);
  const [selectedImageShotKey, setSelectedImageShotKey] = React.useState<string | null>(null);
  const [shotVideoArtifactJSON, setShotVideoArtifactJSON] = React.useState<unknown>(null);
  const [shotVideoArtifactLoading, setShotVideoArtifactLoading] = React.useState(false);
  const [shotVideoArtifactError, setShotVideoArtifactError] = React.useState<string | null>(null);
  const [selectedShotVideoClipKey, setSelectedShotVideoClipKey] = React.useState<string | null>(null);
  const [ttsArtifactJSON, setTTSArtifactJSON] = React.useState<unknown>(null);
  const [ttsArtifactLoading, setTTSArtifactLoading] = React.useState(false);
  const [ttsArtifactError, setTTSArtifactError] = React.useState<string | null>(null);
  const imageArtifact = isImageArtifact(imageArtifactJSON) ? imageArtifactJSON : null;
  const selectedImageShot = imageArtifact?.shot_images.find(
    (shot) => `${shot.segment_index}:${shot.shot_index}` === selectedImageShotKey,
  ) ?? null;
  const shotVideoArtifact = isShotVideoArtifact(shotVideoArtifactJSON) ? shotVideoArtifactJSON : null;
  const selectedShotVideoClip = shotVideoArtifact?.clips.find(
    (clip) => `${clip.segment_index}:${clip.shot_index}` === selectedShotVideoClipKey,
  ) ?? null;
  const ttsArtifact = isTTSArtifact(ttsArtifactJSON) ? ttsArtifactJSON : null;

  React.useEffect(() => {
    setActiveTab(isVideoNode || isImageNode || isShotVideoNode || isTTSNode || isScriptNode ? 'OUTPUT' : 'DETAILS');
  }, [isImageNode, isScriptNode, isShotVideoNode, isTTSNode, isVideoNode, node.id]);

  React.useEffect(() => {
    let cancelled = false;

    async function loadImageArtifact() {
      if (!isImageNode || !jobSummary || !imageArtifactPath || imageArtifactPath === '-' || activeTab !== 'OUTPUT') {
        setImageArtifactJSON(null);
        setImageArtifactError(null);
        setSelectedImageShotKey(null);
        return;
      }

      setImageArtifactLoading(true);
      setImageArtifactError(null);
      try {
        const artifact = await getJobArtifact(jobSummary.job_id, imageArtifactPath);
        if (cancelled) {
          return;
        }
        if (artifact.kind !== 'json') {
          setImageArtifactJSON(null);
          setImageArtifactError('Image artifact did not return a JSON response.');
          return;
        }
        const parsed = artifact.json;
        setImageArtifactJSON(parsed);
        if (isImageArtifact(parsed) && parsed.shot_images.length > 0) {
          setSelectedImageShotKey((current) => {
            if (
              current &&
              parsed.shot_images.some((shot) => `${shot.segment_index}:${shot.shot_index}` === current)
            ) {
              return current;
            }
            const first = parsed.shot_images[0];
            return `${first.segment_index}:${first.shot_index}`;
          });
        } else {
          setSelectedImageShotKey(null);
        }
      } catch (error) {
        if (cancelled) {
          return;
        }
        setImageArtifactJSON(null);
        setImageArtifactError(error instanceof Error ? error.message : 'Failed to load image artifact.');
      } finally {
        if (!cancelled) {
          setImageArtifactLoading(false);
        }
      }
    }

    void loadImageArtifact();
    return () => {
      cancelled = true;
    };
  }, [activeTab, imageArtifactPath, isImageNode, jobSummary]);

  React.useEffect(() => {
    let cancelled = false;

    async function loadShotVideoArtifact() {
      if (!isShotVideoNode || !jobSummary || !shotVideoArtifactPath || shotVideoArtifactPath === '-' || activeTab !== 'OUTPUT') {
        setShotVideoArtifactJSON(null);
        setShotVideoArtifactError(null);
        setSelectedShotVideoClipKey(null);
        return;
      }

      setShotVideoArtifactLoading(true);
      setShotVideoArtifactError(null);
      try {
        const artifact = await getJobArtifact(jobSummary.job_id, shotVideoArtifactPath);
        if (cancelled) {
          return;
        }
        if (artifact.kind !== 'json') {
          setShotVideoArtifactJSON(null);
          setShotVideoArtifactError('Shot video artifact did not return a JSON response.');
          return;
        }
        const parsed = artifact.json;
        setShotVideoArtifactJSON(parsed);
        if (isShotVideoArtifact(parsed) && parsed.clips.length > 0) {
          setSelectedShotVideoClipKey((current) => {
            if (
              current &&
              parsed.clips.some((clip) => `${clip.segment_index}:${clip.shot_index}` === current)
            ) {
              return current;
            }
            const first = parsed.clips[0];
            return `${first.segment_index}:${first.shot_index}`;
          });
        } else {
          setSelectedShotVideoClipKey(null);
        }
      } catch (error) {
        if (cancelled) {
          return;
        }
        setShotVideoArtifactJSON(null);
        setShotVideoArtifactError(error instanceof Error ? error.message : 'Failed to load shot video artifact.');
      } finally {
        if (!cancelled) {
          setShotVideoArtifactLoading(false);
        }
      }
    }

    void loadShotVideoArtifact();
    return () => {
      cancelled = true;
    };
  }, [activeTab, isShotVideoNode, jobSummary, shotVideoArtifactPath]);

  React.useEffect(() => {
    let cancelled = false;

    async function loadTTSArtifact() {
      if (!isTTSNode || !jobSummary || !ttsArtifactPath || ttsArtifactPath === '-' || activeTab !== 'OUTPUT') {
        setTTSArtifactJSON(null);
        setTTSArtifactError(null);
        return;
      }

      setTTSArtifactLoading(true);
      setTTSArtifactError(null);
      try {
        const artifact = await getJobArtifact(jobSummary.job_id, ttsArtifactPath);
        if (cancelled) {
          return;
        }
        if (artifact.kind !== 'json') {
          setTTSArtifactJSON(null);
          setTTSArtifactError('TTS artifact did not return a JSON response.');
          return;
        }
        setTTSArtifactJSON(artifact.json);
      } catch (error) {
        if (cancelled) {
          return;
        }
        setTTSArtifactJSON(null);
        setTTSArtifactError(error instanceof Error ? error.message : 'Failed to load TTS artifact.');
      } finally {
        if (!cancelled) {
          setTTSArtifactLoading(false);
        }
      }
    }

    void loadTTSArtifact();
    return () => {
      cancelled = true;
    };
  }, [activeTab, isTTSNode, jobSummary, ttsArtifactPath]);

  React.useEffect(() => {
    let cancelled = false;

    async function loadScriptEntries() {
      if (!isScriptNode || !jobSummary || !scriptSegmentArtifactDir || activeTab !== 'OUTPUT') {
        setScriptEntries([]);
        setScriptEntriesError(null);
        setSelectedScriptPath(null);
        setSelectedScriptContent('');
        setSelectedScriptError(null);
        return;
      }

      setScriptEntriesLoading(true);
      setScriptEntriesError(null);
      try {
        const artifact = await getJobArtifact(jobSummary.job_id, scriptSegmentArtifactDir);
        if (cancelled) {
          return;
        }
        if (artifact.kind !== 'directory') {
          setScriptEntries([]);
          setScriptEntriesError('Script artifact dir did not return a directory response.');
          return;
        }

        const entries = artifact.entries.filter((entry) => entry.kind === 'json');
        setScriptEntries(entries);
        setSelectedScriptPath((current) => {
          if (current && entries.some((entry) => entry.path === current)) {
            return current;
          }
          return entries[0]?.path ?? null;
        });
      } catch (error) {
        if (cancelled) {
          return;
        }
        setScriptEntries([]);
        setScriptEntriesError(error instanceof Error ? error.message : 'Failed to load script artifact directory.');
      } finally {
        if (!cancelled) {
          setScriptEntriesLoading(false);
        }
      }
    }

    void loadScriptEntries();
    return () => {
      cancelled = true;
    };
  }, [activeTab, isScriptNode, jobSummary, scriptSegmentArtifactDir]);

  React.useEffect(() => {
    let cancelled = false;

    async function loadSelectedScriptContent() {
      if (!isScriptNode || !jobSummary || !selectedScriptPath || activeTab !== 'OUTPUT') {
        setSelectedScriptContent('');
        setSelectedScriptJSON(null);
        setSelectedScriptError(null);
        return;
      }

      setSelectedScriptLoading(true);
      setSelectedScriptError(null);
      try {
        const artifact = await getJobArtifact(jobSummary.job_id, selectedScriptPath);
        if (cancelled) {
          return;
        }

        if (artifact.kind === 'json') {
          setSelectedScriptJSON(artifact.json);
          setSelectedScriptContent(JSON.stringify(artifact.json, null, 2));
          return;
        }
        if (artifact.kind === 'text') {
          setSelectedScriptJSON(null);
          setSelectedScriptContent(artifact.text);
          return;
        }

        setSelectedScriptJSON(null);
        setSelectedScriptContent('');
        setSelectedScriptError('Selected script artifact is not a file.');
      } catch (error) {
        if (cancelled) {
          return;
        }
        setSelectedScriptJSON(null);
        setSelectedScriptContent('');
        setSelectedScriptError(error instanceof Error ? error.message : 'Failed to load script artifact file.');
      } finally {
        if (!cancelled) {
          setSelectedScriptLoading(false);
        }
      }
    }

    void loadSelectedScriptContent();
    return () => {
      cancelled = true;
    };
  }, [activeTab, isScriptNode, jobSummary, selectedScriptPath]);

  return (
    <motion.div
      initial={{x: 400}}
      animate={{x: 0}}
      exit={{x: 400}}
      className="z-10 flex h-full w-[420px] flex-col border-l border-slate-800 bg-slate-900 shadow-2xl"
    >
      <div className="border-b border-slate-800 p-6">
        <div className="mb-4 flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-slate-100">Node Inspector</h2>
            <p className="mt-1 text-xs uppercase tracking-widest text-slate-500">Precision Configuration</p>
          </div>
          <button onClick={onClose} className="text-slate-500 hover:text-slate-300">
            <ChevronRight className="h-5 w-5" />
          </button>
        </div>
      </div>

      <div className="border-b border-slate-800 px-6">
        <div className="flex items-center gap-6">
          {tabs.map((tab) => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab as 'OUTPUT' | 'DETAILS')}
              className={cn(
                'relative py-4 text-xs font-bold tracking-widest transition-colors',
                activeTab === tab ? 'text-brand-cyan' : 'text-slate-500 hover:text-slate-300',
              )}
            >
              {tab}
              {activeTab === tab && (
                <motion.div
                  layoutId="node-inspector-tab"
                  className="absolute inset-x-0 bottom-0 h-0.5 bg-brand-cyan"
                />
              )}
            </button>
          ))}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
        <AnimatePresence mode="wait">
          {(isVideoNode || isImageNode || isShotVideoNode || isTTSNode || isScriptNode) && activeTab === 'OUTPUT' ? (
            <motion.div
              key="output"
              initial={{opacity: 0, y: 8}}
              animate={{opacity: 1, y: 0}}
              exit={{opacity: 0, y: -8}}
              className="space-y-6"
            >
              <div className="rounded-2xl border border-slate-700/60 bg-slate-800/40 p-4">
                <div className="mb-1 text-lg font-semibold text-slate-100">{node.chineseLabel}</div>
                <div className="text-sm text-slate-500">{node.label}</div>
                <div className={cn('mt-4 inline-flex items-center gap-2 rounded-full border px-3 py-1 text-[11px] font-bold tracking-widest', statusTone(node.status))}>
                  <StatusIcon status={node.status} />
                  <span>{formatStatus(node.status)}</span>
                </div>
              </div>

              <Section
                title={
                  isVideoNode
                    ? 'Final Output'
                    : isImageNode
                      ? 'Image Output'
                      : isShotVideoNode
                        ? 'Shot Video Output'
                        : isTTSNode
                          ? 'TTS Output'
                          : 'Script Output'
                }
                icon={<PlayCircle className="h-3.5 w-3.5" />}
              >
                {isVideoNode && videoResult ? (
                  <div className="space-y-4">
                    {videoPreviewUrl && (
                      <div className="overflow-hidden rounded-2xl border border-slate-800 bg-slate-950 shadow-inner">
                        <video
                          controls
                          preload="metadata"
                          className="aspect-video w-full bg-black"
                          src={videoPreviewUrl}
                        >
                          Your browser does not support video playback.
                        </video>
                      </div>
                    )}

                    <div className="rounded-2xl border border-cyan-500/20 bg-cyan-500/5 p-4">
                      <div className="grid grid-cols-2 gap-3">
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Duration</div>
                          <div className="text-sm text-slate-200">{formatDuration(videoResult.duration)}</div>
                        </div>
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">File Size</div>
                          <div className="text-sm text-slate-200">{formatFileSize(videoResult.file_size)}</div>
                        </div>
                      </div>
                    </div>

                    {downloadUrl && (
                      <a
                        href={downloadUrl}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex w-full items-center justify-center gap-2 rounded-xl bg-brand-cyan px-4 py-3 text-sm font-semibold text-slate-950 transition-colors hover:bg-cyan-400"
                      >
                        <Download className="h-4 w-4" />
                        Download Final Video
                      </a>
                    )}

                    <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                      <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Artifact Path</div>
                      <div className="break-all text-xs text-slate-300">{videoArtifactPath}</div>
                    </div>
                  </div>
                ) : isImageNode ? (
                  <div className="space-y-4">
                    <div className="rounded-2xl border border-cyan-500/20 bg-cyan-500/5 p-4">
                      <div className="grid grid-cols-2 gap-3">
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Segment Images</div>
                          <div className="text-sm text-slate-200">{imageCount ?? '-'}</div>
                        </div>
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Shot Images</div>
                          <div className="text-sm text-slate-200">{shotImageCount ?? '-'}</div>
                        </div>
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Generated</div>
                          <div className="text-sm text-slate-200">{generatedImageCount ?? '-'}</div>
                        </div>
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Fallback</div>
                          <div className="text-sm text-slate-200">{fallbackImageCount ?? '-'}</div>
                        </div>
                      </div>
                    </div>

                    <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                      <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Manifest Path</div>
                      <div className="break-all text-xs text-slate-300">{imageArtifactPath}</div>
                    </div>

                    {(imageScriptRef || imageCharacterRef) && (
                      <div className="grid grid-cols-1 gap-3">
                        {imageScriptRef && (
                          <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                            <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Script Ref</div>
                            <div className="break-all text-xs text-slate-300">{imageScriptRef}</div>
                          </div>
                        )}
                        {imageCharacterRef && (
                          <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                            <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Character Image Ref</div>
                            <div className="break-all text-xs text-slate-300">{imageCharacterRef}</div>
                          </div>
                        )}
                      </div>
                    )}

                    <Section title="Shot Images" icon={<FileOutput className="h-3.5 w-3.5" />}>
                      {imageArtifactLoading ? (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-400">
                          Loading image manifest...
                        </div>
                      ) : imageArtifactError ? (
                        <div className="rounded-xl border border-rose-900/50 bg-rose-950/20 p-3 text-xs text-rose-300">
                          {imageArtifactError}
                        </div>
                      ) : imageArtifact && imageArtifact.shot_images.length > 0 ? (
                        <div className="space-y-4">
                          <div className="grid grid-cols-1 gap-2">
                            {imageArtifact.shot_images.map((shot) => {
                              const shotKey = `${shot.segment_index}:${shot.shot_index}`;
                              return (
                                <button
                                  key={shotKey}
                                  onClick={() => setSelectedImageShotKey(shotKey)}
                                  className={cn(
                                    'rounded-xl border px-3 py-2 text-left text-xs transition-colors',
                                    selectedImageShotKey === shotKey
                                      ? 'border-brand-cyan bg-brand-cyan/10 text-brand-cyan'
                                      : 'border-slate-800 bg-slate-800/20 text-slate-300 hover:border-slate-700',
                                  )}
                                >
                                  <div className="flex items-center justify-between gap-2">
                                    <span>Segment {shot.segment_index} / Shot {shot.shot_index}</span>
                                    <span className="text-[10px] uppercase tracking-widest">
                                      {shot.prompt_type || 'unknown'}
                                    </span>
                                  </div>
                                </button>
                              );
                            })}
                          </div>

                          {selectedImageShot && (
                            <div className="space-y-3 rounded-xl border border-slate-800 bg-slate-800/20 p-4">
                              <div className="flex items-center justify-between gap-2">
                                <div className="text-sm font-semibold text-slate-100">
                                  Segment {selectedImageShot.segment_index} / Shot {selectedImageShot.shot_index}
                                </div>
                                <div className="text-[10px] uppercase tracking-widest text-slate-500">
                                  {selectedImageShot.prompt_type || '-'}
                                </div>
                              </div>

                              <KeyValueGrid
                                items={[
                                  {label: 'File Path', value: selectedImageShot.file_path || '-'},
                                  {
                                    label: 'Dimensions',
                                    value:
                                      typeof selectedImageShot.width === 'number' && typeof selectedImageShot.height === 'number'
                                        ? `${selectedImageShot.width} x ${selectedImageShot.height}`
                                        : '-',
                                  },
                                  {label: 'Fallback', value: selectedImageShot.is_fallback ? 'yes' : 'no'},
                                  {label: 'Filled From Previous', value: selectedImageShot.filled_from_previous ? 'yes' : 'no'},
                                  {label: 'Model', value: selectedImageShot.generation_model || '-'},
                                  {label: 'Source Image URL', value: selectedImageShot.source_image_url || '-'},
                                ]}
                              />

                              {selectedImageShot.involved_characters && selectedImageShot.involved_characters.length > 0 && (
                                <div>
                                  <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Involved Characters</div>
                                  <div className="flex flex-wrap gap-2">
                                    {selectedImageShot.involved_characters.map((character) => (
                                      <span
                                        key={character}
                                        className="rounded-full border border-slate-700 bg-slate-900/60 px-3 py-1 text-xs text-slate-300"
                                      >
                                        {character}
                                      </span>
                                    ))}
                                  </div>
                                </div>
                              )}

                              {selectedImageShot.matched_characters && selectedImageShot.matched_characters.length > 0 && (
                                <div>
                                  <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Matched Characters</div>
                                  <div className="flex flex-wrap gap-2">
                                    {selectedImageShot.matched_characters
                                      .filter(isImageCharacterReferenceArtifact)
                                      .map((reference, index) => (
                                        <span
                                          key={`${reference.character_name || 'match'}-${index}`}
                                          className="rounded-full border border-slate-700 bg-slate-900/60 px-3 py-1 text-xs text-slate-300"
                                        >
                                          {reference.character_name || `character_${index}`}
                                        </span>
                                      ))}
                                  </div>
                                </div>
                              )}

                              {selectedImageShot.prompt && (
                                <div>
                                  <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Prompt</div>
                                  <CodeBlock value={selectedImageShot.prompt} />
                                </div>
                              )}
                            </div>
                          )}
                        </div>
                      ) : imageArtifactJSON ? (
                        <CodeBlock value={JSON.stringify(imageArtifactJSON, null, 2)} />
                      ) : (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-4 text-sm leading-6 text-slate-400">
                          No shot image details available yet.
                        </div>
                      )}
                    </Section>
                  </div>
                ) : isShotVideoNode ? (
                  <div className="space-y-4">
                    <div className="rounded-2xl border border-cyan-500/20 bg-cyan-500/5 p-4">
                      <div className="grid grid-cols-2 gap-3">
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Clip Count</div>
                          <div className="text-sm text-slate-200">{clipCount ?? '-'}</div>
                        </div>
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Generated Video</div>
                          <div className="text-sm text-slate-200">{generatedVideoCount ?? '-'}</div>
                        </div>
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Image Fallback</div>
                          <div className="text-sm text-slate-200">{shotVideoFallbackCount ?? '-'}</div>
                        </div>
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Mode</div>
                          <div className="text-sm text-slate-200">{shotVideoGenerationMode ?? '-'}</div>
                        </div>
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-3">
                      <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                        <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Requested Video Count</div>
                        <div className="text-sm text-slate-200">{requestedVideoCount ?? '-'}</div>
                      </div>
                      <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                        <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Selected Video Count</div>
                        <div className="text-sm text-slate-200">{selectedVideoCount ?? '-'}</div>
                      </div>
                    </div>

                    <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                      <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Artifact Path</div>
                      <div className="break-all text-xs text-slate-300">{shotVideoArtifactPath}</div>
                    </div>

                    <Section title="Clips" icon={<FileOutput className="h-3.5 w-3.5" />}>
                      {shotVideoArtifactLoading ? (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-400">
                          Loading shot video manifest...
                        </div>
                      ) : shotVideoArtifactError ? (
                        <div className="rounded-xl border border-rose-900/50 bg-rose-950/20 p-3 text-xs text-rose-300">
                          {shotVideoArtifactError}
                        </div>
                      ) : shotVideoArtifact && shotVideoArtifact.clips.length > 0 ? (
                        <div className="space-y-4">
                          <div className="grid grid-cols-1 gap-2">
                            {shotVideoArtifact.clips.map((clip) => {
                              const clipKey = `${clip.segment_index}:${clip.shot_index}`;
                              return (
                                <button
                                  key={clipKey}
                                  onClick={() => setSelectedShotVideoClipKey(clipKey)}
                                  className={cn(
                                    'rounded-xl border px-3 py-2 text-left text-xs transition-colors',
                                    selectedShotVideoClipKey === clipKey
                                      ? 'border-brand-cyan bg-brand-cyan/10 text-brand-cyan'
                                      : 'border-slate-800 bg-slate-800/20 text-slate-300 hover:border-slate-700',
                                  )}
                                >
                                  <div className="flex items-center justify-between gap-2">
                                    <span>Segment {clip.segment_index} / Shot {clip.shot_index}</span>
                                    <span className="text-[10px] uppercase tracking-widest">
                                      {clip.status || 'unknown'}
                                    </span>
                                  </div>
                                </button>
                              );
                            })}
                          </div>

                          {selectedShotVideoClip && (
                            <div className="space-y-3 rounded-xl border border-slate-800 bg-slate-800/20 p-4">
                              <div className="flex items-center justify-between gap-2">
                                <div className="text-sm font-semibold text-slate-100">
                                  Segment {selectedShotVideoClip.segment_index} / Shot {selectedShotVideoClip.shot_index}
                                </div>
                                <div className="text-[10px] uppercase tracking-widest text-slate-500">
                                  {selectedShotVideoClip.status || '-'}
                                </div>
                              </div>

                              <KeyValueGrid
                                items={[
                                  {label: 'Duration', value: formatDuration(selectedShotVideoClip.duration_seconds)},
                                  {label: 'Fallback', value: selectedShotVideoClip.is_fallback ? 'yes' : 'no'},
                                  {label: 'Source Type', value: selectedShotVideoClip.source_type || '-'},
                                  {label: 'Model', value: selectedShotVideoClip.generation_model || '-'},
                                  {label: 'Video Path', value: selectedShotVideoClip.video_path || '-'},
                                  {label: 'Image Path', value: selectedShotVideoClip.image_path || '-'},
                                  {label: 'Source Image Path', value: selectedShotVideoClip.source_image_path || '-'},
                                  {label: 'Source Video URL', value: selectedShotVideoClip.source_video_url || '-'},
                                ]}
                              />

                              {selectedShotVideoClip.generation_request_id && (
                                <div>
                                  <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Generation Request ID</div>
                                  <CodeBlock value={selectedShotVideoClip.generation_request_id} />
                                </div>
                              )}
                            </div>
                          )}
                        </div>
                      ) : shotVideoArtifactJSON ? (
                        <CodeBlock value={JSON.stringify(shotVideoArtifactJSON, null, 2)} />
                      ) : (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-4 text-sm leading-6 text-slate-400">
                          No clip details available yet.
                        </div>
                      )}
                    </Section>
                  </div>
                ) : isTTSNode ? (
                  <div className="space-y-4">
                    <div className="rounded-2xl border border-cyan-500/20 bg-cyan-500/5 p-4">
                      <div className="grid grid-cols-2 gap-3">
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Generation Mode</div>
                          <div className="text-sm text-slate-200">{ttsGenerationMode ?? '-'}</div>
                        </div>
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Segment Count</div>
                          <div className="text-sm text-slate-200">{ttsSegmentCount ?? '-'}</div>
                        </div>
                      </div>
                    </div>

                    <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                      <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Artifact Path</div>
                      <div className="break-all text-xs text-slate-300">{ttsArtifactPath}</div>
                    </div>

                    {ttsSegmentationRef && (
                      <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                        <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Segmentation Ref</div>
                        <div className="break-all text-xs text-slate-300">{ttsSegmentationRef}</div>
                      </div>
                    )}

                    {ttsArtifactLoading ? (
                      <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-400">
                        Loading TTS manifest...
                      </div>
                    ) : ttsArtifactError ? (
                      <div className="rounded-xl border border-rose-900/50 bg-rose-950/20 p-3 text-xs text-rose-300">
                        {ttsArtifactError}
                      </div>
                    ) : ttsArtifact ? (
                      <>
                        <Section title="Audio Segments" icon={<FileOutput className="h-3.5 w-3.5" />}>
                          <div className="space-y-3">
                            {ttsArtifact.audio_segments.map((segment) => (
                              <div
                                key={`${segment.segment_index}`}
                                className="rounded-xl border border-slate-800 bg-slate-800/20 p-4"
                              >
                                <div className="mb-3 text-sm font-semibold text-slate-100">
                                  Segment {segment.segment_index}
                                </div>
                                <KeyValueGrid
                                  items={[
                                    {label: 'File Path', value: segment.file_path || '-'},
                                    {label: 'Duration', value: formatDuration(segment.duration)},
                                  ]}
                                />
                              </div>
                            ))}
                          </div>
                        </Section>

                        <Section title="Subtitle Items" icon={<FileCode2 className="h-3.5 w-3.5" />}>
                          <div className="space-y-3">
                            {ttsArtifact.subtitle_items.map((item, index) => (
                              <div
                                key={`${item.segment_index}-${index}`}
                                className="rounded-xl border border-slate-800 bg-slate-800/20 p-4 space-y-3"
                              >
                                <div className="flex items-center justify-between gap-2">
                                  <div className="text-sm font-semibold text-slate-100">Segment {item.segment_index}</div>
                                  <div className="text-[10px] uppercase tracking-widest text-slate-500">
                                    {formatDuration(item.start)} / {formatDuration(item.end)}
                                  </div>
                                </div>
                                <div className="text-sm leading-6 text-slate-300">{item.text || '-'}</div>
                              </div>
                            ))}
                          </div>
                        </Section>

                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Total Duration</div>
                          <div className="text-sm text-slate-200">{formatDuration(ttsArtifact.total_duration_seconds)}</div>
                        </div>
                      </>
                    ) : ttsArtifactJSON ? (
                      <CodeBlock value={JSON.stringify(ttsArtifactJSON, null, 2)} />
                    ) : (
                      <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-4 text-sm leading-6 text-slate-400">
                        No TTS details available yet.
                      </div>
                    )}
                  </div>
                ) : isScriptNode ? (
                  <div className="space-y-4">
                    <div className="rounded-2xl border border-cyan-500/20 bg-cyan-500/5 p-4">
                      <div className="grid grid-cols-2 gap-3">
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Segment Count</div>
                          <div className="text-sm text-slate-200">{scriptSegmentCount ?? '-'}</div>
                        </div>
                        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                          <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Segment Dir</div>
                          <div className="break-all text-sm text-slate-200">{scriptSegmentArtifactDir ?? '-'}</div>
                        </div>
                      </div>
                    </div>

                    <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                      <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Artifact Path</div>
                      <div className="break-all text-xs text-slate-300">{scriptArtifactPath}</div>
                    </div>

                    <div className="grid grid-cols-1 gap-3">
                      {scriptSegmentationRef && (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                          <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Segmentation Ref</div>
                          <div className="break-all text-xs text-slate-300">{scriptSegmentationRef}</div>
                        </div>
                      )}
                      {scriptOutlineRef && (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                          <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Outline Ref</div>
                          <div className="break-all text-xs text-slate-300">{scriptOutlineRef}</div>
                        </div>
                      )}
                      {scriptCharacterRef && (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                          <div className="mb-2 text-[10px] uppercase tracking-widest text-slate-500">Character Ref</div>
                          <div className="break-all text-xs text-slate-300">{scriptCharacterRef}</div>
                        </div>
                      )}
                    </div>

                    <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-4 text-sm leading-6 text-slate-400">
                      Segment-level storyboard content is stored under the segment artifact directory.
                    </div>

                    <Section title="Segment Files" icon={<FileOutput className="h-3.5 w-3.5" />}>
                      {scriptEntriesLoading ? (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-400">
                          Loading segment files...
                        </div>
                      ) : scriptEntriesError ? (
                        <div className="rounded-xl border border-rose-900/50 bg-rose-950/20 p-3 text-xs text-rose-300">
                          {scriptEntriesError}
                        </div>
                      ) : scriptEntries.length === 0 ? (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-500">
                          No segment files found.
                        </div>
                      ) : (
                        <div className="grid grid-cols-1 gap-2">
                          {scriptEntries.map((entry) => (
                            <button
                              key={entry.path}
                              onClick={() => setSelectedScriptPath(entry.path)}
                              className={cn(
                                'rounded-xl border px-3 py-2 text-left text-xs transition-colors',
                                selectedScriptPath === entry.path
                                  ? 'border-brand-cyan bg-brand-cyan/10 text-brand-cyan'
                                  : 'border-slate-800 bg-slate-800/20 text-slate-300 hover:border-slate-700',
                              )}
                            >
                              {entry.name}
                            </button>
                          ))}
                        </div>
                      )}
                    </Section>

                    <Section title="Selected Segment Content" icon={<FileCode2 className="h-3.5 w-3.5" />}>
                      {selectedScriptLoading ? (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-400">
                          Loading segment content...
                        </div>
                      ) : selectedScriptError ? (
                        <div className="rounded-xl border border-rose-900/50 bg-rose-950/20 p-3 text-xs text-rose-300">
                          {selectedScriptError}
                        </div>
                      ) : isScriptSegmentArtifact(selectedScriptJSON) ? (
                        <div className="space-y-4">
                          <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                            <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Segment Index</div>
                            <div className="text-sm text-slate-200">{selectedScriptJSON.index}</div>
                          </div>
                          <div className="space-y-3">
                            {selectedScriptJSON.shots.map((shot) => (
                              <div key={shot.index} className="rounded-xl border border-slate-800 bg-slate-800/20 p-4 space-y-3">
                                <div className="flex items-center justify-between">
                                  <div className="text-sm font-semibold text-slate-100">Shot {shot.index}</div>
                                  <div className="text-[10px] uppercase tracking-widest text-slate-500">
                                    {shot.image_to_image_prompt ? 'Image-to-Image' : 'Text-to-Image'}
                                  </div>
                                </div>

                                {shot.visual_content && (
                                  <div>
                                    <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Visual Content</div>
                                    <div className="text-sm leading-6 text-slate-300">{shot.visual_content}</div>
                                  </div>
                                )}

                                {shot.camera_design && (
                                  <div>
                                    <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Camera Design</div>
                                    <div className="text-sm leading-6 text-slate-300">{shot.camera_design}</div>
                                  </div>
                                )}

                                <div>
                                  <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Involved Characters</div>
                                  {shot.involved_characters && shot.involved_characters.length > 0 ? (
                                    <div className="flex flex-wrap gap-2">
                                      {shot.involved_characters.map((character) => (
                                        <span
                                          key={character}
                                          className="rounded-full border border-slate-700 bg-slate-900/60 px-3 py-1 text-xs text-slate-300"
                                        >
                                          {character}
                                        </span>
                                      ))}
                                    </div>
                                  ) : (
                                    <div className="text-sm text-slate-500">None</div>
                                  )}
                                </div>

                                {shot.image_to_image_prompt && (
                                  <div>
                                    <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Image-to-Image Prompt</div>
                                    <CodeBlock value={shot.image_to_image_prompt} />
                                  </div>
                                )}

                                {shot.text_to_image_prompt && (
                                  <div>
                                    <div className="mb-1 text-[10px] uppercase tracking-widest text-slate-500">Text-to-Image Prompt</div>
                                    <CodeBlock value={shot.text_to_image_prompt} />
                                  </div>
                                )}
                              </div>
                            ))}
                          </div>
                        </div>
                      ) : selectedScriptContent ? (
                        <CodeBlock value={selectedScriptContent} />
                      ) : (
                        <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-500">
                          Select a segment file to preview its content.
                        </div>
                      )}
                    </Section>
                  </div>
                ) : (
                  <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-4 text-sm leading-6 text-slate-400">
                    Final output is not ready yet.
                  </div>
                )}
              </Section>
            </motion.div>
          ) : (
            <motion.div
              key="details"
              initial={{opacity: 0, y: 8}}
              animate={{opacity: 1, y: 0}}
              exit={{opacity: 0, y: -8}}
              className="space-y-8"
            >
              <div className="rounded-2xl border border-slate-700/60 bg-slate-800/40 p-4">
                <div className="mb-1 text-lg font-semibold text-slate-100">{node.chineseLabel}</div>
                <div className="text-sm text-slate-500">{node.label}</div>
                <div className={cn('mt-4 inline-flex items-center gap-2 rounded-full border px-3 py-1 text-[11px] font-bold tracking-widest', statusTone(node.status))}>
                  <StatusIcon status={node.status} />
                  <span>{formatStatus(node.status)}</span>
                </div>

                {node.summary && (
                  <div className="mt-4 rounded-xl border border-slate-800 bg-slate-900/50 p-3 text-sm text-slate-300">
                    {node.summary}
                  </div>
                )}
              </div>

              <Section title="Task Context" icon={<Package className="h-3.5 w-3.5" />}>
                <KeyValueGrid
                  items={[
                    {label: 'Task Key', value: node.id},
                    {label: 'Task Type', value: node.taskType || '-'},
                    {label: 'Resource', value: node.resourceKey || '-'},
                    {label: 'Progress', value: typeof node.progress === 'number' ? `${node.progress}%` : '-'},
                    {label: 'Attempt', value: typeof node.attempt === 'number' && typeof node.maxAttempts === 'number' ? `${node.attempt}/${node.maxAttempts}` : '-'},
                    {label: 'Updated At', value: formatTimestamp(node.updatedAt)},
                  ]}
                />
              </Section>

              <Section title="Dependencies" icon={<Link2 className="h-3.5 w-3.5" />}>
                {node.dependencies.length === 0 ? (
                  <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-500">
                    No upstream dependencies.
                  </div>
                ) : (
                  <div className="flex flex-wrap gap-2">
                    {node.dependencies.map((dependency) => (
                      <span
                        key={dependency}
                        className="rounded-full border border-slate-700 bg-slate-800/40 px-3 py-1 text-xs text-slate-300"
                      >
                        {dependency}
                      </span>
                    ))}
                  </div>
                )}
              </Section>

              <Section title="Error" icon={<AlertCircle className="h-3.5 w-3.5" />}>
                {node.error?.message ? (
                  <div className="rounded-xl border border-rose-900/50 bg-rose-950/20 p-4">
                    {node.error.code && (
                      <div className="mb-2 text-[10px] font-bold uppercase tracking-widest text-rose-300">
                        {node.error.code}
                      </div>
                    )}
                    <div className="text-sm leading-6 text-rose-200">{node.error.message}</div>
                  </div>
                ) : (
                  <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-500">
                    No error recorded.
                  </div>
                )}
              </Section>

              <Section title="Artifacts" icon={<FileOutput className="h-3.5 w-3.5" />}>
                {artifactEntries.length === 0 ? (
                  <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-500">
                    No output_ref fields yet.
                  </div>
                ) : (
                  <div className="space-y-3">
                    {artifactEntries.map((entry) => (
                      <div key={entry.key} className="space-y-2 rounded-xl border border-slate-800 bg-slate-800/20 p-3">
                        <div className="text-[10px] font-bold uppercase tracking-widest text-slate-500">{entry.key}</div>
                        <CodeBlock value={entry.value} />
                      </div>
                    ))}
                  </div>
                )}
              </Section>

              <Section title="Payload" icon={<FileCode2 className="h-3.5 w-3.5" />}>
                {node.payload && Object.keys(node.payload).length > 0 ? (
                  <CodeBlock value={stringifyValue(node.payload)} />
                ) : (
                  <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs text-slate-500">
                    No payload fields exposed.
                  </div>
                )}
              </Section>

              <Section title="Timestamps" icon={<Clock3 className="h-3.5 w-3.5" />}>
                <KeyValueGrid items={[{label: 'Last Update', value: formatTimestamp(node.updatedAt)}]} />
              </Section>

              <Section title="Data Source" icon={<Database className="h-3.5 w-3.5" />}>
                <div className="rounded-xl border border-slate-800 bg-slate-800/20 p-3 text-xs leading-6 text-slate-400">
                  Inspector data is mapped directly from backend task fields and `output_ref`.
                </div>
              </Section>
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </motion.div>
  );
};

export default NodeInspector;
