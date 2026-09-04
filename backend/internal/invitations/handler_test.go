package invitations

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type fakeInvitationStore struct {
	createCalls int
	createErrs  []error
	created     Invitation
	codeHash    []byte
	items       []Invitation
	revoked     Invitation
	revokeErr   error
}

func (store *fakeInvitationStore) Create(_ context.Context, invitation Invitation, hash []byte, _, _, _ string) error {
	store.createCalls++
	store.created = invitation
	store.codeHash = append([]byte(nil), hash...)
	if len(store.createErrs) >= store.createCalls {
		return store.createErrs[store.createCalls-1]
	}
	return nil
}

func (store *fakeInvitationStore) List(context.Context, int) ([]Invitation, error) {
	return store.items, nil
}

func (store *fakeInvitationStore) Revoke(context.Context, string, string, string, string, string, time.Time) (Invitation, error) {
	return store.revoked, store.revokeErr
}

func TestCreateReturnsFullCodeOnceAndPersistsOnlyHashAndSuffix(t *testing.T) {
	store := &fakeInvitationStore{}
	handler := testHandler(store)
	handler.generateCode = func() (string, error) { return "7KDM-N4PX", nil }

	request := httptest.NewRequest(http.MethodPost, "/admin/invitation-codes", nil).WithContext(
		context.WithValue(context.Background(), middleware.RequestIDKey, "01K00000000000000000000009"),
	)
	response := httptest.NewRecorder()
	handler.create(response, request)

	if response.Code != http.StatusCreated || store.createCalls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, store.createCalls, response.Body.String())
	}
	if store.created.CodeSuffix != "N4PX" || len(store.codeHash) != 32 {
		t.Fatalf("stored invitation=%#v hash length=%d", store.created, len(store.codeHash))
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data["code"] != "7KDM-N4PX" || envelope.Data["codeHint"] != "••••-N4PX" {
		t.Fatalf("response data=%#v", envelope.Data)
	}
}

func TestCreateRetriesHashCollision(t *testing.T) {
	store := &fakeInvitationStore{createErrs: []error{ErrCodeCollision, nil}}
	handler := testHandler(store)
	codes := []string{"7KDM-N4PX", "8KDM-N4PX"}
	handler.generateCode = func() (string, error) {
		code := codes[0]
		codes = codes[1:]
		return code, nil
	}

	response := httptest.NewRecorder()
	handler.create(response, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusCreated || store.createCalls != 2 {
		t.Fatalf("status=%d calls=%d", response.Code, store.createCalls)
	}
}

func TestListNeverReturnsFullInvitationCode(t *testing.T) {
	store := &fakeInvitationStore{items: []Invitation{{
		ID: "01K00000000000000000000001", CodeSuffix: "N4PX", CreatedByAdmin: "admin",
		CreatedAt: time.Date(2026, 8, 19, 1, 0, 0, 0, time.UTC),
	}}}
	handler := testHandler(store)
	response := httptest.NewRecorder()
	handler.list(response, httptest.NewRequest(http.MethodGet, "/?limit=50", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, "••••-N4PX") || strings.Contains(body, "7KDM-N4PX") || strings.Contains(body, "codeHash") {
		t.Fatalf("unsafe list response: %s", body)
	}
}

func testHandler(store invitationStore) *Handler {
	return NewHandler(store, func(*http.Request) (string, string) {
		return "01K00000000000000000000001", "admin"
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}
