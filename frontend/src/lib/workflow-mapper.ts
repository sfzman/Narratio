import type {JobSummaryResponse, JobTaskResponse, TaskProgressSnapshot} from '@/types/api';
import type {Project, WorkflowNode} from '@/types/workflow';

interface TaskMeta {
  label: string;
  chineseLabel: string;
}

const TASK_ORDER = [
  'segmentation',
  'outline',
  'character_sheet',
  'script',
  'character_image',
  'tts',
  'image',
  'shot_video',
  'video',
] as const;

const TASK_META: Record<string, TaskMeta> = {
  segmentation: {label: 'Segmentation', chineseLabel: '文本分段'},
  outline: {label: 'Outline', chineseLabel: '提炼大纲'},
  character_sheet: {label: 'Character Sheet', chineseLabel: '人物表'},
  script: {label: 'Script', chineseLabel: '分镜脚本'},
  character_image: {label: 'Character Image', chineseLabel: '人物参考图'},
  tts: {label: 'TTS', chineseLabel: '配音生成'},
  image: {label: 'Image', chineseLabel: '分镜图'},
  shot_video: {label: 'Shot Video', chineseLabel: '分镜视频'},
  video: {label: 'Final Video', chineseLabel: '视频合成'},
};

function formatProgress(progress?: TaskProgressSnapshot): number | undefined {
  if (!progress) {
    return undefined;
  }
  if (typeof progress.current === 'number' && typeof progress.total === 'number' && progress.total > 0) {
    return Math.max(0, Math.min(100, Math.round((progress.current / progress.total) * 100)));
  }
  if ((progress.unit === '%' || progress.unit === 'percent') && typeof progress.current === 'number') {
    return Math.max(0, Math.min(100, Math.round(progress.current)));
  }
  return undefined;
}

function summarizeRunningTask(task: JobTaskResponse): string {
  const progress = task.output_ref.progress;
  if (progress?.message) {
    return progress.message;
  }
  if (typeof progress?.current === 'number' && typeof progress?.total === 'number' && progress.total > 0) {
    return `执行中 ${progress.current}/${progress.total}`;
  }
  if (progress?.phase) {
    return `执行中：${progress.phase}`;
  }
  return '执行中';
}

function summarizeSucceededTask(task: JobTaskResponse): string {
  const output = task.output_ref;
  switch (task.key) {
    case 'segmentation':
      if (typeof output.segment_count === 'number') {
        return `已分段 ${output.segment_count} 段`;
      }
      return '分段完成';
    case 'outline':
      return '大纲已生成';
    case 'character_sheet':
      return '人物表已生成';
    case 'script':
      if (typeof output.segment_count === 'number') {
        return `已生成 ${output.segment_count} 段分镜`;
      }
      return '分镜已生成';
    case 'character_image':
      if (typeof output.image_count === 'number') {
        return `已生成 ${output.image_count} 张参考图`;
      }
      return '人物参考图已生成';
    case 'tts':
      if (typeof output.segment_count === 'number') {
        return `已生成 ${output.segment_count} 段音频`;
      }
      return '配音已生成';
    case 'image':
      if (typeof output.shot_image_count === 'number') {
        return `已生成 ${output.shot_image_count} 张分镜图`;
      }
      if (typeof output.image_count === 'number') {
        return `已生成 ${output.image_count} 张图片`;
      }
      return '图片已生成';
    case 'shot_video':
      if (typeof output.generated_video_count === 'number' && typeof output.clip_count === 'number') {
        return `已生成 ${output.generated_video_count}/${output.clip_count} 个分镜视频`;
      }
      return '分镜视频已生成';
    case 'video':
      return '成片已输出';
    default:
      return '执行完成';
  }
}

function summarizeTask(task: JobTaskResponse): string {
  switch (task.status) {
    case 'running':
      return summarizeRunningTask(task);
    case 'succeeded':
      return summarizeSucceededTask(task);
    case 'failed':
      return task.error?.message || '执行失败';
    case 'ready':
      return '依赖已满足，等待调度';
    case 'pending':
      return '等待上游任务';
    case 'cancelled':
      return '任务已取消';
    case 'skipped':
      return '因上游状态被跳过';
    default:
      return '';
  }
}

function getUpdatedAt(task: JobTaskResponse, fallback: string): string {
  return task.updated_at || task.created_at || fallback;
}

export function mapJobTasksToWorkflowNodes(tasks: JobTaskResponse[], fallbackUpdatedAt = ''): WorkflowNode[] {
  const orderedTasks = [...tasks].sort((left, right) => {
    const leftIndex = TASK_ORDER.indexOf(left.key as (typeof TASK_ORDER)[number]);
    const rightIndex = TASK_ORDER.indexOf(right.key as (typeof TASK_ORDER)[number]);
    if (leftIndex === -1 && rightIndex === -1) {
      return left.key.localeCompare(right.key);
    }
    if (leftIndex === -1) {
      return 1;
    }
    if (rightIndex === -1) {
      return -1;
    }
    return leftIndex - rightIndex;
  });

  return orderedTasks.map((task) => {
    const meta = TASK_META[task.key] || {
      label: task.key,
      chineseLabel: task.key,
    };

    return {
      id: task.key,
      type: 'process',
      taskType: task.type,
      label: meta.label,
      chineseLabel: meta.chineseLabel,
      status: task.status,
      progress: formatProgress(task.output_ref.progress),
      summary: summarizeTask(task),
      updatedAt: getUpdatedAt(task, fallbackUpdatedAt),
      resourceKey: task.resource_key,
      dependencies: task.depends_on,
      attempt: task.attempt,
      maxAttempts: task.max_attempts,
      payload: task.payload,
      artifacts: task.output_ref,
      error: task.error,
    };
  });
}

export function mapJobToProject(job: JobSummaryResponse, tasks: JobTaskResponse[]): Project {
  return {
    id: job.job_id,
    name: job.name || job.job_id,
    status: job.status,
    createdAt: job.created_at,
    nodes: mapJobTasksToWorkflowNodes(tasks, job.updated_at),
  };
}
