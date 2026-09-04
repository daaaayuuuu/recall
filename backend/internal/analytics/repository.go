package analytics

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gamegen/backend/internal/platform/security"

	"github.com/go-sql-driver/mysql"
)

type databaseExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Repository struct {
	db  databaseExecutor
	now func() time.Time
}

func NewRepository(db databaseExecutor) *Repository {
	return &Repository{db: db, now: time.Now}
}

func (repository *Repository) RecordEvent(ctx context.Context, input RecordInput) (RecordResult, error) {
	properties, err := ValidateRecordInput(input)
	if err != nil {
		return RecordResult{}, err
	}
	input.Properties = properties
	if input.OccurredAt != nil {
		occurredAt := input.OccurredAt.UTC().Truncate(time.Microsecond)
		input.OccurredAt = &occurredAt
	}
	if input.ClientEventID != "" {
		existing, err := repository.findByClientEventID(ctx, input.ClientEventID)
		if err == nil {
			return duplicateResult(existing, input)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return RecordResult{}, err
		}
	}

	id, err := security.NewID()
	if err != nil {
		return RecordResult{}, fmt.Errorf("generate behavior event id: %w", err)
	}
	createdAt := repository.now().UTC().Truncate(time.Microsecond)
	_, err = repository.db.ExecContext(ctx, `
		INSERT INTO behavior_events
		(id, event_name, source, actor_type, user_id, user_session_id, game_id,
		 game_version_id, generation_run_id, share_link_id, play_session_id,
		 request_id, client_event_id, properties, occurred_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.EventName, input.Source, input.ActorType,
		nullString(input.CreatorID), nullString(input.UserSessionID), nullString(input.GameID),
		nullString(input.GameVersionID), nullString(input.GenerationRunID), nullString(input.ShareID),
		nullString(input.PlaySessionID), nullString(input.RequestID), nullString(input.ClientEventID),
		[]byte(input.Properties), nullTime(input.OccurredAt), createdAt,
	)
	if err != nil {
		var mysqlError *mysql.MySQLError
		if input.ClientEventID != "" && errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			existing, findErr := repository.findByClientEventID(ctx, input.ClientEventID)
			if findErr != nil {
				return RecordResult{}, fmt.Errorf("resolve duplicate behavior event: %w", findErr)
			}
			return duplicateResult(existing, input)
		}
		return RecordResult{}, fmt.Errorf("insert behavior event: %w", err)
	}
	return RecordResult{Event: eventFromInput(id, createdAt, input)}, nil
}

func (repository *Repository) ListAdminEvents(ctx context.Context, filter ListFilter) (EventPage, error) {
	filter, err := ValidateListFilter(filter)
	if err != nil {
		return EventPage{}, err
	}
	query := strings.Builder{}
	query.WriteString(`
		SELECT e.id, e.event_name, e.source, e.actor_type, e.user_id, u.login_id,
		       e.user_session_id, e.game_id, e.game_version_id, e.generation_run_id,
		       e.share_link_id, e.play_session_id, e.request_id, e.client_event_id,
		       e.properties, e.occurred_at, e.created_at
		FROM behavior_events e
		LEFT JOIN users u ON u.id = e.user_id
		WHERE 1 = 1`)
	arguments := make([]any, 0, 12)
	appendCondition := func(sql string, value any) {
		query.WriteString(sql)
		arguments = append(arguments, value)
	}
	if filter.EventName != "" {
		appendCondition(" AND e.event_name = ?", filter.EventName)
	}
	if filter.CreatorID != "" {
		appendCondition(" AND e.user_id = ?", filter.CreatorID)
	}
	if filter.LoginID != "" {
		appendCondition(" AND u.login_id = ?", filter.LoginID)
	}
	if filter.GameID != "" {
		appendCondition(" AND e.game_id = ?", filter.GameID)
	}
	if filter.Source != "" {
		appendCondition(" AND e.source = ?", filter.Source)
	}
	if filter.From != nil {
		appendCondition(" AND e.created_at >= ?", filter.From.UTC())
	}
	if filter.To != nil {
		appendCondition(" AND e.created_at < ?", filter.To.UTC())
	}
	if filter.Cursor != nil {
		query.WriteString(" AND (e.created_at < ? OR (e.created_at = ? AND e.id < ?))")
		arguments = append(arguments, filter.Cursor.CreatedAt.UTC(), filter.Cursor.CreatedAt.UTC(), filter.Cursor.ID)
	}
	query.WriteString(" ORDER BY e.created_at DESC, e.id DESC LIMIT ?")
	arguments = append(arguments, filter.Limit+1)

	rows, err := repository.db.QueryContext(ctx, query.String(), arguments...)
	if err != nil {
		return EventPage{}, fmt.Errorf("list behavior events: %w", err)
	}
	defer rows.Close()
	items := make([]Event, 0, filter.Limit)
	for rows.Next() {
		event, err := scanEvent(rows, true)
		if err != nil {
			return EventPage{}, err
		}
		properties, err := ValidateProperties(event.EventName, event.Properties)
		if err != nil {
			return EventPage{}, fmt.Errorf("validate stored behavior event %s: %w", event.ID, err)
		}
		event.Properties = properties
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return EventPage{}, fmt.Errorf("iterate behavior events: %w", err)
	}
	page := EventPage{Items: items}
	if len(items) > filter.Limit {
		page.Items = items[:filter.Limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &Cursor{Version: 1, CreatedAt: last.CreatedAt.UTC(), ID: last.ID}
	}
	return page, nil
}

func (repository *Repository) findByClientEventID(ctx context.Context, clientEventID string) (Event, error) {
	row := repository.db.QueryRowContext(ctx, `
		SELECT id, event_name, source, actor_type, user_id, user_session_id, game_id,
		       game_version_id, generation_run_id, share_link_id, play_session_id,
		       request_id, client_event_id, properties, occurred_at, created_at
		FROM behavior_events WHERE client_event_id = ?`, clientEventID)
	event, err := scanEvent(row, false)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Event{}, sql.ErrNoRows
		}
		return Event{}, fmt.Errorf("find behavior event by client id: %w", err)
	}
	properties, err := ValidateProperties(event.EventName, event.Properties)
	if err != nil {
		return Event{}, fmt.Errorf("validate stored behavior event %s: %w", event.ID, err)
	}
	event.Properties = properties
	return event, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanEvent(scanner rowScanner, withLoginID bool) (Event, error) {
	var event Event
	var creatorID, loginID, userSessionID, gameID, gameVersionID sql.NullString
	var generationRunID, shareID, playSessionID, requestID, clientEventID sql.NullString
	var occurredAt sql.NullTime
	var properties []byte
	destinations := []any{
		&event.ID, &event.EventName, &event.Source, &event.ActorType, &creatorID,
	}
	if withLoginID {
		destinations = append(destinations, &loginID)
	}
	destinations = append(destinations,
		&userSessionID, &gameID, &gameVersionID, &generationRunID, &shareID,
		&playSessionID, &requestID, &clientEventID, &properties, &occurredAt, &event.CreatedAt,
	)
	if err := scanner.Scan(destinations...); err != nil {
		return Event{}, err
	}
	event.CreatorID = stringPointer(creatorID)
	event.LoginID = stringPointer(loginID)
	event.UserSessionID = stringPointer(userSessionID)
	event.GameID = stringPointer(gameID)
	event.GameVersionID = stringPointer(gameVersionID)
	event.GenerationRunID = stringPointer(generationRunID)
	event.ShareID = stringPointer(shareID)
	event.PlaySessionID = stringPointer(playSessionID)
	event.RequestID = stringPointer(requestID)
	event.ClientEventID = stringPointer(clientEventID)
	event.Properties = append(json.RawMessage(nil), properties...)
	if occurredAt.Valid {
		value := occurredAt.Time.UTC()
		event.OccurredAt = &value
	}
	event.CreatedAt = event.CreatedAt.UTC()
	return event, nil
}

func duplicateResult(existing Event, input RecordInput) (RecordResult, error) {
	if !sameSemantics(existing, input) {
		return RecordResult{}, ErrClientEventIDConflict
	}
	return RecordResult{Event: existing, Duplicate: true}, nil
}

func sameSemantics(existing Event, input RecordInput) bool {
	return existing.EventName == input.EventName &&
		existing.Source == input.Source &&
		existing.ActorType == input.ActorType &&
		pointerValue(existing.CreatorID) == input.CreatorID &&
		pointerValue(existing.UserSessionID) == input.UserSessionID &&
		pointerValue(existing.GameID) == input.GameID &&
		pointerValue(existing.GameVersionID) == input.GameVersionID &&
		pointerValue(existing.GenerationRunID) == input.GenerationRunID &&
		pointerValue(existing.ShareID) == input.ShareID &&
		pointerValue(existing.PlaySessionID) == input.PlaySessionID &&
		pointerValue(existing.ClientEventID) == input.ClientEventID &&
		bytes.Equal(existing.Properties, input.Properties)
}

func eventFromInput(id string, createdAt time.Time, input RecordInput) Event {
	return Event{
		ID:              id,
		EventName:       input.EventName,
		Source:          input.Source,
		ActorType:       input.ActorType,
		CreatorID:       optionalPointer(input.CreatorID),
		UserSessionID:   optionalPointer(input.UserSessionID),
		GameID:          optionalPointer(input.GameID),
		GameVersionID:   optionalPointer(input.GameVersionID),
		GenerationRunID: optionalPointer(input.GenerationRunID),
		ShareID:         optionalPointer(input.ShareID),
		PlaySessionID:   optionalPointer(input.PlaySessionID),
		RequestID:       optionalPointer(input.RequestID),
		ClientEventID:   optionalPointer(input.ClientEventID),
		Properties:      append(json.RawMessage(nil), input.Properties...),
		OccurredAt:      input.OccurredAt,
		CreatedAt:       createdAt,
	}
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func optionalPointer(value string) *string {
	if value == "" {
		return nil
	}
	result := value
	return &result
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
