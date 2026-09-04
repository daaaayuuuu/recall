package gametemplates

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	LoveJourneyID            = "love-journey"
	LoveJourneyVersion       = "1.1.0"
	LoveJourneyLegacyVersion = "1.0.0"
	CoverSlotKey             = "cover"
)

type TextInput struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	HelpText    string `json:"helpText,omitempty"`
	InputType   string `json:"inputType,omitempty"`
	Required    bool   `json:"required"`
	MinLength   int    `json:"minLength,omitempty"`
	MaxLength   int    `json:"maxLength"`
	Format      string `json:"format,omitempty"`
}

type AssetSlot struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	HelpText string `json:"helpText,omitempty"`
	Required bool   `json:"required"`
	MinItems int    `json:"minItems"`
	MaxItems int    `json:"maxItems"`
	Sortable bool   `json:"sortable"`
}

type Scene struct {
	Key        string      `json:"key"`
	Name       string      `json:"name"`
	Summary    string      `json:"summary"`
	TextInputs []TextInput `json:"textInputs"`
	AssetSlots []AssetSlot `json:"assetSlots"`
}

type Definition struct {
	ID                 string    `json:"id"`
	Version            string    `json:"version"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	InputSchemaVersion int       `json:"inputSchemaVersion"`
	GenerationEnabled  bool      `json:"generationEnabled"`
	Cover              AssetSlot `json:"cover"`
	Scenes             []Scene   `json:"scenes"`
}

var loveJourney = Definition{
	ID:                 LoveJourneyID,
	Version:            LoveJourneyVersion,
	Name:               "爱的旅程",
	Description:        "上传你们的照片、写下一封情书，为对方准备一份可以亲手拆开的礼物。",
	InputSchemaVersion: 3,
	GenerationEnabled:  true,
	Cover: AssetSlot{
		Key: CoverSlotKey, Label: "双人自拍正脸合照（可选）", HelpText: "最多上传 1 张，建议选择两人正脸清晰的自拍合照。",
		Required: false, MinItems: 0, MaxItems: 1,
	},
	Scenes: []Scene{
		{
			Key: "materials", Name: "礼物资料", Summary: "一次填写完成，不再按游戏场景拆分提交。",
			TextInputs: []TextInput{
				{
					Key: "loveLetter", Label: "写给对方的情书", Placeholder: "写下现在最想对 TA 说的话……",
					HelpText:  "必填，最多 1000 字。点击 AI 润色会把当前文字发送给 DeepSeek，并将结果回填供你确认。",
					InputType: "textarea", Required: true, MaxLength: 1000,
				},
				{
					Key: "passwordHint", Label: "密码提示（可选）", Placeholder: "例如：我们第一次见面的日期",
					HelpText: "给对方一点提示，但不要直接写出密码。", InputType: "text", MaxLength: 100,
				},
				{
					Key: "letterPassword", Label: "拆信密码", Placeholder: "请输入 4 位数字",
					HelpText:  "这是对方拆开情书礼物时需要输入的密码。",
					InputType: "password", Required: true, MinLength: 4, MaxLength: 4, Format: "four-digit-code",
				},
			},
			AssetSlots: []AssetSlot{{
				Key: "travelPhotos", Label: "旅行照片（可选）", HelpText: "照片 1 和照片 2 分开选择，并按编号顺序展示。",
				Required: false, MinItems: 0, MaxItems: 2, Sortable: true,
			}},
		},
	},
}

var loveJourneyLegacy = Definition{
	ID:                 LoveJourneyID,
	Version:            LoveJourneyLegacyVersion,
	Name:               "爱的旅程（旧版资料）",
	Description:        "旧版五场景资料协议，仅用于兼容已有草稿。",
	InputSchemaVersion: 2,
	GenerationEnabled:  true,
	Cover:              AssetSlot{Key: CoverSlotKey, Label: "游戏封面", HelpText: "可选，上传后会替换当前封面。", MaxItems: 1},
	Scenes: []Scene{
		{
			Key: "firstMeeting", Name: "初见", Summary: "手机聊天，用 emoji 拼出最初的心动对话。",
			TextInputs: []TextInput{{Key: "firstMeetingDescription", Label: "初遇场景描述（可选）", Placeholder: "例如：我们在朋友的生日聚会上第一次见面……", MaxLength: 500}},
			AssetSlots: []AssetSlot{{Key: "firstMeetingPartnerPhoto", Label: "伴侣单人照片", HelpText: "必须上传 1 张。", Required: true, MinItems: 1, MaxItems: 1}},
		},
		{
			Key: "dining", Name: "吃饭", Summary: "点点点吃光食物，在轻松烟火气中回温。",
			TextInputs: []TextInput{{Key: "diningDescription", Label: "一起吃饭的回忆（可选）", Placeholder: "例如：那家小店很挤，但我们聊到了打烊……", MaxLength: 500}},
			AssetSlots: []AssetSlot{},
		},
		{
			Key: "movie", Name: "看电影", Summary: "牵手，试探靠近的核心高光。",
			TextInputs: []TextInput{{Key: "movieDescription", Label: "第一次看电影的回忆（可选）", Placeholder: "例如：电影讲了什么已经记不清，只记得第一次牵手……", MaxLength: 500}},
			AssetSlots: []AssetSlot{{Key: "moviePhoto", Label: "看电影的照片（可选）", HelpText: "最多上传 1 张。", MaxItems: 1}},
		},
		{
			Key: "travel", Name: "旅行", Summary: "收拾行李、甩出照片，让共同经历逐张揭晓。",
			TextInputs: []TextInput{},
			AssetSlots: []AssetSlot{{Key: "travelPhotos", Label: "旅行照片", HelpText: "必须上传 3 张，可拖动调整揭晓顺序。", Required: true, MinItems: 3, MaxItems: 3, Sortable: true}},
		},
		{
			Key: "today", Name: "今天", Summary: "打开情书，像拆礼物一样完成终局揭晓。",
			TextInputs: []TextInput{{Key: "loveLetter", Label: "写给伴侣的情书", Placeholder: "写下今天最想对 TA 说的话……", Required: true, MaxLength: 1000}},
			AssetSlots: []AssetSlot{},
		},
	},
}

func List() []Definition {
	return []Definition{loveJourney, loveJourneyLegacy}
}

func Find(id, version string) (Definition, bool) {
	if id == LoveJourneyID {
		switch version {
		case loveJourney.Version:
			return loveJourney, true
		case loveJourneyLegacy.Version:
			return loveJourneyLegacy, true
		}
	}
	return Definition{}, false
}

func (definition Definition) TextInput(key string) (TextInput, bool) {
	for _, scene := range definition.Scenes {
		for _, input := range scene.TextInputs {
			if input.Key == key {
				return input, true
			}
		}
	}
	return TextInput{}, false
}

func (definition Definition) AssetSlot(key string) (AssetSlot, bool) {
	if key == definition.Cover.Key {
		return definition.Cover, true
	}
	for _, scene := range definition.Scenes {
		for _, slot := range scene.AssetSlots {
			if slot.Key == key {
				return slot, true
			}
		}
	}
	return AssetSlot{}, false
}

func (definition Definition) ValidateSceneInputs(values map[string]string) (map[string]string, map[string]string) {
	normalized := make(map[string]string, len(values))
	fields := make(map[string]string)
	for key, value := range values {
		input, ok := definition.TextInput(key)
		if !ok {
			fields["sceneInputs."+key] = "模板不包含这个文本输入项"
			continue
		}
		value = strings.TrimSpace(value)
		length := utf8.RuneCountInString(value)
		if length > input.MaxLength {
			fields["sceneInputs."+key] = fmt.Sprintf("不能超过 %d 个字符", input.MaxLength)
			continue
		}
		if value != "" && input.MinLength > 0 && length < input.MinLength {
			fields["sceneInputs."+key] = fmt.Sprintf("不能少于 %d 个字符", input.MinLength)
			continue
		}
		if value != "" && input.Format == "four-digit-code" && !isFourDigitCode(value) {
			fields["sceneInputs."+key] = "请输入 4 位数字密码"
			continue
		}
		normalized[key] = value
	}
	for _, scene := range definition.Scenes {
		for _, input := range scene.TextInputs {
			fieldKey := "sceneInputs." + input.Key
			if input.Required && strings.TrimSpace(normalized[input.Key]) == "" {
				if _, alreadyInvalid := fields[fieldKey]; !alreadyInvalid {
					fields[fieldKey] = "此项为必填项"
				}
			}
			if _, ok := normalized[input.Key]; !ok {
				normalized[input.Key] = ""
			}
		}
	}
	return normalized, fields
}

func isFourDigitCode(value string) bool {
	if len(value) != 4 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
