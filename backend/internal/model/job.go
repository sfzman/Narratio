package model

import "time"

type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusRunning    JobStatus = "running"
	JobStatusCancelling JobStatus = "cancelling"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusCancelled  JobStatus = "cancelled"
)

type RenderOptions struct {
	VoiceID     string      `json:"voice_id"`
	ImageStyle  string      `json:"image_style"`
	AspectRatio AspectRatio `json:"aspect_ratio,omitempty"`
	VideoCount  *int        `json:"video_count,omitempty"`
}

type JobSpec struct {
	Article string        `json:"article"`
	Options RenderOptions `json:"options"`
}

type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type JobResult struct {
	VideoPath string  `json:"video_path"`
	Duration  float64 `json:"duration"`
	FileSize  int64   `json:"file_size"`
}

type Job struct {
	ID int64
	// PublicID is the external job identifier exposed to APIs and clients.
	PublicID string
	// Token is reserved for weak isolation before a real user system exists.
	Token string
	// Status is the lifecycle status derived from task execution.
	Status JobStatus
	// Progress is a 0-100 aggregate progress value for the whole job.
	Progress int
	// Spec stores the normalized user request payload for this job.
	Spec JobSpec
	// Warnings stores non-fatal issues collected during execution.
	Warnings []string
	// Error stores the terminal job-level error when the workflow cannot continue.
	Error *JobError
	// Result stores the final output metadata after the job completes.
	Result    *JobResult
	CreatedAt time.Time
	UpdatedAt time.Time
}
