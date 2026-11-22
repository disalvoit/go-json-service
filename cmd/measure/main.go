// Command measure runs the two claims of this episode against the code in the
// tree, and prints the numbers the video quotes. Re-run it after a change:
//
//	go run ./cmd/measure
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"tasks"
)

func main() {
	shutdownSample()
	panicSample()
}

// shutdownSample: a request is inside the handler when the context is cancelled.
// Either it finishes (200) or it is cut off — and the number says which.
func shutdownSample() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("listen:", err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	svc := tasks.New()
	served := make(chan error, 1)
	go func() { served <- svc.Serve(ctx, ln) }()
	base := "http://" + ln.Addr().String()

	type res struct {
		code int
		err  error
		ms   int64
	}
	out := make(chan res, 1)
	go func() {
		start := time.Now()
		resp, err := http.Get(base + "/debug/slow")
		if err != nil {
			out <- res{err: err, ms: time.Since(start).Milliseconds()}
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		out <- res{code: resp.StatusCode, ms: time.Since(start).Milliseconds()}
	}()

	time.Sleep(120 * time.Millisecond)
	start := time.Now()
	cancel()

	r := <-out
	if r.err != nil {
		fmt.Printf("shutdown in-flight: DROPPED after %d ms (%v) | Serve returned %v\n",
			time.Since(start).Milliseconds()+120, r.err, <-served)
		return
	}
	fmt.Printf("shutdown in-flight: %d after %d ms (drained) | Serve returned %v\n",
		r.code, r.ms, <-served)
}

// panicSample: what the caller sees when a handler panics.
func panicSample() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("listen:", err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := tasks.New()
	go svc.Serve(ctx, ln)
	base := "http://" + ln.Addr().String()
	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	resp, err := http.Get(base + "/debug/panic")
	if err != nil {
		fmt.Printf("panic: caller got %v after %d ms\n", err, time.Since(start).Milliseconds())
	} else {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		fmt.Printf("panic: caller got %d after %d ms\n", resp.StatusCode, time.Since(start).Milliseconds())
	}

	// Is the server still answering after the panic?
	start = time.Now()
	resp2, err2 := http.Get(base + "/tasks")
	if err2 != nil {
		fmt.Printf("after panic: %v\n", err2)
		return
	}
	resp2.Body.Close()
	fmt.Printf("after panic: %d after %d ms\n", resp2.StatusCode, time.Since(start).Milliseconds())
}
