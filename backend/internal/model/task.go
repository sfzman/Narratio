package model

import "time"

type TaskType string

const (
	TaskTypeSegmentation   TaskType = "segmentation"
	TaskTypeOutline        TaskType = "outline"
	TaskTypeCharacterSheet TaskType = "character_sheet"
	TaskTypeScript         TaskType = "script"
	TaskTypeCharacterImage TaskType = "character_image"
	TaskTypeTTS            TaskType = "tts"
	TaskTypeImage          TaskType = "image"
	TaskTypeVideo          TaskType = "video"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusReady     TaskStatus = "ready"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusSkipped   TaskStatus = "skipped"
)

type ResourceKey string

const (
	ResourceLocalCPU    ResourceKey = "local_cpu"
	ResourceLLMText     ResourceKey = "llm_text"
	ResourceTTS         ResourceKey = "tts"
	ResourceImageGen    ResourceKey = "image_gen"
	ResourceVideoRender ResourceKey = "video_render"
)

type TaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Task struct {
	ID int64
	// JobID links the task to its parent job primary key.
	JobID int64
	// Key is the stable task identifier within one job DAG, e.g. "outline" or "video".
	Key string
	// Type is the business meaning of the task and determines which executor handles it.
	Type TaskType
	// Status is the execution state of the task inside the scheduler.
	Status TaskStatus
	// ResourceKey determines which shared resource pool limits this task.
	ResourceKey ResourceKey
	// DependsOn stores upstream task keys instead of database ids so DAG creation is simpler.
	DependsOn []string
	// Attempt is the current execution attempt count.
	Attempt int
	// MaxAttempts is the maximum number of attempts allowed for this task.
	MaxAttempts int
	// Payload stores the task input snapshot needed by its executor.
	Payload map[string]any
	// OutputRef stores structured references to task outputs for downstream tasks.
	OutputRef map[string]any
	// Error stores the latest terminal error for the task.
	Error     *TaskError
	CreatedAt time.Time
	UpdatedAt time.Time
}
