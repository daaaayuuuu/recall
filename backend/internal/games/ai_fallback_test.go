package games

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPolishLoveLetterKeepsOriginalWhenNoKeyConfigured(t *testing.T) {
	handler, _ := newGameAnalyticsEntryHandler(t, nil)
	request := httptest.NewRequest(http.MethodPost, "/games/polish-love-letter", bytes.NewBufferString(`{"text":"  原文内容  "}`))
	response := httptest.NewRecorder()

	handler.polishLoveLetter(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"polishedText":"原文内容"`) || !strings.Contains(body, `"skipped":true`) {
		t.Fatalf("unexpected response body: %s", body)
	}
}
