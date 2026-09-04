package analytics

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultListLimit  = 50
	maximumListLimit  = 100
	maximumProperties = 4096
)

var (
	templateIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	uuidV4Pattern     = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	ulidPattern       = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
)

type eventRule struct {
	source           Source
	actor            ActorType
	requiredLinks    linkSet
	propertyValidate func(map[string]any) error
}

type linkSet uint16

const (
	linkCreator linkSet = 1 << iota
	linkUserSession
	linkGame
	linkGameVersion
	linkGenerationRun
	linkShare
	linkPlaySession
)

var eventRules = map[EventName]eventRule{
	EventCreatorPageViewed:  {SourceFrontend, ActorCreator, linkCreator | linkUserSession, propertiesPage},
	EventCreatorRegistered:  {SourceAPI, ActorCreator, linkCreator, propertiesNone},
	EventCreatorLoggedIn:    {SourceAPI, ActorCreator, linkCreator | linkUserSession, propertiesNone},
	EventGameCreated:        {SourceAPI, ActorCreator, linkCreator | linkGame | linkGameVersion, propertiesTemplate},
	EventGameVersionCreated: {SourceAPI, ActorCreator, linkCreator | linkGame | linkGameVersion, propertiesVersion},
	EventAssetUploaded:      {SourceAPI, ActorCreator, linkCreator | linkGame | linkGameVersion, propertiesAsset},
	EventGenerationSubmitted: {
		SourceAPI, ActorCreator, linkCreator | linkGame | linkGameVersion | linkGenerationRun, propertiesGenerationSubmitted,
	},
	EventGenerationSucceeded: {
		SourceWorker, ActorSystem, linkCreator | linkGame | linkGameVersion | linkGenerationRun, propertiesGenerationSucceeded,
	},
	EventGenerationFailed: {
		SourceWorker, ActorSystem, linkCreator | linkGame | linkGameVersion | linkGenerationRun, propertiesGenerationFailed,
	},
	EventShareCreated:  {SourceAPI, ActorCreator, linkCreator | linkGame | linkGameVersion | linkShare, propertiesShare},
	EventShareOpened:   {SourceAPI, ActorReceiver, linkGame | linkGameVersion | linkShare, propertiesNone},
	EventPlayStarted:   {SourceAPI, ActorReceiver, linkGame | linkGameVersion | linkShare | linkPlaySession, propertiesNone},
	EventPlayCompleted: {SourceFrontend, ActorReceiver, linkGame | linkGameVersion | linkShare | linkPlaySession, propertiesPlay},
	EventPlayReplayed:  {SourceFrontend, ActorReceiver, linkGame | linkGameVersion | linkShare | linkPlaySession, propertiesPlay},
}

var generationErrorCodes = map[string]struct{}{
	"INPUT_VALIDATION_FAILED":       {},
	"ASSET_READ_FAILED":             {},
	"GENERATION_TIMEOUT":            {},
	"PROVIDER_RATE_LIMITED":         {},
	"PROVIDER_UNAVAILABLE":          {},
	"CONTENT_REJECTED":              {},
	"ASSET_GENERATION_FAILED":       {},
	"GAME_CONFIG_ASSEMBLY_FAILED":   {},
	"GAME_CONFIG_INVALID":           {},
	"TEMPLATE_COMPATIBILITY_FAILED": {},
	"GAME_VALIDATION_FAILED":        {},
	"STORAGE_WRITE_FAILED":          {},
	"STORAGE_CAPACITY_UNAVAILABLE":  {},
	"TASK_LEASE_EXHAUSTED":          {},
	"INTERNAL_ERROR":                {},
}

