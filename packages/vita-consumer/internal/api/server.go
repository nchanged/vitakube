package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/nchanged/vitakube/packages/vita-consumer/internal/buffer"
	"github.com/nchanged/vitakube/packages/vita-consumer/internal/store"
)

type Server struct {
	sqlite *store.SQLiteStore
	ring   *buffer.RingBuffer
}

func NewServer(sqlite *store.SQLiteStore, ring *buffer.RingBuffer) *Server {
	return &Server{
		sqlite: sqlite,
		ring:   ring,
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// List endpoints
	mux.HandleFunc("/api/v1/nodes", s.handleListNodes)
	mux.HandleFunc("/api/v1/namespaces", s.handleListNamespaces)
	mux.HandleFunc("/api/v1/deployments", s.handleListDeployments)
	mux.HandleFunc("/api/v1/pods", s.handleListPods)
	mux.HandleFunc("/api/v1/pvcs", s.handleListPVCs)

	// Live metrics
	mux.HandleFunc("/api/v1/metrics/live", s.handleLiveMetrics)
}

// Helper functions
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func getQueryInt(r *http.Request, param string) (int64, bool) {
	val := r.URL.Query().Get(param)
	if val == "" {
		return 0, false
	}
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false
	}
	return i, true
}
