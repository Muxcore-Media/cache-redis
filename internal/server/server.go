package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Muxcore-Media/cache-redis/internal/cache"
)

type Server struct {
	cache       *cache.Cache
	getCount    atomic.Int64
	setCount    atomic.Int64
	delCount    atomic.Int64
	incrCount   atomic.Int64
}

func New(c *cache.Cache) *Server {
	return &Server{cache: c}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/cache/", s.handleKey)
	mux.HandleFunc("/v1/cache", s.handleList)
	mux.HandleFunc("/v1/cache/exists/", s.handleExists)
	mux.HandleFunc("/v1/cache/incr/", s.handleIncr)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/metrics", s.handleMetrics)
	return mux
}

func (s *Server) handleKey(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/v1/cache/")
	key = strings.TrimSuffix(key, "/")
	if key == "" {
		http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGet(w, r, key)
	case http.MethodPut:
		s.handleSet(w, r, key)
	case http.MethodDelete:
		s.handleDelete(w, r, key)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, key string) {
	data, err := s.cache.Get(r.Context(), key)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		slog.Error("cache: get failed", "key", key, "error", err)
		return
	}
	s.getCount.Add(1)
	if data == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

func (s *Server) handleSet(w http.ResponseWriter, r *http.Request, key string) {
	ttl := 0 * time.Second
	if ttlStr := r.Header.Get("X-Cache-TTL"); ttlStr != "" {
		if sec, err := strconv.Atoi(ttlStr); err == nil && sec > 0 {
			ttl = time.Duration(sec) * time.Second
		}
	}
	defer r.Body.Close()
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		http.Error(w, `{"error":"body too large"}`, http.StatusRequestEntityTooLarge)
		return
	}
	if err := s.cache.Set(r.Context(), key, data, ttl); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		slog.Error("cache: set failed", "key", key, "error", err)
		return
	}
	s.setCount.Add(1)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, key string) {
	if err := s.cache.Delete(r.Context(), key); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		slog.Error("cache: delete failed", "key", key, "error", err)
		return
	}
	s.delCount.Add(1)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleExists(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/v1/cache/exists/")
	key = strings.TrimSuffix(key, "/")
	if key == "" {
		http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
		return
	}
	exists, err := s.cache.Exists(r.Context(), key)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		slog.Error("cache: exists failed", "key", key, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"exists": exists})
}

func (s *Server) handleIncr(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/v1/cache/incr/")
	key = strings.TrimSuffix(key, "/")
	if key == "" {
		http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
		return
	}
	delta := int64(1)
	if d := r.URL.Query().Get("delta"); d != "" {
		if n, err := strconv.ParseInt(d, 10, 64); err == nil {
			delta = n
		}
	}
	n, err := s.cache.Incr(r.Context(), key, delta)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		slog.Error("cache: incr failed", "key", key, "error", err)
		return
	}
	s.incrCount.Add(1)
	writeJSON(w, http.StatusOK, map[string]int64{"value": n})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.cache.Health(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("# HELP cache_get_total Total cache get operations\n")
	b.WriteString("# TYPE cache_get_total counter\n")
	fmt.Fprintf(&b, "cache_get_total %d\n", s.getCount.Load())
	b.WriteString("# HELP cache_set_total Total cache set operations\n")
	b.WriteString("# TYPE cache_set_total counter\n")
	fmt.Fprintf(&b, "cache_set_total %d\n", s.setCount.Load())
	b.WriteString("# HELP cache_delete_total Total cache delete operations\n")
	b.WriteString("# TYPE cache_delete_total counter\n")
	fmt.Fprintf(&b, "cache_delete_total %d\n", s.delCount.Load())
	b.WriteString("# HELP cache_incr_total Total cache increment operations\n")
	b.WriteString("# TYPE cache_incr_total counter\n")
	fmt.Fprintf(&b, "cache_incr_total %d\n", s.incrCount.Load())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write([]byte(b.String()))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
