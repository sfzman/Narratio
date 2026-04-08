package script

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

type OutlineOutput struct {
	StoryPosition            OutlineStoryPosition `json:"story_position"`
	Mainline                 string               `json:"mainline"`
	PlotStages               []OutlineStage       `json:"plot_stages"`
	RelationshipStateChanges []string             `json:"relationship_state_changes"`
	ContinuityNotes          []string             `json:"continuity_notes"`
	SegmentReadingNotes      []string             `json:"segment_reading_notes"`
}

type OutlineStoryPosition struct {
	Genre         string `json:"genre"`
	EraBackground string `json:"era_background"`
	CoreConflict  string `json:"core_conflict"`
	EmotionalTone string `json:"emotional_tone"`
	EndingType    string `json:"ending_type"`
}

type OutlineStage struct {
	Name     string `json:"name"`
	Happened string `json:"happened"`
	Goal     string `json:"goal"`
	Obstacle string `json:"obstacle"`
	Outcome  string `json:"outcome"`
}

type CharacterSheetOutput struct {
	Characters []CharacterProfile `json:"characters"`
}

type SegmentationOutput struct {
	Segments []TextSegment `json:"segments"`
}

type TextSegment struct {
	Index     int    `json:"index"`
	Text      string `json:"text"`
	CharCount int    `json:"char_count"`
}

type CharacterProfile struct {
	Name                      string   `json:"name"`
	Role                      string   `json:"role"`
	Age                       string   `json:"age"`
	Gender                    string   `json:"gender"`
	Appearance                string   `json:"appearance"`
	Temperament               string   `json:"temperament"`
	PersonalityTraits         []string `json:"personality_traits"`
	Identity                  string   `json:"identity"`
	RelationshipToProtagonist string   `json:"relationship_to_protagonist"`
	VisualSignature           string   `json:"visual_signature"`
	ReferenceSubjectType      string   `json:"reference_subject_type"`
	ImagePromptFocus          string   `json:"image_prompt_focus"`
}

type ScriptOutput struct {
	Segments []Segment `json:"segments"`
}

const defaultShotsPerSegment = 10

type Segment struct {
	Index   int    `json:"index"`
	Text    string `json:"text"`
	Script  string `json:"script"`
	Summary string `json:"summary,omitempty"`
	Shots   []Shot `json:"shots"`
}

type Shot struct {
	Index  int    `json:"index"`
	Prompt string `json:"prompt"`
}

func buildSegmentationOutput(article string) SegmentationOutput {
	segments := segmentArticle(article, 250)
	if len(segments) == 0 {
		trimmed := strings.TrimSpace(article)
		if trimmed != "" {
			segments = []string{trimmed}
		}
	}

	output := SegmentationOutput{
		Segments: make([]TextSegment, 0, len(segments)),
	}
	for index, segmentText := range segments {
		trimmed := strings.TrimSpace(segmentText)
		if trimmed == "" {
			continue
		}
		output.Segments = append(output.Segments, TextSegment{
			Index:     index,
			Text:      trimmed,
			CharCount: countNonPunctuationChars(trimmed),
		})
	}

	return output
}

func buildOutlineOutput(article string, responseText string) (OutlineOutput, error) {
	if strings.TrimSpace(responseText) == "" {
		return stubOutlineOutput(article), nil
	}

	var output OutlineOutput
	if err := json.Unmarshal([]byte(responseText), &output); err != nil {
		return OutlineOutput{}, fmt.Errorf("parse outline response: %w", err)
	}

	normalizeOutlineOutput(&output)
	return output, nil
}

func buildCharacterSheetOutput(
	article string,
	responseText string,
) (CharacterSheetOutput, error) {
	if strings.TrimSpace(responseText) == "" {
		return stubCharacterSheetOutput(article), nil
	}

	var output CharacterSheetOutput
	if err := json.Unmarshal([]byte(responseText), &output); err != nil {
		return CharacterSheetOutput{}, fmt.Errorf("parse character sheet response: %w", err)
	}

	normalizeCharacterSheetOutput(&output)
	return output, nil
}

