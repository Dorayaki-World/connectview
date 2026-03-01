# serveモード Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `connectview serve` — a local HTTP server that dynamically generates ConnectRPC documentation from .proto files with a CORS proxy and hot reload.

**Architecture:** Uses `protocompile` to parse .proto files directly (no protoc needed), bridges to the existing parser via a synthetic `CodeGeneratorRequest` → `protogen.Plugin`, then reuses the existing resolver/renderer pipeline. HTTP server uses stdlib `net/http`. File watching via `fsnotify` with SSE push to browsers.

**Tech Stack:** Go 1.26, `github.com/bufbuild/protocompile`, `github.com/fsnotify/fsnotify`, `net/http`, SSE

---

### Task 1: Add Dependencies

**Files:**
- Modify: `go.mod`

**Step 1: Add protocompile and fsnotify**

```bash
cd /Users/abe/git/oss/connectview
go get github.com/bufbuild/protocompile
go get github.com/fsnotify/fsnotify
```

**Step 2: Verify build**

```bash
go build ./...
```
Expected: SUCCESS

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add protocompile and fsnotify dependencies"
```

---

### Task 2: Implement Compiler (protocompile → ir.Root)

The compiler bridges .proto files on disk to the existing parser pipeline. It uses protocompile to parse .proto files into `protoreflect.FileDescriptor`, converts them to `descriptorpb.FileDescriptorProto`, builds a synthetic `CodeGeneratorRequest`, creates a `protogen.Plugin`, and calls the existing `parser.Parse()`.

**Files:**
- Create: `internal/compiler/compiler.go`
- Create: `internal/compiler/compiler_test.go`

**Step 1: Write compiler tests**

Tests use the testdata proto files already in the repo.

```go
// internal/compiler/compiler_test.go
package compiler_test

import (
	"testing"

	"github.com/Dorayaki-World/connectview/internal/compiler"
)

func TestCompile_BasicProto(t *testing.T) {
	root, err := compiler.Compile([]string{"../../testdata/proto"}, []string{
		"greet/v1/greet.proto",
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(root.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(root.Services))
	}
	if root.Services[0].Name != "GreetService" {
		t.Errorf("expected GreetService, got %s", root.Services[0].Name)
	}
	if len(root.Services[0].RPCs) != 2 {
		t.Errorf("expected 2 RPCs, got %d", len(root.Services[0].RPCs))
	}

	// Verify messages were parsed
	if _, ok := root.Messages[".connectrpc.greet.v1.GreetRequest"]; !ok {
		t.Error("GreetRequest message not found")
	}
}

func TestCompile_MultipleFiles(t *testing.T) {
	root, err := compiler.Compile([]string{"../../testdata/proto"}, []string{
		"greet/v1/greet.proto",
		"user/v1/user.proto",
	})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if len(root.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(root.Services))
	}
}

func TestCompile_InvalidPath(t *testing.T) {
	_, err := compiler.Compile([]string{"../../testdata/proto"}, []string{
		"nonexistent/v1/nope.proto",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent proto file")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/compiler/ -v
```
Expected: FAIL

**Step 3: Implement compiler**

```go
// internal/compiler/compiler.go
package compiler

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"github.com/Dorayaki-World/connectview/internal/parser"
	"github.com/Dorayaki-World/connectview/internal/resolver"
	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/reporter"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	pluginpb "google.golang.org/protobuf/types/pluginpb"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Compile parses .proto files from the given import paths and returns a resolved IR.
// importPaths are directories to search for .proto files.
// files are the .proto file paths relative to the import paths.
func Compile(importPaths []string, files []string) (*ir.Root, error) {
	// Use protocompile to parse
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(
			&protocompile.SourceResolver{
				ImportPaths: importPaths,
			},
		),
		Reporter: reporter.NewReporter(
			func(err reporter.ErrorWithPos) error { return err },
			nil, // no warnings handler
		),
	}

	ctx := context.Background()
	compiled, err := compiler.Compile(ctx, files...)
	if err != nil {
		return nil, fmt.Errorf("proto compilation failed: %w", err)
	}

	// Convert compiled files to FileDescriptorProto
	var fdProtos []*descriptorpb.FileDescriptorProto
	fileNames := make([]string, 0, len(files))

	// Collect all file descriptors (compiled files + their dependencies)
	for _, f := range compiled.Files {
		fdProto := protodesc.ToFileDescriptorProto(f)
		fdProtos = append(fdProtos, fdProto)
	}

	// The files we want to generate are the explicitly requested ones
	fileNames = append(fileNames, files...)

	// Build a synthetic CodeGeneratorRequest
	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: fileNames,
		ProtoFile:      fdProtos,
	}

	// Create protogen.Plugin to reuse existing parser
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		return nil, fmt.Errorf("protogen plugin creation failed: %w", err)
	}

	// Reuse existing parser and resolver
	root := parser.Parse(plugin)

	r := resolver.New(root)
	if err := r.Resolve(); err != nil {
		return nil, fmt.Errorf("resolve failed: %w", err)
	}

	return root, nil
}

