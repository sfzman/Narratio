package model

import "strings"

const DefaultVoicePresetID = "male_calm"

type VoicePreset struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ReferenceAudio string `json:"reference_audio"`
}

func DefaultVoicePresets() []VoicePreset {
	return []VoicePreset{
		{
			ID:             "male_calm",
			Name:           "男_沉稳青年音",
			ReferenceAudio: "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E6%B2%89%E7%A8%B3%E9%9D%92%E5%B9%B4%E9%9F%B3.MP3",
		},
		{
			ID:             "male_strong",
			Name:           "男_王明军",
			ReferenceAudio: "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E7%8E%8B%E6%98%8E%E5%86%9B.MP3",
		},
		{
			ID:             "female_explainer",
			Name:           "女_解说小美",
			ReferenceAudio: "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E5%A5%B3_%E8%A7%A3%E8%AF%B4%E5%B0%8F%E7%BE%8E.MP3",
		},
		{
			ID:             "female_documentary",
			Name:           "女_专题片配音",
			ReferenceAudio: "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E5%A5%B3_%E4%B8%93%E9%A2%98%E7%89%87%E9%85%8D%E9%9F%B3.MP3",
		},
		{
			ID:             "boy",
			Name:           "正太",
			ReferenceAudio: "https://oneclicktoon.kongyuxingx.cn/cdn/oneclicktoon/%E7%94%B7_%E6%AD%A3%E5%A4%AA.wav",
		},
	}
}

func NormalizeVoicePresetID(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" || normalized == "default" {
		return DefaultVoicePresetID
	}

	return normalized
}
