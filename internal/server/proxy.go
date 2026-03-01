package server

import (
	"io"
	"net/http"
	"strings"
)

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

		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}

		connectPath := strings.TrimPrefix(r.URL.Path, "/proxy")
		targetURL := p.targetURL + connectPath
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for key, values := range r.Header {
			for _, v := range values {
				proxyReq.Header.Add(key, v)
			}
		}

		resp, err := p.client.Do(proxyReq)
		if err != nil {
			http.Error(w, "proxy error: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for key, values := range resp.Header {
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}

		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}
