package script

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type fakeTextClient struct {
	request   TextRequest
	requests  []TextRequest
	response  TextResponse
	responses []TextResponse
	err       error
	errs      []error
}

func (f *fakeTextClient) Generate(_ context.Context, request TextRequest) (TextResponse, error) {
	f.request = request
	f.requests = append(f.requests, request)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return TextResponse{}, err
		}
	}
	if f.err != nil {
		return TextResponse{}, f.err
	}
	if len(f.responses) > 0 {
		response := f.responses[0]
		f.responses = f.responses[1:]
		return response, nil
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
			"article": "This is a test article for outline generation.",
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
		responses: []TextResponse{
			{
				RequestID: "req_script_1",
				Model:     "qwen-plus",
				Choices: []Choice{
					{
						Message: ChatMessage{
							Role:    "assistant",
							Content: `{"segments":[{"text":"第一段原文","script":"第一段旁白。","summary":"主角现身。","shots":[{"index":0,"prompt":"主角在雨夜现身。"},{"index":1,"prompt":"镜头跟随主角前行。"}]}]}`,
						},
					},
				},
			},
			{
				RequestID: "req_script_2",
				Model:     "qwen-plus",
				Choices: []Choice{
					{
						Message: ChatMessage{
							Role:    "assistant",
							Content: `{"segments":[{"text":"第二段原文","script":"第二段旁白。","summary":"冲突升级。","shots":[{"index":0,"prompt":"对手逼近。"}]}]}`,
						},
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

	if len(client.requests) != 2 {
		t.Fatalf("len(client.requests) = %d, want 2", len(client.requests))
	}
	if client.requests[0].ResponseFormat == nil || client.requests[0].ResponseFormat.Type != "json_object" {
		t.Fatalf("response_format = %#v", client.requests[0].ResponseFormat)
	}
	if client.requests[0].MaxTokens != defaultScriptMaxTokens {
		t.Fatalf("max_tokens = %d, want %d", client.requests[0].MaxTokens, defaultScriptMaxTokens)
	}
	if !strings.Contains(client.requests[0].Messages[0].Content, "中文影视分镜设计助手") {
		t.Fatalf("system prompt = %q", client.requests[0].Messages[0].Content)
	}
	if !strings.Contains(client.requests[0].Messages[0].Content, "3 个镜头") {
		t.Fatalf("system prompt = %q, want dynamic shot count", client.requests[0].Messages[0].Content)
	}
	userPrompt := client.requests[0].Messages[1].Content
	if !strings.Contains(userPrompt, "【当前分段开始】") {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if !strings.Contains(userPrompt, "当前段目标分镜数是 3") {
		t.Fatalf("user prompt = %q, want dynamic shot count hint", userPrompt)
	}
	if !strings.Contains(userPrompt, `"text": "第一段原文"`) {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if strings.Contains(userPrompt, `"text": "第二段原文"`) {
		t.Fatalf("user prompt = %q, want single-segment prompt", userPrompt)
	}
	if !strings.Contains(userPrompt, "【剧情大纲上下文开始】") {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if !strings.Contains(userPrompt, `"mainline": "主角调查真相。"`) {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if !strings.Contains(userPrompt, "【人物设定上下文开始】") {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	if !strings.Contains(userPrompt, `"name": "阿莲"`) {
		t.Fatalf("user prompt = %q", userPrompt)
	}
	secondPrompt := client.requests[1].Messages[1].Content
	if !strings.Contains(secondPrompt, `"text": "第二段原文"`) {
		t.Fatalf("second user prompt = %q", secondPrompt)
	}
	if strings.Contains(secondPrompt, `"text": "第一段原文"`) {
		t.Fatalf("second user prompt = %q, want single-segment prompt", secondPrompt)
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
	if artifact.Segments[0].Index != 0 {
		t.Fatalf("segments[0].index = %d", artifact.Segments[0].Index)
	}
	if artifact.Segments[1].Index != 1 {
		t.Fatalf("segments[1].index = %d", artifact.Segments[1].Index)
	}
	wantShots := targetShotCount(TextSegment{Text: "第一段原文", CharCount: 5})
	if len(artifact.Segments[0].Shots) != wantShots {
		t.Fatalf("len(segments[0].shots) = %d, want %d", len(artifact.Segments[0].Shots), wantShots)
	}
	if effectiveShotPrompt(artifact.Segments[0].Shots[0]) != "主角在雨夜现身。" {
		t.Fatalf("segments[0].shots[0] effective prompt = %q", effectiveShotPrompt(artifact.Segments[0].Shots[0]))
	}
	if effectiveShotPrompt(artifact.Segments[1].Shots[0]) != "对手逼近。" {
		t.Fatalf("segments[1].shots[0] effective prompt = %q", effectiveShotPrompt(artifact.Segments[1].Shots[0]))
	}
	if effectiveShotPrompt(artifact.Segments[1].Shots[wantShots-1]) == "" {
		t.Fatalf("segments[1].shots[%d] effective prompt = empty, want fallback-filled shot", wantShots-1)
	}
	if got.OutputRef["segment_count"] != 2 {
		t.Fatalf("segment_count = %#v", got.OutputRef["segment_count"])
	}
	if _, err := os.Stat(
		artifactFullPath(workspaceDir, "jobs/job_script_llm/script/segment_000.json"),
	); err != nil {
		t.Fatalf("Stat(segment_000.json) error = %v", err)
	}
	if _, err := os.Stat(
		artifactFullPath(workspaceDir, "jobs/job_script_llm/script/segment_001.json"),
	); err != nil {
		t.Fatalf("Stat(segment_001.json) error = %v", err)
	}
}

func TestScriptExecutorExecuteParsesWrappedJSONResponse(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	writer := newArtifactWriter(workspaceDir)
	client := &fakeTextClient{
		response: TextResponse{
			RequestID: "req_script_wrapped_1",
			Model:     "qwen-plus",
			Choices: []Choice{
				{
					Message: ChatMessage{
						Role: "assistant",
						Content: "```json\n" +
							`{"segments":[{"text":"第一段原文","script":"第一段旁白。","summary":"主角现身。","shots":[{"index":0,"prompt":"主角在雨夜现身。"}]}]}` +
							"\n```",
					},
				},
			},
		},
	}
	executor := NewScriptExecutorWithClient(client, TextGenerationConfig{
		Model: "qwen-plus",
	}, workspaceDir)
	job := model.Job{ID: 3, PublicID: "job_script_wrapped_llm"}
	task := model.Task{
		ID:   21,
		Key:  "script",
		Type: model.TaskTypeScript,
		Payload: map[string]any{
			"article":  "A short article for script generation.",
			"voice_id": "default",
		},
		OutputRef: map[string]any{},
	}
	dependencies := map[string]model.Task{
		"segmentation": {
			Key: "segmentation",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_wrapped_llm/segments.json",
			},
		},
		"outline": {
			Key: "outline",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_wrapped_llm/outline.json",
			},
		},
		"character_sheet": {
			Key: "character_sheet",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_wrapped_llm/character_sheet.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_script_wrapped_llm/segments.json", SegmentationOutput{
		Segments: []TextSegment{
			{Index: 0, Text: "第一段原文", CharCount: 5},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(segmentation) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_script_wrapped_llm/outline.json", OutlineOutput{
		Mainline: "主角调查真相。",
	}); err != nil {
		t.Fatalf("WriteJSON(outline) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_script_wrapped_llm/character_sheet.json", CharacterSheetOutput{
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

	artifact := readJSONArtifact[ScriptOutput](
		t,
		workspaceDir,
		got.OutputRef["artifact_path"].(string),
	)
	if len(artifact.Segments) != 1 {
		t.Fatalf("len(segments) = %d, want 1", len(artifact.Segments))
	}
	if effectiveShotPrompt(artifact.Segments[0].Shots[0]) != "主角在雨夜现身。" {
		t.Fatalf("segments[0].shots[0] effective prompt = %q", effectiveShotPrompt(artifact.Segments[0].Shots[0]))
	}
}

func TestScriptExecutorExecuteResumesFromSavedSegmentArtifacts(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	writer := newArtifactWriter(workspaceDir)
	firstClient := &fakeTextClient{
		responses: []TextResponse{
			{
				RequestID: "req_script_resume_1",
				Model:     "qwen-plus",
				Choices: []Choice{
					{
						Message: ChatMessage{
							Role:    "assistant",
							Content: `{"segments":[{"text":"第一段原文","script":"第一段旁白。","summary":"主角现身。","shots":[{"index":0,"prompt":"主角在雨夜现身。"}]}]}`,
						},
					},
				},
			},
		},
		errs: []error{nil, fmt.Errorf("upstream interrupted")},
	}
	executor := NewScriptExecutorWithClient(firstClient, TextGenerationConfig{
		Model: "qwen-plus",
	}, workspaceDir)
	job := model.Job{ID: 4, PublicID: "job_script_resume"}
	task := model.Task{
		ID:   22,
		Key:  "script",
		Type: model.TaskTypeScript,
		Payload: map[string]any{
			"article":  "A short article for script generation.",
			"voice_id": "default",
		},
		OutputRef: map[string]any{},
	}
	dependencies := map[string]model.Task{
		"segmentation": {
			Key: "segmentation",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_resume/segments.json",
			},
		},
		"outline": {
			Key: "outline",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_resume/outline.json",
			},
		},
		"character_sheet": {
			Key: "character_sheet",
			OutputRef: map[string]any{
				"artifact_path": "jobs/job_script_resume/character_sheet.json",
			},
		},
	}
	if err := writer.WriteJSON("jobs/job_script_resume/segments.json", SegmentationOutput{
		Segments: []TextSegment{
			{Index: 0, Text: "第一段原文", CharCount: 5},
			{Index: 1, Text: "第二段原文", CharCount: 5},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(segmentation) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_script_resume/outline.json", OutlineOutput{
		Mainline: "主角调查真相。",
	}); err != nil {
		t.Fatalf("WriteJSON(outline) error = %v", err)
	}
	if err := writer.WriteJSON("jobs/job_script_resume/character_sheet.json", CharacterSheetOutput{
		Characters: []CharacterProfile{
			{Name: "阿莲", Role: "主角", Appearance: "警惕冷静", ReferenceSubjectType: "人"},
		},
	}); err != nil {
		t.Fatalf("WriteJSON(character_sheet) error = %v", err)
	}

	_, err := executor.Execute(context.Background(), job, task, dependencies)
	if err == nil {
		t.Fatal("first Execute() error = nil, want interrupted error")
	}
	if len(firstClient.requests) != 2 {
		t.Fatalf("len(firstClient.requests) = %d, want 2", len(firstClient.requests))
	}
	if _, err := os.Stat(
		artifactFullPath(workspaceDir, "jobs/job_script_resume/script/segment_000.json"),
	); err != nil {
		t.Fatalf("Stat(saved segment_000.json) error = %v", err)
	}
	if _, err := os.Stat(
		artifactFullPath(workspaceDir, "jobs/job_script_resume/script.json"),
	); !os.IsNotExist(err) {
		t.Fatalf("Stat(script.json) error = %v, want not exist", err)
	}

	resumeClient := &fakeTextClient{
		responses: []TextResponse{
			{
				RequestID: "req_script_resume_2",
				Model:     "qwen-plus",
				Choices: []Choice{
					{
						Message: ChatMessage{
							Role:    "assistant",
							Content: `{"segments":[{"text":"第二段原文","script":"第二段旁白。","summary":"冲突升级。","shots":[{"index":0,"prompt":"对手逼近。"}]}]}`,
						},
					},
				},
			},
		},
	}
	resumeExecutor := NewScriptExecutorWithClient(resumeClient, TextGenerationConfig{
		Model: "qwen-plus",
	}, workspaceDir)

	got, err := resumeExecutor.Execute(context.Background(), job, task, dependencies)
	if err != nil {
		t.Fatalf("resume Execute() error = %v", err)
	}
	if len(resumeClient.requests) != 1 {
		t.Fatalf("len(resumeClient.requests) = %d, want 1", len(resumeClient.requests))
	}
	if !strings.Contains(resumeClient.requests[0].Messages[1].Content, `"text": "第二段原文"`) {
		t.Fatalf("resume prompt = %q", resumeClient.requests[0].Messages[1].Content)
	}
	if strings.Contains(resumeClient.requests[0].Messages[1].Content, `"text": "第一段原文"`) {
		t.Fatalf("resume prompt = %q, want only remaining segment", resumeClient.requests[0].Messages[1].Content)
	}

	artifact := readJSONArtifact[ScriptOutput](
		t,
		workspaceDir,
		got.OutputRef["artifact_path"].(string),
	)
	if len(artifact.Segments) != 2 {
		t.Fatalf("len(artifact.Segments) = %d, want 2", len(artifact.Segments))
	}
	if artifact.Segments[0].Index != 0 {
		t.Fatalf("segments[0].index = %d", artifact.Segments[0].Index)
	}
	if artifact.Segments[1].Index != 1 {
		t.Fatalf("segments[1].index = %d", artifact.Segments[1].Index)
	}
	if effectiveShotPrompt(artifact.Segments[0].Shots[0]) != "主角在雨夜现身。" {
		t.Fatalf("segments[0].shots[0] effective prompt = %q", effectiveShotPrompt(artifact.Segments[0].Shots[0]))
	}
	if effectiveShotPrompt(artifact.Segments[1].Shots[0]) != "对手逼近。" {
		t.Fatalf("segments[1].shots[0] effective prompt = %q", effectiveShotPrompt(artifact.Segments[1].Shots[0]))
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
			"article": "This is a test article for outline generation.",
		},
		OutputRef: map[string]any{},
	}

	_, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err == nil {
		t.Fatal("Execute() error = nil, want client error")
	}
}
