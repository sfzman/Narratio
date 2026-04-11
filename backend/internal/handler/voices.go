package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sfzman/Narratio/backend/internal/model"
)

type voicePresetResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ReferenceAudio string `json:"reference_audio"`
	PreviewURL     string `json:"preview_url"`
}

func (h Handlers) listVoices(c *gin.Context) {
	presets := model.DefaultVoicePresets()
	voices := make([]voicePresetResponse, 0, len(presets))
	for _, preset := range presets {
		voices = append(voices, voicePresetResponse{
			ID:             preset.ID,
			Name:           preset.Name,
			ReferenceAudio: preset.ReferenceAudio,
			PreviewURL:     preset.ReferenceAudio,
		})
	}

	success(c, http.StatusOK, gin.H{
		"default_voice_id": model.DefaultVoicePresetID,
		"voices":           voices,
	})
}
