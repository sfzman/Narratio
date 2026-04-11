package tts

import (
	"fmt"
	"strings"
)

const defaultSegmentDuration = 6.5

type segmentationArtifact struct {
	Segments []segmentationSegment `json:"segments"`
}

type segmentationSegment struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

type TTSOutput struct {
	AudioSegments []AudioSegment `json:"audio_segments"`
	TotalDuration float64        `json:"total_duration_seconds"`
	SubtitleItems []SubtitleItem `json:"subtitle_items"`
}

type AudioSegment struct {
	SegmentIndex int     `json:"segment_index"`
	FilePath     string  `json:"file_path"`
	Duration     float64 `json:"duration"`
}

type SubtitleItem struct {
	SegmentIndex int     `json:"segment_index"`
	Start        float64 `json:"start"`
	End          float64 `json:"end"`
	Text         string  `json:"text"`
}

func buildPlaceholderTTSOutput(jobPublicID string, segmentation segmentationArtifact) TTSOutput {
	segments := segmentation.Segments
	if len(segments) == 0 {
		segments = []segmentationSegment{{Index: 0, Text: ""}}
	}

	audioSegments := make([]AudioSegment, 0, len(segments))
	subtitleItems := make([]SubtitleItem, 0, len(segments))
	currentStart := 0.0
	for index, segment := range segments {
		segmentIndex := segment.Index
		if segmentIndex < 0 {
			segmentIndex = index
		}

		audioSegments = append(audioSegments, AudioSegment{
			SegmentIndex: segmentIndex,
			FilePath:     audioSegmentPath(jobPublicID, segmentIndex),
			Duration:     defaultSegmentDuration,
		})
		subtitleItems = append(subtitleItems, SubtitleItem{
			SegmentIndex: segmentIndex,
			Start:        currentStart,
			End:          currentStart + defaultSegmentDuration,
			Text:         segment.Text,
		})
		currentStart += defaultSegmentDuration
	}

	return TTSOutput{
		AudioSegments: audioSegments,
		TotalDuration: currentStart,
		SubtitleItems: subtitleItems,
	}
}

func audioSegmentPath(jobPublicID string, segmentIndex int) string {
	return fmt.Sprintf("jobs/%s/audio/segment_%03d.wav", jobPublicID, segmentIndex)
}

func normalizedSegmentIndex(raw int, fallback int) int {
	if raw >= 0 {
		return raw
	}

	return fallback
}

func collectAudioSegmentPaths(segments []AudioSegment) []string {
	paths := make([]string, 0, len(segments))
	for _, segment := range segments {
		paths = append(paths, segment.FilePath)
	}

	return paths
}

func buildSRT(subtitleItems []SubtitleItem) string {
	var builder strings.Builder
	for index, item := range subtitleItems {
		builder.WriteString(fmt.Sprintf("%d\n", index+1))
		builder.WriteString(formatSRTTimestamp(item.Start))
		builder.WriteString(" --> ")
		builder.WriteString(formatSRTTimestamp(item.End))
		builder.WriteString("\n")
		builder.WriteString(strings.TrimSpace(item.Text))
		builder.WriteString("\n\n")
	}

	return builder.String()
}

func formatSRTTimestamp(seconds float64) string {
	totalMilliseconds := int(seconds*1000 + 0.5)
	if totalMilliseconds < 0 {
		totalMilliseconds = 0
	}

	hours := totalMilliseconds / 3_600_000
	totalMilliseconds %= 3_600_000
	minutes := totalMilliseconds / 60_000
	totalMilliseconds %= 60_000
	secondsPart := totalMilliseconds / 1000
	milliseconds := totalMilliseconds % 1000

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, secondsPart, milliseconds)
}

func splitSentencesByPeriod(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	sentences := make([]string, 0, 8)
	var builder strings.Builder
	for _, char := range trimmed {
		switch char {
		case '\r', '\n':
			sentence := strings.TrimSpace(builder.String())
			if sentence != "" {
				sentences = append(sentences, sentence)
			}
			builder.Reset()
			continue
		}

		builder.WriteRune(char)
		if char == '。' || char == '.' {
			sentence := strings.TrimSpace(builder.String())
			if sentence != "" {
				sentences = append(sentences, sentence)
			}
			builder.Reset()
		}
	}

	remainder := strings.TrimSpace(builder.String())
	if remainder != "" {
		sentences = append(sentences, remainder)
	}

	return sentences
}
