package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"websearch/pkg/cache"
	"websearch/pkg/config"
	"websearch/pkg/quota"
	"websearch/pkg/telemetry"
)

//go:embed web/*
var webFiles embed.FS

const defaultEventLimit = 50

type Handler struct {
	conf    config.Config
	store   *telemetry.Store
	quota   *quota.Service
	cache   *cache.Cache
	restart func()
}

func New(conf config.Config, store *telemetry.Store, searchCache *cache.Cache, restart func()) *Handler {
	return &Handler{conf: conf, store: store, quota: quota.New(conf), cache: searchCache, restart: restart}
}

func (h *Handler) Register(mux *http.ServeMux, guard func(http.HandlerFunc) http.HandlerFunc) {
	assets, _ := fs.Sub(webFiles, "web")
	fileServer := http.FileServer(http.FS(assets))
	mux.Handle("/dashboard/", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dashboard/" {
			http.ServeFileFS(w, r, assets, "index.html")
			return
		}
		http.StripPrefix("/dashboard/", fileServer).ServeHTTP(w, r)
	}))
	mux.HandleFunc("/dashboard", guard(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusTemporaryRedirect)
	}))
	mux.HandleFunc("/__admin/api/overview", guard(h.overview))
	mux.HandleFunc("/__admin/api/events", guard(h.events))
	mux.HandleFunc("/__admin/api/quotas", guard(h.quotas))
	mux.HandleFunc("/__admin/api/settings", guard(h.settings))
	mux.HandleFunc("/__admin/api/secrets", guard(h.secrets))
	mux.HandleFunc("/__admin/api/restart", guard(h.restartService))
	mux.HandleFunc("/__admin/api/cache/clear", guard(h.clearCache))
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dashboard telemetry disabled"})
		return
	}
	observed, err := h.store.Overview()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, buildDashboardOverview(h.conf, observed))
}

// eventLimit accepts the dashboard page sizes 20/50/100; any other value
// (including a missing or unparsable one) falls back to 50.
func eventLimit(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return defaultEventLimit
	}
	switch n {
	case 20, 50, 100:
		return n
	}
	return defaultEventLimit
}

// eventKind accepts kind=provider|tool|none; empty or unknown values mean
// "all". "none" is the explicit no-match sentinel used when the dashboard
// combines a tool filter with a provider filter.
func eventKind(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "provider" || value == "tool" || value == "none" {
		return value
	}
	return ""
}

// eventStatus accepts status=all|success|failure; empty or unknown values
// mean "all".
func eventStatus(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "success" || value == "failure" {
		return value
	}
	return ""
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dashboard telemetry disabled"})
		return
	}
	query := r.URL.Query()
	kind := eventKind(query.Get("kind"))
	if kind == "none" {
		writeJSON(w, http.StatusOK, []telemetry.StoredEvent{})
		return
	}
	out, err := h.store.RecentFiltered(telemetry.EventFilter{
		Kind:   kind,
		Status: eventStatus(query.Get("status")),
		Source: strings.TrimSpace(query.Get("source")),
		Limit:  eventLimit(query.Get("limit")),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) quotas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, h.quota.Get(ctx))
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, settingsView(h.conf))
	case http.MethodPost:
		var req SettingsRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		out, err := applySettings(h.conf, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, out)
	default:
		methodNotAllowed(w)
	}
}

func (h *Handler) secrets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req SecretRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := saveSecret(h.conf, req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "restart_required": true})
}

func (h *Handler) restartService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := decodeJSON(r, &req); err != nil || !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "confirm=true required"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"restarting": true})
	if h.restart != nil {
		go func() { time.Sleep(350 * time.Millisecond); h.restart() }()
	}
}

func (h *Handler) clearCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := decodeJSON(r, &req); err != nil || !req.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "confirm=true required"})
		return
	}
	if h.cache == nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "cache is disabled"})
		return
	}
	n, err := h.cache.Clear()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("%v", err)})
}
func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
}
