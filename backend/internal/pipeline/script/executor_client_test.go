package script

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type fakeTextClient struct {
	request  TextRequest
	response TextResponse
	err      error
}

func (f *fakeTextClient) Generate(_ context.Context, request TextRequest) (TextResponse, error) {
	f.request = request
	if f.err != nil {
		return TextResponse{}, f.err
	}

	return f.response, nil
}

func TestOutlineExecutorExecuteWithInjectedTextClient(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	client := &fakeTextClient{
		response: TextResponse{
			RequestID: "req_outline_1",
			Model:     "qwen-plus",
			Choices: []Choice{
				{
					Message: ChatMessage{
						Role:    "assistant",
						Content: `{"story_position":{"genre":"悬疑","era_background":"当代都市","core_conflict":"主角被迫追查真相","emotional_tone":"压抑","ending_type":"开放式"},"mainline":"主角在不断逼近真相的过程中被更大秘密反噬。","plot_stages":[{"name":"开端","happened":"命案发生","goal":"确认线索","obstacle":"线索不足","outcome":"开始追查"},{"name":"发展","happened":"调查深入","goal":"锁定嫌疑人","obstacle":"信息混乱","outcome":"关系紧张"},{"name":"转折","happened":"关键证据反转","goal":"重新判断真相","obstacle":"旧判断失效","outcome":"局势逆转"},{"name":"高潮","happened":"正面冲突爆发","goal":"揭开真相","obstacle":"对手反击","outcome":"秘密暴露"},{"name":"结局","happened":"事件收束","goal":"承受代价","obstacle":"创伤延续","outcome":"留下余波"}],"relationship_state_changes":["主角与搭档从试探转为互信"],"continuity_notes":["主角手部受伤持续存在"],"segment_reading_notes":["不要忘记主角始终怀疑搭档。"]}`,
					},
				},
			},
		},
	}
	executor := NewOutlineExecutorWithClient(client, TextGenerationConfig{
		Model: "qwen-plus",
	}, workspaceDir)
	job := model.Job{ID: 1, PublicID: "job_outline_llm"}
	task := model.Task{
		ID:      10,
		Key:     "outline",
		Type:    model.TaskTypeOutline,
		Attempt: 1,
		Payload: map[string]any{
			"article":  "This is a test article for outline generation.",
			"language": "zh",
		},
		OutputRef: map[string]any{},
	}

	got, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if client.request.Model != "qwen-plus" {
		t.Fatalf("request model = %q", client.request.Model)
	}
	if len(client.request.Messages) != 2 {
		t.Fatalf("request messages = %d", len(client.request.Messages))
	}
	if client.request.ResponseFormat == nil || client.request.ResponseFormat.Type != "json_object" {
		t.Fatalf("response_format = %#v", client.request.ResponseFormat)
	}
	if !strings.Contains(client.request.Messages[0].Content, "中文影视改编策划与分镜前置分析助手") {
		t.Fatalf("system prompt = %q", client.request.Messages[0].Content)
	}
	if !strings.Contains(client.request.Messages[1].Content, "剧情大纲整理") {
		t.Fatalf("user prompt = %q", client.request.Messages[1].Content)
	}
	if !strings.Contains(client.request.Messages[1].Content, "【小说全文开始】") {
		t.Fatalf("user prompt = %q", client.request.Messages[1].Content)
	}
	if got.OutputRef["llm_request_id"] != "req_outline_1" {
		t.Fatalf("llm_request_id = %#v", got.OutputRef["llm_request_id"])
	}
	if got.OutputRef["llm_model"] != "qwen-plus" {
		t.Fatalf("llm_model = %#v", got.OutputRef["llm_model"])
	}
	preview, ok := got.OutputRef["llm_response_preview"].(string)
	if !ok {
		t.Fatalf("llm_response_preview type = %T", got.OutputRef["llm_response_preview"])
	}
	if !strings.Contains(preview, `"story_position"`) {
		t.Fatalf("llm_response_preview = %#v", got.OutputRef["llm_response_preview"])
	}

	artifact := readJSONArtifact[OutlineOutput](
		t,
		workspaceDir,
		got.OutputRef["artifact_path"].(string),
	)
	if len(artifact.PlotStages) != 5 {
		t.Fatalf("len(plot_stages) = %d, want 5", len(artifact.PlotStages))
	}
	if artifact.StoryPosition.Genre != "悬疑" {
		t.Fatalf("story_position.genre = %q, want %q", artifact.StoryPosition.Genre, "悬疑")
	}
	if artifact.PlotStages[0].Name != "开端" {
		t.Fatalf("plot_stages[0].name = %q, want %q", artifact.PlotStages[0].Name, "开端")
	}
}

