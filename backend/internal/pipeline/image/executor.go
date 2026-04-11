package image

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sfzman/Narratio/backend/internal/model"
)

const (
	defaultImageWidth        = 1280
	defaultImageHeight       = 720
	defaultImageMaxEdge      = defaultImageWidth
	maxPromptShots           = 3
	maxReferenceImages       = 3
	maxImageGenerateAttempts = 3
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
	aspectRatio := resolveImageAspectRatio(task.Payload)

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
	output, err := e.generateOutput(
		ctx,
		job.PublicID,
		imageStyle,
		aspectRatio,
		scriptOutput,
		characterImages,
	)
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
		"aspect_ratio":                 string(aspectRatio),
		"image_count":                  len(output.Images),
		"shot_image_count":             len(output.ShotImages),
		"generated_image_count":        countGeneratedShotImages(output.ShotImages),
		"fallback_image_count":         countFallbackShotImages(output.ShotImages),
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
	aspectRatio model.AspectRatio,
	scriptOutput scriptArtifactOutput,
	characterImages CharacterImageOutput,
) (ImageOutput, error) {
	output := buildImageOutput(
		jobPublicID,
		imageStyle,
		aspectRatio,
		scriptOutput,
		characterImages,
	)
	if err := e.generateShotImages(ctx, imageStyle, output.ShotImages); err != nil {
		return ImageOutput{}, err
	}
	output.Images = buildSegmentSummaryImages(
		scriptOutput.Segments,
		imageStyle,
		aspectRatio,
		output.ShotImages,
		characterImages,
	)

	return output, nil
}

func (e *Executor) generateShotImages(
	ctx context.Context,
	imageStyle string,
	images []GeneratedShotImage,
) error {
	if e.client == nil {
		for index := range images {
			if err := e.writeFallbackShotImage(images[index]); err != nil {
				return err
			}
			images[index].IsFallback = true
		}
		return nil
	}

	var lastSuccessful *generatedShotResult
	for index := range images {
		result, err := e.generateShotImage(ctx, imageStyle, images[index])
		if err != nil {
			e.log.Warn("shot image generation failed after retries",
				"segment_index", images[index].SegmentIndex,
				"shot_index", images[index].ShotIndex,
				"prompt_type", images[index].PromptType,
				"error", err,
			)
			if lastSuccessful != nil {
				if err := e.artifacts.WriteBytes(images[index].FilePath, lastSuccessful.ImageData); err != nil {
					return fmt.Errorf("write reused image file: %w", err)
				}
				images[index].IsFallback = false
				images[index].FilledFromPrevious = true
				images[index].GenerationRequestID = lastSuccessful.RequestID
				images[index].GenerationModel = lastSuccessful.Model
				images[index].SourceImageURL = lastSuccessful.ImageURL
				e.log.Info("shot image filled from previous success",
					"segment_index", images[index].SegmentIndex,
					"shot_index", images[index].ShotIndex,
					"prompt_type", images[index].PromptType,
				)
				continue
			}

			if err := e.writeFallbackShotImage(images[index]); err != nil {
				return err
			}
			images[index].IsFallback = true
			e.log.Info("shot image fell back to local placeholder",
				"segment_index", images[index].SegmentIndex,
				"shot_index", images[index].ShotIndex,
				"prompt_type", images[index].PromptType,
			)
			continue
		}
		if err := e.artifacts.WriteBytes(images[index].FilePath, result.ImageData); err != nil {
			return fmt.Errorf("write generated shot image file: %w", err)
		}
		images[index].IsFallback = false
		images[index].FilledFromPrevious = false
		images[index].GenerationRequestID = result.RequestID
		images[index].GenerationModel = result.Model
		images[index].SourceImageURL = result.ImageURL
		lastSuccessful = result

		e.log.Info("shot image generated",
			"segment_index", images[index].SegmentIndex,
			"shot_index", images[index].ShotIndex,
			"prompt_type", images[index].PromptType,
			"reference_count", len(result.ReferenceImages),
		)
	}

	return nil
}

