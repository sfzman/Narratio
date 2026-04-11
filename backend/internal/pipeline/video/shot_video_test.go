package video

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func readShotVideoArtifact(t *testing.T, workspaceDir string, relativePath string) ShotVideoOutput {
	t.Helper()

	fullPath := filepath.Join(workspaceDir, filepath.Clean(relativePath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", fullPath, err)
	}

	var output ShotVideoOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", fullPath, err)
	}

	return output
}

type fakeShotVideoClient struct {
	response Response
	err      error
	requests []Request
}

func (c *fakeShotVideoClient) Generate(
	_ context.Context,
	request Request,
) (Response, error) {
	c.requests = append(c.requests, request)
	if c.err != nil {
		return Response{}, c.err
	}

	return c.response, nil
}

func TestShotVideoExecutorBuildsFallbackManifestFromShotImages(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewShotVideoExecutor(workspaceDir, 3)
	job := model.Job{ID: 1, PublicID: "job_shot_video_123"}
	task := model.Task{
		ID:   41,
		Key:  "shot_video",
		Type: model.TaskTypeShotVideo,
		Payload: map[string]any{
			"aspect_ratio": "9:16",
		},
	}
	dependencies := map[string]model.Task{
		"image": {
			Key: "image",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_shot_video_123/images/image_manifest.json",
			},
		},
	}
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_shot_video_123/images/image_manifest.json", map[string]any{
		"shot_images": []map[string]any{
			{
				"segment_index": 0,
				"shot_index":    0,
				"file_path":     "jobs/job_shot_video_123/images/segment_000_shot_000.jpg",
			},
			{
				"segment_index": 1,
				"shot_index":    0,
				"file_path":     "jobs/job_shot_video_123/images/segment_001_shot_000.jpg",
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		"jobs/job_shot_video_123/images/segment_000_shot_000.jpg",
		"jobs/job_shot_video_123/images/segment_001_shot_000.jpg",
	)

	updated, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["artifact_type"] != "shot_video" {
		t.Fatalf("artifact_type = %#v, want %q", updated.OutputRef["artifact_type"], "shot_video")
	}
	if updated.OutputRef["artifact_path"] != "jobs/job_shot_video_123/shot_videos/manifest.json" {
		t.Fatalf("artifact_path = %#v", updated.OutputRef["artifact_path"])
	}
	if updated.OutputRef["image_artifact_ref"] != "jobs/job_shot_video_123/images/image_manifest.json" {
		t.Fatalf("image_artifact_ref = %#v", updated.OutputRef["image_artifact_ref"])
	}
	if updated.OutputRef["image_source_type"] != "shot_images" {
		t.Fatalf("image_source_type = %#v, want %q", updated.OutputRef["image_source_type"], "shot_images")
	}
	if updated.OutputRef["clip_count"] != 2 {
		t.Fatalf("clip_count = %#v, want 2", updated.OutputRef["clip_count"])
	}
	if updated.OutputRef["generated_video_count"] != 0 {
		t.Fatalf("generated_video_count = %#v, want 0", updated.OutputRef["generated_video_count"])
	}
	if updated.OutputRef["fallback_image_count"] != 2 {
		t.Fatalf("fallback_image_count = %#v, want 2", updated.OutputRef["fallback_image_count"])
	}
	if updated.OutputRef["aspect_ratio"] != "9:16" {
		t.Fatalf("aspect_ratio = %#v, want %q", updated.OutputRef["aspect_ratio"], "9:16")
	}

	artifact := readShotVideoArtifact(t, workspaceDir, "jobs/job_shot_video_123/shot_videos/manifest.json")
	if len(artifact.Clips) != 2 {
		t.Fatalf("clips len = %d, want 2", len(artifact.Clips))
	}
	if artifact.Clips[0].ImagePath != "jobs/job_shot_video_123/images/segment_000_shot_000.jpg" {
		t.Fatalf("clip[0].ImagePath = %q", artifact.Clips[0].ImagePath)
	}
	if artifact.Clips[0].SourceImagePath != "jobs/job_shot_video_123/images/segment_000_shot_000.jpg" {
		t.Fatalf("clip[0].SourceImagePath = %q", artifact.Clips[0].SourceImagePath)
	}
	if artifact.Clips[0].Status != "image_fallback" {
		t.Fatalf("clip[0].Status = %q, want %q", artifact.Clips[0].Status, ShotVideoStatusImageFallback)
	}
	if artifact.Clips[0].DurationSeconds != 3 {
		t.Fatalf("clip[0].DurationSeconds = %v, want 3", artifact.Clips[0].DurationSeconds)
	}
	if artifact.Clips[0].SourceType != "image_fallback" {
		t.Fatalf("clip[0].SourceType = %q, want %q", artifact.Clips[0].SourceType, "image_fallback")
	}
	if !artifact.Clips[0].IsFallback {
		t.Fatal("clip[0].IsFallback = false, want true")
	}
}

