package tts

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const (
	defaultSegmentCount = 1
	defaultAudioFormat  = "wav"
)

type Executor struct {
	log       *slog.Logger
	client    Client
	artifacts artifactWriter
}

func NewExecutor(workspaceDir ...string) *Executor {
	return NewExecutorWithClient(nil, workspaceDir...)
}

func NewExecutorWithClient(client Client, workspaceDir ...string) *Executor {
	resolvedWorkspaceDir := ""
	if len(workspaceDir) > 0 {
		resolvedWorkspaceDir = strings.TrimSpace(workspaceDir[0])
	}

	return &Executor{
		log:       slog.Default().With("executor", "tts"),
		client:    client,
		artifacts: newArtifactWriter(resolvedWorkspaceDir),
	}
}

func (e *Executor) Execute(
	ctx context.Context,
	job model.Job,
	task model.Task,
	dependencies map[string]model.Task,
) (model.Task, error) {
	voiceID, err := payloadString(task.Payload, "voice_id")
	if err != nil {
		e.log.Error("tts payload invalid",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	segmentationTask, ok := dependencies["segmentation"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "segmentation")
	}
	segmentation, err := loadArtifactJSON[segmentationArtifact](
		e.artifacts.workspaceDir,
		segmentationTask.OutputRef["artifact_path"],
	)
	if err != nil {
		return task, fmt.Errorf("load segmentation artifact: %w", err)
	}

	e.log.Debug("tts execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)

	artifactPath := fmt.Sprintf("jobs/%s/audio/tts_manifest.json", job.PublicID)
	output, generationMode, err := e.generateAndPersistOutput(
		ctx,
		job.PublicID,
		voiceID,
		segmentation,
		artifactPath,
	)
	if err != nil {
		return task, err
	}
	segmentCount := len(output.AudioSegments)
	if segmentCount == 0 {
		segmentCount = defaultSegmentCount
	}

	task.OutputRef = map[string]any{
		"artifact_type":             "tts",
		"artifact_path":             artifactPath,
		"segmentation_artifact_ref": segmentationTask.OutputRef["artifact_path"],
		"voice_id":                  voiceID,
		"generation_mode":           generationMode,
		"segment_count":             segmentCount,
		"audio_segment_paths":       collectAudioSegmentPaths(output.AudioSegments),
		"total_duration_seconds":    output.TotalDuration,
	}

	e.log.Info("tts execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", task.OutputRef["artifact_path"],
	)

	return task, nil
}

func (e *Executor) generateAndPersistOutput(
	ctx context.Context,
	jobPublicID string,
	voiceID string,
	segmentation segmentationArtifact,
	artifactPath string,
) (TTSOutput, string, error) {
	if e == nil || e.client == nil {
		output, err := e.generatePlaceholderOutput(jobPublicID, segmentation, artifactPath)
		if err != nil {
			return TTSOutput{}, "", err
		}
		return output, "placeholder", nil
	}

	output, err := e.generateLiveOutput(ctx, jobPublicID, voiceID, segmentation, artifactPath)
	if err != nil {
		return TTSOutput{}, "", err
	}

	return output, "sentence_serial", nil
}

func (e *Executor) generatePlaceholderOutput(
	jobPublicID string,
	segmentation segmentationArtifact,
	artifactPath string,
) (TTSOutput, error) {
	output := TTSOutput{}

	for index, segment := range normalizedSegmentationSegments(segmentation) {
		segmentIndex := normalizedSegmentIndex(segment.Index, index)
		filePath := audioSegmentPath(jobPublicID, segmentIndex)
		audioBytes, err := buildSilentWAV(defaultSegmentDuration)
		if err != nil {
			return TTSOutput{}, fmt.Errorf("build placeholder segment %d wav: %w", segmentIndex, err)
		}
		if err := e.persistSegmentOutput(
			artifactPath,
			&output,
			segmentIndex,
			filePath,
			defaultSegmentDuration,
			segment.Text,
			audioBytes,
		); err != nil {
			return TTSOutput{}, err
		}
	}

	return output, nil
}

func (e *Executor) generateLiveOutput(
	ctx context.Context,
	jobPublicID string,
	voiceID string,
	segmentation segmentationArtifact,
	artifactPath string,
) (TTSOutput, error) {
	output := TTSOutput{}

	for index, segment := range normalizedSegmentationSegments(segmentation) {
		segmentIndex := normalizedSegmentIndex(segment.Index, index)
		filePath := audioSegmentPath(jobPublicID, segmentIndex)
		audioBytes, duration, sentenceCount, err := e.synthesizeSegment(
			ctx,
			segment.Text,
			voiceID,
		)
		if err != nil {
			return TTSOutput{}, fmt.Errorf(
				"synthesize segment %d: %w",
				segmentIndex,
				err,
			)
		}

		e.log.Debug("tts segment synthesized",
			"segment_index", segmentIndex,
			"sentence_count", sentenceCount,
			"duration_seconds", duration,
		)

		if err := e.persistSegmentOutput(
			artifactPath,
			&output,
			segmentIndex,
			filePath,
			duration,
			segment.Text,
			audioBytes,
		); err != nil {
			return TTSOutput{}, err
		}
	}

	return output, nil
}

func (e *Executor) synthesizeSegment(
	ctx context.Context,
	segmentText string,
	voiceID string,
) ([]byte, float64, int, error) {
	sentences := splitSentencesByPeriod(segmentText)
	if len(sentences) == 0 {
		return nil, 0, 0, fmt.Errorf("segment text has no valid sentences")
	}

	sentenceWAVs := make([][]byte, 0, len(sentences))
	for _, sentence := range sentences {
		audioBytes, err := e.client.Synthesize(ctx, Request{
			Text:       sentence,
			VoiceID:    voiceID,
			Format:     defaultAudioFormat,
			SampleRate: wavSampleRate,
		})
		if err != nil {
			return nil, 0, 0, fmt.Errorf("synthesize sentence %q: %w", sentence, err)
		}
		sentenceWAVs = append(sentenceWAVs, audioBytes)
	}

	merged, duration, err := mergeSentenceWAVs(sentenceWAVs, defaultSentenceGapSeconds)
	if err != nil {
		return nil, 0, 0, err
	}

	return merged, duration, len(sentences), nil
}

func (e *Executor) persistSegmentOutput(
	artifactPath string,
	output *TTSOutput,
	segmentIndex int,
	filePath string,
	duration float64,
	text string,
	audioBytes []byte,
) error {
	if err := e.artifacts.WriteBytes(filePath, audioBytes); err != nil {
		return fmt.Errorf("write audio file: %w", err)
	}

	appendSegmentOutput(output, segmentIndex, filePath, duration, text)
	if err := e.artifacts.WriteJSON(artifactPath, output); err != nil {
		return fmt.Errorf("write tts artifact: %w", err)
	}

	return nil
}

func appendSegmentOutput(
	output *TTSOutput,
	segmentIndex int,
	filePath string,
	duration float64,
	text string,
) {
	if output == nil {
		return
	}

	start := output.TotalDuration
	output.AudioSegments = append(output.AudioSegments, AudioSegment{
		SegmentIndex: segmentIndex,
		FilePath:     filePath,
		Duration:     duration,
	})
	output.SubtitleItems = append(output.SubtitleItems, SubtitleItem{
		SegmentIndex: segmentIndex,
		Start:        start,
		End:          start + duration,
		Text:         strings.TrimSpace(text),
	})
	output.TotalDuration += duration
}

func normalizedSegmentationSegments(segmentation segmentationArtifact) []segmentationSegment {
	segments := segmentation.Segments
	if len(segments) == 0 {
		return []segmentationSegment{{Index: 0, Text: ""}}
	}

	return segments
}

func payloadString(payload map[string]any, key string) (string, error) {
	value, ok := payload[key]
	if !ok {
		return "", fmt.Errorf("missing payload field %q", key)
	}

	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("payload field %q is not a string", key)
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("payload field %q is empty", key)
	}

	return s, nil
}
