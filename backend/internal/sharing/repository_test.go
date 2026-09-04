package sharing

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"gamegen/backend/internal/platform/database"
)

func TestEqualHash(t *testing.T) {
	if !equalHash([]byte{1, 2, 3}, []byte{1, 2, 3}) {
		t.Fatal("expected equal hashes")
	}
	if equalHash([]byte{1, 2, 3}, []byte{1, 2, 4}) || equalHash([]byte{1}, []byte{1, 2}) {
		t.Fatal("expected unequal hashes")
	}
}

func TestFindPlaySessionRequiresBothSessionAndShareToBeUnexpired(t *testing.T) {
	now := time.Date(2026, 8, 16, 4, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		shareExpires time.Time
		wantErr      error
		wantUpdates  int
	}{
		{"valid", now.Add(time.Microsecond), nil, 1},
		{"share expires exactly now", now, ErrPlayExpired, 0},
		{"share expired before now", now.Add(-time.Microsecond), ErrPlayExpired, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, backend := openPlaySessionRepository(t, now, test.shareExpires)
			session, err := repository.FindPlaySession(context.Background(), []byte("token-hash"), now)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("FindPlaySession() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && session.ID != "01K00000000000000000000001" {
				t.Fatalf("session = %#v", session)
			}
			if backend.lastSeenUpdates != test.wantUpdates {
				t.Fatalf("last_seen updates = %d, want %d", backend.lastSeenUpdates, test.wantUpdates)
			}
			if !strings.Contains(backend.query, "s.expires_at > ?") {
				t.Fatalf("query does not enforce share expiry: %s", backend.query)
			}
			if len(backend.queryArgs) != 3 {
				t.Fatalf("query args = %#v, want token + session time + share time", backend.queryArgs)
			}
			for _, index := range []int{1, 2} {
				got, ok := backend.queryArgs[index].Value.(time.Time)
				if !ok || !got.Equal(now) {
					t.Errorf("query arg %d = %#v, want %s", index, backend.queryArgs[index].Value, now)
				}
			}
		})
	}
}

type playSessionTestBackend struct {
	now             time.Time
	shareExpires    time.Time
	query           string
	queryArgs       []driver.NamedValue
	lastSeenUpdates int
}

func openPlaySessionRepository(t *testing.T, now, shareExpires time.Time) (*Repository, *playSessionTestBackend) {
	t.Helper()
	backend := &playSessionTestBackend{now: now, shareExpires: shareExpires}
	db := sql.OpenDB(&playSessionTestConnector{backend: backend})
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(&database.DB{DB: db}), backend
}

type playSessionTestConnector struct{ backend *playSessionTestBackend }

func (connector *playSessionTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &playSessionTestConnection{backend: connector.backend}, nil
}

func (*playSessionTestConnector) Driver() driver.Driver { return playSessionTestDriver{} }

type playSessionTestDriver struct{}

func (playSessionTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

type playSessionTestConnection struct{ backend *playSessionTestBackend }

func (*playSessionTestConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (*playSessionTestConnection) Close() error { return nil }
func (*playSessionTestConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}

func (connection *playSessionTestConnection) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	connection.backend.query = query
	connection.backend.queryArgs = append([]driver.NamedValue(nil), args...)
	rows := &playSessionTestRows{columns: []string{
		"id", "share_link_id", "expires_at", "game_id", "game_version_id", "title",
		"template_id", "template_version", "config_id", "bucket", "object_key", "nickname",
		"share_expires_at", "share_revoked_at", "game_status", "version_status",
	}}
	sharePredicatePresent := strings.Contains(query, "s.expires_at > ?") && len(args) == 3
	if !sharePredicatePresent || connection.backend.shareExpires.After(connection.backend.now) {
		rows.values = [][]driver.Value{{
			"01K00000000000000000000001", "01K00000000000000000000002", connection.backend.now.Add(time.Hour),
			"01K00000000000000000000003", "01K00000000000000000000004", "Memory", "memory-game", "1.0.0",
			"01K00000000000000000000005", "gamegen-artifacts", "config.json", nil,
			connection.backend.shareExpires, nil, "ready", "ready",
		}}
	}
	return rows, nil
}

func (connection *playSessionTestConnection) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "UPDATE play_sessions SET last_seen_at") {
		connection.backend.lastSeenUpdates++
	}
	return driver.RowsAffected(1), nil
}

type playSessionTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (rows *playSessionTestRows) Columns() []string { return rows.columns }
func (*playSessionTestRows) Close() error           { return nil }
func (rows *playSessionTestRows) Next(dest []driver.Value) error {
	if rows.index >= len(rows.values) {
		return io.EOF
	}
	copy(dest, rows.values[rows.index])
	rows.index++
	return nil
}
