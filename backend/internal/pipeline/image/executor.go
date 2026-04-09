package image

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const (
	defaultImageWidth  = 1280
	defaultImageHeight = 720
	maxPromptShots     = 3
)

type Executor struct {
	log              *slog.Logger
	client           Client
	generationConfig GenerationConfig
	artifacts        artifactWriter
}

func NewExecutor(workspaceDir string) *Executor {
	return NewExecutorWithClient(nil, GenerationConfig{}, workspaceDir)
}

func NewExecutorWithClient(
	client Client,
	generationConfig GenerationConfig,
	workspaceDir string,
) *Executor {
	return &Executor{
		log:              slog.Default().With("executor", "image"),
		client:           client,
		generationConfig: normalizeGenerationConfig(generationConfig),
		artifacts:        newArtifactWriter(workspaceDir),
	}
}

func (e *Executor) Execute(
	ctx context.Context,
	job model.Job,
	task model.Task,
	dependencies map[string]model.Task,
) (model.Task, error) {
	imageStyle, err := payloadString(task.Payload, "image_style")
	if err != nil {
		e.log.Error("image payload invalid",
			"job_id", job.ID,
			"job_public_id", job.PublicID,
			"task_id", task.ID,
			"task_key", task.Key,
			"error", err,
		)
		return task, err
	}

	scriptTask, ok := dependencies["script"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "script")
	}
	characterImageTask, ok := dependencies["character_image"]
	if !ok {
		return task, fmt.Errorf("missing dependency %q", "character_image")
	}

	scriptOutput, err := loadArtifactJSON[scriptArtifactOutput](
		e.artifacts.workspaceDir,
		scriptTask.OutputRef["artifact_path"],
	)
	if err != nil {
		return task, fmt.Errorf("load script artifact: %w", err)
	}
	characterImages, err := loadArtifactJSON[CharacterImageOutput](
		e.artifacts.workspaceDir,
		characterImageTask.OutputRef["artifact_path"],
	)
	if err != nil {
		return task, fmt.Errorf("load character image artifact: %w", err)
	}

	e.log.Debug("image execution started",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"attempt", task.Attempt,
	)

	artifactPath := fmt.Sprintf("jobs/%s/images/image_manifest.json", job.PublicID)
	output, err := e.generateOutput(ctx, job.PublicID, imageStyle, scriptOutput, characterImages)
	if err != nil {
		return task, err
	}
	if err := e.artifacts.WriteJSON(artifactPath, output); err != nil {
		return task, fmt.Errorf("write image artifact: %w", err)
	}

	task.OutputRef = map[string]any{
		"artifact_type":                "image",
		"artifact_path":                artifactPath,
		"script_artifact_ref":          scriptTask.OutputRef["artifact_path"],
		"character_image_artifact_ref": characterImageTask.OutputRef["artifact_path"],
		"image_style":                  imageStyle,
		"image_count":                  len(output.Images),
		"generated_image_count":        countGeneratedImages(output.Images),
		"fallback_image_count":         countFallbackImages(output.Images),
		"character_reference_count":    len(characterImages.Images),
		"images":                       output.Images,
	}

	e.log.Info("image execution completed",
		"job_id", job.ID,
		"job_public_id", job.PublicID,
		"task_id", task.ID,
		"task_key", task.Key,
		"artifact_path", task.OutputRef["artifact_path"],
	)

	return task, nil
}

func (e *Executor) generateOutput(
	ctx context.Context,
	jobPublicID string,
	imageStyle string,
	scriptOutput scriptArtifactOutput,
	characterImages CharacterImageOutput,
) (ImageOutput, error) {
	output := buildImageOutput(jobPublicID, imageStyle, scriptOutput, characterImages)
	if err := e.generateLiveImages(ctx, output.Images); err != nil {
		return ImageOutput{}, err
	}
	if err := writeFallbackImages(e.artifacts, output.Images); err != nil {
		return ImageOutput{}, err
	}

	return output, nil
}

