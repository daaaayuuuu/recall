package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/database"
	"gamegen/backend/internal/platform/security"

	"github.com/go-chi/chi/v5"
)

func TestSessionMiddlewaresEnforceCookieIsolationCSRFAndTrustedIdentity(t *testing.T) {
	now := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)
	creatorToken := "creator-cookie-token"
	adminToken := "admin-cookie-token"
	creatorHash := security.HashToken(creatorToken)
	adminHash := security.HashToken(adminToken)
	csrfHash := security.HashToken("valid-csrf")
	sqlDB := sql.OpenDB(&authSessionConnector{
		now: now, creatorHash: creatorHash, adminHash: adminHash, csrfHash: csrfHash,
	})
	t.Cleanup(func() { _ = sqlDB.Close() })
	repository := NewRepository(&database.DB{DB: sqlDB})
	handler := NewHandler(repository, nil, nil, config.Config{
		App:   config.AppConfig{Environment: "test", AppBaseURL: "https://app.example", Surface: "all"},
		Admin: config.AdminConfig{Username: "admin", PasswordHash: "test-admin-fingerprint"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler.now = func() time.Time { return now }

	router := chi.NewRouter()
	router.With(handler.RequireCreatorSession, handler.RequireCreatorMutation).Post("/creator", func(w http.ResponseWriter, request *http.Request) {
		if CreatorUser(request).ID != "01K00000000000000000000002" || CreatorSessionID(request) != "01K00000000000000000000001" {
			t.Errorf("unexpected authenticated identity: user=%q session=%q", CreatorUser(request).ID, CreatorSessionID(request))
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router.With(handler.RequireAdminSession).Get("/admin", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	router.With(handler.RequireAdminSession, handler.RequireAdminMutation).Post("/admin-mutation", func(w http.ResponseWriter, request *http.Request) {
		sessionID, username := AdminIdentity(request)
		if sessionID != "01K00000000000000000000003" || username != "admin" {
			t.Errorf("unexpected admin identity: session=%q username=%q", sessionID, username)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/creator", nil)
	request.Header.Set("Origin", "https://app.example")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("creator without cookie status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/admin", nil)
	request.AddCookie(&http.Cookie{Name: creatorSessionCookieLocal, Value: creatorToken})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("creator cookie on admin status = %d", response.Code)
	}

	for _, csrf := range []string{"", "wrong-csrf"} {
		request = httptest.NewRequest(http.MethodPost, "/creator", nil)
		request.Header.Set("Origin", "https://app.example")
		request.Header.Set("X-CSRF-Token", csrf)
		request.AddCookie(&http.Cookie{Name: creatorSessionCookieLocal, Value: creatorToken})
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("csrf %q status = %d", csrf, response.Code)
		}
	}

	request = httptest.NewRequest(http.MethodPost, "/creator", nil)
	request.Header.Set("Origin", "https://app.example")
	request.Header.Set("X-CSRF-Token", "valid-csrf")
	request.AddCookie(&http.Cookie{Name: creatorSessionCookieLocal, Value: creatorToken})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("valid creator session status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/admin", nil)
	request.AddCookie(&http.Cookie{Name: adminSessionCookieLocal, Value: adminToken})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("valid admin session status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/admin-mutation", nil)
	request.Header.Set("Origin", "https://app.example")
	request.Header.Set("X-CSRF-Token", "valid-csrf")
	request.AddCookie(&http.Cookie{Name: adminSessionCookieLocal, Value: adminToken})
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("valid admin mutation status = %d, body = %s", response.Code, response.Body.String())
	}
}

type authSessionConnector struct {
	now         time.Time
	creatorHash [32]byte
	adminHash   [32]byte
	csrfHash    [32]byte
}

func (connector *authSessionConnector) Connect(context.Context) (driver.Conn, error) {
	return &authSessionConnection{connector: connector}, nil
}

func (*authSessionConnector) Driver() driver.Driver { return authSessionDriver{} }

type authSessionDriver struct{}

func (authSessionDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type authSessionConnection struct{ connector *authSessionConnector }

func (*authSessionConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (*authSessionConnection) Close() error { return nil }
func (*authSessionConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}

func (connection *authSessionConnection) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if len(args) == 0 {
		return nil, errors.New("missing session hash")
	}
	provided, _ := args[0].Value.([]byte)
	if strings.Contains(query, "FROM user_sessions") {
		rows := &authSessionRows{columns: []string{
			"id", "csrf_token_hash", "expires_at", "user_id", "login_id", "password_hash",
			"nickname", "avatar_asset_id", "status", "created_at", "updated_at",
		}}
		if subtle.ConstantTimeCompare(provided, connection.connector.creatorHash[:]) == 1 {
			rows.values = [][]driver.Value{{
				"01K00000000000000000000001", connection.connector.csrfHash[:], connection.connector.now.Add(time.Hour),
				"01K00000000000000000000002", "creator_01", "password-hash", nil, nil, "active",
				connection.connector.now, connection.connector.now,
			}}
		}
		return rows, nil
	}
	if strings.Contains(query, "FROM admin_sessions") {
		rows := &authSessionRows{columns: []string{"id", "admin_username", "csrf_token_hash", "credential_fingerprint", "expires_at"}}
		if subtle.ConstantTimeCompare(provided, connection.connector.adminHash[:]) == 1 {
			rows.values = [][]driver.Value{{
				"01K00000000000000000000003", "admin", connection.connector.csrfHash[:], []byte("fingerprint"), connection.connector.now.Add(time.Hour),
			}}
		}
		return rows, nil
	}
	return nil, errors.New("unexpected auth session query")
}

type authSessionRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (rows *authSessionRows) Columns() []string { return rows.columns }
func (*authSessionRows) Close() error           { return nil }
func (rows *authSessionRows) Next(dest []driver.Value) error {
	if rows.index >= len(rows.values) {
		return io.EOF
	}
	copy(dest, rows.values[rows.index])
	rows.index++
	return nil
}

func TestNormalizeUserID(t *testing.T) {
	userID, err := normalizeUserID("  Marc_Game-01 ")
	if err != nil {
		t.Fatal(err)
	}
	if userID != "marc_game-01" {
		t.Fatalf("unexpected normalized user id %q", userID)
	}

	for _, input := range []string{"", "ab", "1marc", "marc@example.com", "marc space", "管理员", "admin"} {
		if _, err := normalizeUserID(input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}

func TestNormalizeNickname(t *testing.T) {
	nickname, err := normalizeNickname("  Marc  ")
	if err != nil || !nickname.Valid || nickname.String != "Marc" {
		t.Fatalf("unexpected nickname: %#v err=%v", nickname, err)
	}

	empty, err := normalizeNickname("   ")
	if err != nil || empty.Valid {
		t.Fatalf("empty nickname should become NULL: %#v err=%v", empty, err)
	}
}

func TestUserDTOExposesLoginIDWithoutLegacyEmailFields(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	dto := userDTO(User{
		ID:        "01K00000000000000000000000",
		LoginID:   "creator_01",
		Nickname:  sql.NullString{String: "Marc", Valid: true},
		CreatedAt: now,
		UpdatedAt: now,
	})

	if dto["userId"] != "creator_01" {
		t.Fatalf("unexpected user id: %#v", dto["userId"])
	}
	for _, forbidden := range []string{"email", "emailVerified"} {
		if _, exists := dto[forbidden]; exists {
			t.Fatalf("creator DTO must not contain %q", forbidden)
		}
	}
}

func TestNormalizeOrigin(t *testing.T) {
	origin, err := normalizeOrigin("https://Example.COM/app/path")
	if err != nil {
		t.Fatal(err)
	}
	if origin != "https://example.com" {
		t.Fatalf("unexpected origin %q", origin)
	}

	for _, input := range []string{"", "javascript:alert(1)", "/relative"} {
		if _, err := normalizeOrigin(input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}
