package analytics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestRecordEventInsertsValidatedEvent(t *testing.T) {
	backend := &scriptedBackend{}
	db := openScriptedDB(t, backend)
	repository := NewRepository(db)
	now := time.Date(2026, 8, 16, 2, 35, 1, 123456789, time.UTC)
	repository.now = func() time.Time { return now }
	input := RecordInput{
		EventName: EventCreatorRegistered,
		Source:    SourceAPI, ActorType: ActorCreator,
		CreatorID: newTestID(t), RequestID: newTestID(t), Properties: raw(`{ }`),
	}

	result, err := repository.RecordEvent(context.Background(), input)
	if err != nil {
		t.Fatalf("RecordEvent() error = %v", err)
	}
	if result.Duplicate || !validULID(result.Event.ID) {
		t.Fatalf("result = %#v", result)
	}
	if result.Event.CreatedAt.Nanosecond() != 123456000 || string(result.Event.Properties) != `{}` {
		t.Fatalf("event was not normalized: %#v", result.Event)
	}
	if len(backend.execs) != 1 || !strings.Contains(backend.execs[0].query, "INSERT INTO behavior_events") {
		t.Fatalf("execs = %#v", backend.execs)
	}
	assertQueryArguments(t, backend.execs[0].args, []any{
		result.Event.ID, string(EventCreatorRegistered), string(SourceAPI), string(ActorCreator),
		input.CreatorID, nil, nil, nil, nil, nil, nil, input.RequestID, nil, []byte(`{}`), nil,
		now.UTC().Truncate(time.Microsecond),
	})
}

func TestRecordEventReturnsIdempotentDuplicateAndConflict(t *testing.T) {
	input, existing := frontendRecordFixture(t)
	input.RequestID = newTestID(t)
	changedOccurredAt := existing.OccurredAt.Add(5 * time.Minute)
	input.OccurredAt = &changedOccurredAt
	backend := &scriptedBackend{queries: []queryResponse{rowsForStoredEvent(existing)}}
	repository := NewRepository(openScriptedDB(t, backend))
	result, err := repository.RecordEvent(context.Background(), input)
	if err != nil || !result.Duplicate || result.Event.ID != existing.ID {
		t.Fatalf("RecordEvent() = (%#v, %v)", result, err)
	}
	if len(backend.execs) != 0 {
		t.Fatal("idempotent lookup unexpectedly inserted a row")
	}
	assertQueryArguments(t, backend.queryCalls[0].args, []any{input.ClientEventID})

	input.RequestID = ""
	input.OccurredAt = nil
	backend = &scriptedBackend{queries: []queryResponse{rowsForStoredEvent(existing)}}
	repository = NewRepository(openScriptedDB(t, backend))
	result, err = repository.RecordEvent(context.Background(), input)
	if err != nil || !result.Duplicate {
		t.Fatalf("RecordEvent() with omitted requestId/occurredAt = (%#v, %v)", result, err)
	}
	assertQueryArguments(t, backend.queryCalls[0].args, []any{input.ClientEventID})

	input.EventName = EventPlayReplayed
	backend = &scriptedBackend{queries: []queryResponse{rowsForStoredEvent(existing)}}
	repository = NewRepository(openScriptedDB(t, backend))
	_, err = repository.RecordEvent(context.Background(), input)
	if !errors.Is(err, ErrClientEventIDConflict) {
		t.Fatalf("RecordEvent() error = %v, want conflict", err)
	}
	assertQueryArguments(t, backend.queryCalls[0].args, []any{input.ClientEventID})
}