// FindProtoFiles walks the given directory and returns all .proto file paths
// relative to the directory.
func FindProtoFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".proto") {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}
```

Note: The exact protocompile API usage may need adjustment based on the actual library version. The key insight is:
1. protocompile.Compiler parses .proto files
2. Convert results to descriptorpb.FileDescriptorProto
3. Build CodeGeneratorRequest
4. Use protogen.Options{}.New() to get a Plugin
5. Call parser.Parse(plugin) — full code reuse

**Step 4: Run tests**

```bash
go test ./internal/compiler/ -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/compiler/
git commit -m "feat: implement proto compiler using protocompile library"
```

---

### Task 3: Implement Reverse Proxy

**Files:**
- Create: `internal/server/proxy.go`
- Create: `internal/server/proxy_test.go`

**Step 1: Write proxy tests**

```go
// internal/server/proxy_test.go
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
	// Mock target ConnectRPC server
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify path
		if r.URL.Path != "/connectrpc.greet.v1.GreetService/Greet" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Connect-Protocol-Version") != "1" {
			t.Errorf("missing Connect-Protocol-Version header")
		}
		// Echo response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"greeting":"hello"}`))
	}))
	defer target.Close()

	proxy := server.NewProxy(target.URL)
	handler := proxy.Handler()

	// Simulate POST /proxy/connectrpc.greet.v1.GreetService/Greet
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
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/server/ -v -run TestProxy
```
Expected: FAIL

**Step 3: Implement proxy**

```go
// internal/server/proxy.go
package server

import (
	"io"
	"net/http"
	"strings"
)

// Proxy handles reverse proxying requests to a ConnectRPC target server.
type Proxy struct {
	targetURL string
	client    *http.Client
}

func NewProxy(targetURL string) *Proxy {
	return &Proxy{
		targetURL: strings.TrimRight(targetURL, "/"),
		client:    &http.Client{},
	}
}

func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS headers on all responses
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version, Authorization, Connect-Timeout-Ms")

		// Handle preflight
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}

		// Strip /proxy prefix to get the ConnectRPC path
		connectPath := strings.TrimPrefix(r.URL.Path, "/proxy")

		// Build target URL
		targetURL := p.targetURL + connectPath
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		// Create proxy request
		proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Forward headers
		for key, values := range r.Header {
			for _, v := range values {
				proxyReq.Header.Add(key, v)
			}
		}

		// Send to target
		resp, err := p.client.Do(proxyReq)
		if err != nil {
			http.Error(w, "proxy error: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Copy response headers
		for key, values := range resp.Header {
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}

		// Copy status and body
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}
```

**Step 4: Run tests**

```bash
go test ./internal/server/ -v -run TestProxy
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat: implement reverse proxy for ConnectRPC target server"
```

---

### Task 4: Implement File Watcher

**Files:**
- Create: `internal/server/watcher.go`
- Create: `internal/server/watcher_test.go`

**Step 1: Write watcher tests**

```go
// internal/server/watcher_test.go
package server_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dorayaki-World/connectview/internal/server"
)

func TestWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	protoFile := filepath.Join(dir, "test.proto")
	os.WriteFile(protoFile, []byte(`syntax = "proto3";`), 0644)

	var called atomic.Int32
	w, err := server.NewWatcher(dir, func() {
		called.Add(1)
	})
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	go w.Run()

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Modify the file
	os.WriteFile(protoFile, []byte(`syntax = "proto3"; // modified`), 0644)

	// Wait for debounce + callback
	time.Sleep(300 * time.Millisecond)

	if called.Load() == 0 {
		t.Error("expected onChange callback to be called")
	}
}