func TestScriptExecutorExecuteWithInjectedTextClient(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	writer := newArtifactWriter(workspaceDir)
	client := &fakeTextClient{
		response: TextResponse{
			RequestID: "req_script_1",
			Model:     "qwen-plus",
			Choices: []Choice{
				{
					Message: ChatMessage{
						Role:    "assistant",
						Content: `{"segments":[{"text":"第一段原文","script":"第一段旁白。","summary":"主角现身。"},{"text":"第二段原文","script":"第二段旁白。","summary":"冲突升级。"}]}`,
					},
				},
			},
		},
	}
	executor := NewScriptExecutorWithClient(client, TextGenerationConfig{
		Model: "qwen-plus",
	}, workspaceDir)
	job := model.Job{ID: 2, PublicID: "job_script_llm"}
	task := model.Task{
		ID:   20,
		Key:  "script",
		Type: model.TaskTypeScript,
		Payload: map[string]any{
			"article":  "A short article for script generation.",
			"language": "zh",
			"voice_id": "default",
		},
		OutputRef: map[string]any{},
	}
	dependencies := map[string]model.Task{
		"segmentation": {
			Key: "segmentation",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_llm/segments.json",
			},
		},
		"outline": {
			Key: "outline",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_llm/outline.json",
			},
		},
		"character_sheet": {
			Key: "character_sheet",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_llm/character_sheet.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_script_llm/segments.json", SegmentationOutput{
		Segments: []TextSegment{
			{Index: 0, Text: "第一段原文", CharCount: 5},
			{Index: 1, Text: "第二段原文", CharCount: 5},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(segmentation) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_script_llm/outline.json", OutlineOutput{
		Mainline: "主角调查真相。",
		PlotStages: []OutlineStage{
			{Name: "开端", Happened: "命案发生", Goal: "确认线索", Obstacle: "线索不足", Outcome: "开始追查"},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(outline) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_script_llm/character_sheet.json", CharacterSheetOutput{
		Characters: []CharacterProfile{
			{Name: "阿莲", Role: "主角", Appearance: "警惕冷静", ReferenceSubjectType: "人"},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(character_sheet) error = %v", err)
	}

	got, err := executor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if client.request.ResponseFormat == nil || client.request.ResponseFormat.Type != "json_object" {
		t.Fatalf("response_format = %#v", client.request.ResponseFormat)
	}
	if !strings.Contains(client.request.Messages[0].Content, "中文旁白脚本整理助手") {
		t.Fatalf("system prompt = %q", client.request.Messages[0].Content)
	}
	userPrompt := client.request.Messages[1].Content
	if !strings.Contains(userPrompt, "【分段结果开始】") {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if !strings.Contains(userPrompt, `"text": "第一段原文"`) {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if !strings.Contains(userPrompt, "【完整故事大纲开始】") {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if !strings.Contains(userPrompt, `"mainline": "主角调查真相。"`) {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if !strings.Contains(userPrompt, `"name": "阿莲"`) {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if got.OutputRef["llm_request_id"] != "req_script_1" {
		t.Fatalf("llm_request_id = %#v", got.OutputRef["llm_request_id"])
	}
	if got.OutputRef["llm_model"] != "qwen-plus" {
		t.Fatalf("llm_model = %#v", got.OutputRef["llm_model"])
	}

	artifact := readJSONArtifact[ScriptOutput](
		t,
		workspaceDir,
		got.OutputRef["artifact_path"].(string),
	)
	if len(artifact.Segments) != 2 {
		t.Fatalf("len(segments) = %d, want 2", len(artifact.Segments))
	}
	if artifact.Segments[0].Text != "第一段原文" {
		t.Fatalf("segments[0].text = %q", artifact.Segments[0].Text)
	}
	if artifact.Segments[0].Script != "第一段旁白。" {
		t.Fatalf("segments[0].script = %q", artifact.Segments[0].Script)
	}
	if got.OutputRef["segment_count"] != 2 {
		t.Fatalf("segment_count = %#v", got.OutputRef["segment_count"])
	}
}

func TestOutlineExecutorExecuteReturnsClientError(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewOutlineExecutorWithClient(&fakeTextClient{
		err: fmt.Errorf("upstream unavailable"),
	}, TextGenerationConfig{}, workspaceDir)
	job := model.Job{ID: 1, PublicID: "job_outline_error"}
	task := model.Task{
		ID:      10,
		Key:     "outline",
		Type:    model.TaskTypeOutline,
		Attempt: 1,
		Payload: map[string]any{
			"article":  "This is a test article for outline generation.",
			"language": "en",
		},
		OutputRef: map[string]any{},
	}

	_, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err == nil {
		t.Fatal("Execute() error = nil, want client error")
	}
}
