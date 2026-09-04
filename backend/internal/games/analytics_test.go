package games

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/auth"
	"gamegen/backend/internal/imagemoderation"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/platform/contentcrypto"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func TestGameBusinessEventsMatchFrozenContract(t *testing.T) {
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, nil)
	handler := &Handler{analytics: recorder, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := httptest.NewRequest("POST", "/", nil).WithContext(context.WithValue(
		context.Background(), middleware.RequestIDKey, "01K00000000000000000000009",
	))
	common := analytics.RecordInput{
		Source: analytics.SourceAPI, ActorType: analytics.ActorCreator,
		CreatorID: "01K00000000000000000000001", GameID: "01K00000000000000000000002",
		GameVersionID: "01K00000000000000000000003",
	}
	created := common
	created.EventName = analytics.EventGameCreated
	created.Properties = eventProperties(map[string]any{"templateId": "memory-game"})
	handler.recordEvent(request, created)
	version := common
	version.EventName = analytics.EventGameVersionCreated
	version.Properties = eventProperties(map[string]any{"versionNumber": 2, "templateId": "memory-game"})
	handler.recordEvent(request, version)
	asset := common
	asset.EventName = analytics.EventAssetUploaded
	asset.Properties = eventProperties(map[string]any{"kind": "game_source", "mimeType": "image/png", "sizeBytes": int64(42)})
	handler.recordEvent(request, asset)

	inputs := recorder.RecordedInputs()
	if len(inputs) != 3 {
		t.Fatalf("recorded %d game events, want 3", len(inputs))
	}
	for _, input := range inputs {
		if _, err := analytics.ValidateRecordInput(input); err != nil {
			t.Errorf("invalid %s event: %v", input.EventName, err)
		}
	}
}

func TestCreateGameAndVersionEntriesRecordOnlySuccessfulTransactions(t *testing.T) {
	t.Run("game success, validation, database, and analytics failure", func(t *testing.T) {
		tests := []struct {
			name        string
			body        string
			dbErr       error
			recorderErr error
			wantStatus  int
			wantRecords int
		}{
			{"success", `{"title":"Memory","description":"","templateId":"love-journey","templateVersion":"1.0.0","sceneInputs":{"loveLetter":"hello"}}`, nil, nil, http.StatusCreated, 1},
			{"current four-digit password", `{"title":"Memory","description":"","templateId":"love-journey","templateVersion":"1.1.0","sceneInputs":{"loveLetter":"hello","letterPassword":"0820"}}`, nil, nil, http.StatusCreated, 1},
			{"validation", `{"title":"","description":"","templateId":"love-journey","templateVersion":"1.0.0","sceneInputs":{"loveLetter":"hello"}}`, nil, nil, http.StatusUnprocessableEntity, 0},
			{"database", `{"title":"Memory","description":"","templateId":"love-journey","templateVersion":"1.0.0","sceneInputs":{"loveLetter":"hello"}}`, errors.New("database unavailable"), nil, http.StatusInternalServerError, 0},
			{"analytics", `{"title":"Memory","description":"","templateId":"love-journey","templateVersion":"1.0.0","sceneInputs":{"loveLetter":"hello"}}`, nil, errors.New("analytics unavailable"), http.StatusCreated, 1},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				handler, recorder := newGameAnalyticsEntryHandler(t, test.recorderErr)
				dbCalls := 0
				handler.createGameRecord = func(_ context.Context, gameID, versionID, userID, _ string, _ sql.NullString, _ EncryptedInput, _ int, templateID, templateVersion string, now time.Time) (Game, Version, error) {
					dbCalls++
					if test.dbErr != nil {
						return Game{}, Version{}, test.dbErr
					}
					return Game{ID: gameID, UserID: userID, Status: "draft", CreatedAt: now, UpdatedAt: now}, Version{ID: versionID, GameID: gameID, VersionNumber: 1, Status: "draft", TemplateID: templateID, TemplateVersion: templateVersion, CreatedAt: now, UpdatedAt: now}, nil
				}
				request := httptest.NewRequest(http.MethodPost, "/games", bytes.NewBufferString(test.body))
				response := httptest.NewRecorder()
				handler.createGame(response, request)
				if response.Code != test.wantStatus || len(recorder.RecordedInputs()) != test.wantRecords {
					t.Fatalf("status=%d dbCalls=%d records=%d body=%s", response.Code, dbCalls, len(recorder.RecordedInputs()), response.Body.String())
				}
				if test.wantRecords == 1 && recorder.RecordedInputs()[0].EventName != analytics.EventGameCreated {
					t.Fatalf("event=%#v", recorder.RecordedInputs()[0])
				}
			})
		}
	})

	t.Run("version success and database failure", func(t *testing.T) {
		for _, dbErr := range []error{nil, errors.New("database unavailable")} {
			handler, recorder := newGameAnalyticsEntryHandler(t, nil)
			handler.getGameRecord = func(context.Context, string, string) (Game, error) {
				return Game{ID: "01K00000000000000000000002", CurrentVersionID: sql.NullString{String: "01K00000000000000000000003", Valid: true}}, nil
			}
			handler.getVersionRecord = func(context.Context, string, string, string) (Version, error) {
				return Version{ID: "01K00000000000000000000003", GameID: "01K00000000000000000000002", InputSchemaVersion: 2, TemplateID: "love-journey", TemplateVersion: "1.0.0"}, nil
			}
			handler.createVersionRecord = func(_ context.Context, versionID, _ string, gameID string, _ EncryptedInput, _ int, now time.Time) (Version, error) {
				if dbErr != nil {
					return Version{}, dbErr
				}
				return Version{ID: versionID, GameID: gameID, VersionNumber: 2, Status: "draft", TemplateID: "love-journey", TemplateVersion: "1.0.0", CreatedAt: now, UpdatedAt: now}, nil
			}
			request := requestWithGameID(httptest.NewRequest(http.MethodPost, "/games/game/versions", bytes.NewBufferString(`{"sceneInputs":{"loveLetter":"next"}}`)), "01K00000000000000000000002")
			response := httptest.NewRecorder()
			handler.createVersion(response, request)
			wantStatus, wantRecords := http.StatusCreated, 1
			if dbErr != nil {
				wantStatus, wantRecords = http.StatusInternalServerError, 0
			}
			if response.Code != wantStatus || len(recorder.RecordedInputs()) != wantRecords {
				t.Fatalf("dbErr=%v status=%d records=%d body=%s", dbErr, response.Code, len(recorder.RecordedInputs()), response.Body.String())
			}
		}
	})
}

