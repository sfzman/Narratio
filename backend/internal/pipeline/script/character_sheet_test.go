package script

import (
	"context"
	"strings"
	"testing"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func TestCharacterSheetExecutorExecute(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	executor := NewCharacterSheetExecutorWithClient(nil, TextGenerationConfig{}, workspaceDir)
	job := model.Job{
		ID:       3,
		PublicID: "job_test_character_sheet",
	}
	task := model.Task{
		ID:      30,
		Key:     "character_sheet",
		Type:    model.TaskTypeCharacterSheet,
		Attempt: 1,
		Payload: map[string]any{
			"article":  "Alice meets Bob in a test story.",
			"language": "en",
		},
		OutputRef: map[string]any{},
	}

	got, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got.OutputRef["artifact_type"] != "character_sheet" {
		t.Fatalf("artifact_type = %#v, want %#v", got.OutputRef["artifact_type"], "character_sheet")
	}
	if got.OutputRef["artifact_path"] != "jobs/job_test_character_sheet/character_sheet.json" {
		t.Fatalf("artifact_path = %#v, want %#v", got.OutputRef["artifact_path"], "jobs/job_test_character_sheet/character_sheet.json")
	}
	if got.OutputRef["character_count"] != 1 {
		t.Fatalf("character_count = %#v, want %#v", got.OutputRef["character_count"], 1)
	}

	artifact := readJSONArtifact[CharacterSheetOutput](
		t,
		workspaceDir,
		got.OutputRef["artifact_path"].(string),
	)
	if len(artifact.Characters) != 1 {
		t.Fatalf("len(characters) = %d, want 1", len(artifact.Characters))
	}
	if artifact.Characters[0].Name != "旁白视角角色" {
		t.Fatalf("characters[0].name = %q, want %q", artifact.Characters[0].Name, "旁白视角角色")
	}
	if artifact.Characters[0].ReferenceSubjectType != "人" {
		t.Fatalf("characters[0].reference_subject_type = %q, want %q", artifact.Characters[0].ReferenceSubjectType, "人")
	}
}

func TestCharacterSheetExecutorExecuteWithInjectedTextClient(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	client := &fakeTextClient{
		response: TextResponse{
			RequestID: "req_character_1",
			Model:     "qwen-plus",
			Choices: []Choice{
				{
					Message: ChatMessage{
						Role:    "assistant",
						Content: `{"characters":[{"name":"阿莲","role":"主角","age":"二十多岁","gender":"女","appearance":"眉眼清秀，黑色低马尾，深蓝粗布上衣，灰色长裤，偏瘦利落，神情警惕","temperament":"冷静克制","personality_traits":["坚韧","敏感"],"identity":"山村药铺学徒","relationship_to_protagonist":"主角本人","visual_signature":"腰间旧铜铃","reference_subject_type":"人","image_prompt_focus":"平视角、正面、单人、半身可见，关键特征完整露出，背景简洁。"}]}`,
					},
				},
			},
		},
	}
	executor := NewCharacterSheetExecutorWithClient(client, TextGenerationConfig{
		Model: "qwen-plus",
	}, workspaceDir)
	job := model.Job{
		ID:       4,
		PublicID: "job_test_character_sheet_live",
	}
	task := model.Task{
		ID:      40,
		Key:     "character_sheet",
		Type:    model.TaskTypeCharacterSheet,
		Attempt: 1,
		Payload: map[string]any{
			"article":  "Alice meets Bob in a test story.",
			"language": "zh",
		},
		OutputRef: map[string]any{},
	}

	got, err := executor.Execute(context.Background(), job, task, map[string]model.Task{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if client.request.ResponseFormat == nil || client.request.ResponseFormat.Type != "json_object" {
		t.Fatalf("response_format = %#v", client.request.ResponseFormat)
	}
	if !strings.Contains(client.request.Messages[0].Content, "中文影视角色设定整理助手") {
		t.Fatalf("system prompt = %q", client.request.Messages[0].Content)
	}
	if !strings.Contains(client.request.Messages[1].Content, "主要人物表整理") {
		t.Fatalf("user prompt = %q", client.request.Messages[1].Content)
	}
	if !strings.Contains(client.request.Messages[1].Content, "【小说全文开始】") {
		t.Fatalf("user prompt = %q", client.request.Messages[1].Content)
	}

	artifact := readJSONArtifact[CharacterSheetOutput](
		t,
		workspaceDir,
		got.OutputRef["artifact_path"].(string),
	)
	if len(artifact.Characters) != 1 {
		t.Fatalf("len(characters) = %d, want 1", len(artifact.Characters))
	}
	if artifact.Characters[0].Name != "阿莲" {
		t.Fatalf("characters[0].name = %q, want %q", artifact.Characters[0].Name, "阿莲")
	}
	if artifact.Characters[0].ReferenceSubjectType != "人" {
		t.Fatalf("characters[0].reference_subject_type = %q, want %q", artifact.Characters[0].ReferenceSubjectType, "人")
	}
	if artifact.Characters[0].ImagePromptFocus != "平视角、正面、单人、半身可见，关键特征完整露出，背景简洁。" {
		t.Fatalf("characters[0].image_prompt_focus = %q", artifact.Characters[0].ImagePromptFocus)
	}
	if got.OutputRef["llm_request_id"] != "req_character_1" {
		t.Fatalf("llm_request_id = %#v", got.OutputRef["llm_request_id"])
	}
}
