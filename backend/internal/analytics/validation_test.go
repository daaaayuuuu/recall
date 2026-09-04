package analytics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gamegen/backend/internal/platform/security"
)

func TestValidateRecordInputAcceptsEveryFrozenEvent(t *testing.T) {
	creatorID := newTestID(t)
	userSessionID := newTestID(t)
	gameID := newTestID(t)
	versionID := newTestID(t)
	runID := newTestID(t)
	shareID := newTestID(t)
	playSessionID := newTestID(t)
	requestID := newTestID(t)
	occurredAt := time.Date(2026, 8, 16, 2, 35, 1, 123456000, time.UTC)

	tests := []RecordInput{
		{EventName: EventCreatorPageViewed, Source: SourceFrontend, ActorType: ActorCreator, CreatorID: creatorID, UserSessionID: userSessionID, RequestID: requestID, ClientEventID: "9b2ce6ac-8d0f-4afb-8c47-a331707861ea", Properties: raw(`{"page":"generation-progress"}`), OccurredAt: &occurredAt},
		{EventName: EventCreatorRegistered, Source: SourceAPI, ActorType: ActorCreator, CreatorID: creatorID, RequestID: requestID, Properties: raw(`{}`)},
		{EventName: EventCreatorLoggedIn, Source: SourceAPI, ActorType: ActorCreator, CreatorID: creatorID, UserSessionID: userSessionID, RequestID: requestID, Properties: raw(`{}`)},
		{EventName: EventGameCreated, Source: SourceAPI, ActorType: ActorCreator, CreatorID: creatorID, GameID: gameID, GameVersionID: versionID, RequestID: requestID, Properties: raw(`{"templateId":"memory-game"}`)},
		{EventName: EventGameVersionCreated, Source: SourceAPI, ActorType: ActorCreator, CreatorID: creatorID, GameID: gameID, GameVersionID: versionID, RequestID: requestID, Properties: raw(`{"versionNumber":1,"templateId":"memory-game"}`)},
		{EventName: EventAssetUploaded, Source: SourceAPI, ActorType: ActorCreator, CreatorID: creatorID, GameID: gameID, GameVersionID: versionID, RequestID: requestID, Properties: raw(`{"kind":"game_source","mimeType":"image/png","sizeBytes":0}`)},
		{EventName: EventGenerationSubmitted, Source: SourceAPI, ActorType: ActorCreator, CreatorID: creatorID, GameID: gameID, GameVersionID: versionID, GenerationRunID: runID, RequestID: requestID, Properties: raw(`{"attemptNumber":1,"deduplicated":false}`)},
		{EventName: EventGenerationSucceeded, Source: SourceWorker, ActorType: ActorSystem, CreatorID: creatorID, GameID: gameID, GameVersionID: versionID, GenerationRunID: runID, Properties: raw(`{"executionCount":1}`)},
		{EventName: EventGenerationFailed, Source: SourceWorker, ActorType: ActorSystem, CreatorID: creatorID, GameID: gameID, GameVersionID: versionID, GenerationRunID: runID, Properties: raw(`{"errorCode":"INTERNAL_ERROR","retryable":true,"executionCount":2}`)},
		{EventName: EventShareCreated, Source: SourceAPI, ActorType: ActorCreator, CreatorID: creatorID, GameID: gameID, GameVersionID: versionID, ShareID: shareID, RequestID: requestID, Properties: raw(`{"lifetimeDays":90}`)},
		{EventName: EventShareOpened, Source: SourceAPI, ActorType: ActorReceiver, GameID: gameID, GameVersionID: versionID, ShareID: shareID, RequestID: requestID, Properties: raw(`{}`)},
		{EventName: EventPlayStarted, Source: SourceAPI, ActorType: ActorReceiver, GameID: gameID, GameVersionID: versionID, ShareID: shareID, PlaySessionID: playSessionID, RequestID: requestID, Properties: raw(`{}`)},
		{EventName: EventPlayCompleted, Source: SourceFrontend, ActorType: ActorReceiver, GameID: gameID, GameVersionID: versionID, ShareID: shareID, PlaySessionID: playSessionID, RequestID: requestID, ClientEventID: "2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2", Properties: raw(`{"mode":"public"}`), OccurredAt: &occurredAt},
		{EventName: EventPlayReplayed, Source: SourceFrontend, ActorType: ActorReceiver, GameID: gameID, GameVersionID: versionID, ShareID: shareID, PlaySessionID: playSessionID, RequestID: requestID, ClientEventID: "ba725f80-c782-4f41-a3d5-654ed77ae43a", Properties: raw(`{"mode":"public"}`), OccurredAt: &occurredAt},
	}

	for _, input := range tests {
		t.Run(string(input.EventName), func(t *testing.T) {
			properties, err := ValidateRecordInput(input)
			if err != nil {
				t.Fatalf("ValidateRecordInput() error = %v", err)
			}
			if len(properties) == 0 || properties[0] != '{' {
				t.Fatalf("properties were not normalized: %q", properties)
			}
		})
	}
}