func TestUploadAssetEntryRecordsOnlyAfterStorageAndDatabaseSuccess(t *testing.T) {
	tests := []struct {
		name        string
		storageErr  error
		dbErr       error
		recorderErr error
		wantStatus  int
		wantRecords int
	}{
		{"success", nil, nil, nil, http.StatusCreated, 1},
		{"storage failure", errors.New("storage unavailable"), nil, nil, http.StatusInternalServerError, 0},
		{"database failure", nil, errors.New("database unavailable"), nil, http.StatusInternalServerError, 0},
		{"analytics failure", nil, nil, errors.New("analytics unavailable"), http.StatusCreated, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, recorder := newGameAnalyticsEntryHandler(t, test.recorderErr)
			storageCalls, dbCalls := 0, 0
			handler.putAssetFile = func(context.Context, string, string, string, string) (int64, error) {
				storageCalls++
				return 68, test.storageErr
			}
			handler.getVersionRecord = func(context.Context, string, string, string) (Version, error) { return Version{}, nil }
			handler.addAssetRecord = func(context.Context, string, string, string, Asset, bool, int) ([]ObjectRef, error) {
				dbCalls++
				return nil, test.dbErr
			}
			handler.removeAssetObject = func(context.Context, string, string) error { return nil }
			handler.presignAsset = func(context.Context, string, string, time.Duration) (*url.URL, error) {
				return url.Parse("https://assets.example/preview.png")
			}
			request := requestWithGameVersionIDs(newPNGUploadRequest(t), "01K00000000000000000000002", "01K00000000000000000000003")
			response := httptest.NewRecorder()
			handler.uploadAsset(response, request)
			if response.Code != test.wantStatus || len(recorder.RecordedInputs()) != test.wantRecords {
				t.Fatalf("status=%d storage=%d db=%d records=%d body=%s", response.Code, storageCalls, dbCalls, len(recorder.RecordedInputs()), response.Body.String())
			}
			if test.storageErr != nil && dbCalls != 0 {
				t.Fatal("database called after storage failure")
			}
		})
	}
}

func newGameAnalyticsEntryHandler(t *testing.T, recorderErr error) (*Handler, *analytics.FakeRecorder) {
	t.Helper()
	cipher, err := contentcrypto.New(base64.StdEncoding.EncodeToString(make([]byte, 32)), 1)
	if err != nil {
		t.Fatal(err)
	}
	recorder := analytics.NewFakeRecorder(analytics.RecordResult{}, recorderErr)
	handler := NewHandler(nil, nil, cipher, nil, recorder, config.Config{Uploads: config.UploadConfig{StreamBufferBytes: 1024, MaxConcurrentPerUser: 1}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler.imageReviewer = imagemoderation.DevelopmentReviewer{}
	handler.creatorUser = func(*http.Request) auth.User { return auth.User{ID: "01K00000000000000000000001"} }
	return handler, recorder
}

func requestWithGameID(request *http.Request, gameID string) *http.Request {
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("gameId", gameID)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, ctx))
}

func requestWithGameVersionIDs(request *http.Request, gameID, versionID string) *http.Request {
	request = requestWithGameID(request, gameID)
	chi.RouteContext(request.Context()).URLParams.Add("versionId", versionID)
	return request
}

func newPNGUploadRequest(t *testing.T) *http.Request {
	t.Helper()
	var pngData bytes.Buffer
	imageData := image.NewRGBA(image.Rect(0, 0, 2, 2))
	imageData.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&pngData, imageData); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "private-name.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(pngData.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}