func (e *Executor) generateLiveImages(ctx context.Context, images []GeneratedImage) error {
	if e.client == nil {
		return nil
	}

	for index := range images {
		generated, err := e.client.Generate(ctx, Request{
			Model:          e.generationConfig.Model,
			Prompt:         images[index].Prompt,
			Size:           e.generationConfig.Size,
			NegativePrompt: e.generationConfig.NegativePrompt,
		})
		if err != nil {
			e.log.Warn("image generation failed, writing fallback image",
				"segment_index", images[index].SegmentIndex,
				"error", err,
			)
			continue
		}
		if err := e.artifacts.WriteBytes(images[index].FilePath, generated.ImageData); err != nil {
			return fmt.Errorf("write generated image file: %w", err)
		}
		images[index].IsFallback = false
		images[index].GenerationRequestID = strings.TrimSpace(generated.RequestID)
		images[index].GenerationModel = strings.TrimSpace(generated.Model)
		images[index].SourceImageURL = strings.TrimSpace(generated.ImageURL)
	}

	return nil
}

type ImageOutput struct {
	Images []GeneratedImage `json:"images"`
}

type GeneratedImage struct {
	SegmentIndex        int                       `json:"segment_index"`
	FilePath            string                    `json:"file_path"`
	Width               int                       `json:"width"`
	Height              int                       `json:"height"`
	IsFallback          bool                      `json:"is_fallback"`
	Prompt              string                    `json:"prompt"`
	PromptSourceType    string                    `json:"prompt_source_type"`
	PromptSourceText    string                    `json:"prompt_source_text,omitempty"`
	PromptSourceShots   []string                  `json:"prompt_source_shots,omitempty"`
	GenerationRequestID string                    `json:"generation_request_id,omitempty"`
	GenerationModel     string                    `json:"generation_model,omitempty"`
	SourceImageURL      string                    `json:"source_image_url,omitempty"`
	CharacterReferences []ImageCharacterReference `json:"character_references"`
	MatchedCharacters   []ImageCharacterReference `json:"matched_characters"`
}

type ImageCharacterReference struct {
	CharacterIndex int      `json:"character_index"`
	CharacterName  string   `json:"character_name"`
	FilePath       string   `json:"file_path"`
	Prompt         string   `json:"prompt"`
	MatchTerms     []string `json:"match_terms"`
}

type scriptArtifactOutput struct {
	Segments []scriptArtifactSegment `json:"segments"`
}

type scriptArtifactSegment struct {
	Index int                  `json:"index"`
	Shots []scriptArtifactShot `json:"shots"`
}

type scriptArtifactShot struct {
	Index              int      `json:"index"`
	VisualContent      string   `json:"visual_content,omitempty"`
	CameraDesign       string   `json:"camera_design,omitempty"`
	InvolvedCharacters []string `json:"involved_characters,omitempty"`
	ImagePrompt        string   `json:"image_to_image_prompt,omitempty"`
	TextPrompt         string   `json:"text_to_image_prompt,omitempty"`
	Prompt             string   `json:"prompt,omitempty"`
}

func buildImageOutput(
	jobPublicID string,
	imageStyle string,
	scriptOutput scriptArtifactOutput,
	characterImages CharacterImageOutput,
) ImageOutput {
	output := ImageOutput{Images: make([]GeneratedImage, 0, len(scriptOutput.Segments))}
	references := buildImageCharacterReferences(characterImages)
	for _, segment := range scriptOutput.Segments {
		matched := matchSegmentCharacters(segment, references)
		source := resolveSegmentPromptSource(segment, matched)
		selected, matchedSelected := selectPromptCharacters(references, matched)
		output.Images = append(output.Images, GeneratedImage{
			SegmentIndex:        segment.Index,
			FilePath:            fmt.Sprintf("jobs/%s/images/segment_%03d.jpg", jobPublicID, segment.Index),
			Width:               defaultImageWidth,
			Height:              defaultImageHeight,
			IsFallback:          true,
			Prompt:              buildSegmentImagePrompt(source.Text, imageStyle, selected, matchedSelected),
			PromptSourceType:    source.Type,
			PromptSourceText:    source.Text,
			PromptSourceShots:   source.Shots,
			CharacterReferences: references,
			MatchedCharacters:   matched,
		})
	}

	return output
}