func TestValidateRecordInputRejectsInvalidEventsAndProperties(t *testing.T) {
	creatorID := newTestID(t)
	valid := RecordInput{
		EventName:     EventCreatorPageViewed,
		Source:        SourceFrontend,
		ActorType:     ActorCreator,
		CreatorID:     creatorID,
		UserSessionID: newTestID(t),
		ClientEventID: "9b2ce6ac-8d0f-4afb-8c47-a331707861ea",
		Properties:    raw(`{"page":"games"}`),
	}

	tests := []struct {
		name   string
		mutate func(*RecordInput)
	}{
		{"unknown event", func(input *RecordInput) { input.EventName = "creator.unknown" }},
		{"wrong source", func(input *RecordInput) { input.Source = SourceAPI }},
		{"wrong actor", func(input *RecordInput) { input.ActorType = ActorReceiver }},
		{"missing association", func(input *RecordInput) { input.UserSessionID = "" }},
		{"unexpected association", func(input *RecordInput) { input.GameID = newTestID(t) }},
		{"invalid ulid", func(input *RecordInput) { input.CreatorID = "not-an-id" }},
		{"invalid uuid version", func(input *RecordInput) { input.ClientEventID = "9b2ce6ac-8d0f-3afb-8c47-a331707861ea" }},
		{"uppercase uuid", func(input *RecordInput) { input.ClientEventID = "9B2CE6AC-8D0F-4AFB-8C47-A331707861EA" }},
		{"unknown property", func(input *RecordInput) { input.Properties = raw(`{"page":"games","path":"/app/games"}`) }},
		{"duplicate property", func(input *RecordInput) { input.Properties = raw(`{"page":"games","page":"create"}`) }},
		{"wrong property type", func(input *RecordInput) { input.Properties = raw(`{"page":1}`) }},
		{"wrong property value", func(input *RecordInput) { input.Properties = raw(`{"page":"/app/games"}`) }},
		{"nested property", func(input *RecordInput) { input.Properties = raw(`{"page":{"value":"games"}}`) }},
		{"non object", func(input *RecordInput) { input.Properties = raw(`[]`) }},
		{"null", func(input *RecordInput) { input.Properties = raw(`null`) }},
		{"trailing json", func(input *RecordInput) { input.Properties = raw(`{} {}`) }},
		{"oversized compact json", func(input *RecordInput) { input.Properties = raw(`{"unknown":"` + strings.Repeat("a", 4090) + `"}`) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			if _, err := ValidateRecordInput(input); err == nil {
				t.Fatal("ValidateRecordInput() unexpectedly succeeded")
			}
		})
	}
}

func TestPropertyRangesAndFormats(t *testing.T) {
	tests := []struct {
		event EventName
		raw   string
	}{
		{EventGameCreated, `{"templateId":"Memory Game"}`},
		{EventGameVersionCreated, `{"versionNumber":0,"templateId":"memory-game"}`},
		{EventGameVersionCreated, `{"versionNumber":1.5,"templateId":"memory-game"}`},
		{EventAssetUploaded, `{"kind":"avatar","mimeType":"image/png","sizeBytes":1}`},
		{EventAssetUploaded, `{"kind":"game_source","mimeType":"image/png; charset=binary","sizeBytes":1}`},
		{EventAssetUploaded, `{"kind":"game_source","mimeType":"image/\npng","sizeBytes":1}`},
		{EventGenerationSubmitted, `{"attemptNumber":4294967296,"deduplicated":false}`},
		{EventGenerationFailed, `{"errorCode":"RAW_PROVIDER_ERROR","retryable":true,"executionCount":1}`},
		{EventShareCreated, `{"lifetimeDays":91}`},
		{EventPlayCompleted, `{"mode":"private"}`},
	}
	for _, test := range tests {
		if _, err := ValidateProperties(test.event, raw(test.raw)); err == nil {
			t.Errorf("ValidateProperties(%q, %s) unexpectedly succeeded", test.event, test.raw)
		}
	}
}

