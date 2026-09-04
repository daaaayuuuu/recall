package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

type EventName string

const (
	EventCreatorPageViewed   EventName = "creator.page_viewed"
	EventCreatorRegistered   EventName = "creator.registered"
	EventCreatorLoggedIn     EventName = "creator.logged_in"
	EventGameCreated         EventName = "game.created"
	EventGameVersionCreated  EventName = "game.version_created"
	EventAssetUploaded       EventName = "asset.uploaded"
	EventGenerationSubmitted EventName = "generation.submitted"
	EventGenerationSucceeded EventName = "generation.succeeded"
	EventGenerationFailed    EventName = "generation.failed"
	EventShareCreated        EventName = "share.created"
	EventShareOpened         EventName = "share.opened"
	EventPlayStarted         EventName = "play.started"
	EventPlayCompleted       EventName = "play.completed"
	EventPlayReplayed        EventName = "play.replayed"
)

type Source string

const (
	SourceFrontend Source = "frontend"
	SourceAPI      Source = "api"
	SourceWorker   Source = "worker"
)

type ActorType string

const (
	ActorCreator  ActorType = "creator"
	ActorReceiver ActorType = "receiver"
	ActorSystem   ActorType = "system"
)

var ErrClientEventIDConflict = errors.New("client event id conflicts with an existing event")

type ValidationError struct {
	Field   string
	Message string
}

func (err *ValidationError) Error() string {
	if err.Field == "" {
		return err.Message
	}
	return err.Field + ": " + err.Message
}

type Event struct {
	ID              string          `json:"id"`
	EventName       EventName       `json:"eventName"`
	Source          Source          `json:"source"`
	ActorType       ActorType       `json:"actorType"`
	CreatorID       *string         `json:"creatorId"`
	LoginID         *string         `json:"loginId"`
	UserSessionID   *string         `json:"userSessionId"`
	GameID          *string         `json:"gameId"`
	GameVersionID   *string         `json:"gameVersionId"`
	GenerationRunID *string         `json:"generationRunId"`
	ShareID         *string         `json:"shareId"`
	PlaySessionID   *string         `json:"playSessionId"`
	RequestID       *string         `json:"requestId"`
	ClientEventID   *string         `json:"clientEventId,omitempty"`
	Properties      json.RawMessage `json:"properties"`
	OccurredAt      *time.Time      `json:"occurredAt"`
	CreatedAt       time.Time       `json:"createdAt"`
}

type RecordInput struct {
	EventName       EventName
	Source          Source
	ActorType       ActorType
	CreatorID       string
	UserSessionID   string
	GameID          string
	GameVersionID   string
	GenerationRunID string
	ShareID         string
	PlaySessionID   string
	RequestID       string
	ClientEventID   string
	Properties      json.RawMessage
	OccurredAt      *time.Time
}

type RecordResult struct {
	Event     Event
	Duplicate bool
}

type Cursor struct {
	Version   int       `json:"v"`
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

type ListFilter struct {
	EventName EventName
	CreatorID string
	LoginID   string
	GameID    string
	Source    Source
	From      *time.Time
	To        *time.Time
	Cursor    *Cursor
	Limit     int
}

type EventPage struct {
	Items      []Event
	NextCursor *Cursor
}

type Recorder interface {
	RecordEvent(context.Context, RecordInput) (RecordResult, error)
}

type NoopRecorder struct{}

func (NoopRecorder) RecordEvent(context.Context, RecordInput) (RecordResult, error) {
	return RecordResult{}, nil
}

// FakeRecorder is a concurrency-safe recorder for tests and local composition.
// It keeps defensive copies so callers cannot mutate recorded values across goroutines.
type FakeRecorder struct {
	mu     sync.RWMutex
	inputs []RecordInput
	result RecordResult
	err    error
}

func NewFakeRecorder(result RecordResult, err error) *FakeRecorder {
	return &FakeRecorder{result: cloneRecordResult(result), err: err}
}

func (recorder *FakeRecorder) RecordEvent(_ context.Context, input RecordInput) (RecordResult, error) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.inputs = append(recorder.inputs, cloneRecordInput(input))
	return cloneRecordResult(recorder.result), recorder.err
}

func (recorder *FakeRecorder) SetResult(result RecordResult, err error) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.result = cloneRecordResult(result)
	recorder.err = err
}

func (recorder *FakeRecorder) RecordedInputs() []RecordInput {
	recorder.mu.RLock()
	defer recorder.mu.RUnlock()
	inputs := make([]RecordInput, len(recorder.inputs))
	for index, input := range recorder.inputs {
		inputs[index] = cloneRecordInput(input)
	}
	return inputs
}

func cloneRecordInput(input RecordInput) RecordInput {
	input.Properties = append(json.RawMessage(nil), input.Properties...)
	if input.OccurredAt != nil {
		occurredAt := *input.OccurredAt
		input.OccurredAt = &occurredAt
	}
	return input
}

func cloneRecordResult(result RecordResult) RecordResult {
	result.Event = cloneEvent(result.Event)
	return result
}

func cloneEvent(event Event) Event {
	event.CreatorID = cloneStringPointer(event.CreatorID)
	event.LoginID = cloneStringPointer(event.LoginID)
	event.UserSessionID = cloneStringPointer(event.UserSessionID)
	event.GameID = cloneStringPointer(event.GameID)
	event.GameVersionID = cloneStringPointer(event.GameVersionID)
	event.GenerationRunID = cloneStringPointer(event.GenerationRunID)
	event.ShareID = cloneStringPointer(event.ShareID)
	event.PlaySessionID = cloneStringPointer(event.PlaySessionID)
	event.RequestID = cloneStringPointer(event.RequestID)
	event.ClientEventID = cloneStringPointer(event.ClientEventID)
	event.Properties = append(json.RawMessage(nil), event.Properties...)
	if event.OccurredAt != nil {
		occurredAt := *event.OccurredAt
		event.OccurredAt = &occurredAt
	}
	return event
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