func ValidateRecordInput(input RecordInput) (json.RawMessage, error) {
	rule, ok := eventRules[input.EventName]
	if !ok {
		return nil, invalid("eventName", "unknown event")
	}
	if input.Source != rule.source {
		return nil, invalid("source", fmt.Sprintf("must be %q for this event", rule.source))
	}
	if input.ActorType != rule.actor {
		return nil, invalid("actorType", fmt.Sprintf("must be %q for this event", rule.actor))
	}

	links := []struct {
		name  string
		value string
		bit   linkSet
	}{
		{"creatorId", input.CreatorID, linkCreator},
		{"userSessionId", input.UserSessionID, linkUserSession},
		{"gameId", input.GameID, linkGame},
		{"gameVersionId", input.GameVersionID, linkGameVersion},
		{"generationRunId", input.GenerationRunID, linkGenerationRun},
		{"shareId", input.ShareID, linkShare},
		{"playSessionId", input.PlaySessionID, linkPlaySession},
	}
	for _, link := range links {
		required := rule.requiredLinks&link.bit != 0
		if required && link.value == "" {
			return nil, invalid(link.name, "is required for this event")
		}
		if !required && link.value != "" {
			return nil, invalid(link.name, "is not allowed for this event")
		}
		if link.value != "" && !validULID(link.value) {
			return nil, invalid(link.name, "must be a 26-character ULID")
		}
	}
	if input.RequestID != "" && !validULID(input.RequestID) {
		return nil, invalid("requestId", "must be a 26-character ULID")
	}
	if input.Source == SourceWorker && input.RequestID != "" {
		return nil, invalid("requestId", "is not allowed for worker events")
	}
	if input.Source == SourceFrontend {
		if !uuidV4Pattern.MatchString(input.ClientEventID) {
			return nil, invalid("clientEventId", "must be a canonical lowercase UUID v4")
		}
	} else if input.ClientEventID != "" {
		return nil, invalid("clientEventId", "is only allowed for frontend events")
	}
	if input.Source != SourceFrontend && input.OccurredAt != nil {
		return nil, invalid("occurredAt", "is only allowed for frontend events")
	}

	properties, values, err := decodeProperties(input.Properties)
	if err != nil {
		return nil, err
	}
	if err := rule.propertyValidate(values); err != nil {
		return nil, err
	}
	return properties, nil
}

func ValidateProperties(eventName EventName, raw json.RawMessage) (json.RawMessage, error) {
	rule, ok := eventRules[eventName]
	if !ok {
		return nil, invalid("eventName", "unknown event")
	}
	properties, values, err := decodeProperties(raw)
	if err != nil {
		return nil, err
	}
	if err := rule.propertyValidate(values); err != nil {
		return nil, err
	}
	return properties, nil
}

func ValidateListFilter(filter ListFilter) (ListFilter, error) {
	if filter.EventName != "" {
		if _, ok := eventRules[filter.EventName]; !ok {
			return ListFilter{}, invalid("eventName", "unknown event")
		}
	}
	if filter.Source != "" && filter.Source != SourceFrontend && filter.Source != SourceAPI && filter.Source != SourceWorker {
		return ListFilter{}, invalid("source", "must be frontend, api, or worker")
	}
	if filter.CreatorID != "" && !validULID(filter.CreatorID) {
		return ListFilter{}, invalid("creatorId", "must be a 26-character ULID")
	}
	if filter.GameID != "" && !validULID(filter.GameID) {
		return ListFilter{}, invalid("gameId", "must be a 26-character ULID")
	}
	if filter.LoginID != "" {
		loginID, err := normalizeLoginID(filter.LoginID)
		if err != nil {
			return ListFilter{}, err
		}
		filter.LoginID = loginID
	}
	if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
		return ListFilter{}, invalid("from", "must be earlier than to")
	}
	if filter.Limit == 0 {
		filter.Limit = defaultListLimit
	}
	if filter.Limit < 1 || filter.Limit > maximumListLimit {
		return ListFilter{}, invalid("limit", "must be between 1 and 100")
	}
	if filter.Cursor != nil {
		if filter.Cursor.Version != 1 {
			return ListFilter{}, invalid("cursor", "has an unsupported version")
		}
		if !validULID(filter.Cursor.ID) || filter.Cursor.CreatedAt.IsZero() {
			return ListFilter{}, invalid("cursor", "is invalid")
		}
		filter.Cursor.CreatedAt = filter.Cursor.CreatedAt.UTC()
	}
	return filter, nil
}

