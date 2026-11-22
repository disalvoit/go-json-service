# go-json-service — Zero to Hero, Go episode 6

A small JSON API on `net/http`: routing, a middleware chain, and a shutdown that
waits for the requests already inside instead of cutting them off.

```go
type Service struct{ /* in-memory store */ }

func New() *Service
func (s *Service) Handler() http.Handler                              // routes + middleware
func (s *Service) Serve(ctx context.Context, ln net.Listener) error   // serve until ctx, then drain
```

## The promise, in full

1. `POST /tasks` with `Content-Type: application/json` answers **201**, a `Location: /tasks/<id>`
   header and the created task as JSON.
2. `GET /tasks/<id>` reads it back; a miss is **404** with a JSON body `{"error": ...}`.
3. Every response carries a non-empty `X-Request-Id`, distinct per request.
4. A body over **1 MiB** is refused with **413**; a body not declared as JSON is refused with **415**.
5. A method the route does not serve is **405** with an `Allow` header.
6. A panicking handler becomes **500** with a JSON body, and the next request is still served.
7. `Serve` returns `nil` when `ctx` is cancelled, and a request already inside the handler at that
   moment **completes** (200) before the listener stops.
8. 64 concurrent creates produce 64 distinct ids and 64 tasks in the list, clean under `-race`.

## Run it

```bash
go test .                # the suite that ships with the starter: green
go test ./heldout/       # the held-out conformance suite: ten checks, nine of them red
go test ./...            # both -- on the starter this dies at check 7: the panic
                         # in /debug/panic takes the test binary with it
go run ./cmd/measure     # the two numbers the episode quotes
go run ./cmd/server 127.0.0.1:8080
```

`/debug/slow` (sleeps 400 ms) and `/debug/panic` exist so claims 6 and 7 are
*measurable* instead of merely asserted. They are the only two routes that are
not part of the API.

Module `tasks`, Go 1.24, standard library only — no router, no framework.
