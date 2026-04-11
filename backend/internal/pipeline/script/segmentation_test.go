package script

import (
	"context"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestSegmentationExecutorExecute(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewSegmentationExecutor(workspaceDir)
	job := model.Job{
		ID:       1,
		PublicID: "job_test_segmentation",
	}
	task := model.Task{
		ID:   10,
		Key:  "segmentation",
		Type: model.TaskTypeSegmentation,
		Payload: map[string]any{
			"article": "第一句。第二句。第三句。",
		},
		OutputRef: map[string]any{},
	}

	got, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got.OutputRef["artifact_type"] != "segmentation" {
		t.Fatalf("artifact_type = %#v, want %#v", got.OutputRef["artifact_type"], "segmentation")
	}
	if got.OutputRef["artifact_path"] != "jobs/job_test_segmentation/segments.json" {
		t.Fatalf("artifact_path = %#v", got.OutputRef["artifact_path"])
	}
	if got.OutputRef["segment_count"] != 1 {
		t.Fatalf("segment_count = %#v, want %#v", got.OutputRef["segment_count"], 1)
	}

	artifact := readJSONArtifact[SegmentationOutput](
		t,
		workspaceDir,
		got.OutputRef["artifact_path"].(string),
	)
	if len(artifact.Segments) != 1 {
		t.Fatalf("len(segments) = %d, want 1", len(artifact.Segments))
	}
	if artifact.Segments[0].Text != "第一句。第二句。第三句。" {
		t.Fatalf("segments[0].text = %q", artifact.Segments[0].Text)
	}
	if artifact.Segments[0].CharCount != 9 {
		t.Fatalf("segments[0].char_count = %d, want 9", artifact.Segments[0].CharCount)
	}
}

func TestSegmentArticleMergesShortTailIntoPreviousSegment(t *testing.T) {
	t.Parallel()

	segments := segmentArticle("甲乙丙丁。戊己庚辛。壬癸子丑。尾。", 10)
	if len(segments) != 1 {
		t.Fatalf("len(segments) = %d, want 1", len(segments))
	}
	if segments[0] != "甲乙丙丁。戊己庚辛。壬癸子丑。尾。" {
		t.Fatalf("segments[0] = %q", segments[0])
	}
}

func TestTargetShotCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		segment TextSegment
		want    int
	}{
		{
			name:    "short segment keeps minimum shots",
			segment: TextSegment{Text: "很短的一段。", CharCount: 5},
			want:    minShotsPerSegment,
		},
		{
			name:    "medium segment scales with char count",
			segment: TextSegment{Text: "这一段的内容长度适中。", CharCount: 120},
			want:    4,
		},
		{
			name:    "long segment caps at max shots",
			segment: TextSegment{Text: "长段落", CharCount: 400},
			want:    defaultShotsPerSegment,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := targetShotCount(tc.segment); got != tc.want {
				t.Fatalf("targetShotCount() = %d, want %d", got, tc.want)
			}
		})
	}
}
