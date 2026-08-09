package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxyUsesFirstUntrustedAddress(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "172.18.0.2:4321"
	request.Header.Set("X-Forwarded-For", "198.51.100.25, 172.18.0.3")

	handler := trustedProxy([]string{"172.16.0.0/12"})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "198.51.100.25" {
			t.Fatalf("RemoteAddr = %q", r.RemoteAddr)
		}
		if r.Header.Get("X-Forwarded-For") != "" {
			t.Fatal("forwarded headers were not removed")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), request)
}

func TestTrustedProxyIgnoresHeadersFromUntrustedPeer(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "203.0.113.10:4321"
	request.Header.Set("X-Forwarded-For", "198.51.100.25")

	handler := trustedProxy([]string{"172.16.0.0/12"})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "203.0.113.10" {
			t.Fatalf("RemoteAddr = %q", r.RemoteAddr)
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), request)
}
