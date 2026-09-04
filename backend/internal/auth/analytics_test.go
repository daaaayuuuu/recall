package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/invitations"
	"gamegen/backend/internal/platform/config"

	"github.com/go-chi/chi/v5/middleware"
)

func TestAuthBusinessEventsUseTrustedIdentifiersAndAreBestEffort(t *testing.T) {
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, errors.New("secret analytics backend detail"))
	var logs bytes.Buffer
	handler := &Handler{analytics: recorder, logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	request := httptest.NewRequest("POST", "/", nil).WithContext(context.WithValue(
		context.Background(), middleware.RequestIDKey, "01K00000000000000000000009",
	))

	handler.recordEvent(request, analytics.RecordInput{
		EventName: analytics.EventCreatorRegistered, Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
		CreatorID: "01K00000000000000000000001", Properties: json.RawMessage(`{}`),
	})
	handler.recordEvent(request, analytics.RecordInput{
		EventName: analytics.EventCreatorLoggedIn, Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
		CreatorID: "01K00000000000000000000001", UserSessionID: "01K00000000000000000000002", Properties: json.RawMessage(`{}`),
	})

	inputs := recorder.RecordedInputs()
	if len(inputs) != 2 {
		t.Fatalf("recorded %d auth events, want 2", len(inputs))
	}
	for _, input := range inputs {
		if _, err := analytics.ValidateRecordInput(input); err != nil {
			t.Errorf("invalid %s event: %v", input.EventName, err)
		}
		if input.RequestID != "01K00000000000000000000009" {
			t.Errorf("request id = %q", input.RequestID)
		}
	}
	logOutput := logs.String()
	if strings.Contains(logOutput, "secret analytics backend detail") || strings.Contains(logOutput, "login_id") {
		t.Fatalf("analytics warning leaked details: %s", logOutput)
	}
}

func TestRegisterEntryRecordsOnlyCommittedRegistration(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		createErr   error
		recorderErr error
		wantStatus  int
		wantRecords int
		wantDBCalls int
	}{
		{"success", `{"invitationCode":"7KDM-N4PX","userId":"creator_01","password":"password-123","nickname":"Creator"}`, nil, nil, http.StatusCreated, 1, 1},
		{"validation failure", `{"invitationCode":"7KDM-N4PX","userId":"bad id","password":"password-123","nickname":"Creator"}`, nil, nil, http.StatusUnprocessableEntity, 0, 0},
		{"invalid invitation", `{"invitationCode":"7KDM-N4PX","userId":"creator_01","password":"password-123","nickname":"Creator"}`, invitations.ErrInvalidOrUsed, nil, http.StatusUnprocessableEntity, 0, 1},
		{"database failure", `{"invitationCode":"7KDM-N4PX","userId":"creator_01","password":"password-123","nickname":"Creator"}`, errors.New("database unavailable"), nil, http.StatusInternalServerError, 0, 1},
		{"analytics failure", `{"invitationCode":"7KDM-N4PX","userId":"creator_01","password":"password-123","nickname":"Creator"}`, nil, errors.New("analytics unavailable"), http.StatusCreated, 1, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, test.recorderErr)
			handler := newAuthAnalyticsEntryHandler(recorder)
			dbCalls := 0
			handler.createUserWithInvite = func(_ context.Context, _ User, hash []byte) error {
				dbCalls++
				if len(hash) != 32 {
					t.Fatalf("invitation hash length = %d", len(hash))
				}
				return test.createErr
			}
			request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			handler.register(response, request)
			if response.Code != test.wantStatus || dbCalls != test.wantDBCalls || len(recorder.RecordedInputs()) != test.wantRecords {
				t.Fatalf("status=%d dbCalls=%d records=%d body=%s", response.Code, dbCalls, len(recorder.RecordedInputs()), response.Body.String())
			}
			if test.wantRecords == 1 {
				input := recorder.RecordedInputs()[0]
				if input.EventName != analytics.EventCreatorRegistered || input.CreatorID == "" {
					t.Fatalf("registration event = %#v", input)
				}
			}
		})
	}
}

func TestLoginEntryRecordsOnlyCreatedSession(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		sessionErr  error
		recorderErr error
		wantStatus  int
		wantRecords int
	}{
		{"success", "password-123", nil, nil, http.StatusOK, 1},
		{"invalid password", "wrong-password", nil, nil, http.StatusUnauthorized, 0},
		{"session database failure", "password-123", errors.New("database unavailable"), nil, http.StatusInternalServerError, 0},
		{"analytics failure", "password-123", nil, errors.New("analytics unavailable"), http.StatusOK, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, test.recorderErr)
			handler := newAuthAnalyticsEntryHandler(recorder)
			passwordHash, err := handler.passwords.Hash("password-123")
			if err != nil {
				t.Fatal(err)
			}
			handler.findUserByLoginID = func(context.Context, string) (User, error) {
				return User{ID: "01K00000000000000000000001", LoginID: "creator_01", PasswordHash: passwordHash, Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
			}
			sessionCalls := 0
			handler.createUserSession = func(_ context.Context, _ string, _ string, _ []byte, _ []byte, _ time.Time, _ time.Time) error {
				sessionCalls++
				return test.sessionErr
			}
			request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"userId":"creator_01","password":"`+test.password+`"}`))
			response := httptest.NewRecorder()
			handler.login(response, request)
			if response.Code != test.wantStatus || len(recorder.RecordedInputs()) != test.wantRecords {
				t.Fatalf("status=%d records=%d body=%s", response.Code, len(recorder.RecordedInputs()), response.Body.String())
			}
			if test.wantRecords == 1 {
				input := recorder.RecordedInputs()[0]
				if input.EventName != analytics.EventCreatorLoggedIn || input.UserSessionID == "" || sessionCalls != 1 {
					t.Fatalf("login event=%#v sessionCalls=%d", input, sessionCalls)
				}
			}
		})
	}
}

func newAuthAnalyticsEntryHandler(recorder analytics.Recorder) *Handler {
	return NewHandler(nil, nil, recorder, config.Config{
		App:     config.AppConfig{Environment: "test", AppBaseURL: "https://app.example"},
		Uploads: config.UploadConfig{StreamBufferBytes: 1024},
	}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
}