func TestShotVideoExecutorReportsProgress(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewShotVideoExecutor(workspaceDir, 3)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_shot_video_progress/images/image_manifest.json", map[string]any{
		"shot_images": []map[string]any{
			{
				"segment_index": 0,
				"shot_index":    0,
				"file_path":     "jobs/job_shot_video_progress/images/segment_000_shot_000.jpg",
			},
			{
				"segment_index": 0,
				"shot_index":    1,
				"file_path":     "jobs/job_shot_video_progress/images/segment_000_shot_001.jpg",
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		"jobs/job_shot_video_progress/images/segment_000_shot_000.jpg",
		"jobs/job_shot_video_progress/images/segment_000_shot_001.jpg",
	)

	reporter := &recordingProgressReporter{}
	ctx := model.WithTaskProgressReporter(context.Background(), reporter)

	_, err := executor.Execute(
		ctx,
		model.Job{PublicID: "job_shot_video_progress"},
		model.Task{Key: "shot_video", Type: model.TaskTypeShotVideo},
		map[string]model.Task{
			"image": {
				Key: "image",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_shot_video_progress/images/image_manifest.json",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	progress := reporter.snapshot()
	if len(progress) < 3 {
		t.Fatalf("len(progress) = %d, want >= 3", len(progress))
	}
	if progress[0].Phase != "generating_clip" || progress[0].Current != 1 || progress[0].Total != 2 {
		t.Fatalf("progress[0] = %#v, want first clip progress", progress[0])
	}
	if progress[1].Phase != "generating_clip" || progress[1].Current != 2 || progress[1].Total != 2 {
		t.Fatalf("progress[1] = %#v, want second clip progress", progress[1])
	}
	last := progress[len(progress)-1]
	if last.Phase != "writing_artifact" {
		t.Fatalf("last progress phase = %#v, want writing_artifact", last.Phase)
	}
}

func TestShotVideoStatusValidation(t *testing.T) {
	t.Parallel()

	if !isValidShotVideoStatus(ShotVideoStatusGeneratedVideo) {
		t.Fatal("generated_video should be valid")
	}
	if !isValidShotVideoStatus(ShotVideoStatusImageFallback) {
		t.Fatal("image_fallback should be valid")
	}
	if isValidShotVideoStatus("failed") {
		t.Fatal("failed should be invalid at current contract stage")
	}
}

func TestShotVideoExecutorRequiresImageDependency(t *testing.T) {
	t.Parallel()

	executor := NewShotVideoExecutor(t.TempDir(), 3)
	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_shot_video_123"},
		model.Task{Key: "shot_video", Type: model.TaskTypeShotVideo},
		map[string]model.Task{},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestShotVideoExecutorManifestReservesLiveGenerationFields(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewShotVideoExecutor(workspaceDir, 5)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_shot_video_live_fields_123/images/image_manifest.json", map[string]any{
		"shot_images": []map[string]any{
			{
				"segment_index": 0,
				"shot_index":    0,
				"file_path":     "jobs/job_shot_video_live_fields_123/images/segment_000_shot_000.jpg",
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		"jobs/job_shot_video_live_fields_123/images/segment_000_shot_000.jpg",
	)

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_shot_video_live_fields_123"},
		model.Task{Key: "shot_video", Type: model.TaskTypeShotVideo},
		map[string]model.Task{
			"image": {
				Key: "image",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_shot_video_live_fields_123/images/image_manifest.json",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	artifact := readShotVideoArtifact(t, workspaceDir, "jobs/job_shot_video_live_fields_123/shot_videos/manifest.json")
	if len(artifact.Clips) != 1 {
		t.Fatalf("clips len = %d, want 1", len(artifact.Clips))
	}
	if artifact.Clips[0].GenerationRequestID != "" {
		t.Fatalf("clip[0].GenerationRequestID = %q, want empty", artifact.Clips[0].GenerationRequestID)
	}
	if artifact.Clips[0].GenerationModel != "" {
		t.Fatalf("clip[0].GenerationModel = %q, want empty", artifact.Clips[0].GenerationModel)
	}
	if artifact.Clips[0].SourceVideoURL != "" {
		t.Fatalf("clip[0].SourceVideoURL = %q, want empty", artifact.Clips[0].SourceVideoURL)
	}
	if artifact.Clips[0].SourceImagePath != "jobs/job_shot_video_live_fields_123/images/segment_000_shot_000.jpg" {
		t.Fatalf("clip[0].SourceImagePath = %q", artifact.Clips[0].SourceImagePath)
	}
	if artifact.Clips[0].DurationSeconds != 5 {
		t.Fatalf("clip[0].DurationSeconds = %v, want 5", artifact.Clips[0].DurationSeconds)
	}
}

func TestShotVideoExecutorUsesInjectedClientWhenAvailable(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	client := &fakeShotVideoClient{
		response: Response{
			RequestID:       "req-shot-video-123",
			Model:           "wan2.6-i2v-flash",
			VideoURL:        "https://example.com/generated.mp4",
			VideoData:       []byte("fake-mp4-data"),
			DurationSeconds: 4.5,
		},
	}
	executor := NewShotVideoExecutorWithClient(
		client,
		GenerationConfig{Model: "wan2.6-i2v-flash"},
		workspaceDir,
		3,
	)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_shot_video_generated_123/images/image_manifest.json", map[string]any{
		"shot_images": []map[string]any{
			{
				"segment_index": 0,
				"shot_index":    0,
				"file_path":     "jobs/job_shot_video_generated_123/images/segment_000_shot_000.jpg",
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		"jobs/job_shot_video_generated_123/images/segment_000_shot_000.jpg",
	)

	updated, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_shot_video_generated_123"},
		model.Task{
			Key:     "shot_video",
			Type:    model.TaskTypeShotVideo,
			Payload: map[string]any{"video_count": 1},
		},
		map[string]model.Task{
			"image": {
				Key: "image",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_shot_video_generated_123/images/image_manifest.json",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("client requests len = %d, want 1", len(client.requests))
	}
	if client.requests[0].SourceImagePath != "jobs/job_shot_video_generated_123/images/segment_000_shot_000.jpg" {
		t.Fatalf("request source image path = %q", client.requests[0].SourceImagePath)
	}
	if updated.OutputRef["generated_video_count"] != 1 {
		t.Fatalf("generated_video_count = %#v, want 1", updated.OutputRef["generated_video_count"])
	}
	if updated.OutputRef["fallback_image_count"] != 0 {
		t.Fatalf("fallback_image_count = %#v, want 0", updated.OutputRef["fallback_image_count"])
	}
	if updated.OutputRef["requested_video_count"] != 1 {
		t.Fatalf("requested_video_count = %#v, want 1", updated.OutputRef["requested_video_count"])
	}
	if updated.OutputRef["selected_video_count"] != 1 {
		t.Fatalf("selected_video_count = %#v, want 1", updated.OutputRef["selected_video_count"])
	}
	if updated.OutputRef["generation_mode"] != "generated_video" {
		t.Fatalf("generation_mode = %#v, want %q", updated.OutputRef["generation_mode"], "generated_video")
	}

	artifact := readShotVideoArtifact(t, workspaceDir, "jobs/job_shot_video_generated_123/shot_videos/manifest.json")
	if len(artifact.Clips) != 1 {
		t.Fatalf("clips len = %d, want 1", len(artifact.Clips))
	}
	if artifact.Clips[0].Status != ShotVideoStatusGeneratedVideo {
		t.Fatalf("clip[0].Status = %q, want %q", artifact.Clips[0].Status, ShotVideoStatusGeneratedVideo)
	}
	if artifact.Clips[0].VideoPath != "jobs/job_shot_video_generated_123/shot_videos/segment_000_shot_000.mp4" {
		t.Fatalf("clip[0].VideoPath = %q", artifact.Clips[0].VideoPath)
	}
	if artifact.Clips[0].SourceImagePath != "jobs/job_shot_video_generated_123/images/segment_000_shot_000.jpg" {
		t.Fatalf("clip[0].SourceImagePath = %q", artifact.Clips[0].SourceImagePath)
	}
	if artifact.Clips[0].GenerationRequestID != "req-shot-video-123" {
		t.Fatalf("clip[0].GenerationRequestID = %q", artifact.Clips[0].GenerationRequestID)
	}
	if artifact.Clips[0].SourceVideoURL != "https://example.com/generated.mp4" {
		t.Fatalf("clip[0].SourceVideoURL = %q", artifact.Clips[0].SourceVideoURL)
	}
}

func TestShotVideoExecutorOnlyGeneratesLeadingShots(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	client := &fakeShotVideoClient{
		response: Response{
			RequestID: "req-shot-video-limit-123",
			Model:     "wan2.6-i2v-flash",
			VideoURL:  "https://example.com/generated-limit.mp4",
			VideoData: []byte("fake-mp4-data"),
		},
	}
	executor := NewShotVideoExecutorWithClient(
		client,
		GenerationConfig{Model: "wan2.6-i2v-flash"},
		workspaceDir,
		3,
	)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_shot_video_limit_123/images/image_manifest.json", map[string]any{
		"shot_images": []map[string]any{
			{
				"segment_index": 1,
				"shot_index":    1,
				"file_path":     "jobs/job_shot_video_limit_123/images/segment_001_shot_001.jpg",
			},
			{
				"segment_index": 0,
				"shot_index":    2,
				"file_path":     "jobs/job_shot_video_limit_123/images/segment_000_shot_002.jpg",
			},
			{
				"segment_index": 0,
				"shot_index":    1,
				"file_path":     "jobs/job_shot_video_limit_123/images/segment_000_shot_001.jpg",
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		"jobs/job_shot_video_limit_123/images/segment_001_shot_001.jpg",
		"jobs/job_shot_video_limit_123/images/segment_000_shot_002.jpg",
		"jobs/job_shot_video_limit_123/images/segment_000_shot_001.jpg",
	)

	updated, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_shot_video_limit_123"},
		model.Task{
			Key:     "shot_video",
			Type:    model.TaskTypeShotVideo,
			Payload: map[string]any{"video_count": 2},
		},
		map[string]model.Task{
			"image": {
				Key: "image",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_shot_video_limit_123/images/image_manifest.json",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("client requests len = %d, want 2", len(client.requests))
	}
	if client.requests[0].SourceImagePath != "jobs/job_shot_video_limit_123/images/segment_000_shot_001.jpg" {
		t.Fatalf("request[0] source image path = %q", client.requests[0].SourceImagePath)
	}
	if client.requests[1].SourceImagePath != "jobs/job_shot_video_limit_123/images/segment_000_shot_002.jpg" {
		t.Fatalf("request[1] source image path = %q", client.requests[1].SourceImagePath)
	}
	if updated.OutputRef["generated_video_count"] != 2 {
		t.Fatalf("generated_video_count = %#v, want 2", updated.OutputRef["generated_video_count"])
	}
	if updated.OutputRef["fallback_image_count"] != 1 {
		t.Fatalf("fallback_image_count = %#v, want 1", updated.OutputRef["fallback_image_count"])
	}
	if updated.OutputRef["selected_video_count"] != 2 {
		t.Fatalf("selected_video_count = %#v, want 2", updated.OutputRef["selected_video_count"])
	}
	if updated.OutputRef["generation_mode"] != "mixed" {
		t.Fatalf("generation_mode = %#v, want %q", updated.OutputRef["generation_mode"], "mixed")
	}

	artifact := readShotVideoArtifact(t, workspaceDir, "jobs/job_shot_video_limit_123/shot_videos/manifest.json")
	if len(artifact.Clips) != 3 {
		t.Fatalf("clips len = %d, want 3", len(artifact.Clips))
	}
	if artifact.Clips[0].SegmentIndex != 0 || artifact.Clips[0].ShotIndex != 1 {
		t.Fatalf("clip[0] order = (%d,%d), want (0,1)", artifact.Clips[0].SegmentIndex, artifact.Clips[0].ShotIndex)
	}
	if artifact.Clips[2].Status != ShotVideoStatusImageFallback {
		t.Fatalf("clip[2].Status = %q, want %q", artifact.Clips[2].Status, ShotVideoStatusImageFallback)
	}
}

func TestShotVideoExecutorAllowsZeroRequestedVideos(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	client := &fakeShotVideoClient{
		response: Response{
			RequestID: "req-shot-video-zero-123",
			Model:     "wan2.6-i2v-flash",
			VideoURL:  "https://example.com/generated-zero.mp4",
			VideoData: []byte("fake-mp4-data"),
		},
	}
	executor := NewShotVideoExecutorWithClient(
		client,
		GenerationConfig{Model: "wan2.6-i2v-flash"},
		workspaceDir,
		3,
	)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_shot_video_zero_123/images/image_manifest.json", map[string]any{
		"shot_images": []map[string]any{
			{
				"segment_index": 0,
				"shot_index":    0,
				"file_path":     "jobs/job_shot_video_zero_123/images/segment_000_shot_000.jpg",
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		"jobs/job_shot_video_zero_123/images/segment_000_shot_000.jpg",
	)

	updated, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_shot_video_zero_123"},
		model.Task{
			Key:     "shot_video",
			Type:    model.TaskTypeShotVideo,
			Payload: map[string]any{"video_count": 0},
		},
		map[string]model.Task{
			"image": {
				Key: "image",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_shot_video_zero_123/images/image_manifest.json",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("client requests len = %d, want 0", len(client.requests))
	}
	if updated.OutputRef["generated_video_count"] != 0 {
		t.Fatalf("generated_video_count = %#v, want 0", updated.OutputRef["generated_video_count"])
	}
	if updated.OutputRef["fallback_image_count"] != 1 {
		t.Fatalf("fallback_image_count = %#v, want 1", updated.OutputRef["fallback_image_count"])
	}
	if updated.OutputRef["selected_video_count"] != 0 {
		t.Fatalf("selected_video_count = %#v, want 0", updated.OutputRef["selected_video_count"])
	}
}

func TestShotVideoExecutorFallsBackWhenInjectedClientFails(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	client := &fakeShotVideoClient{err: fmt.Errorf("upstream unavailable")}
	executor := NewShotVideoExecutorWithClient(
		client,
		GenerationConfig{Model: "wan2.6-i2v-flash"},
		workspaceDir,
		3,
	)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_shot_video_client_fallback_123/images/image_manifest.json", map[string]any{
		"shot_images": []map[string]any{
			{
				"segment_index": 0,
				"shot_index":    0,
				"file_path":     "jobs/job_shot_video_client_fallback_123/images/segment_000_shot_000.jpg",
			},
		},
	})
	writeVideoTestMediaFiles(
		t,
		workspaceDir,
		"jobs/job_shot_video_client_fallback_123/images/segment_000_shot_000.jpg",
	)

	updated, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_shot_video_client_fallback_123"},
		model.Task{Key: "shot_video", Type: model.TaskTypeShotVideo},
		map[string]model.Task{
			"image": {
				Key: "image",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_shot_video_client_fallback_123/images/image_manifest.json",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if updated.OutputRef["generated_video_count"] != 0 {
		t.Fatalf("generated_video_count = %#v, want 0", updated.OutputRef["generated_video_count"])
	}
	if updated.OutputRef["fallback_image_count"] != 1 {
		t.Fatalf("fallback_image_count = %#v, want 1", updated.OutputRef["fallback_image_count"])
	}
	if updated.OutputRef["generation_mode"] != "image_fallback" {
		t.Fatalf("generation_mode = %#v, want %q", updated.OutputRef["generation_mode"], "image_fallback")
	}

	artifact := readShotVideoArtifact(t, workspaceDir, "jobs/job_shot_video_client_fallback_123/shot_videos/manifest.json")
	if artifact.Clips[0].Status != ShotVideoStatusImageFallback {
		t.Fatalf("clip[0].Status = %q, want %q", artifact.Clips[0].Status, ShotVideoStatusImageFallback)
	}
	if artifact.Clips[0].ImagePath != "jobs/job_shot_video_client_fallback_123/images/segment_000_shot_000.jpg" {
		t.Fatalf("clip[0].ImagePath = %q", artifact.Clips[0].ImagePath)
	}
}

func TestShotVideoExecutorRequiresShotImagesArtifact(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewShotVideoExecutor(workspaceDir, 3)
	writeVideoTestJSONArtifact(t, workspaceDir, "jobs/job_shot_video_123/images/image_manifest.json", map[string]any{
		"images": []map[string]any{{
			"file_path": "jobs/job_shot_video_123/images/segment_000.jpg",
		}},
	})

	_, err := executor.Execute(
		context.Background(),
		model.Job{PublicID: "job_shot_video_123"},
		model.Task{Key: "shot_video", Type: model.TaskTypeShotVideo},
		map[string]model.Task{
			"image": {
				Key: "image",
				OutputRef: map[string]any{
					"artifact_path": "jobs/job_shot_video_123/images/image_manifest.json",
				},
			},
		},
	)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}