type ImageOutput struct {
	Images     []GeneratedImage     `json:"images"`
	ShotImages []GeneratedShotImage `json:"shot_images,omitempty"`
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

type GeneratedShotImage struct {
	SegmentIndex        int                       `json:"segment_index"`
	ShotIndex           int                       `json:"shot_index"`
	FilePath            string                    `json:"file_path"`
	Width               int                       `json:"width"`
	Height              int                       `json:"height"`
	Prompt              string                    `json:"prompt"`
	PromptType          string                    `json:"prompt_type"`
	IsFallback          bool                      `json:"is_fallback"`
	FilledFromPrevious  bool                      `json:"filled_from_previous,omitempty"`
	GenerationRequestID string                    `json:"generation_request_id,omitempty"`
	GenerationModel     string                    `json:"generation_model,omitempty"`
	SourceImageURL      string                    `json:"source_image_url,omitempty"`
	InvolvedCharacters  []string                  `json:"involved_characters,omitempty"`
	CharacterReferences []ImageCharacterReference `json:"character_references"`
	MatchedCharacters   []ImageCharacterReference `json:"matched_characters"`
}

type ImageCharacterReference struct {
	CharacterIndex int      `json:"character_index"`
	CharacterName  string   `json:"character_name"`
	FilePath       string   `json:"file_path"`
	Prompt         string   `json:"prompt"`
	MatchTerms     []string `json:"match_terms"`
	SourceImageURL string   `json:"source_image_url,omitempty"`
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
}

func buildImageOutput(
	jobPublicID string,
	imageStyle string,
	aspectRatio model.AspectRatio,
	scriptOutput scriptArtifactOutput,
	characterImages CharacterImageOutput,
) ImageOutput {
	output := ImageOutput{
		Images:     make([]GeneratedImage, 0, len(scriptOutput.Segments)),
		ShotImages: make([]GeneratedShotImage, 0, countShotImages(scriptOutput.Segments)),
	}
	references := buildImageCharacterReferences(characterImages)
	for _, segment := range scriptOutput.Segments {
		output.ShotImages = append(
			output.ShotImages,
			buildShotImages(jobPublicID, imageStyle, aspectRatio, segment, references)...,
		)
	}

	return output
}

func countShotImages(segments []scriptArtifactSegment) int {
	total := 0
	for _, segment := range segments {
		for _, shot := range segment.Shots {
			if effectiveShotPrompt(shot) == "" {
				continue
			}
			total++
		}
	}

	return total
}

func countGeneratedShotImages(images []GeneratedShotImage) int {
	count := 0
	for _, image := range images {
		if image.IsFallback {
			continue
		}
		count++
	}

	return count
}

func countFallbackShotImages(images []GeneratedShotImage) int {
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
			SourceImageURL: item.SourceImageURL,
		})
	}

	return references
}

func buildShotImages(
	jobPublicID string,
	imageStyle string,
	aspectRatio model.AspectRatio,
	segment scriptArtifactSegment,
	references []ImageCharacterReference,
) []GeneratedShotImage {
	images := make([]GeneratedShotImage, 0, len(segment.Shots))
	width, height := aspectRatio.Dimensions(defaultImageMaxEdge)
	for _, shot := range segment.Shots {
		prompt := buildShotImagePrompt(shot, imageStyle, aspectRatio)
		if prompt == "" {
			continue
		}

		images = append(images, GeneratedShotImage{
			SegmentIndex:        segment.Index,
			ShotIndex:           shot.Index,
			FilePath:            fmt.Sprintf("jobs/%s/images/segment_%03d_shot_%03d.jpg", jobPublicID, segment.Index, shot.Index),
			Width:               width,
			Height:              height,
			Prompt:              prompt,
			PromptType:          shotPromptType(shot),
			IsFallback:          true,
			InvolvedCharacters:  cloneStringSlice(shot.InvolvedCharacters),
			CharacterReferences: references,
			MatchedCharacters:   matchShotCharacters(shot, references),
		})
	}

	return images
}

func buildShotImagePrompt(
	shot scriptArtifactShot,
	imageStyle string,
	aspectRatio model.AspectRatio,
) string {
	base := strings.TrimSpace(effectiveShotPrompt(shot))
	if base == "" {
		return ""
	}

	parts := []string{
		base,
		"style: " + strings.TrimSpace(imageStyle),
		aspectRatioPromptSuffix(aspectRatio),
		"no face close-up",
	}
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		filtered = append(filtered, strings.TrimSpace(part))
	}

	return strings.Join(filtered, "; ")
}

