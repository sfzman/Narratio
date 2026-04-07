package script

import (
	"encoding/json"
	"fmt"
	"strings"
)

func buildOutlinePrompts(article, language string) (string, string) {
	if strings.EqualFold(strings.TrimSpace(language), "zh") || strings.TrimSpace(language) == "" {
		return buildChineseOutlinePrompts(article)
	}

	systemPrompt := "You are a story analysis assistant for storyboard preparation. Respond with JSON only."
	userPrompt := fmt.Sprintf(
		"Language: %s\nTask: produce a storyboard-ready story outline as JSON with story_position, mainline, plot_stages, relationship_state_changes, continuity_notes, and segment_reading_notes.\nArticle:\n%s",
		language,
		article,
	)

	return systemPrompt, userPrompt
}

func buildCharacterSheetPrompts(article, language string) (string, string) {
	if strings.EqualFold(strings.TrimSpace(language), "zh") || strings.TrimSpace(language) == "" {
		return buildChineseCharacterSheetPrompts(article)
	}

	systemPrompt := "You are a character sheet assistant for storyboard and reference-image preparation. Respond with JSON only."
	userPrompt := fmt.Sprintf(
		"Language: %s\nTask: extract the main characters as JSON for downstream storyboard understanding and reference-image preparation. Each character should include role, identity, motivation, one consolidated appearance field, and a stable reference_subject_type.\nArticle:\n%s",
		language,
		article,
	)

	return systemPrompt, userPrompt
}

func buildScriptPrompts(
	language string,
	voiceID string,
	segmentation SegmentationOutput,
	outline OutlineOutput,
	characters CharacterSheetOutput,
) (string, string) {
	if strings.EqualFold(strings.TrimSpace(language), "zh") || strings.TrimSpace(language) == "" {
		return buildChineseScriptPrompts(voiceID, segmentation, outline, characters)
	}

	systemPrompt := "You rewrite pre-segmented article chunks into narrated scripts. Respond with JSON only."
	userPrompt := fmt.Sprintf(
		"Language: %s\nVoiceID: %s\nSegmentationContext:\n%s\nOutlineContext:\n%s\nCharacterContext:\n%s\nTask: keep the provided segment order unchanged, and return JSON with one narration-ready script and one image summary for each input segment.\n",
		language,
		voiceID,
		mustJSONString(segmentation),
		mustJSONString(outline),
		mustJSONString(characters),
	)

	return systemPrompt, userPrompt
}

func buildChineseOutlinePrompts(article string) (string, string) {
	systemPrompt := `你是一名中文影视改编策划与分镜前置分析助手。

你的唯一任务，是把用户提供的短篇小说整理成一份给后续 AI 写分镜时使用的上下文大纲。

输出目标：
1. 帮后续 AI 快速理解完整剧情，不遗漏关键因果。
2. 帮后续 AI 在分段阅读时保持人物动机、信息状态、冲突进展和剧情连续性。
3. 输出必须完全基于原文，允许低推断补足，但必须克制。
4. 只输出 JSON，不要输出额外说明。

请严格使用以下 JSON 结构：
{
  "story_position": {
    "genre": "...",
    "era_background": "...",
    "core_conflict": "...",
    "emotional_tone": "...",
    "ending_type": "..."
  },
  "mainline": "...",
  "plot_stages": [
    {
      "name": "开端",
      "happened": "...",
      "goal": "...",
      "obstacle": "...",
      "outcome": "..."
    }
  ],
  "relationship_state_changes": ["..."],
  "continuity_notes": ["..."],
  "segment_reading_notes": ["..."]
}

额外要求：
1. plot_stages 至少包含：开端、发展、转折、高潮、结局。
2. relationship_state_changes 只保留最关键的人物关系变化与信息差变化。
3. continuity_notes 要覆盖人物动机、受伤状态、位置变化、已知秘密、误会、承诺、关键物品去向、情绪延续。
4. segment_reading_notes 写 5 到 10 条，帮助后续 AI 在只看到局部章节时不写出脱节剧情。`
	userPrompt := buildNovelTaskPrompt(article, "剧情大纲整理")

	return systemPrompt, userPrompt
}

func buildNovelTaskPrompt(article string, taskName string) string {
	return fmt.Sprintf(
		"下面是需要分析的中文短篇小说全文，请执行“%s”任务。\n\n【小说全文开始】\n%s\n【小说全文结束】",
		taskName,
		strings.TrimSpace(article),
	)
}

