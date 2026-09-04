package httpapi

import (
	"net"
	"net/http"
	"strings"
)

// ClientAddress returns the address used for per-client security controls.
// Proxy headers are considered only when the service is explicitly configured
// to trust its upstream proxy.
func ClientAddress(request *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		for _, candidate := range strings.Split(request.Header.Get("X-Forwarded-For"), ",") {
			if address := normalizedIP(candidate); address != "" {
				return address
			}
		}
		if address := normalizedIP(request.Header.Get("X-Real-IP")); address != "" {
			return address
		}
	}
	if address := normalizedIP(request.RemoteAddr); address != "" {
		return address
	}
	return request.RemoteAddr
}

func normalizedIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	host := value
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}