func buildScriptOutput(segmentation SegmentationOutput, responseText string) (ScriptOutput, error) {
	if strings.TrimSpace(responseText) == "" {
		return stubScriptOutput(segmentation), nil
	}

	var output ScriptOutput
	if err := unmarshalScriptResponse(responseText, &output); err != nil {
		return ScriptOutput{}, fmt.Errorf("parse script response: %w", err)
	}

	normalizeScriptOutput(&output, segmentation)
	return output, nil
}

func unmarshalScriptResponse(responseText string, output *ScriptOutput) error {
	trimmed := strings.TrimSpace(responseText)
	if err := json.Unmarshal([]byte(trimmed), output); err == nil {
		return nil
	} else {
		payload := extractJSONObject(trimmed)
		if payload != "" && payload != trimmed {
			if fallbackErr := json.Unmarshal([]byte(payload), output); fallbackErr == nil {
				return nil
			}
		}
		return err
	}
}

func extractJSONObject(text string) string {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(text); index++ {
		ch := text[index]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(text[start : index+1])
			}
		}
	}

	return ""
}

func stubOutlineOutput(article string) OutlineOutput {
	summary := summarizeArticle(article, 120)
	return OutlineOutput{
		StoryPosition: OutlineStoryPosition{
			Genre:         "待补充",
			EraBackground: "待补充",
			CoreConflict:  summary,
			EmotionalTone: "克制叙事",
			EndingType:    "待补充",
		},
		Mainline: summary,
		PlotStages: []OutlineStage{
			{
				Name:     "开端",
				Happened: summary,
				Goal:     "建立故事起点",
				Obstacle: "待补充",
				Outcome:  "进入后续情节",
			},
			{
				Name:     "发展",
				Happened: summary,
				Goal:     "推进主线冲突",
				Obstacle: "待补充",
				Outcome:  "矛盾继续累积",
			},
			{
				Name:     "转折",
				Happened: summary,
				Goal:     "改变局势",
				Obstacle: "待补充",
				Outcome:  "角色处境发生变化",
			},
			{
				Name:     "高潮",
				Happened: summary,
				Goal:     "正面处理核心冲突",
				Obstacle: "待补充",
				Outcome:  "故事进入收束阶段",
			},
			{
				Name:     "结局",
				Happened: summary,
				Goal:     "交代最终结果",
				Obstacle: "待补充",
				Outcome:  "形成结局",
			},
		},
		RelationshipStateChanges: []string{
			"保留人物关系、信息差和情绪变化的连续性。",
		},
		ContinuityNotes: []string{
			"后续分镜需要持续记住人物动机、场景变化和关键物品去向。",
		},
		SegmentReadingNotes: []string{
			"分段阅读时不要丢失前文已经建立的因果链。",
		},
	}
}

func stubCharacterSheetOutput(article string) CharacterSheetOutput {
	summary := summarizeArticle(article, 120)
	return CharacterSheetOutput{
		Characters: []CharacterProfile{
			{
				Name:                      "旁白视角角色",
				Role:                      "主叙事角色",
				Age:                       "待补充",
				Gender:                    "待补充",
				Appearance:                "保留文中最稳定的外貌、发型、服装和体态信息。摘要：" + summary,
				Temperament:               "克制",
				PersonalityTraits:         []string{"观察者"},
				Identity:                  "待补充",
				RelationshipToProtagonist: "主角本人或主视角人物",
				VisualSignature:           "保留文中最稳定的视觉锚点",
				ReferenceSubjectType:      "人",
				ImagePromptFocus:          "平视角、正面、单人、半身或全身可见，画面干净，关键特征完整露出。",
			},
		},
	}
}

func stubScriptOutput(segmentation SegmentationOutput) ScriptOutput {
	return ScriptOutput{Segments: buildStubScriptSegments(segmentation.Segments)}
}