func TestSameSemanticsIgnoresRequestAndOccurredAtButConflictsOnPropertiesAndTrustedLinks(t *testing.T) {
	values := []string{
		newTestID(t), newTestID(t), newTestID(t), newTestID(t), newTestID(t), newTestID(t), newTestID(t),
	}
	clientID := "2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2"
	originalOccurredAt := time.Date(2026, 8, 16, 2, 35, 1, 0, time.UTC)
	existing := Event{
		EventName: EventGenerationSubmitted, Source: SourceFrontend, ActorType: ActorCreator,
		CreatorID: &values[0], UserSessionID: &values[1], GameID: &values[2], GameVersionID: &values[3],
		GenerationRunID: &values[4], ShareID: &values[5], PlaySessionID: &values[6],
		ClientEventID: &clientID, Properties: raw(`{"attemptNumber":1}`), OccurredAt: &originalOccurredAt,
	}
	input := RecordInput{
		EventName: existing.EventName, Source: existing.Source, ActorType: existing.ActorType,
		CreatorID: values[0], UserSessionID: values[1], GameID: values[2], GameVersionID: values[3],
		GenerationRunID: values[4], ShareID: values[5], PlaySessionID: values[6], ClientEventID: clientID,
		RequestID: newTestID(t), Properties: raw(`{"attemptNumber":1}`), OccurredAt: nil,
	}
	if _, err := duplicateResult(existing, input); err != nil {
		t.Fatalf("requestId/occurredAt-only change returned %v", err)
	}

	mutations := []struct {
		name   string
		mutate func(*RecordInput)
	}{
		{"properties", func(value *RecordInput) { value.Properties = raw(`{"attemptNumber":2}`) }},
		{"creator", func(value *RecordInput) { value.CreatorID = newTestID(t) }},
		{"user session", func(value *RecordInput) { value.UserSessionID = newTestID(t) }},
		{"game", func(value *RecordInput) { value.GameID = newTestID(t) }},
		{"game version", func(value *RecordInput) { value.GameVersionID = newTestID(t) }},
		{"generation run", func(value *RecordInput) { value.GenerationRunID = newTestID(t) }},
		{"share", func(value *RecordInput) { value.ShareID = newTestID(t) }},
		{"play session", func(value *RecordInput) { value.PlaySessionID = newTestID(t) }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := input
			mutation.mutate(&changed)
			if _, err := duplicateResult(existing, changed); !errors.Is(err, ErrClientEventIDConflict) {
				t.Fatalf("duplicateResult() error = %v, want conflict", err)
			}
		})
	}
}

func TestRecordEventResolvesConcurrentUniqueKeyRace(t *testing.T) {
	input, existing := frontendRecordFixture(t)
	backend := &scriptedBackend{
		queries:    []queryResponse{{columns: storedEventColumns}, rowsForStoredEvent(existing)},
		execErrors: []error{&mysql.MySQLError{Number: 1062, Message: "duplicate client id"}},
	}
	repository := NewRepository(openScriptedDB(t, backend))
	result, err := repository.RecordEvent(context.Background(), input)
	if err != nil || !result.Duplicate || result.Event.ID != existing.ID {
		t.Fatalf("RecordEvent() = (%#v, %v)", result, err)
	}
}

func TestListAdminEventsUsesParameterizedFiltersAndStableCursor(t *testing.T) {
	createdAt := time.Date(2026, 8, 16, 2, 35, 1, 123456000, time.UTC)
	creatorID := newTestID(t)
	gameID := newTestID(t)
	cursorID := newTestID(t)
	ids := []string{newTestID(t), newTestID(t), newTestID(t)}
	backend := &scriptedBackend{queries: []queryResponse{{
		columns: listedEventColumns,
		values: [][]driver.Value{
			listedEventValues(ids[0], creatorID, "creator_01", gameID, createdAt),
			listedEventValues(ids[1], creatorID, "creator_01", gameID, createdAt),
			listedEventValues(ids[2], creatorID, "creator_01", gameID, createdAt),
		},
	}}}
	repository := NewRepository(openScriptedDB(t, backend))
	from := createdAt.Add(-time.Hour)
	to := createdAt.Add(time.Hour)
	page, err := repository.ListAdminEvents(context.Background(), ListFilter{
		EventName: EventGameCreated, CreatorID: creatorID, LoginID: " Creator_01 ", GameID: gameID,
		Source: SourceAPI, From: &from, To: &to,
		Cursor: &Cursor{Version: 1, CreatedAt: createdAt.Add(time.Minute), ID: cursorID}, Limit: 2,
	})
	if err != nil {
		t.Fatalf("ListAdminEvents() error = %v", err)
	}
	if len(page.Items) != 2 || page.NextCursor == nil || page.NextCursor.ID != ids[1] {
		t.Fatalf("page = %#v", page)
	}
	if page.Items[0].LoginID == nil || *page.Items[0].LoginID != "creator_01" {
		t.Fatalf("login join result = %#v", page.Items[0].LoginID)
	}
	if string(page.Items[0].Properties) != `{"templateId":"memory-game"}` {
		t.Fatalf("properties = %s", page.Items[0].Properties)
	}
	call := backend.queryCalls[0]
	for _, clause := range []string{
		"e.event_name = ?", "e.user_id = ?", "u.login_id = ?", "e.game_id = ?", "e.source = ?",
		"e.created_at >= ?", "e.created_at < ?", "e.created_at = ? AND e.id < ?",
		"ORDER BY e.created_at DESC, e.id DESC LIMIT ?",
	} {
		if !strings.Contains(call.query, clause) {
			t.Errorf("query lacks %q: %s", clause, call.query)
		}
	}
	if strings.Contains(call.query, "creator_01") {
		t.Fatal("login ID was interpolated into SQL")
	}
	wantArguments := []any{
		string(EventGameCreated), creatorID, "creator_01", gameID, string(SourceAPI),
		from.UTC(), to.UTC(), createdAt.Add(time.Minute).UTC(), createdAt.Add(time.Minute).UTC(), cursorID, int64(3),
	}
	assertQueryArguments(t, call.args, wantArguments)
}