func TestTemplateIDAndMIMETypeLengthBoundaries(t *testing.T) {
	template64 := "a" + strings.Repeat("b", 63)
	if _, err := ValidateProperties(EventGameCreated, raw(`{"templateId":"`+template64+`"}`)); err != nil {
		t.Fatalf("64-character templateId rejected: %v", err)
	}
	if _, err := ValidateProperties(EventGameCreated, raw(`{"templateId":"`+template64+`b"}`)); err == nil {
		t.Fatal("65-character templateId accepted")
	}
	mime128 := strings.Repeat("a", 128)
	if _, err := ValidateProperties(EventAssetUploaded, raw(`{"kind":"game_source","mimeType":"`+mime128+`","sizeBytes":1}`)); err != nil {
		t.Fatalf("128-character mimeType rejected: %v", err)
	}
	if _, err := ValidateProperties(EventAssetUploaded, raw(`{"kind":"game_source","mimeType":"`+mime128+`a","sizeBytes":1}`)); err == nil {
		t.Fatal("129-character mimeType accepted")
	}
}

func TestPropertiesCompactSizeBoundary(t *testing.T) {
	atLimit := raw(`{"x":"` + strings.Repeat("a", 4088) + `"}`)
	properties, _, err := decodeProperties(atLimit)
	if err != nil || len(properties) != 4096 {
		t.Fatalf("decodeProperties(at limit) = (%d bytes, %v)", len(properties), err)
	}
	overLimit := raw(`{"x":"` + strings.Repeat("a", 4089) + `"}`)
	if _, _, err := decodeProperties(overLimit); err == nil {
		t.Fatal("decodeProperties() accepted 4097 compact bytes")
	}
	spaced := raw("{\n  \"x\" : \"" + strings.Repeat("a", 4088) + "\"\n}")
	properties, _, err = decodeProperties(spaced)
	if err != nil || len(properties) != 4096 {
		t.Fatalf("decodeProperties(spaced) = (%d bytes, %v)", len(properties), err)
	}
}

func TestListFilterDefaultsNormalizesLoginAndValidatesBounds(t *testing.T) {
	filter, err := ValidateListFilter(ListFilter{LoginID: "  Creator_01  "})
	if err != nil {
		t.Fatalf("ValidateListFilter() error = %v", err)
	}
	if filter.Limit != 50 || filter.LoginID != "creator_01" {
		t.Fatalf("normalized filter = %#v", filter)
	}
	for _, invalidFilter := range []ListFilter{
		{Limit: -1}, {Limit: 101}, {EventName: "unknown.event"}, {Source: "browser"},
		{CreatorID: "bad"}, {GameID: "bad"}, {LoginID: "root"},
		{From: timePointer(time.Unix(2, 0)), To: timePointer(time.Unix(1, 0))},
	} {
		if _, err := ValidateListFilter(invalidFilter); err == nil {
			t.Fatalf("ValidateListFilter(%#v) unexpectedly succeeded", invalidFilter)
		}
	}
}

func TestCursorRoundTripAndStrictDecoding(t *testing.T) {
	cursor := Cursor{Version: 1, CreatedAt: time.Date(2026, 8, 16, 2, 35, 1, 123456000, time.UTC), ID: newTestID(t)}
	encoded, err := EncodeCursor(cursor)
	if err != nil {
		t.Fatalf("EncodeCursor() error = %v", err)
	}
	decoded, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
	if decoded.ID != cursor.ID || !decoded.CreatedAt.Equal(cursor.CreatedAt) || decoded.Version != 1 {
		t.Fatalf("decoded cursor = %#v", decoded)
	}

	badPayloads := []string{
		`{"v":2,"createdAt":"2026-08-16T02:35:01Z","id":"` + cursor.ID + `"}`,
		`{"v":1,"v":1,"createdAt":"2026-08-16T02:35:01Z","id":"` + cursor.ID + `"}`,
		`{"v":1,"createdAt":"2026-08-16T10:35:01+08:00","id":"` + cursor.ID + `"}`,
		`{"v":1,"createdAt":"2026-08-16T02:35:01Z","id":"` + cursor.ID + `","extra":true}`,
	}
	for _, payload := range badPayloads {
		if _, err := DecodeCursor(base64.RawURLEncoding.EncodeToString([]byte(payload))); err == nil {
			t.Fatalf("DecodeCursor(%s) unexpectedly succeeded", payload)
		}
	}
	if _, err := DecodeCursor(encoded + "="); err == nil {
		t.Fatal("DecodeCursor() accepted padded input")
	}
}