func buildSegmentSummaryImages(
	segments []scriptArtifactSegment,
	imageStyle string,
	aspectRatio model.AspectRatio,
	shotImages []GeneratedShotImage,
	characterImages CharacterImageOutput,
) []GeneratedImage {
	references := buildImageCharacterReferences(characterImages)
	bySegment := groupShotImagesBySegment(shotImages)
	summaries := make([]GeneratedImage, 0, len(segments))
	for _, segment := range segments {
		shots := bySegment[segment.Index]
		if len(shots) == 0 {
			continue
		}

		matched := matchSegmentCharacters(segment, references)
		source := resolveSegmentPromptSource(segment, matched)
		selected, matchedSelected := selectPromptCharacters(references, matched)
		summaryShot := chooseSummaryShot(shots)
		summaries = append(summaries, GeneratedImage{
			SegmentIndex: segment.Index,
			FilePath:     summaryShot.FilePath,
			Width:        summaryShot.Width,
			Height:       summaryShot.Height,
			IsFallback:   summaryShot.IsFallback,
			Prompt: buildSegmentImagePrompt(
				source.Text,
				imageStyle,
				aspectRatio,
				selected,
				matchedSelected,
			),
			PromptSourceType:    source.Type,
			PromptSourceText:    source.Text,
			PromptSourceShots:   source.Shots,
			GenerationRequestID: summaryShot.GenerationRequestID,
			GenerationModel:     summaryShot.GenerationModel,
			SourceImageURL:      summaryShot.SourceImageURL,
			CharacterReferences: references,
			MatchedCharacters:   matched,
		})
	}

	return summaries
}

func groupShotImagesBySegment(images []GeneratedShotImage) map[int][]GeneratedShotImage {
	grouped := make(map[int][]GeneratedShotImage, len(images))
	for _, image := range images {
		grouped[image.SegmentIndex] = append(grouped[image.SegmentIndex], image)
	}

	return grouped
}

func chooseSummaryShot(images []GeneratedShotImage) GeneratedShotImage {
	for _, image := range images {
		if !image.IsFallback {
			return image
		}
	}

	return images[0]
}

func buildSegmentImagePrompt(
	base string,
	imageStyle string,
	aspectRatio model.AspectRatio,
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
	parts = append(parts, aspectRatioPromptSuffix(aspectRatio))
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

type generatedShotResult struct {
	RequestID       string
	Model           string
	ImageURL        string
	ImageData       []byte
	ReferenceImages []string
}

func (e *Executor) generateShotImage(
	ctx context.Context,
	imageStyle string,
	image GeneratedShotImage,
) (*generatedShotResult, error) {
	if e.client == nil {
		return nil, fmt.Errorf("live image client is not configured")
	}

	request, referenceImages, err := e.buildShotGenerationRequest(imageStyle, image)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 1; attempt <= maxImageGenerateAttempts; attempt++ {
		generated, err := e.client.Generate(ctx, request)
		if err == nil {
			return &generatedShotResult{
				RequestID:       strings.TrimSpace(generated.RequestID),
				Model:           strings.TrimSpace(generated.Model),
				ImageURL:        strings.TrimSpace(generated.ImageURL),
				ImageData:       generated.ImageData,
				ReferenceImages: referenceImages,
			}, nil
		}
		lastErr = err
		e.log.Warn("shot image attempt failed",
			"segment_index", image.SegmentIndex,
			"shot_index", image.ShotIndex,
			"attempt", attempt,
			"prompt_type", image.PromptType,
			"error", err,
		)
	}

	return nil, lastErr
}

func (e *Executor) buildShotGenerationRequest(
	imageStyle string,
	image GeneratedShotImage,
) (Request, []string, error) {
	request := Request{
		Model:          e.generationConfig.Model,
		Prompt:         image.Prompt,
		Size:           formatGeneratedImageSize(image.Width, image.Height),
		NegativePrompt: e.generationConfig.NegativePrompt,
	}
	if image.PromptType != "image_to_image" {
		return request, nil, nil
	}

	selected := selectShotReferenceCandidates(image)
	if len(selected) == 0 {
		return request, nil, nil
	}

	if len(selected) > maxReferenceImages {
		selected = selected[:maxReferenceImages]
	}
	referenceImages, err := e.resolveReferenceImages(selected)
	if err != nil {
		return Request{}, nil, err
	}
	request.ReferenceImages = referenceImages
	request.Prompt = replacePromptCharacterNamesWithPlaceholders(image.Prompt, selected)
	request.Prompt = ensurePromptStyle(request.Prompt, imageStyle)

	return request, referenceImages, nil
}

func selectShotReferenceCandidates(image GeneratedShotImage) []ImageCharacterReference {
	if len(image.MatchedCharacters) > 0 {
		return image.MatchedCharacters
	}

	return image.CharacterReferences
}

func (e *Executor) resolveReferenceImages(references []ImageCharacterReference) ([]string, error) {
	resolved := make([]string, 0, len(references))
	for _, reference := range references {
		if sourceURL := strings.TrimSpace(reference.SourceImageURL); sourceURL != "" {
			resolved = append(resolved, sourceURL)
			continue
		}

		dataURL, err := e.referenceFileAsDataURL(reference.FilePath)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, dataURL)
	}

	return resolved, nil
}