func countGeneratedImages(images []GeneratedImage) int {
	count := 0
	for _, image := range images {
		if image.IsFallback {
			continue
		}
		count++
	}

	return count
}

func countFallbackImages(images []GeneratedImage) int {
	count := 0
	for _, image := range images {
		if !image.IsFallback {
			continue
		}
		count++
	}

	return count
}

func buildImageCharacterReferences(
	characterImages CharacterImageOutput,
) []ImageCharacterReference {
	references := make([]ImageCharacterReference, 0, len(characterImages.Images))
	for _, item := range characterImages.Images {
		references = append(references, ImageCharacterReference{
			CharacterIndex: item.CharacterIndex,
			CharacterName:  item.CharacterName,
			FilePath:       item.FilePath,
			Prompt:         item.Prompt,
			MatchTerms:     item.MatchTerms,
		})
	}

	return references
}

func buildSegmentImagePrompt(
	base string,
	imageStyle string,
	selected []ImageCharacterReference,
	matchedSelected bool,
) string {
	parts := []string{strings.TrimSpace(base)}
	if len(selected) > 0 {
		label := "candidate characters: "
		if matchedSelected {
			label = "matched characters: "
		}
		parts = append(parts, label+joinReferenceNames(selected))
		parts = append(parts, "character reference details: "+joinReferencePrompts(selected))
	}
	parts = append(parts, "style: "+strings.TrimSpace(imageStyle))
	parts = append(parts, "cinematic composition, high quality, 16:9")
	parts = append(parts, "no face close-up")

	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		filtered = append(filtered, strings.TrimSpace(part))
	}

	return strings.Join(filtered, "; ")
}

func selectPromptCharacters(
	references []ImageCharacterReference,
	matched []ImageCharacterReference,
) ([]ImageCharacterReference, bool) {
	if len(matched) > 0 {
		return matched, true
	}

	return references, false
}

func matchSegmentCharacters(
	segment scriptArtifactSegment,
	references []ImageCharacterReference,
) []ImageCharacterReference {
	searchText := strings.ToLower(joinShotMatchText(segment.Shots))
	matched := make([]ImageCharacterReference, 0, len(references))
	for _, item := range references {
		if !referenceMatched(searchText, item) {
			continue
		}
		matched = append(matched, item)
	}

	return matched
}

func referenceMatched(searchText string, item ImageCharacterReference) bool {
	matchTerms := item.MatchTerms
	if len(matchTerms) == 0 {
		matchTerms = []string{item.CharacterName}
	}

	return textMatchesTerms(searchText, matchTerms)
}

func joinReferenceNames(references []ImageCharacterReference) string {
	names := make([]string, 0, len(references))
	for _, item := range references {
		name := strings.TrimSpace(item.CharacterName)
		if name == "" {
			continue
		}
		names = append(names, name)
	}

	return strings.Join(names, ", ")
}

func joinReferencePrompts(references []ImageCharacterReference) string {
	details := make([]string, 0, len(references))
	for _, item := range references {
		name := strings.TrimSpace(item.CharacterName)
		prompt := strings.TrimSpace(item.Prompt)
		switch {
		case name != "" && prompt != "":
			details = append(details, name+": "+prompt)
		case prompt != "":
			details = append(details, prompt)
		case name != "":
			details = append(details, name)
		}
	}

	return strings.Join(details, " | ")
}

type segmentPromptSource struct {
	Type  string
	Text  string
	Shots []string
}

func resolveSegmentPromptSource(
	segment scriptArtifactSegment,
	matched []ImageCharacterReference,
) segmentPromptSource {
	shotPrompts := selectSegmentShotPrompts(segment.Shots, matched)
	if len(shotPrompts) > 0 {
		return segmentPromptSource{
			Type:  "shots",
			Text:  strings.Join(shotPrompts, " | "),
			Shots: shotPrompts,
		}
	}

	return segmentPromptSource{Type: "empty"}
}

