package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/groundtruth"
	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/mockprovider"
)

type Config struct {
	Alias    string
	Upstream string
}

type Router struct {
	mu       sync.RWMutex
	config   Config
	layers   groundtruth.Layers
	upstream http.Handler
	backend  *mockprovider.Server
}

func New(config Config, upstream http.Handler, backend *mockprovider.Server) (*Router, error) {
	if config.Alias == "" {
		return nil, errors.New("router alias cannot be empty")
	}
	if upstream == nil {
		return nil, errors.New("router upstream cannot be nil")
	}
	return &Router{config: config, upstream: upstream, backend: backend}, nil
}

func (r *Router) SetLayers(layers groundtruth.Layers) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.layers = layers
	if r.backend != nil {
		r.backend.SetLayers(mockprovider.Layers{
			Model: layers.Model, Safety: layers.Safety,
			RequestConfig: layers.RequestConfig, Serving: layers.Serving,
		})
	}
}

func (r *Router) Layers() groundtruth.Layers {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.layers
}

func (r *Router) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, req)
			return
		}
		var payload struct {
			Model string `json:"model"`
		}
		body, err := io.ReadAll(io.LimitReader(req.Body, 4<<20))
		if err != nil {
			http.Error(w, "read request: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &payload); err != nil || payload.Model != r.config.Alias {
			http.Error(w, "only the configured alias is accepted", http.StatusBadRequest)
			return
		}
		upstreamRequest := req.Clone(req.Context())
		upstreamRequest.Body = io.NopCloser(bytes.NewReader(body))
		upstreamRequest.URL.Path = "/v1/chat/completions"
		r.upstream.ServeHTTP(w, upstreamRequest)
	})
}

func (r *Router) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return fmt.Sprintf("alias=%s upstream=%s layers=%s", r.config.Alias, r.config.Upstream, strings.TrimSpace(fmt.Sprint(r.layers)))
}