func EncodeCursor(cursor Cursor) (string, error) {
	filter, err := ValidateListFilter(ListFilter{Limit: 1, Cursor: &cursor})
	if err != nil {
		return "", err
	}
	payload := struct {
		Version   int    `json:"v"`
		CreatedAt string `json:"createdAt"`
		ID        string `json:"id"`
	}{filter.Cursor.Version, filter.Cursor.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), filter.Cursor.ID}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal analytics cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func DecodeCursor(encoded string) (Cursor, error) {
	if encoded == "" || strings.Contains(encoded, "=") {
		return Cursor{}, invalid("cursor", "must be unpadded Base64URL")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != encoded {
		return Cursor{}, invalid("cursor", "must be unpadded Base64URL")
	}
	values, err := decodeStrictObject(raw)
	if err != nil || len(values) != 3 {
		return Cursor{}, invalid("cursor", "must contain exactly v, createdAt, and id")
	}
	version, ok := values["v"].(json.Number)
	if !ok || version.String() != "1" {
		return Cursor{}, invalid("cursor", "has an unsupported version")
	}
	createdText, ok := values["createdAt"].(string)
	if !ok || !strings.HasSuffix(createdText, "Z") {
		return Cursor{}, invalid("cursor", "createdAt must be a UTC RFC 3339 timestamp")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdText)
	if err != nil {
		return Cursor{}, invalid("cursor", "createdAt must be a UTC RFC 3339 timestamp")
	}
	id, ok := values["id"].(string)
	if !ok || !validULID(id) {
		return Cursor{}, invalid("cursor", "id must be a 26-character ULID")
	}
	for key := range values {
		if key != "v" && key != "createdAt" && key != "id" {
			return Cursor{}, invalid("cursor", "contains an unknown field")
		}
	}
	return Cursor{Version: 1, CreatedAt: createdAt.UTC(), ID: id}, nil
}

func decodeProperties(raw json.RawMessage) (json.RawMessage, map[string]any, error) {
	if len(raw) == 0 || !utf8.Valid(raw) {
		return nil, nil, invalid("properties", "must be a UTF-8 JSON object")
	}
	values, err := decodeStrictObject(raw)
	if err != nil {
		return nil, nil, invalid("properties", err.Error())
	}
	for _, value := range values {
		switch value.(type) {
		case string, bool, json.Number:
		default:
			return nil, nil, invalid("properties", "values must be strings, integers, or booleans")
		}
	}
	canonical, err := json.Marshal(values)
	if err != nil {
		return nil, nil, invalid("properties", "cannot be encoded")
	}
	if len(canonical) > maximumProperties {
		return nil, nil, invalid("properties", "must not exceed 4096 bytes")
	}
	return canonical, values, nil
}

func decodeStrictObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, errors.New("must be a JSON object")
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '{' {
		return nil, errors.New("must be a JSON object")
	}
	values := make(map[string]any)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, errors.New("must be valid JSON")
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("must be a JSON object")
		}
		if _, duplicate := values[key]; duplicate {
			return nil, errors.New("contains a duplicate key")
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.New("must be valid JSON")
		}
		values[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, errors.New("must be valid JSON")
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) || token != nil {
		return nil, errors.New("must contain exactly one JSON object")
	}
	return values, nil
}

func propertiesNone(values map[string]any) error {
	return requireKeys(values)
}

func propertiesPage(values map[string]any) error {
	if err := requireKeys(values, "page"); err != nil {
		return err
	}
	return enumString(
		values,
		"page",
		"create",
		"games",
		"game-edit",
		"game-preview",
		"game-share",
		"generation-progress",
		"settings",
	)
}

func propertiesTemplate(values map[string]any) error {
	if err := requireKeys(values, "templateId"); err != nil {
		return err
	}
	value, ok := values["templateId"].(string)
	if !ok || !templateIDPattern.MatchString(value) {
		return invalid("properties.templateId", "must be a valid template identifier")
	}
	return nil
}

func propertiesVersion(values map[string]any) error {
	if err := requireKeys(values, "versionNumber", "templateId"); err != nil {
		return err
	}
	if err := unsignedInteger(values, "versionNumber", 1, 4294967295); err != nil {
		return err
	}
	return propertiesTemplate(map[string]any{"templateId": values["templateId"]})
}

