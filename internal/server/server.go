package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/Dorayaki-World/connectview/internal/ir"
	"github.com/Dorayaki-World/connectview/internal/renderer"
)

type Server struct {
	targetURL string
	proxy     *Proxy
	renderer  *renderer.Renderer

	mu   sync.RWMutex
	root *ir.Root

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
	mux.HandleFunc("GET /schema.json", s.handleSchema)
	mux.HandleFunc("GET /events", s.handleSSE)
	mux.Handle("/proxy/", s.proxy.Handler())
	// Register the index handler as a fallback that wraps the mux.
	// This avoids a Go 1.22+ ServeMux conflict between "GET /" and "/proxy/".
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Let the mux handle known routes first.
		if r.URL.Path == "/schema.json" || r.URL.Path == "/events" || strings.HasPrefix(r.URL.Path, "/proxy/") {
			mux.ServeHTTP(w, r)
			return
		}
		// Everything else serves the index HTML.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleIndex(w, r)
	})
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
		}
	}
}
