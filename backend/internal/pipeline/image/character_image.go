package image

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type CharacterImageExecutor struct {
	log       *slog.Logger
	artifacts artifactWriter
}

type CharacterImageOutput struct {
	Images []CharacterReferenceImage `json:"images"`
}

type CharacterReferenceImage struct {
	CharacterIndex       int      `json:"character_index"`
	CharacterName        string   `json:"character_name"`
	ReferenceSubjectType string   `json:"reference_subject_type"`
	FilePath             string   `json:"file_path"`
	Prompt               string   `json:"prompt"`
	MatchTerms           []string `json:"match_terms"`
	IsFallback           bool     `json:"is_fallback"`
}

type characterSheetArtifact struct {
	Characters []characterProfileArtifact `json:"characters"`
}

type characterProfileArtifact struct {
	Name                 string `json:"name"`
	Appearance           string `json:"appearance"`
	VisualSignature      string `json:"visual_signature"`
	ReferenceSubjectType string `json:"reference_subject_type"`
	ImagePromptFocus     string `json:"image_prompt_focus"`
}

func NewCharacterImageExecutor(workspaceDir string) *CharacterImageExecutor {
	return &CharacterImageExecutor{
		log:       slog.Default().With("executor", "character_image"),
		artifacts: newArtifactWriter(workspaceDir),
	}
}

func (e *CharacterImageExecutor) Execute(
	_ context.Context,
	job model.Job,
	task model.Task,
	dependencies map[string]model.Task,
) (model.Task, error) {
	characterSheetTask, ok := dependencies["character_sheet"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "character_sheet")
	}

	characterSheet, err := loadArtifactJSON[characterSheetArtifact](
		e.artifacts.workspaceDir,
		characterSheetTask.OutputRef["artifact_path"],
	)
	if err != nil {
		return task, fmt.Errorf("load character sheet artifact: %w", err)
	}

	artifactPath := fmt.Sprintf("jobs/%s/character_images/manifest.json", job.PublicID)
	output := buildCharacterImageOutput(job.PublicID, characterSheet)
	if err := e.artifacts.WriteJSON(artifactPath, output); err != nil {
		return task, fmt.Errorf("write character image artifact: %w", err)
	}

	task.OutputRef = map[string]any{
		"artifact_type":         "character_image",
		"artifact_path":         artifactPath,
		"character_sheet_ref":   characterSheetTask.OutputRef["artifact_path"],
		"character_image_count": len(output.Images),
	}

	e.log.Info("character image execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", artifactPath,
		"image_count", len(output.Images),
	)

	return task, nil
}

func buildCharacterImageOutput(
	jobPublicID string,
	characterSheet characterSheetArtifact,
) CharacterImageOutput {
	output := CharacterImageOutput{
		Images: make([]CharacterReferenceImage, 0, len(characterSheet.Characters)),
	}

	for index, character := range characterSheet.Characters {
		name := strings.TrimSpace(character.Name)
		if name == "" {
			name = fmt.Sprintf("character_%d", index+1)
		}

		output.Images = append(output.Images, CharacterReferenceImage{
			CharacterIndex:       index,
			CharacterName:        name,
			ReferenceSubjectType: fallbackString(character.ReferenceSubjectType, "person"),
			FilePath: fmt.Sprintf(
				"jobs/%s/character_images/character_%03d.jpg",
				jobPublicID,
				index,
			),
			Prompt:     buildCharacterImagePrompt(character),
			MatchTerms: buildCharacterMatchTerms(name),
			IsFallback: true,
		})
	}

	return output
}

func buildCharacterImagePrompt(character characterProfileArtifact) string {
	parts := []string{
		strings.TrimSpace(character.Name),
		strings.TrimSpace(character.Appearance),
		strings.TrimSpace(character.VisualSignature),
		strings.TrimSpace(character.ImagePromptFocus),
	}

	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}

	return strings.Join(filtered, "; ")
}

func buildCharacterMatchTerms(name string) []string {
	terms := make([]string, 0, 4)
	appendTerm := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		for _, existing := range terms {
			if strings.EqualFold(existing, trimmed) {
				return
			}
		}
		terms = append(terms, trimmed)
	}

	appendTerm(name)
	for _, token := range splitAliasTerms(name) {
		appendTerm(token)
		compacted := strings.Join(strings.Fields(token), "")
		if compacted != token {
			appendTerm(compacted)
		}
	}

	return terms
}

func splitAliasTerms(value string) []string {
	replacer := strings.NewReplacer(
		"/", "\n",
		"|", "\n",
		"、", "\n",
		"，", "\n",
		",", "\n",
		"；", "\n",
		";", "\n",
	)
	return strings.Split(replacer.Replace(value), "\n")
}

func fallbackString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}

	return fallback
}

func loadArtifactJSON[T any](workspaceDir string, ref any) (T, error) {
	var zero T

	path, ok := ref.(string)
	if !ok || strings.TrimSpace(path) == "" {
		return zero, fmt.Errorf("artifact ref is invalid: %v", ref)
	}

	data, err := os.ReadFile(artifactFullPath(workspaceDir, path))
	if err != nil {
		return zero, fmt.Errorf("read artifact file: %w", err)
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return zero, fmt.Errorf("decode artifact json: %w", err)
	}

	return value, nil
}