func selectSegmentShotPrompts(
	shots []scriptArtifactShot,
	matched []ImageCharacterReference,
) []string {
	prompts := collectShotPrompts(shots)
	if len(prompts) <= maxPromptShots {
		return prompts
	}

	selected := selectShotIndexes(prompts, matched)
	result := make([]string, 0, len(selected))
	for _, index := range selected {
		if index < 0 || index >= len(prompts) {
			continue
		}
		result = append(result, prompts[index])
	}

	return result
}

func selectShotIndexes(
	prompts []string,
	matched []ImageCharacterReference,
) []int {
	selected := make([]int, 0, maxPromptShots)
	seen := make(map[int]struct{}, maxPromptShots)
	appendIndex := func(index int) {
		if len(selected) == maxPromptShots {
			return
		}
		if index < 0 || index >= len(prompts) {
			return
		}
		if _, ok := seen[index]; ok {
			return
		}
		seen[index] = struct{}{}
		selected = append(selected, index)
	}

	for _, index := range matchedShotIndexes(prompts, matched) {
		appendIndex(index)
	}
	for _, index := range coverageShotIndexes(len(prompts)) {
		appendIndex(index)
	}
	for index := range prompts {
		appendIndex(index)
	}

	sort.Ints(selected)
	return selected
}

func matchedShotIndexes(
	prompts []string,
	matched []ImageCharacterReference,
) []int {
	terms := buildMatchTermSet(matched)
	if len(terms) == 0 {
		return nil
	}

	indexes := make([]int, 0, len(prompts))
	for index, prompt := range prompts {
		if !textMatchesTerms(prompt, terms) {
			continue
		}
		indexes = append(indexes, index)
	}

	return indexes
}

func coverageShotIndexes(total int) []int {
	if total <= 0 {
		return nil
	}
	if total <= maxPromptShots {
		indexes := make([]int, 0, total)
		for index := 0; index < total; index++ {
			indexes = append(indexes, index)
		}
		return indexes
	}

	return []int{0, total / 2, total - 1}
}

func joinShotPrompts(shots []scriptArtifactShot) string {
	return strings.Join(collectShotPrompts(shots), " | ")
}

func joinShotMatchText(shots []scriptArtifactShot) string {
	parts := make([]string, 0, len(shots)*2)
	for _, shot := range shots {
		if prompt := effectiveShotPrompt(shot); prompt != "" {
			parts = append(parts, prompt)
		}
		if len(shot.InvolvedCharacters) > 0 {
			parts = append(parts, strings.Join(shot.InvolvedCharacters, " "))
		}
	}

	return strings.Join(parts, " | ")
}

func collectShotPrompts(shots []scriptArtifactShot) []string {
	prompts := make([]string, 0, len(shots))
	for _, shot := range shots {
		prompt := effectiveShotPrompt(shot)
		if prompt == "" {
			continue
		}
		prompts = append(prompts, prompt)
	}

	return prompts
}

func effectiveShotPrompt(shot scriptArtifactShot) string {
	return firstNonEmpty(
		strings.TrimSpace(shot.ImagePrompt),
		strings.TrimSpace(shot.TextPrompt),
		strings.TrimSpace(shot.Prompt),
	)
}

func buildMatchTermSet(references []ImageCharacterReference) []string {
	terms := make([]string, 0, len(references)*2)
	for _, item := range references {
		if len(item.MatchTerms) == 0 {
			terms = append(terms, item.CharacterName)
			continue
		}
		terms = append(terms, item.MatchTerms...)
	}

	return terms
}

func textMatchesTerms(text string, terms []string) bool {
	searchText := strings.ToLower(text)
	for _, term := range terms {
		trimmed := strings.TrimSpace(term)
		if trimmed == "" {
			continue
		}
		if strings.Contains(searchText, strings.ToLower(trimmed)) {
			return true
		}
	}

	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
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
