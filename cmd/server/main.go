package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"tasks"
)

func main() {
	addr := "127.0.0.1:8080"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	// The signal turns into a cancelled context. Everything downstream reads
	// that one value instead of registering its own handler.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc := tasks.New()
	log.Printf("listening on http://%s", ln.Addr())
	if err := svc.Serve(ctx, ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	log.Print("shutdown complete")
}
