package script

import (
	"encoding/json"
	"fmt"
	"strings"
)

func buildOutlinePrompts(article string) (string, string) {
	return buildChineseOutlinePrompts(article)
}

func buildCharacterSheetPrompts(article string) (string, string) {
	return buildChineseCharacterSheetPrompts(article)
}

func buildScriptPrompts(
	segmentation SegmentationOutput,
	outline OutlineOutput,
	characters CharacterSheetOutput,
) (string, string) {
	return buildChineseScriptPrompts(segmentation, outline, characters)
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
	segmentation SegmentationOutput,
	outline OutlineOutput,
	characters CharacterSheetOutput,
) (string, string) {
	targetShots := defaultShotsPerSegment
	if len(segmentation.Segments) > 0 {
		targetShots = targetShotCount(segmentation.Segments[0])
	}

	systemPrompt := fmt.Sprintf(`你是一名中文影视分镜设计助手，负责把当前小说分段拆成适合“图像分镜生成”的 %d 个镜头。

你会同时收到三类上下文：
1. 完整故事大纲
2. 主要人物总表
3. 当前要处理的单个分段原文

你的任务目标：
1. 只围绕“当前分段原文”生成镜头，不要把后文剧情提前写进来。
2. 必须参考完整故事大纲和主要人物表，确保人物状态、关系、服装、伤势、情绪、秘密、地点连续性不丢失。
3. 输出要给后续生图模型直接消费，因此每个镜头最重要的是“图生图提示词(image_to_image_prompt)”或“文生图提示词(text_to_image_prompt)”。
4. 不要寒暄，不要解释思路，不要输出代码块。
5. 只输出 JSON，不要输出额外说明。

请严格遵守：
1. 必须输出且只输出 %d 个分镜，不能少也不能多。
2. 不得新增原文没有的重要人物、关键事件、关键道具。
3. 可以做低风险视觉补足，但必须与原文和人物表一致。
4. 每个镜头都要有明确视觉主体、动作、场景、氛围。
5. 镜头设计必须尽量包含景别、视角、运镜，且描述克制、可执行。
6. 每个镜头都必须额外输出 involved_characters：
   - 如果出现主要人物，必须优先填写人物表里的准确名称。
   - 多个角色时，只保留最关键的 1 到 3 个主要人物名称。
   - 如果没有主要人物、只有空镜或群演环境镜头，involved_characters 返回空数组 []。
7. 如果 involved_characters 非空，则该镜头必须输出 image_to_image_prompt：
   - 必须直接包含人物表里的准确名称，不能改成泛称、关系称呼或临时别名。
   - 重点写：人物名、动作关系、当前镜头新增或必须强调的道具/状态、场景、构图、光线。
   - 不要重复大段固定外观锚点，外观一致性交给人物参考图。
8. 如果 involved_characters 为空，则该镜头必须输出 text_to_image_prompt：
   - 必须写成可独立生图的完整中文描述。
   - 不要写抽象创作指令或主观评价，要把气氛翻译成可见信息。
9. visual_content 和 camera_design 要简洁，但要服务后续图生视频可用性。
10. 对于image_to_image_prompt字段, 如果该镜头出现主要人物则填写，否则留空; text_to_image_prompt字段则相反，如果该镜头是空镜或群演镜头则填写，否则留空。两者不允许同时填写，也不允许同时留空。

请严格使用以下 JSON 结构：
{
  "segments": [
    {
      "index": 0,
      "shots": [
        {
          "index": 0,
          "visual_content": "...",
          "camera_design": "...",
          "involved_characters": ["人物A", "人物B"],
          "image_to_image_prompt": "...",
          "text_to_image_prompt": "..." 
        }
      ]
    }
  ]
}`, targetShots, targetShots)
	userPrompt := fmt.Sprintf(
		"下面请基于提供的上下文，只为当前这一个 segment 生成分镜 script。当前段目标分镜数是 %d，请严格与目标数量保持一致。\n\n【当前分段开始】\n%s\n【当前分段结束】\n\n【剧情大纲上下文开始】\n%s\n【剧情大纲上下文结束】\n\n【人物设定上下文开始】\n%s\n【人物设定上下文结束】",
		targetShots,
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
