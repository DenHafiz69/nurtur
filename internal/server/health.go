package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"nurtur/internal/store"
)

type healthResponse struct {
	Status  string      `json:"status"`
	Version string      `json:"version"`
	DB      string      `json:"db"`
	Time    string      `json:"time"`
	Mode    string      `json:"mode"`
	Sync    *syncHealth `json:"sync,omitempty"`
}

type syncHealth struct {
	Status     store.SyncStatus `json:"status"`
	Stats      *store.SyncStats `json:"stats,omitempty"`
	StatsError string           `json:"stats_error,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	pingErr := s.store.Ping(ctx)
	response := healthResponse{
		Status:  "ok",
		Version: s.version,
		DB:      "ok",
		Time:    s.clock.Now().UTC().Format(time.RFC3339),
		Mode:    "local",
	}
	code := http.StatusOK
	if pingErr != nil {
		response.Status = "down"
		response.DB = "error"
		code = http.StatusServiceUnavailable
		s.log.Error("health: database ping failed", "error", pingErr)
	}

	if s.worker != nil {
		response.Mode = "sync"
		sh := &syncHealth{Status: s.worker.Status()}
		if sh.Status.LastPushError != "" ||
			sh.Status.LastPullError != "" ||
			(sh.Status.Dirty && sh.Status.PendingAgeSeconds > 300) {
			response.Status = "degraded"
		}
		if verbose(r) {
			stats, err := s.worker.Stats(ctx)
			if err != nil {
				sh.StatsError = err.Error()
			} else {
				sh.Stats = &stats
			}
		}
		response.Sync = sh
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(response)
}

func verbose(r *http.Request) bool {
	value := r.URL.Query().Get("verbose")
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}
