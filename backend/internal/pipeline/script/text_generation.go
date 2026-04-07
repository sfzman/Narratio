package script

import (
	"context"
)

const defaultResponseFormatType = "json_object"

type TextGenerationConfig struct {
	Model              string
	MaxTokens          int
	ResponseFormatType string
}

func normalizeTextGenerationConfig(cfg TextGenerationConfig) TextGenerationConfig {
	if cfg.Model == "" {
		cfg.Model = "qwen-max"
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.ResponseFormatType == "" {
		cfg.ResponseFormatType = defaultResponseFormatType
	}

	return cfg
}

func generateTextPreview(
	ctx context.Context,
	client TextClient,
	cfg TextGenerationConfig,
	systemPrompt string,
	userPrompt string,
) (TextResponse, string, error) {
	response, text, preview, err := generateTextContent(
		ctx,
		client,
		cfg,
		systemPrompt,
		userPrompt,
	)
	if err != nil {
		return TextResponse{}, "", err
	}

	_ = text
	return response, preview, nil
}

func generateTextContent(
	ctx context.Context,
	client TextClient,
	cfg TextGenerationConfig,
	systemPrompt string,
	userPrompt string,
) (TextResponse, string, string, error) {
	if client == nil {
		return TextResponse{}, "", "", nil
	}

	response, err := client.Generate(ctx, TextRequest{
		Model:     cfg.Model,
		Messages:  buildPromptMessages(systemPrompt, userPrompt),
		MaxTokens: cfg.MaxTokens,
		ResponseFormat: &ResponseFormatSpec{
			Type: cfg.ResponseFormatType,
		},
	})
	if err != nil {
		return TextResponse{}, "", "", err
	}

	text, err := response.FirstText()
	if err != nil {
		return TextResponse{}, "", "", err
	}

	return response, text, summarizeArticle(text, 120), nil
}

func buildPromptMessages(systemPrompt string, userPrompt string) []ChatMessage {
	return []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
}

func appendLLMMetadata(outputRef map[string]any, response TextResponse, preview string) {
	if response.RequestID != "" {
		outputRef["llm_request_id"] = response.RequestID
	}
	if response.Model != "" {
		outputRef["llm_model"] = response.Model
	}
	if preview != "" {
		outputRef["llm_response_preview"] = preview
	}
}
