package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"github.com/Dorayaki-World/connectview/internal/server"
)

func testRoot() *ir.Root {
	return &ir.Root{
		Services: []*ir.Service{
			{
				Name:            "TestService",
				FullName:        "test.v1.TestService",
				ConnectBasePath: "/test.v1.TestService/",
				RPCs: []*ir.RPC{
					{
						Name:        "DoThing",
						ConnectPath: "/test.v1.TestService/DoThing",
						HTTPMethod:  "POST",
						Request:     &ir.MessageRef{TypeName: ".test.v1.Req", Resolved: &ir.Message{Name: "Req", Fields: []*ir.Field{}}},
						Response:    &ir.MessageRef{TypeName: ".test.v1.Resp", Resolved: &ir.Message{Name: "Resp", Fields: []*ir.Field{}}},
					},
				},
			},
		},
		Messages: map[string]*ir.Message{},
		Enums:    map[string]*ir.Enum{},
	}
}

func TestServer_ServesHTML(t *testing.T) {
	srv := server.New("http://localhost:8080", testRoot())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("response is not HTML")
	}
	if !strings.Contains(html, "TestService") {
		t.Error("HTML doesn't contain service name")
	}
	if !strings.Contains(html, "__CONNECTVIEW_SCHEMA__") {
		t.Error("HTML doesn't contain schema")
	}
	if !strings.Contains(html, "__CONNECTVIEW_SERVE_MODE__") {
		t.Error("HTML doesn't contain serve mode flag")
	}
}

func TestServer_ServesSchemaJSON(t *testing.T) {
	srv := server.New("http://localhost:8080", testRoot())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/schema.json")
	if err != nil {
		t.Fatalf("GET /schema.json failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %s", ct)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := data["services"]; !ok {
		t.Error("schema JSON missing 'services' field")
	}
}

func TestServer_SSEEndpoint(t *testing.T) {
	srv := server.New("http://localhost:8080", testRoot())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/events")
	if err != nil {
		t.Fatalf("GET /events failed: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}
}

func TestServer_UpdateSchema(t *testing.T) {
	srv := server.New("http://localhost:8080", testRoot())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	newRoot := &ir.Root{
		Services: []*ir.Service{
			{Name: "UpdatedService", FullName: "updated.v1.UpdatedService"},
		},
		Messages: map[string]*ir.Message{},
		Enums:    map[string]*ir.Enum{},
	}
	srv.UpdateSchema(newRoot)

	resp, _ := http.Get(ts.URL + "/schema.json")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "UpdatedService") {
		t.Error("schema.json didn't reflect updated schema")
	}
}