func normalizeOutlineOutput(output *OutlineOutput) {
	output.StoryPosition.Genre = strings.TrimSpace(output.StoryPosition.Genre)
	output.StoryPosition.EraBackground = strings.TrimSpace(output.StoryPosition.EraBackground)
	output.StoryPosition.CoreConflict = strings.TrimSpace(output.StoryPosition.CoreConflict)
	output.StoryPosition.EmotionalTone = strings.TrimSpace(output.StoryPosition.EmotionalTone)
	output.StoryPosition.EndingType = strings.TrimSpace(output.StoryPosition.EndingType)
	output.Mainline = strings.TrimSpace(output.Mainline)

	for i := range output.PlotStages {
		output.PlotStages[i].Name = strings.TrimSpace(output.PlotStages[i].Name)
		output.PlotStages[i].Happened = strings.TrimSpace(output.PlotStages[i].Happened)
		output.PlotStages[i].Goal = strings.TrimSpace(output.PlotStages[i].Goal)
		output.PlotStages[i].Obstacle = strings.TrimSpace(output.PlotStages[i].Obstacle)
		output.PlotStages[i].Outcome = strings.TrimSpace(output.PlotStages[i].Outcome)
	}

	output.RelationshipStateChanges = normalizeStringList(output.RelationshipStateChanges)
	output.ContinuityNotes = normalizeStringList(output.ContinuityNotes)
	output.SegmentReadingNotes = normalizeStringList(output.SegmentReadingNotes)
}

func normalizeCharacterSheetOutput(output *CharacterSheetOutput) {
	for i := range output.Characters {
		output.Characters[i].Name = strings.TrimSpace(output.Characters[i].Name)
		output.Characters[i].Role = strings.TrimSpace(output.Characters[i].Role)
		output.Characters[i].Age = strings.TrimSpace(output.Characters[i].Age)
		output.Characters[i].Gender = strings.TrimSpace(output.Characters[i].Gender)
		output.Characters[i].Appearance = strings.TrimSpace(output.Characters[i].Appearance)
		output.Characters[i].Temperament = strings.TrimSpace(output.Characters[i].Temperament)
		output.Characters[i].PersonalityTraits = normalizeStringList(output.Characters[i].PersonalityTraits)
		output.Characters[i].Identity = strings.TrimSpace(output.Characters[i].Identity)
		output.Characters[i].RelationshipToProtagonist = strings.TrimSpace(output.Characters[i].RelationshipToProtagonist)
		output.Characters[i].VisualSignature = strings.TrimSpace(output.Characters[i].VisualSignature)
		output.Characters[i].ReferenceSubjectType = strings.TrimSpace(output.Characters[i].ReferenceSubjectType)
		output.Characters[i].ImagePromptFocus = strings.TrimSpace(output.Characters[i].ImagePromptFocus)
	}
}

func normalizeSegmentationOutput(output *SegmentationOutput) {
	for i := range output.Segments {
		output.Segments[i].Index = i
		output.Segments[i].Text = strings.TrimSpace(output.Segments[i].Text)
		output.Segments[i].CharCount = countNonPunctuationChars(output.Segments[i].Text)
	}
}

func normalizeScriptOutput(output *ScriptOutput, segmentation SegmentationOutput) {
	aligned := make([]Segment, 0, len(segmentation.Segments))
	for index, source := range segmentation.Segments {
		defaultSummary := summarizeArticle(source.Text, 120)
		item := Segment{
			Index:   index,
			Text:    strings.TrimSpace(source.Text),
			Script:  strings.TrimSpace(source.Text),
			Summary: defaultSummary,
			Shots:   buildDefaultShots(source.Text, defaultSummary),
		}
		if index < len(output.Segments) {
			item.Script = strings.TrimSpace(output.Segments[index].Script)
			item.Summary = strings.TrimSpace(output.Segments[index].Summary)
			item.Shots = normalizeShots(output.Segments[index].Shots, source.Text, item.Summary)
		}
		if item.Script == "" {
			item.Script = item.Text
		}
		if item.Summary == "" {
			item.Summary = defaultSummary
		}
		item.Shots = normalizeShots(item.Shots, item.Text, item.Summary)
		aligned = append(aligned, item)
	}

	if len(aligned) == 0 {
		aligned = buildStubScriptSegments(segmentation.Segments)
	}
	output.Segments = aligned
}