func TestNoopRecorder(t *testing.T) {
	var recorder Recorder = NoopRecorder{}
	if _, err := recorder.RecordEvent(context.Background(), RecordInput{}); err != nil {
		t.Fatalf("NoopRecorder.RecordEvent() error = %v", err)
	}
}

func TestFakeRecorderRecordsDefensiveCopiesAndInjectedOutcome(t *testing.T) {
	injectedError := errors.New("analytics unavailable")
	wantResult := RecordResult{Duplicate: true}
	recorder := NewFakeRecorder(wantResult, injectedError)
	properties := raw(`{"page":"games"}`)
	occurredAt := time.Now().UTC()
	input := RecordInput{EventName: EventCreatorPageViewed, Properties: properties, OccurredAt: &occurredAt}
	result, err := recorder.RecordEvent(context.Background(), input)
	if !errors.Is(err, injectedError) || result.Duplicate != wantResult.Duplicate {
		t.Fatalf("RecordEvent() = (%#v, %v)", result, err)
	}
	properties[2] = 'X'
	occurredAt = occurredAt.Add(time.Hour)
	recorded := recorder.RecordedInputs()
	if len(recorded) != 1 || string(recorded[0].Properties) != `{"page":"games"}` || recorded[0].OccurredAt.Equal(occurredAt) {
		t.Fatalf("RecordedInputs() = %#v", recorded)
	}
	recorded[0].Properties[2] = 'Y'
	if string(recorder.RecordedInputs()[0].Properties) != `{"page":"games"}` {
		t.Fatal("RecordedInputs returned mutable recorder storage")
	}
}

func TestFakeRecorderIsRaceSafe(t *testing.T) {
	recorder := NewFakeRecorder(RecordResult{}, nil)
	const writers = 64
	var wait sync.WaitGroup
	for index := range writers {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, _ = recorder.RecordEvent(context.Background(), RecordInput{Properties: raw(`{}`), RequestID: string(rune(index))})
			_ = recorder.RecordedInputs()
			recorder.SetResult(RecordResult{Duplicate: index%2 == 0}, nil)
		}(index)
	}
	wait.Wait()
	if got := len(recorder.RecordedInputs()); got != writers {
		t.Fatalf("recorded inputs = %d, want %d", got, writers)
	}
}

func TestValidationErrorsCanBeClassified(t *testing.T) {
	_, err := ValidateProperties(EventCreatorRegistered, raw(`{"secret":"value"}`))
	var validationError *ValidationError
	if !errors.As(err, &validationError) || validationError.Field == "" {
		t.Fatalf("error = %#v, want ValidationError", err)
	}
}

func TestDuplicatePropertyKeyErrorDoesNotReflectSensitiveKey(t *testing.T) {
	const canary = "SENSITIVE_SECRET_URL_LOGIN_ID_USER_TEXT_CANARY"
	_, err := ValidateProperties(EventCreatorPageViewed, raw(`{"page":"games","`+canary+`":true,"`+canary+`":false}`))
	if err == nil {
		t.Fatal("ValidateProperties() accepted a duplicate key")
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatalf("validation error reflected sensitive key: %v", err)
	}
	var validationError *ValidationError
	if !errors.As(err, &validationError) || validationError.Field != "properties" || validationError.Message != "contains a duplicate key" {
		t.Fatalf("error = %#v, want fixed properties validation error", err)
	}
}

func newTestID(t *testing.T) string {
	t.Helper()
	id, err := security.NewID()
	if err != nil {
		t.Fatalf("security.NewID() error = %v", err)
	}
	return id
}

func raw(value string) json.RawMessage {
	return json.RawMessage(value)
}

func timePointer(value time.Time) *time.Time {
	return &value
}