func TestListAdminEventsPaginatesSameTimestampWithoutDuplicatesOrGaps(t *testing.T) {
	createdAt := time.Date(2026, 8, 16, 2, 35, 1, 123456000, time.UTC)
	creatorID := newTestID(t)
	gameID := newTestID(t)
	ids := []string{
		"01K00000000000000000000004",
		"01K00000000000000000000003",
		"01K00000000000000000000002",
		"01K00000000000000000000001",
	}
	backend := &scriptedBackend{queries: []queryResponse{
		{columns: listedEventColumns, values: [][]driver.Value{
			listedEventValues(ids[0], creatorID, "creator_01", gameID, createdAt),
			listedEventValues(ids[1], creatorID, "creator_01", gameID, createdAt),
			listedEventValues(ids[2], creatorID, "creator_01", gameID, createdAt),
		}},
		{columns: listedEventColumns, values: [][]driver.Value{
			listedEventValues(ids[2], creatorID, "creator_01", gameID, createdAt),
			listedEventValues(ids[3], creatorID, "creator_01", gameID, createdAt),
		}},
	}}
	repository := NewRepository(openScriptedDB(t, backend))
	first, err := repository.ListAdminEvents(context.Background(), ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("first ListAdminEvents() error = %v", err)
	}
	if first.NextCursor == nil || first.NextCursor.ID != ids[1] || !first.NextCursor.CreatedAt.Equal(createdAt) {
		t.Fatalf("first page cursor = %#v", first.NextCursor)
	}
	second, err := repository.ListAdminEvents(context.Background(), ListFilter{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("second ListAdminEvents() error = %v", err)
	}
	if second.NextCursor != nil {
		t.Fatalf("second page cursor = %#v, want nil", second.NextCursor)
	}
	combined := append([]Event(nil), first.Items...)
	combined = append(combined, second.Items...)
	if len(combined) != len(ids) {
		t.Fatalf("combined item count = %d, want %d", len(combined), len(ids))
	}
	seen := make(map[string]struct{}, len(combined))
	for index, event := range combined {
		if event.ID != ids[index] {
			t.Errorf("combined[%d].ID = %q, want %q", index, event.ID, ids[index])
		}
		if _, duplicate := seen[event.ID]; duplicate {
			t.Errorf("duplicate event %q", event.ID)
		}
		seen[event.ID] = struct{}{}
	}
	if len(backend.queryCalls) != 2 {
		t.Fatalf("query calls = %d, want 2", len(backend.queryCalls))
	}
	assertQueryArguments(t, backend.queryCalls[0].args, []any{int64(3)})
	assertQueryArguments(t, backend.queryCalls[1].args, []any{createdAt, createdAt, ids[1], int64(3)})
}

func TestListAdminEventsReturnsNonNilEmptySlice(t *testing.T) {
	backend := &scriptedBackend{queries: []queryResponse{{columns: listedEventColumns}}}
	repository := NewRepository(openScriptedDB(t, backend))
	page, err := repository.ListAdminEvents(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListAdminEvents() error = %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || page.NextCursor != nil {
		t.Fatalf("page = %#v", page)
	}
	assertQueryArguments(t, backend.queryCalls[0].args, []any{int64(51)})
}

func TestListAdminEventsReadsHistoricalEventWithNullCreatorAndLogin(t *testing.T) {
	createdAt := time.Date(2026, 8, 16, 2, 35, 1, 0, time.UTC)
	gameID := newTestID(t)
	versionID := newTestID(t)
	shareID := newTestID(t)
	values := []driver.Value{
		newTestID(t), string(EventShareOpened), string(SourceAPI), string(ActorReceiver), nil, nil,
		nil, gameID, versionID, nil, shareID, nil, newTestID(t), nil, []byte(`{}`), nil, createdAt,
	}
	backend := &scriptedBackend{queries: []queryResponse{{columns: listedEventColumns, values: [][]driver.Value{values}}}}
	repository := NewRepository(openScriptedDB(t, backend))
	page, err := repository.ListAdminEvents(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("ListAdminEvents() error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].CreatorID != nil || page.Items[0].LoginID != nil {
		t.Fatalf("historical event = %#v", page.Items)
	}
	assertQueryArguments(t, backend.queryCalls[0].args, []any{int64(51)})
}

func TestListAdminEventsRejectsStoredUnknownProperties(t *testing.T) {
	createdAt := time.Date(2026, 8, 16, 2, 35, 1, 0, time.UTC)
	values := listedEventValues(newTestID(t), newTestID(t), "creator_01", newTestID(t), createdAt)
	values[14] = []byte(`{"templateId":"memory-game","secret":"do-not-return"}`)
	backend := &scriptedBackend{queries: []queryResponse{{columns: listedEventColumns, values: [][]driver.Value{values}}}}
	repository := NewRepository(openScriptedDB(t, backend))
	if _, err := repository.ListAdminEvents(context.Background(), ListFilter{}); err == nil {
		t.Fatal("ListAdminEvents() returned an event containing unknown stored properties")
	}
}

func frontendRecordFixture(t *testing.T) (RecordInput, Event) {
	t.Helper()
	gameID := newTestID(t)
	versionID := newTestID(t)
	shareID := newTestID(t)
	playSessionID := newTestID(t)
	requestID := newTestID(t)
	clientID := "2afbf4ca-4dc4-40e9-80d2-2e31dca70aa2"
	occurredAt := time.Date(2026, 8, 16, 2, 35, 1, 123456000, time.UTC)
	createdAt := occurredAt.Add(time.Second)
	input := RecordInput{
		EventName: EventPlayCompleted, Source: SourceFrontend, ActorType: ActorReceiver,
		GameID: gameID, GameVersionID: versionID, ShareID: shareID, PlaySessionID: playSessionID,
		RequestID: requestID, ClientEventID: clientID, Properties: raw(`{ "mode": "public" }`), OccurredAt: &occurredAt,
	}
	existing := Event{
		ID: newTestID(t), EventName: input.EventName, Source: input.Source, ActorType: input.ActorType,
		GameID: &gameID, GameVersionID: &versionID, ShareID: &shareID, PlaySessionID: &playSessionID,
		RequestID: &requestID, ClientEventID: &clientID, Properties: raw(`{ "mode": "public" }`),
		OccurredAt: &occurredAt, CreatedAt: createdAt,
	}
	return input, existing
}

var storedEventColumns = []string{
	"id", "event_name", "source", "actor_type", "user_id", "user_session_id", "game_id",
	"game_version_id", "generation_run_id", "share_link_id", "play_session_id", "request_id",
	"client_event_id", "properties", "occurred_at", "created_at",
}

var listedEventColumns = []string{
	"id", "event_name", "source", "actor_type", "user_id", "login_id", "user_session_id", "game_id",
	"game_version_id", "generation_run_id", "share_link_id", "play_session_id", "request_id",
	"client_event_id", "properties", "occurred_at", "created_at",
}

func rowsForStoredEvent(event Event) queryResponse {
	return queryResponse{columns: storedEventColumns, values: [][]driver.Value{{
		event.ID, string(event.EventName), string(event.Source), string(event.ActorType), pointerDriverValue(event.CreatorID),
		pointerDriverValue(event.UserSessionID), pointerDriverValue(event.GameID), pointerDriverValue(event.GameVersionID),
		pointerDriverValue(event.GenerationRunID), pointerDriverValue(event.ShareID), pointerDriverValue(event.PlaySessionID),
		pointerDriverValue(event.RequestID), pointerDriverValue(event.ClientEventID), []byte(event.Properties),
		timeDriverValue(event.OccurredAt), event.CreatedAt,
	}}}
}

func listedEventValues(id, creatorID, loginID, gameID string, createdAt time.Time) []driver.Value {
	return []driver.Value{
		id, string(EventGameCreated), string(SourceAPI), string(ActorCreator), creatorID, loginID,
		nil, gameID, newTestStaticID(), nil, nil, nil, newTestStaticID(), nil,
		[]byte(`{ "templateId": "memory-game" }`), nil, createdAt,
	}
}

func newTestStaticID() string { return "01K00000000000000000000000" }

func pointerDriverValue(value *string) driver.Value {
	if value == nil {
		return nil
	}
	return *value
}

func timeDriverValue(value *time.Time) driver.Value {
	if value == nil {
		return nil
	}
	return *value
}

func assertQueryArguments(t *testing.T, got []driver.NamedValue, want []any) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("query arguments = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index].Ordinal != index+1 {
			t.Errorf("argument %d ordinal = %d, want %d", index, got[index].Ordinal, index+1)
		}
		if !reflect.DeepEqual(got[index].Value, want[index]) {
			t.Errorf("argument %d = %#v (%T), want %#v (%T)", index, got[index].Value, got[index].Value, want[index], want[index])
		}
	}
}

type recordedCall struct {
	query string
	args  []driver.NamedValue
}

type queryResponse struct {
	columns []string
	values  [][]driver.Value
	err     error
}

type scriptedBackend struct {
	mu         sync.Mutex
	queries    []queryResponse
	execErrors []error
	queryCalls []recordedCall
	execs      []recordedCall
}

func (backend *scriptedBackend) nextQuery(query string, args []driver.NamedValue) (driver.Rows, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.queryCalls = append(backend.queryCalls, recordedCall{query: query, args: append([]driver.NamedValue(nil), args...)})
	if len(backend.queries) == 0 {
		return &staticRows{columns: []string{"empty"}}, nil
	}
	response := backend.queries[0]
	backend.queries = backend.queries[1:]
	if response.err != nil {
		return nil, response.err
	}
	return &staticRows{columns: response.columns, values: response.values}, nil
}

func (backend *scriptedBackend) nextExec(query string, args []driver.NamedValue) (driver.Result, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.execs = append(backend.execs, recordedCall{query: query, args: append([]driver.NamedValue(nil), args...)})
	if len(backend.execErrors) > 0 {
		err := backend.execErrors[0]
		backend.execErrors = backend.execErrors[1:]
		if err != nil {
			return nil, err
		}
	}
	return driver.RowsAffected(1), nil
}

var scriptedDriverCounter atomic.Uint64

func openScriptedDB(t *testing.T, backend *scriptedBackend) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("analytics-scripted-%d", scriptedDriverCounter.Add(1))
	sql.Register(name, &scriptedDriver{backend: backend})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type scriptedDriver struct{ backend *scriptedBackend }

func (driverInstance *scriptedDriver) Open(string) (driver.Conn, error) {
	return &scriptedConn{backend: driverInstance.backend}, nil
}

type scriptedConn struct{ backend *scriptedBackend }

func (*scriptedConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (*scriptedConn) Close() error              { return nil }
func (*scriptedConn) Begin() (driver.Tx, error) { return nil, errors.New("transactions not supported") }
func (connection *scriptedConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return connection.backend.nextQuery(query, args)
}
func (connection *scriptedConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return connection.backend.nextExec(query, args)
}

type staticRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (rows *staticRows) Columns() []string { return rows.columns }
func (*staticRows) Close() error           { return nil }
func (rows *staticRows) Next(destination []driver.Value) error {
	if rows.index >= len(rows.values) {
		return io.EOF
	}
	copy(destination, rows.values[rows.index])
	rows.index++
	return nil
}
