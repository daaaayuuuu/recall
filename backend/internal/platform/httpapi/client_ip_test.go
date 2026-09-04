package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestClientAddressIgnoresProxyHeadersByDefault(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "10.0.0.8:4321"
	request.Header.Set("X-Forwarded-For", "203.0.113.9")

	if got := ClientAddress(request, false); got != "10.0.0.8" {
		t.Fatalf("expected peer address, got %q", got)
	}
}

func TestClientAddressUsesTrustedForwardedAddress(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "10.0.0.8:4321"
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.4")

	if got := ClientAddress(request, true); got != "203.0.113.9" {
		t.Fatalf("expected original forwarded address, got %q", got)
	}
}

func TestClientAddressSkipsMalformedForwardedValues(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "[2001:db8::2]:4321"
	request.Header.Set("X-Forwarded-For", "unknown")
	request.Header.Set("X-Real-IP", "2001:db8::1")

	if got := ClientAddress(request, true); got != "2001:db8::1" {
		t.Fatalf("expected X-Real-IP fallback, got %q", got)
	}
}