func TestWatcher_DebouncesBurstChanges(t *testing.T) {
	dir := t.TempDir()
	protoFile := filepath.Join(dir, "test.proto")
	os.WriteFile(protoFile, []byte(`syntax = "proto3";`), 0644)

	var callCount atomic.Int32
	w, err := server.NewWatcher(dir, func() {
		callCount.Add(1)
	})
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	go w.Run()
	time.Sleep(50 * time.Millisecond)

	// Rapid-fire changes
	for i := 0; i < 5; i++ {
		os.WriteFile(protoFile, []byte(fmt.Sprintf("// change %d", i)), 0644)
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(300 * time.Millisecond)

	count := callCount.Load()
	if count > 2 {
		t.Errorf("expected debounced calls (<=2), got %d", count)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/server/ -v -run TestWatcher
```
Expected: FAIL

**Step 3: Implement watcher**

```go
// internal/server/watcher.go
package server

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches a directory for .proto file changes and calls onChange.
type Watcher struct {
	watcher  *fsnotify.Watcher
	onChange func()
	done     chan struct{}
}

func NewWatcher(dir string, onChange func()) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Add directory and subdirectories
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			fw.Add(path)
		}
		return nil
	})

	return &Watcher{
		watcher:  fw,
		onChange: onChange,
		done:     make(chan struct{}),
	}, nil
}

func (w *Watcher) Run() {
	var debounceTimer *time.Timer
	debounceInterval := 100 * time.Millisecond

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if !strings.HasSuffix(event.Name, ".proto") {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) == 0 {
				continue
			}

			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceInterval, w.onChange)

		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
		case <-w.done:
			return
		}
	}
}

func (w *Watcher) Close() error {
	close(w.done)
	return w.watcher.Close()
}
```

**Step 4: Run tests**

```bash
go test ./internal/server/ -v -run TestWatcher
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/watcher.go internal/server/watcher_test.go
git commit -m "feat: implement file watcher with debounce for proto changes"
```

---

### Task 5: Implement HTTP Server with SSE

The server ties everything together: serves the HTML, schema JSON, SSE events, and proxy.

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/server_test.go`

**Step 1: Write server tests**

```go
// internal/server/server_test.go
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

	// Update schema
	newRoot := &ir.Root{
		Services: []*ir.Service{
			{Name: "UpdatedService", FullName: "updated.v1.UpdatedService"},
		},
		Messages: map[string]*ir.Message{},
		Enums:    map[string]*ir.Enum{},
	}
	srv.UpdateSchema(newRoot)

	// Verify /schema.json reflects the update
	resp, _ := http.Get(ts.URL + "/schema.json")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "UpdatedService") {
		t.Error("schema.json didn't reflect updated schema")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/server/ -v -run TestServer
```
Expected: FAIL

**Step 3: Implement server**

```go
// internal/server/server.go
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"github.com/Dorayaki-World/connectview/internal/renderer"
)

// Server is the connectview serve mode HTTP server.
type Server struct {
	targetURL string
	proxy     *Proxy
	renderer  *renderer.Renderer

	mu   sync.RWMutex
	root *ir.Root

	// SSE clients
	sseClients   map[chan string]struct{}
	sseClientsMu sync.Mutex
}

func New(targetURL string, root *ir.Root) *Server {
	return &Server{
		targetURL:  targetURL,
		proxy:      NewProxy(targetURL),
		renderer:   renderer.New(),
		root:       root,
		sseClients: make(map[chan string]struct{}),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /schema.json", s.handleSchema)
	mux.HandleFunc("GET /events", s.handleSSE)
	mux.Handle("/proxy/", s.proxy.Handler())
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	root := s.root
	s.mu.RUnlock()

	html, err := s.renderer.RenderServeMode(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	root := s.root
	s.mu.RUnlock()

	schema := renderer.BuildSchemaJSON(root)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schema)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan string, 1)
	s.sseClientsMu.Lock()
	s.sseClients[ch] = struct{}{}
	s.sseClientsMu.Unlock()

	defer func() {
		s.sseClientsMu.Lock()
		delete(s.sseClients, ch)
		s.sseClientsMu.Unlock()
	}()

	// Send initial keepalive
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// UpdateSchema replaces the current schema and notifies all SSE clients.
func (s *Server) UpdateSchema(root *ir.Root) {
	s.mu.Lock()
	s.root = root
	s.mu.Unlock()

	s.notifyClients(`{"type":"schema-updated"}`)
}

func (s *Server) notifyClients(msg string) {
	s.sseClientsMu.Lock()
	defer s.sseClientsMu.Unlock()

	for ch := range s.sseClients {
		select {
		case ch <- msg:
		default:
			// Client not keeping up, skip
		}
	}
}
```