func propertiesAsset(values map[string]any) error {
	if err := requireKeys(values, "kind", "mimeType", "sizeBytes"); err != nil {
		return err
	}
	if err := enumString(values, "kind", "game_source", "game_cover"); err != nil {
		return err
	}
	mimeType, ok := values["mimeType"].(string)
	if !ok || len(mimeType) < 1 || len(mimeType) > 128 || strings.Contains(mimeType, ";") || !printableASCII(mimeType) {
		return invalid("properties.mimeType", "must be 1-128 printable ASCII characters without parameters")
	}
	return unsignedInteger(values, "sizeBytes", 0, 9223372036854775807)
}

func propertiesGenerationSubmitted(values map[string]any) error {
	if err := requireKeys(values, "attemptNumber", "deduplicated"); err != nil {
		return err
	}
	if err := unsignedInteger(values, "attemptNumber", 1, 4294967295); err != nil {
		return err
	}
	return boolean(values, "deduplicated")
}

func propertiesGenerationSucceeded(values map[string]any) error {
	if err := requireKeys(values, "executionCount"); err != nil {
		return err
	}
	return unsignedInteger(values, "executionCount", 1, 4294967295)
}

func propertiesGenerationFailed(values map[string]any) error {
	if err := requireKeys(values, "errorCode", "retryable", "executionCount"); err != nil {
		return err
	}
	code, ok := values["errorCode"].(string)
	if !ok {
		return invalid("properties.errorCode", "must be a string")
	}
	if _, ok := generationErrorCodes[code]; !ok {
		return invalid("properties.errorCode", "is not an allowed error code")
	}
	if err := boolean(values, "retryable"); err != nil {
		return err
	}
	return unsignedInteger(values, "executionCount", 1, 4294967295)
}

func propertiesShare(values map[string]any) error {
	if err := requireKeys(values, "lifetimeDays"); err != nil {
		return err
	}
	return unsignedInteger(values, "lifetimeDays", 1, 90)
}

func propertiesPlay(values map[string]any) error {
	if err := requireKeys(values, "mode"); err != nil {
		return err
	}
	return enumString(values, "mode", "public")
}

func requireKeys(values map[string]any, keys ...string) error {
	if len(values) != len(keys) {
		return invalid("properties", "contains missing or unknown fields")
	}
	for _, key := range keys {
		if _, ok := values[key]; !ok {
			return invalid("properties."+key, "is required")
		}
	}
	return nil
}

func enumString(values map[string]any, key string, allowed ...string) error {
	value, ok := values[key].(string)
	if !ok || hasControl(value) {
		return invalid("properties."+key, "must be a string without control characters")
	}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return invalid("properties."+key, "has an unsupported value")
}

func boolean(values map[string]any, key string) error {
	if _, ok := values[key].(bool); !ok {
		return invalid("properties."+key, "must be a boolean")
	}
	return nil
}

func unsignedInteger(values map[string]any, key string, minimum, maximum int64) error {
	number, ok := values[key].(json.Number)
	if !ok {
		return invalid("properties."+key, "must be an integer")
	}
	value, err := number.Int64()
	if err != nil || value < minimum || value > maximum {
		return invalid("properties."+key, fmt.Sprintf("must be an integer between %d and %d", minimum, maximum))
	}
	return nil
}

func printableASCII(value string) bool {
	for index := range len(value) {
		if value[index] < 0x20 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func hasControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func validULID(value string) bool {
	return ulidPattern.MatchString(value)
}

func normalizeLoginID(value string) (string, error) {
	loginID := strings.ToLower(strings.TrimSpace(value))
	if len(loginID) < 3 || len(loginID) > 32 || loginID[0] < 'a' || loginID[0] > 'z' {
		return "", invalid("loginId", "must follow the account login ID rules")
	}
	for index := 1; index < len(loginID); index++ {
		character := loginID[index]
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' && character != '-' {
			return "", invalid("loginId", "must follow the account login ID rules")
		}
	}
	switch loginID {
	case "admin", "administrator", "root", "support", "system":
		return "", invalid("loginId", "is reserved")
	}
	return loginID, nil
}

func invalid(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}
