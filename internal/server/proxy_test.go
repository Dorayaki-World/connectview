package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dorayaki-World/connectview/internal/server"
)

func TestProxy_ForwardsRequest(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connectrpc.greet.v1.GreetService/Greet" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Connect-Protocol-Version") != "1" {
			t.Errorf("missing Connect-Protocol-Version header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"greeting":"hello"}`))
	}))
	defer target.Close()

	proxy := server.NewProxy(target.URL)
	handler := proxy.Handler()

	req := httptest.NewRequest("POST", "/proxy/connectrpc.greet.v1.GreetService/Greet",
		strings.NewReader(`{"name":"world"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body, _ := io.ReadAll(w.Result().Body)
	if !strings.Contains(string(body), "hello") {
		t.Errorf("unexpected body: %s", string(body))
	}
}

func TestProxy_CORS_Preflight(t *testing.T) {
	proxy := server.NewProxy("http://localhost:8080")
	handler := proxy.Handler()

	req := httptest.NewRequest("OPTIONS", "/proxy/svc/Method", nil)
	req.Header.Set("Origin", "http://localhost:9000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS allow-origin header")
	}
	if w.Code != 204 {
		t.Errorf("expected 204 for preflight, got %d", w.Code)
	}
}

func TestProxy_ForwardsCustomHeaders(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization header not forwarded")
		}
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer target.Close()

	proxy := server.NewProxy(target.URL)
	handler := proxy.Handler()

	req := httptest.NewRequest("POST", "/proxy/svc/Method", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