Note: The server uses `renderer.RenderServeMode(root)` and `renderer.BuildSchemaJSON(root)`. These need to be exported from the renderer package. `RenderServeMode` is like `Render` but sets `__CONNECTVIEW_SERVE_MODE__ = true`. `BuildSchemaJSON` exposes the existing `buildSchema` function.

**Step 4: Update renderer to export needed functions**

Add to `internal/renderer/renderer.go`:
- `RenderServeMode(root) (string, error)` — like Render but with serve mode flag
- `BuildSchemaJSON(root) any` — exports buildSchema

**Step 5: Run tests**

```bash
go test ./internal/server/ -v -run TestServer
```
Expected: PASS

**Step 6: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go internal/renderer/renderer.go
git commit -m "feat: implement HTTP server with SSE for serve mode"
```

---

### Task 6: Update app.js for Serve Mode

Add SSE listener and serve-mode-aware base URL handling.

**Files:**
- Modify: `internal/renderer/assets/app.js`
- Modify: `internal/renderer/assets/index.html.tmpl`

**Step 1: Update HTML template**

Add serve mode flag to the template. The renderer will set this to `true` when in serve mode.

In `index.html.tmpl`, add before the schema script tag:

```html
<script>window.__CONNECTVIEW_SERVE_MODE__ = {{.ServeMode}};</script>
```

**Step 2: Add SSE and hot reload to app.js**

Add at the end of `init()` function:

```javascript
// Serve mode: SSE for hot reload
if (window.__CONNECTVIEW_SERVE_MODE__) {
  connectSSE();
}

function connectSSE() {
  var es = new EventSource("/events");
  es.onmessage = function(e) {
    try {
      var data = JSON.parse(e.data);
      if (data.type === "schema-updated") {
        reloadSchema();
      }
    } catch (err) {
      // ignore
    }
  };
  es.onerror = function() {
    // Reconnect after 3 seconds
    setTimeout(function() {
      es.close();
      connectSSE();
    }, 3000);
  };
}

function reloadSchema() {
  fetch("/schema.json")
    .then(function(resp) { return resp.json(); })
    .then(function(schema) {
      window.__CONNECTVIEW_SCHEMA__ = schema;
      // Re-render with new schema
      document.getElementById("app").innerHTML = "";
      init();
    })
    .catch(function(err) {
      console.error("Failed to reload schema:", err);
    });
}
```

Also update `getBaseURL()` to default to `/proxy` in serve mode:

```javascript
function getBaseURL() {
  if (window.__CONNECTVIEW_SERVE_MODE__) {
    return "/proxy";
  }
  var input = document.getElementById("base-url-input");
  var url = input ? input.value.trim() : "http://localhost:8080";
  return url.replace(/\/+$/, "");
}
```

**Step 3: Run renderer tests**

```bash
go test ./internal/renderer/ -v
```
Expected: PASS (existing tests should still pass)

**Step 4: Commit**

```bash
git add internal/renderer/assets/
git commit -m "feat: add SSE hot reload and serve mode support to frontend"
```

---

### Task 7: Wire Up CLI (serve subcommand)

Restructure `cmd/connectview/main.go` to support both `generate` (protoc plugin) and `serve` subcommands.

**Files:**
- Modify: `cmd/connectview/main.go`

**Step 1: Implement CLI routing**

When invoked by protoc, stdin is a binary CodeGeneratorRequest. When invoked with `serve`, the first arg is "serve". Detection strategy: check `os.Args[1] == "serve"`.

```go
// cmd/connectview/main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Dorayaki-World/connectview/internal/compiler"
	"github.com/Dorayaki-World/connectview/internal/parser"
	"github.com/Dorayaki-World/connectview/internal/renderer"
	"github.com/Dorayaki-World/connectview/internal/resolver"
	"github.com/Dorayaki-World/connectview/internal/server"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		runServe(os.Args[2:])
		return
	}
	runGenerate()
}

