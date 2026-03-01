package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dorayaki-World/connectview/internal/compiler"
	"github.com/Dorayaki-World/connectview/internal/server"
)

func TestServeMode_Integration(t *testing.T) {
	protoDir := testdataDir()

	files, err := compiler.FindProtoFiles(protoDir)
	if err != nil {
		t.Fatalf("FindProtoFiles failed: %v", err)
	}

	root, err := compiler.Compile([]string{protoDir}, files)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	srv := server.New("http://localhost:8080", root)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	t.Run("index_html", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/")
		if err != nil {
			t.Fatalf("GET / failed: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		html := string(body)

		if !strings.Contains(html, "<!DOCTYPE html>") {
			t.Error("not valid HTML")
		}
		if !strings.Contains(html, "GreetService") {
			t.Error("missing GreetService")
		}
		if !strings.Contains(html, "__CONNECTVIEW_SERVE_MODE__") {
			t.Error("missing serve mode flag")
		}
	})

	t.Run("schema_json", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/schema.json")
		if err != nil {
			t.Fatalf("GET /schema.json failed: %v", err)
		}
		defer resp.Body.Close()

		var data map[string]any
		json.NewDecoder(resp.Body).Decode(&data)
		if _, ok := data["services"]; !ok {
			t.Error("schema missing services")
		}
	})

	t.Run("proxy", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"greeting":"hello"}`))
		}))
		defer target.Close()

		proxySrv := server.New(target.URL, root)
		proxyTS := httptest.NewServer(proxySrv.Handler())
		defer proxyTS.Close()

		resp, err := http.Post(
			proxyTS.URL+"/proxy/connectrpc.greet.v1.GreetService/Greet",
			"application/json",
			strings.NewReader(`{"name":"test"}`),
		)
		if err != nil {
			t.Fatalf("proxy request failed: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "hello") {
			t.Errorf("unexpected proxy response: %s", body)
		}
	})
}