func (e *Executor) referenceFileAsDataURL(relativePath string) (string, error) {
	fullPath := artifactFullPath(e.artifacts.workspaceDir, relativePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("read reference image file: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("reference image file is empty")
	}

	return "data:" + mimeTypeForImagePath(relativePath) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func mimeTypeForImagePath(path string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func replacePromptCharacterNamesWithPlaceholders(
	prompt string,
	references []ImageCharacterReference,
) string {
	replaced := strings.TrimSpace(prompt)
	for index, reference := range references {
		placeholder := fmt.Sprintf("图%d中的人物", index+1)
		terms := reference.MatchTerms
		if len(terms) == 0 {
			terms = []string{reference.CharacterName}
		}
		sort.SliceStable(terms, func(i, j int) bool {
			return len(terms[i]) > len(terms[j])
		})
		for _, term := range terms {
			trimmed := strings.TrimSpace(term)
			if trimmed == "" {
				continue
			}
			replaced = strings.ReplaceAll(replaced, trimmed, placeholder)
		}
	}

	return replaced
}

func ensurePromptStyle(prompt string, imageStyle string) string {
	if strings.TrimSpace(imageStyle) == "" {
		return strings.TrimSpace(prompt)
	}
	if strings.Contains(strings.ToLower(prompt), "style:") {
		return strings.TrimSpace(prompt)
	}

	return strings.TrimSpace(prompt + "; style: " + strings.TrimSpace(imageStyle))
}

func (e *Executor) writeFallbackShotImage(image GeneratedShotImage) error {
	data, err := buildFallbackJPEG(image.Width, image.Height)
	if err != nil {
		return fmt.Errorf("build fallback jpeg: %w", err)
	}
	if err := e.artifacts.WriteBytes(image.FilePath, data); err != nil {
		return fmt.Errorf("write fallback image: %w", err)
	}

	return nil
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

func matchShotCharacters(
	shot scriptArtifactShot,
	references []ImageCharacterReference,
) []ImageCharacterReference {
	searchText := strings.ToLower(joinShotMatchText([]scriptArtifactShot{shot}))
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
	if prompt := strings.TrimSpace(shot.ImagePrompt); prompt != "" {
		return prompt
	}

	return strings.TrimSpace(shot.TextPrompt)
}

func shotPromptType(shot scriptArtifactShot) string {
	if strings.TrimSpace(shot.ImagePrompt) != "" {
		return "image_to_image"
	}
	if strings.TrimSpace(shot.TextPrompt) != "" {
		return "text_to_image"
	}

	return ""
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		cloned = append(cloned, trimmed)
	}
	if len(cloned) == 0 {
		return nil
	}

	return cloned
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

func resolveImageAspectRatio(payload map[string]any) model.AspectRatio {
	return resolveAspectRatio(payload, model.AspectRatioLandscape16x9)
}

func resolveAspectRatio(
	payload map[string]any,
	fallback model.AspectRatio,
) model.AspectRatio {
	if payload == nil {
		return fallback
	}

	value, ok := payload["aspect_ratio"]
	if !ok {
		return fallback
	}

	s, ok := value.(string)
	if !ok {
		return fallback
	}

	aspectRatio := model.ParseAspectRatio(s)
	if !aspectRatio.IsValid() {
		return fallback
	}

	return aspectRatio.Normalized()
}

func aspectRatioPromptSuffix(aspectRatio model.AspectRatio) string {
	return "cinematic composition, high quality, " + string(aspectRatio.Normalized())
}

func formatGeneratedImageSize(width int, height int) string {
	if width <= 0 || height <= 0 {
		return defaultImageSize
	}

	return fmt.Sprintf("%d*%d", width, height)
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