func runGenerate() {
	var flags flag.FlagSet
	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(plugin *protogen.Plugin) error {
		plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

		root := parser.Parse(plugin)
		r := resolver.New(root)
		if err := r.Resolve(); err != nil {
			return err
		}

		html, err := renderer.New().Render(root)
		if err != nil {
			return err
		}

		outFile := plugin.NewGeneratedFile("index.html", "")
		_, err = outFile.Write([]byte(html))
		return err
	})
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	protoDir := fs.String("proto", "", "proto file root directory (required)")
	target := fs.String("target", "", "ConnectRPC target URL (required)")
	port := fs.Int("port", 9000, "listen port")
	var importPaths multiString
	fs.Var(&importPaths, "I", "additional import paths (can be specified multiple times)")
	fs.Parse(args)

	if *protoDir == "" || *target == "" {
		fmt.Fprintln(os.Stderr, "Usage: connectview serve --proto DIR --target URL [--port PORT] [-I PATH]...")
		fs.PrintDefaults()
		os.Exit(1)
	}

	// Include proto dir as import path
	allImportPaths := append([]string{*protoDir}, importPaths...)

	// Initial compile
	files, err := compiler.FindProtoFiles(*protoDir)
	if err != nil {
		log.Fatalf("failed to find proto files: %v", err)
	}
	if len(files) == 0 {
		log.Fatalf("no .proto files found in %s", *protoDir)
	}

	root, err := compiler.Compile(allImportPaths, files)
	if err != nil {
		log.Fatalf("initial compilation failed: %v", err)
	}

	// Create server
	srv := server.New(*target, root)

	// Start file watcher
	watcher, err := server.NewWatcher(*protoDir, func() {
		log.Println("proto files changed, recompiling...")
		newFiles, err := compiler.FindProtoFiles(*protoDir)
		if err != nil {
			log.Printf("find proto files: %v", err)
			return
		}
		newRoot, err := compiler.Compile(allImportPaths, newFiles)
		if err != nil {
			log.Printf("recompilation failed: %v", err)
			return
		}
		srv.UpdateSchema(newRoot)
		log.Println("schema updated")
	})
	if err != nil {
		log.Fatalf("failed to start watcher: %v", err)
	}
	defer watcher.Close()
	go watcher.Run()

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("connectview serve listening on http://localhost%s", addr)
	log.Printf("  proto:  %s", *protoDir)
	log.Printf("  target: %s", *target)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// multiString implements flag.Value for repeated -I flags.
type multiString []string

func (m *multiString) String() string { return fmt.Sprint(*m) }
func (m *multiString) Set(val string) error {
	*m = append(*m, val)
	return nil
}
```

**Step 2: Build and verify**

```bash
go build ./cmd/connectview
./connectview serve --help
```
Expected: prints usage with --proto, --target, --port, -I flags

**Step 3: Manual integration test**

If a ConnectRPC server is available:
```bash
./connectview serve --proto testdata/proto --target http://localhost:8080
```
Then open http://localhost:9000 in browser.

**Step 4: Commit**

```bash
git add cmd/connectview/main.go
git commit -m "feat: wire up serve subcommand with compiler, watcher, and server"
```

---

### Task 8: E2E Test for Serve Mode

**Files:**
- Create: `e2e/serve_test.go`

**Step 1: Write E2E test**

```go
// e2e/serve_test.go
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

	// Compile test protos
	files, err := compiler.FindProtoFiles(protoDir)
	if err != nil {
		t.Fatalf("FindProtoFiles failed: %v", err)
	}

	root, err := compiler.Compile([]string{protoDir}, files)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// Create server
	srv := server.New("http://localhost:8080", root)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Test GET /
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

	// Test GET /schema.json
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

	// Test proxy (with mock target)
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
```

**Step 2: Run test**

```bash
go test ./e2e/ -v -run TestServeMode
```
Expected: PASS

**Step 3: Commit**

```bash
git add e2e/serve_test.go
git commit -m "test: add serve mode E2E integration tests"
```

---

### Task 9: Update Design Doc

**Files:**
- Modify: `docs/design.md`

Update Section 2.1 (動作モード) to reflect the actual serve mode implementation:
- Default port 9000 (not 3000)
- SSE for browser notification (not WebSocket)
- protocompile for proto parsing (not protoc)
- Remove `internal/plugin/` from architecture
- Add `internal/compiler/` to architecture

**Step 1: Update design doc**

**Step 2: Commit**

```bash
git add docs/design.md
git commit -m "docs: update design doc for serve mode implementation"
```
