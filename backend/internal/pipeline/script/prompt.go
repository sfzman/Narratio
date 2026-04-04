package script

import (
	"fmt"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func buildOutlinePrompts(article, language string) (string, string) {
	systemPrompt := "You extract a concise reading outline. Respond with JSON only."
	userPrompt := fmt.Sprintf(
		"Language: %s\nTask: produce a concise outline for the following article.\nArticle:\n%s",
		language,
		article,
	)

	return systemPrompt, userPrompt
}

func buildCharacterSheetPrompts(article, language string) (string, string) {
	systemPrompt := "You extract named characters for downstream visual generation. Respond with JSON only."
	userPrompt := fmt.Sprintf(
		"Language: %s\nTask: extract the main characters and short visual descriptors from the article.\nArticle:\n%s",
		language,
		article,
	)

	return systemPrompt, userPrompt
}

func buildScriptPrompts(
	article string,
	language string,
	voiceID string,
	dependencies map[string]model.Task,
) (string, string) {
	systemPrompt := "You rewrite article text into short narrated segments. Respond with JSON only."
	userPrompt := fmt.Sprintf(
		"Language: %s\nVoiceID: %s\nOutlineArtifact: %v\nCharacterSheetArtifact: %v\nTask: rewrite the article into narrated segments with image summaries.\nArticle:\n%s",
		language,
		voiceID,
		dependencies["outline"].OutputRef["artifact_path"],
		dependencies["character_sheet"].OutputRef["artifact_path"],
		article,
	)

	return systemPrompt, userPrompt
}