func buildStubScriptSegments(segmentation []TextSegment) []Segment {
	items := make([]Segment, 0, len(segmentation))
	for index, source := range segmentation {
		trimmed := strings.TrimSpace(source.Text)
		if trimmed == "" {
			continue
		}
		items = append(items, Segment{
			Index:   index,
			Text:    trimmed,
			Script:  trimmed,
			Summary: summarizeArticle(trimmed, 120),
			Shots:   buildDefaultShots(trimmed, summarizeArticle(trimmed, 120)),
		})
	}

	return items
}

func normalizeShots(shots []Shot, text string, summary string) []Shot {
	normalized := make([]Shot, 0, defaultShotsPerSegment)
	for _, shot := range shots {
		prompt := strings.TrimSpace(shot.Prompt)
		if prompt == "" {
			continue
		}
		normalized = append(normalized, Shot{
			Index:  len(normalized),
			Prompt: prompt,
		})
		if len(normalized) == defaultShotsPerSegment {
			return normalized
		}
	}

	fallback := buildDefaultShots(text, summary)
	for _, shot := range fallback {
		if len(normalized) == defaultShotsPerSegment {
			break
		}
		normalized = append(normalized, Shot{
			Index:  len(normalized),
			Prompt: shot.Prompt,
		})
	}

	return normalized
}

func buildDefaultShots(text string, summary string) []Shot {
	trimmedText := strings.TrimSpace(text)
	trimmedSummary := strings.TrimSpace(summary)
	if trimmedSummary == "" {
		trimmedSummary = summarizeArticle(trimmedText, 120)
	}

	units := splitSentences(trimmedText)
	if len(units) == 0 {
		if trimmedSummary != "" {
			units = []string{trimmedSummary}
		} else if trimmedText != "" {
			units = []string{trimmedText}
		}
	}

	shots := make([]Shot, 0, defaultShotsPerSegment)
	for index := 0; index < defaultShotsPerSegment; index++ {
		prompt := trimmedSummary
		if len(units) > 0 {
			prompt = strings.TrimSpace(units[index%len(units)])
		}
		if prompt == "" {
			prompt = trimmedText
		}
		shots = append(shots, Shot{
			Index:  index,
			Prompt: prompt,
		})
	}

	return shots
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func segmentArticle(article string, targetChars int) []string {
	sentences := splitSentences(article)
	if len(sentences) == 0 {
		return nil
	}

	segments := make([]string, 0, len(sentences))
	current := make([]string, 0, 8)
	currentCount := 0

	for _, sentence := range sentences {
		current = append(current, sentence)
		currentCount += countNonPunctuationChars(sentence)
		if currentCount > targetChars {
			segments = append(segments, strings.TrimSpace(strings.Join(current, "")))
			current = nil
			currentCount = 0
		}
	}

	if len(current) > 0 {
		segments = append(segments, strings.TrimSpace(strings.Join(current, "")))
	}

	return segments
}

func splitSentences(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	var (
		sentences []string
		current   []rune
	)
	for _, r := range trimmed {
		if r == '\r' {
			continue
		}
		current = append(current, r)
		if isSentenceBoundary(r) {
			sentence := strings.TrimSpace(string(current))
			if sentence != "" {
				sentences = append(sentences, sentence)
			}
			current = nil
		}
	}

	if len(current) > 0 {
		sentence := strings.TrimSpace(string(current))
		if sentence != "" {
			sentences = append(sentences, sentence)
		}
	}

	return sentences
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '。', '.', '\n':
		return true
	default:
		return false
	}
}

func countNonPunctuationChars(text string) int {
	count := 0
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue
		}
		count++
	}

	return count
}