func buildChineseCharacterSheetPrompts(article string) (string, string) {
	systemPrompt := `你是一名中文影视角色设定整理助手，服务目标是后续角色参考图生成与分镜生成。

你的唯一任务，是根据用户提供的短篇小说整理一份主要人物表。

输出目标：
1. 帮后续绘图模型和分镜 AI 准确理解人物外观与身份。
2. 保持内容完全基于原文，不乱编，不凭空增加设定。
3. 如果原文没有明确写出某项信息，可以做低风险推断，但不要写任何免责声明。
4. 只输出 JSON，不要输出额外说明。

请严格使用以下 JSON 结构：
{
  "characters": [
    {
      "name": "...",
      "role": "...",
      "age": "...",
      "gender": "...",
      "appearance": "...",
      "temperament": "...",
      "personality_traits": ["..."],
      "identity": "...",
      "relationship_to_protagonist": "...",
      "visual_signature": "...",
      "reference_subject_type": "...",
      "image_prompt_focus": "..."
    }
  ]
}

额外要求：
1. characters 以剧情核心角色为准，通常保留 2 到 8 人；如果人物很多，只保留对主线最重要的人物。
2. appearance 是统一视觉描述字段，直接合并外貌、发型、服装、体态等稳定信息，不要再拆成多个重复字段。
3. reference_subject_type 必须是简短稳定的主体类别短语，例如：人、狐狸、幼狐、婴儿、妖怪；不要写人名，不要写整句。
4. image_prompt_focus 只写人物参考图的构图与出图约束，不要重复 appearance 或 visual_signature 已经表达的外观信息。明确写平视角、正面、单人、半身或全身可见、关键特征完整露出，不要写多人互动，不要写剧情动作。
5. 如果同一人物存在不同时期、不同造型、不同形态，必须拆成多个独立人物条目分别写，并在 name 中直接区分。`
	userPrompt := buildNovelTaskPrompt(article, "主要人物表整理")

	return systemPrompt, userPrompt
}

func buildChineseScriptPrompts(
	voiceID string,
	segmentation SegmentationOutput,
	outline OutlineOutput,
	characters CharacterSheetOutput,
) (string, string) {
	systemPrompt := `你是一名中文旁白脚本整理助手，服务目标是后续 TTS、配图和视频合成。

你的唯一任务，是把已经完成分段的小说原文整理成适合朗读的分段旁白脚本。

输出目标：
1. 严格沿用上游已经给定的分段顺序和分段边界，不负责重新分段，不要合并或拆分段落。
2. 每段都要把对应原文整理成更适合朗读的中文旁白文案，可以更口语化、更顺口，但不能改写事实。
3. 保留关键情节、关键信息和必要对话，不要为了顺口而删掉重要信息。
4. 每段补一条简短摘要，直接服务后续配图。
5. 只输出 JSON，不要输出额外说明。

请严格使用以下 JSON 结构：
{
  "segments": [
    {
      "index": 0,
      "text": "...",
      "script": "...",
      "summary": "..."
    }
  ]
}

额外要求：
1. 返回的 segments 数量必须和上游 segmentation 完全一致，index 必须一一对应。
2. 不要改写 segment 的 text；该字段只用于回填对应原文分段。
3. script 必须适合 TTS 朗读，语句自然、信息清晰，可适度增加停顿符号，但不要堆砌修辞。
4. summary 控制在一句话内，聚焦这一段最适合画面的动作、情绪或场景信息。
5. 要参考 outline 和 character_sheet，保证人物身份、关系、状态和剧情连续性不跑偏。
6. voice_id 仅作为语气参考，不改变剧情内容。`
	userPrompt := fmt.Sprintf(
		"下面请基于提供的上下文，为已经切好的小说分段生成中文旁白脚本。\n\n【VoiceID】%s\n\n【分段结果开始】\n%s\n【分段结果结束】\n\n【完整故事大纲开始】\n%s\n【完整故事大纲结束】\n\n【主要人物表开始】\n%s\n【主要人物表结束】",
		voiceID,
		mustJSONString(segmentation),
		mustJSONString(outline),
		mustJSONString(characters),
	)

	return systemPrompt, userPrompt
}

func mustJSONString(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}

	return string(data)
}
