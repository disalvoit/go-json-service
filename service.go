package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Task is the resource the API serves.
type Task struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// Service is an in-memory task store plus the HTTP surface in front of it.
type Service struct {
	mu    sync.Mutex
	n     int
	items map[string]*Task
}

// New returns an empty service.
func New() *Service { return &Service{items: map[string]*Task{}} }

// Handler returns the HTTP handler for the API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks", s.handleTasks)
	mux.HandleFunc("/tasks/", s.handleTask)
	mux.HandleFunc("/debug/slow", s.handleSlow)
	mux.HandleFunc("/debug/panic", s.handlePanic)
	return mux
}

// Serve runs the server on ln until ctx is cancelled, then stops it.
func (s *Service) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------- handlers

func (s *Service) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		out := make([]Task, 0, len(s.items))
		for _, t := range s.items {
			out = append(out, *t)
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var in struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.n++
		t := &Task{ID: strconv.Itoa(s.n), Title: in.Title}
		s.items[t.ID] = t
		s.mu.Unlock()
		writeJSON(w, http.StatusCreated, t)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleTask(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/tasks/"):]
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		t, ok := s.items[id]
		var out Task
		if ok {
			out = *t
		}
		s.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodDelete:
		s.mu.Lock()
		_, ok := s.items[id]
		delete(s.items, id)
		s.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSlow is a diagnostic route: it is the request that has to survive a
// shutdown. Nothing else in the API takes long enough to prove the claim.
func (s *Service) handleSlow(w http.ResponseWriter, r *http.Request) {
	time.Sleep(400 * time.Millisecond)
	writeJSON(w, http.StatusOK, map[string]string{"slept": "400ms"})
}

// handlePanic is a diagnostic route: it is the failure the middleware has to
// contain without taking the connection with it.
func (s *Service) handlePanic(w http.ResponseWriter, r *http.Request) {
	panic("handler exploded")
}

// ------------------------------------------------------------------ helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
