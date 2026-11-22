package tasks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The suite that ships with the starter: the happy path only. It stays green
// while the held-out suite in ./heldout is red — that gap is the lesson.

func TestCreateAndReadBack(t *testing.T) {
	svc := New()
	h := svc.Handler()

	body := `{"title":"ship the api"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /tasks = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	var created Task
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("POST /tasks body is not a Task: %v", err)
	}
	if created.ID == "" || created.Title != "ship the api" {
		t.Fatalf("created = %+v", created)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/"+created.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /tasks/%s = %d, want 200", created.ID, rec.Code)
	}
	var got Task
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("GET body is not a Task: %v", err)
	}
	if got != created {
		t.Fatalf("read back %+v, want %+v", got, created)
	}
}

func TestListReturnsJSONArray(t *testing.T) {
	svc := New()
	h := svc.Handler()
	for _, title := range []string{"one", "two", "three"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"title":"`+title+`"}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /tasks = %d, want 200", rec.Code)
	}
	var all []Task
	if err := json.NewDecoder(rec.Body).Decode(&all); err != nil {
		t.Fatalf("GET /tasks is not a JSON array: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("list holds %d tasks, want 3", len(all))
	}
}

func TestDeleteThenMiss(t *testing.T) {
	svc := New()
	h := svc.Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"title":"temporary"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	var created Task
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("POST body: %v", err)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/tasks/"+created.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d, want 204", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/"+created.ID, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET after DELETE = %d, want 404", rec.Code)
	}
}
