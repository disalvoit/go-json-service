package heldout_test

// Held-out conformance suite for the G6 checkpoint: ten checks written from the
// promise in the README, not from the starter's code. Run it as it ships:
//
//	go test ./heldout/
//
// The starter fails most of them; that is the episode.

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"fmt"
	"sync"
	"testing"
	"time"

	"tasks"
)

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	return tasks.New().Handler()
}

func post(t *testing.T, h http.Handler, path, ct, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

// 1. A create answers 201, says where the resource lives, and echoes it as JSON.
func TestCreateIs201WithLocation(t *testing.T) {
	h := newHandler(t)
	resp := post(t, h, "/tasks", "application/json", `{"title":"write the handler"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /tasks = %d, want 201", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/tasks/") || len(loc) <= len("/tasks/") {
		t.Fatalf("Location = %q, want /tasks/<id>", loc)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if got["title"] != "write the handler" {
		t.Errorf("title = %v, want the one sent", got["title"])
	}
	if !strings.HasSuffix(loc, "/"+toStr(got["id"])) {
		t.Errorf("Location %q does not point at id %v", loc, got["id"])
	}
}

// 2. What was created can be read back at the Location it advertised.
func TestReadBackThroughLocation(t *testing.T) {
	h := newHandler(t)
	resp := post(t, h, "/tasks", "application/json", `{"title":"read me back"}`)
	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Fatalf("no Location header to follow")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, loc, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", loc, rec.Code)
	}
	var got tasks.Task
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("GET %s did not return a Task: %v", loc, err)
	}
	if got.Title != "read me back" {
		t.Errorf("title = %q, want the one sent", got.Title)
	}
}

// 3. A miss is still the API's own JSON contract, not plain text.
func TestMissingIsJSON404(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/does-not-exist", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("404 Content-Type = %q, want application/json", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("404 body is not JSON: %v", err)
	}
	if s, _ := body["error"].(string); s == "" {
		t.Errorf(`404 body = %v, want a non-empty "error"`, body)
	}
}

// 4. A body that is not declared as JSON is refused, not guessed at.
func TestWrongContentTypeIs415(t *testing.T) {
	h := newHandler(t)
	resp := post(t, h, "/tasks", "text/plain", `{"title":"nope"}`)
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("POST text/plain = %d, want 415", resp.StatusCode)
	}
}

// 5. A body over the documented 1 MiB limit is refused, not read into memory.
func TestOversizeBodyIs413(t *testing.T) {
	h := newHandler(t)
	huge := `{"title":"` + strings.Repeat("a", 1200*1024) + `"}`
	resp := post(t, h, "/tasks", "application/json", huge)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST 1.2 MiB = %d, want 413", resp.StatusCode)
	}
}

// 6. A method the route does not serve says which ones it does.
func TestMethodNotAllowed(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/tasks", strings.NewReader(`{}`)))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT /tasks = %d, want 405", rec.Code)
	}
	allow := rec.Header().Get("Allow")
	if !strings.Contains(allow, "GET") || !strings.Contains(allow, "POST") {
		t.Errorf("Allow = %q, want GET and POST", allow)
	}
}

// 7. A panicking handler becomes a 500 for that caller and leaves the server up.
func TestPanicIsContained(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/panic", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panicking route = %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("500 Content-Type = %q, want application/json", ct)
	}
	// The next request must still be served.
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/tasks", nil))
	if rec2.Code != http.StatusOK {
		t.Errorf("request after a panic = %d, want 200", rec2.Code)
	}
}

// 8. Every response carries a request id, and two requests do not share one.
func TestRequestIDOnEveryResponse(t *testing.T) {
	h := newHandler(t)
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks", nil))
		id := rec.Header().Get("X-Request-Id")
		if id == "" {
			t.Fatalf("response %d has no X-Request-Id", i+1)
		}
		if seen[id] {
			t.Fatalf("X-Request-Id %q reused across requests", id)
		}
		seen[id] = true
	}
}

// 9. Shutting down drains the request that is already inside, then stops.
func TestShutdownDrainsInFlight(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := tasks.New()
	served := make(chan error, 1)
	go func() { served <- svc.Serve(ctx, ln) }()
	base := "http://" + ln.Addr().String()

	type result struct {
		code int
		err  error
	}
	inflight := make(chan result, 1)
	go func() {
		resp, err := http.Get(base + "/debug/slow")
		if err != nil {
			inflight <- result{err: err}
			return
		}
		defer resp.Body.Close()
		inflight <- result{code: resp.StatusCode}
	}()

	time.Sleep(120 * time.Millisecond) // let it get inside the handler
	cancel()

	select {
	case r := <-inflight:
		if r.err != nil {
			t.Fatalf("in-flight request died during shutdown: %v", r.err)
		}
		if r.code != http.StatusOK {
			t.Fatalf("in-flight request = %d, want 200", r.code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight request never finished")
	}

	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve returned %v after a clean shutdown, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after ctx was cancelled")
	}
}

// 10. Under -race, parallel creates stay unique and all of them land.
func TestConcurrentCreates(t *testing.T) {
	h := newHandler(t)
	const n = 64
	var wg sync.WaitGroup
	var mu sync.Mutex
	ids := map[string]bool{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp := post(t, h, "/tasks", "application/json", `{"title":"t"}`)
			var got tasks.Task
			if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
				return
			}
			mu.Lock()
			ids[got.ID] = true
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if len(ids) != n {
		t.Fatalf("%d creates produced %d distinct ids", n, len(ids))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks", nil))
	var all []tasks.Task
	if err := json.NewDecoder(rec.Body).Decode(&all); err != nil {
		t.Fatalf("GET /tasks is not a JSON list: %v", err)
	}
	if len(all) != n {
		t.Fatalf("list holds %d tasks, want %d", len(all), n)
	}
}

// toStr renders the decoded id field, which arrives as a float64, for comparison.
func toStr(v any) string { return fmt.Sprintf("%v", v) }
