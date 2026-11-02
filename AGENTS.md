# AGENTS.md

Guidance for AI coding agents working in this repository. This is a Go **1.27.1** project
focused on concurrent, high-performance code. The rules below distill *Concurrency in Go*
(Cox-Buday), *Learn Concurrent Programming with Go* (Cutajar), *100 Go Mistakes* (Harsanyi),
*Efficient Go* (Płotka), *The Go Programming Language* (Donovan & Kernighan), *Go in Practice*
(Butcher et al.), and *Distributed Services with Go* (Jeffery). Follow them unless a task
explicitly says otherwise.

## Project

- Go **1.27.1**: `go.mod` declares `go 1.27.1` and the `Makefile` exports `GOTOOLCHAIN=go1.27.1`, so the locally installed Go version does not matter. Keep the pinned version as-is; downgrade or add a `toolchain` directive only when explicitly asked.
- Layout: `cmd/` holds the binaries — `cmd/omnic/main.go` (the HTTP API) and `cmd/omnic-handset/main.go` (a handset client: CLI flags plus a Bubble Tea TUI). `internal/` for all private packages (`internal/handler`, `internal/stress`, ...). Keep everything under `internal/`; nothing here is a public library, so `pkg/` stays absent.
- Stress tests live in `internal/stress`: each is a `stress.Test` (with `Name`, `Description`, and `Setup`) registered in its own `init`. To add one, drop a file there that calls `stress.Register` — it appears in the `omnic-handset` TUI list and the `-test` flag automatically. `cmd/omnic-handset` is wiring/UI only; the engine and scenarios stay in `internal/stress`.
- **`make` is the only interface.** Build, test, QA, and local infra are `make` targets; always go through them and the flags the Makefile defines, rather than hand-typed `go build`/`go test` incantations. Tool deps (`golangci-lint`, `gofumpt`, `nilaway`, `arch-go`, `govulncheck`, `fieldalignment`, `benchstat`, `dlv`, `sqlc`) are pinned in `go.mod` as `tool` dependencies; always run them via `go tool` from those pinned versions, rather than a fresh `go install`.

## Commands

```sh
make help          # list targets
make install-tools # download deps, warm the tool cache: lint, dlv, sqlc (first run on a machine)
make all           # build server and handset -> build/omnic + build/omnic-handset
make build         # CGO_ENABLED=0 static binary -> build/omnic
make build-debug   # unoptimized server (-N -l) -> build/omnic-debug + pinned dlv -> build/dlv; used by VS Code "Debug Server"
make build-handset # CGO_ENABLED=0 static binary -> build/omnic-handset (alias: make build-phone, make build-stress)
make test          # go test -race -count=1 ./...  (cgo enabled only here, for -race)
make bench         # benchmarks -> build/bench/$(NAME).txt (BENCH=. COUNT=6 BENCHTIME=1s NAME=new)
make bench-compare # benchstat build/bench/old.txt build/bench/new.txt (OLD=/NEW= override)
make qa            # fix -> fmt -> vet -> lint -> nilaway -> arch-go -> loc -> fieldalignment, in that order
make fix           # go fix modernizers: rewrites locally; CI=1 make fix fails on any pending fix
make fmt           # gofumpt: rewrites locally; CI=1 make fmt fails on any unformatted file
make lint          # golangci-lint
make nilaway       # NilAway nil-panic analysis
make arch-go       # architecture rules from arch-go.yml
make loc           # scripts/loc.sh: fail on any Go file over 1500 lines of code (MAXLOC=n override; -a lists all)
make vuln          # govulncheck (needs network; deliberately not part of qa)
make profile       # MODE=local|docker: build, load-test, capture pprof profiles -> build/profile/summary.txt (or /profile in Claude Code)
make infra-build   # docker compose build (layer cache); Dockerfile: golang:1.27-trixie builder -> distroless static:nonroot, digest-pinned
make infra-rebuild # docker compose build --no-cache --pull (clean image, fresh base)
make infra-up      # docker compose up -d --wait (local stack; app on :8090 (container :8080), pprof+/metrics on 127.0.0.1:6060, Prometheus 127.0.0.1:9090, Grafana 127.0.0.1:3000 (anonymous admin, no login))
make infra-debug   # compose up with docker-compose.debug.yml: debug image (-N -l + pinned dlv), dlv headless on 127.0.0.1:2345; attach with VS Code "Debug Server (docker)"
make infra-down    # docker compose down (keeps volumes)
make clean         # rm -rf build/
```

- **After every code change run `make qa && make test`, never individual `go test`/`go vet`/`go build` invocations.** Run them after each edit, not just before declaring work done. If either fails, report the output; claim success only when both pass. Also build the binary you touched: `make build` for `cmd/omnic` or the Dockerfile, `make build-handset` for `cmd/omnic-handset`.
- `make infra-build` when you touched the Dockerfile (`make infra-debug` also exercises the `debug` stage); `make infra-rebuild` when a build looks stale or the base image must be refreshed (it is slow: no layer cache). Always use these make targets for image builds, in place of hand-typed `docker build`/`docker compose build`.
- `make test` runs the race detector on every run; it is the REQUIRED gate for any change touching goroutines, channels, or shared state. `test` is deliberately not part of `qa`: QA is static and hermetic, tests may need `make infra-up`.
- `make qa` order matters: `fix` (`go fix`, modernizers bounded by the `go.mod` version) first because it rewrites code, then `fmt` (gofumpt) so the rewritten code is formatted and later tools' file:line output stays valid; `vet` before `lint` (golangci-lint) to keep noise down; `loc` (`scripts/loc.sh`, 1500 lines of code per file, comments and blanks excluded) after `arch-go` as the file-size counterpart of its per-function caps; `fieldalignment` last because it is a layout optimization, not a correctness check. If `fieldalignment` flags a struct, fix it by reordering the fields, keeping the check enabled. `make vuln` is separate from `qa` because it needs network access to the Go vulnerability DB.
- After changing imports: `go mod tidy`, then commit `go.mod` and `go.sum` together.
- Benchmarks: `make bench` runs `go test -run '^$' -bench . -benchmem -count=6 ./...` and saves the output to `build/bench/new.txt`. Override with `BENCH=<regex>`, `COUNT=<n>`, `BENCHTIME=<dur>`, `PKGS=<pkgs>`, `NAME=<file stem>`. To compare: `make bench NAME=old`, change the code, `make bench`, then `make bench-compare` (`benchstat`, pinned as a `go tool`; `OLD=`/`NEW=` override the files).
- Go version, Makefile `GOTOOLCHAIN`, and the Dockerfile `golang:1.27` tag are bumped together in one dedicated commit, kept separate from any feature branch.

## Using imported packages: `go doc` first

Always look up APIs with `go doc`, rather than guessing from memory or browsing the web. The module cache is the source of truth for the exact pinned versions in `go.mod`, and `go doc` reads it directly:

```sh
go doc github.com/labstack/echo/v4              # package overview and exported identifiers
go doc github.com/labstack/echo/v4.Context       # one type, with all its methods
go doc github.com/labstack/echo/v4.Context.JSON  # one method signature + doc comment
go doc -all github.com/labstack/echo/v4/middleware | less   # everything, including unexported examples
go doc -src net/http.Server.Shutdown             # show the actual source when the doc comment is not enough
go doc golang.org/x/sync/errgroup                # works for stdlib, x/ repos, and any dependency in go.mod
go doc -ex golang.org/x/sync/errgroup            # list the package's runnable examples (Go 1.27+)
go doc golang.org/x/sync/errgroup.ExampleGroup   # print one example's source: the fastest way to see correct usage
```

- **Before calling any function from an imported package** (third-party or stdlib) that you have not already used in this file, run `go doc` on it and read the signature and doc comment. API surfaces change between versions; the version in `go.sum` is what compiles, not what a training set remembers.
- Before adding a new dependency, `go doc` the package after `go get` to confirm it actually exposes what you need; if it does not, remove it (`go mod tidy`) rather than working around it.
- When a `go doc` comment mentions concurrency safety ("safe for concurrent use", "must not be called after Close", "the caller must Close the returned Body"), treat that sentence as a hard requirement and mirror it in your own doc comment if you wrap it.
- If `go doc` says "no required module provides package", the dependency is not in `go.mod`; add it properly with `go get`, rather than papering over it with a guessed import path.
- Prefer `go doc` over opening files in `$(go env GOMODCACHE)` directly; it is shorter, version-correct, and shows exactly the exported surface you are allowed to depend on.

---

## 1. Concurrency: design rules (Cox-Buday, Cutajar, Donovan & Kernighan)

**Concurrency is a property of the code; parallelism is a property of the runtime.** Structure for correctness first, measure before assuming it is faster.

- **Share memory by communicating.** Default to channels to transfer *ownership* of data between goroutines. Use a mutex only to guard a small internal critical section (a struct's own fields, a cache, a counter). Decision rule: transferring ownership or coordinating stages → channel; protecting internal state → `sync.Mutex`/`RWMutex`; the Go team's own guidance.
- **Every goroutine must have a known exit.** Before writing `go f()`, answer: *when does this goroutine stop, and who stops it?* Start it only once you can answer. Goroutines that block forever on a channel or `select` are memory leaks that also pin their stacks and any captured data.
- **The parent owns the lifecycle.** Whoever starts a goroutine is responsible for stopping it and waiting for it (`sync.WaitGroup`, `errgroup`, or a `done` channel). Fire-and-forget goroutines belong only in `main` and top-level servers, and those still need graceful shutdown.
- **Channel ownership:** the goroutine that *writes* to a channel owns it: it creates it, writes to it, and is the only one that closes it. Consumers only read and range. A channel is closed exactly once, always from the sending side, and written to only while open (writing to a closed channel panics).
- **Return receive-only (`<-chan T`) / send-only (`chan<- T`) types** at API boundaries so the compiler enforces ownership.
- **Prefer unbuffered channels** unless you have a measured reason for a buffer. A buffer hides backpressure and turns deadlocks into delayed deadlocks. Buffered channels are justified when: you know the number of writes up front (e.g. fan-in of N results), or you are intentionally decoupling a bursty producer with a bounded queue.
- **Use `context.Context` for exactly three things: cancellation, deadlines, and request-scoped values.** It is always the first parameter (`ctx context.Context`). Every blocking operation (`select`, network call, DB call, channel send) must also listen on `ctx.Done()`. Keep a `Context` flowing through parameters, out of struct fields. Always pass a real context; use `context.TODO()` if genuinely unknown. Call every `cancel` returned by `WithCancel/WithTimeout/WithDeadline` (`defer cancel()`), otherwise the context and its timer leak.
- **Canonical patterns:** reach for these by name instead of inventing ad hoc ones:
  - *Pipeline*: stages connected by channels, each stage owns its output channel and closes it when its input is drained.
  - *Fan-out / fan-in*: N workers reading from one channel, results merged into one channel by a goroutine that closes it after `wg.Wait()`.
  - *Worker pool*: fixed number of goroutines (bounded by CPU or downstream capacity, not one per item) pulling from a jobs channel.
  - *or-done channel*: wrap reads so a stage exits on `ctx.Done()` without checking in every `select`.
  - *Bounded semaphore*: `make(chan struct{}, n)` or `golang.org/x/sync/semaphore` to cap concurrent access to a resource.
  - *errgroup*: `golang.org/x/sync/errgroup` (`g, ctx := errgroup.WithContext(ctx)`) for "run these concurrently, stop all on first error". Prefer it over a hand-rolled WaitGroup + error channel. Use `g.SetLimit(n)` to bound concurrency.
  - *Heartbeats* for long-running workers that must prove liveness; *rate limiting* with `golang.org/x/time/rate`.
- **`select` gotchas:** a `select` with multiple ready cases picks **randomly**; write cases so any order is correct. A `default` branch makes it non-blocking; in a loop, always pair `default` with a wait or you busy-spin. Reading from a `nil` channel blocks forever. This is a feature: set a channel variable to `nil` to disable a case after it is drained.
- **`time.After` no longer leaks:** since Go 1.23 an unreferenced timer is garbage-collected before it fires, and Go 1.27 removed the `asynctimerchan` opt-out, so this is unconditional. `time.After` in a `select` is fine. Reach for `time.NewTimer` + `Reset` only to avoid a per-iteration allocation in a measured hot loop, and `context.WithTimeout` when the deadline should propagate.
- **`sync.WaitGroup`:** call `Add` *before* starting the goroutine (in the parent, not inside it), `defer wg.Done()` as the first line of the goroutine. Always pass `*sync.WaitGroup` as a pointer. Prefer `wg.Go(func())` (Go 1.25+) where it fits.
- **Mutex discipline:** keep critical sections tiny; release a lock before any I/O, channel op, or call into unknown code. `defer mu.Unlock()` immediately after `mu.Lock()` unless the function is hot and the lock scope is obviously tiny. Always pass a struct containing a mutex by pointer, keeping it uncopied (`go vet` copylocks catches this). `RWMutex` only when reads vastly outnumber writes *and* you have measured; `RWMutex` is slower than `Mutex` under low contention. Always acquire multiple locks in the same global order to avoid deadlock.
- **`sync.Once` for lazy init, `sync.Pool` for amortizing short-lived allocations only (the pool may be emptied on every GC; store only state you can afford to lose).**
- **Atomics (`sync/atomic`) only for simple counters/flags** and use the typed API (`atomic.Int64`, `atomic.Bool`, `atomic.Pointer[T]`) in preference to raw `atomic.AddInt64` on a plain field. A multi-field invariant needs a mutex; atomics cover only a single value.
- **The Go memory model is the law:** without a channel op, mutex, `Once`, `WaitGroup`, or atomic establishing happens-before, a read and a write from different goroutines is a **data race**, full stop, even if it "works on my machine." `-race` is mandatory, but it only finds races that *execute*; design so there are none to find.
- **Loop variable capture**: Go ≥1.22 gives each iteration its own variable, so `go func() { use(v) }()` is safe, but still pass values explicitly as arguments when it reads more clearly and for anything shared across iterations.
- **Clean up every goroutine a test starts.** Use `t.Context()`; every test that starts goroutines waits for them to finish and verifies it with `goleak.VerifyNone(t)` (see **Testing**).
- **Deadlock awareness:** the runtime only reports "all goroutines are asleep" if *every* goroutine is blocked. A partial deadlock (two goroutines waiting on each other while the rest of the server runs) is silent. Use timeouts in tests and `ctx.Done()` cases in production selects so deadlocks fail loudly.

## 2. Concurrency: API and error handling (Harsanyi, Cox-Buday)

- Functions that start goroutines say so in their doc comment and document how they are stopped.
- A goroutine's errors must reach someone: return them via a result struct on a channel (`type result struct { val T; err error }`), via `errgroup`, or by logging at the top-level owner. Swallowing errors inside a goroutine is a bug.
- Every `panic` inside a goroutine needs a matching `recover`; an unrecovered panic in any goroutine kills the whole process. Top-level worker goroutines that run untrusted/handler code wrap themselves with a recover that logs and reports.
- Propagate cancellation: when your function's ctx is done, clean up and return `ctx.Err()` (or an error wrapping it) promptly.
- Pass required parameters explicitly; reserve `context.Value` for cross-cutting request metadata (trace IDs, auth principal) only, with unexported key types.

## 3. Performance: method (Płotka, Harsanyi)

- **Correct, then clear, then fast, and only fast where measured.** Optimize only what a measurement has shown to matter. The workflow is: write the straightforward version → write a benchmark → profile → optimize the hot spot → re-benchmark and prove it with numbers.
- **Benchmark properly:** `make bench` (`-benchmem -count=6`), compare with `make bench-compare` (`benchstat`). Use `b.ReportAllocs()`, `b.ResetTimer()` after setup, `b.Loop()` (Go 1.24+) to avoid the compiler optimizing the body away. Benchmarks that assign results to a package-level sink (`var sink T`) are the fallback. Run on a quiet machine; one run proves nothing.
- **Profile, don't guess:** `make profile` (or `/profile` in Claude Code; `MODE=docker` for the compose stack, `scripts/profile.sh url <pprof-url>` for any running instance) captures goroutine/goroutineleak/block/mutex/cpu/heap under load into `build/profile/`; the server enables `-pprof localhost:6060` with block/mutex sampling by default. The `goroutineleak` profile (Go 1.27+, `/debug/pprof/goroutineleak`) lists goroutines blocked on a primitive the GC proved unreachable, so they can never wake; the summary's count must be 0. It is the production counterpart of `goleak` in tests; it misses leaks reachable through globals or a live goroutine's locals, so it complements the design rules rather than replacing them. Also `go test -cpuprofile/-memprofile`, `go tool pprof -http=:0`. Look at *alloc_space* and *alloc_objects* for GC pressure, not just *inuse*. Use `go tool trace` to understand scheduler behavior, goroutine blocking, and GC pauses. Use `runtime/metrics` for production observability.
- **State efficiency goals explicitly** (latency, throughput, RSS) before optimizing; stop when met. Every optimization ships with a stated goal and a measurement.
- **Allocation is the usual enemy.** Check escape analysis with `go build -gcflags=-m`. Reduce allocations by: preallocating slices/maps with known capacity (`make([]T, 0, n)`, `make(map[K]V, n)`), reusing buffers (`bytes.Buffer.Reset`, `sync.Pool`), avoiding `fmt.Sprintf` in hot paths (`strconv.Append*`, `strings.Builder`), passing small structs by value and large ones by pointer, avoiding interface boxing and closures in hot loops, and using `[]byte` over `string` conversions where data flows through I/O.
- **Slice gotchas:** `append` on a sub-slice can overwrite the parent's backing array; use the full slice expression `s[low:high:max]` or `slices.Clone` when handing slices out. Slicing a huge array/slice for a tiny piece keeps the whole backing array alive (memory leak); copy it. `len` vs `cap` matter; `nil` slice and empty slice are both fine to `append`/`range`, but `json` encodes them differently (`null` vs `[]`).
- **Map gotchas:** maps never shrink after deletes; if a map grows huge then empties, recreate it. Map iteration order is random; write code that is correct for any order, sorting keys when output must be stable. Reading a map from multiple goroutines is safe; any concurrent write requires a mutex, or use `sync.Map` only for the append-mostly / disjoint-keys cases it is documented for.
- **Strings:** `strings.Builder` for concatenation in loops; `+=` is quadratic. Ranging over a string yields runes, not bytes; indexing yields bytes. Substrings share memory with the parent string (leak hazard for small substrings of large inputs; use `strings.Clone`).
- **Data layout:** group hot fields together; avoid false sharing in per-CPU counters (pad to 64-byte cache lines or use per-goroutine accumulation and merge). Prefer slices of structs over slices of pointers for cache locality when elements are small.
- **GC:** tune only with evidence: `GOGC`, `GOMEMLIMIT` (soft limit; set it in containers to prevent OOM kills). Reducing allocation rate beats tuning knobs.
- **I/O:** always buffer (`bufio.Reader/Writer`), always set timeouts (`http.Client{Timeout: ...}`, `http.Server` `ReadHeaderTimeout/ReadTimeout/WriteTimeout/IdleTimeout`; the zero-value server has none and is a DoS vector). Reuse a single `http.Client` / transport across requests. Always `defer resp.Body.Close()`; since Go 1.27 `Close` drains a small unread remainder itself so the connection is reused, so an explicit `io.Copy(io.Discard, resp.Body)` is only needed when you deliberately abandon a large body. `io.Copy` over manual read loops.
- **Know the compiler:** function inlining is limited (small bodies, no loops before recent versions); bounds checks can be hoisted by `_ = s[n-1]`; `defer` is cheap now but not free in the innermost loop; `interface` method calls block inlining and devirtualization.

## 4. Common mistakes to avoid (Harsanyi, "100 Go Mistakes")

- Shadowing `err` with `:=` inside an `if`/`for` block so the outer `err` is never checked. `make qa` (vet + staticcheck) catches most of these.
- Comparing errors with `==` instead of `errors.Is`; type-asserting instead of `errors.As`. Wrap with `%w` so callers can inspect; use `%v` only when you deliberately want to hide the underlying error from the API.
- Returning a typed `nil` pointer as an `error` interface (it is non-nil!). Return a literal `nil`.
- Ignoring the return of `defer`ed calls that can fail (`f.Close()`, `tx.Rollback()`); capture and join with `errors.Join` when it matters.
- Using `defer` inside a loop (runs at function exit, not iteration end); extract the loop body into a function.
- `range` evaluates the expression once and copies each element; modifying the loop variable does not modify the slice element. Use the index or a pointer.
- Using `time.Now()` for measuring elapsed time across machines / wall-clock changes; use `time.Since` (monotonic clock). Comparing `time.Time` with `==`; use `.Equal`.
- Forgetting `rows.Close()` / `rows.Err()` with `database/sql`; not using prepared statements in loops; not setting `SetMaxOpenConns`/`SetConnMaxLifetime`.
- Misusing `init()` (hidden side effects, order dependence, untestable). Prefer explicit constructors.
- Interface pollution: define an interface where it is *consumed*, keep it 1–3 methods, and only when there are ≥2 real implementations or a test seam is needed. Return concrete types from constructors ("accept interfaces, return structs").
- Getters/setters, generic "utils" packages, and deep package nesting are anti-patterns here.
- Misusing generics: use them for data structures and algorithms over multiple types; when only one type is involved, write the one concrete function.
- Floating point: compare with a tolerance (epsilon) rather than `==`; beware accumulation order. Money uses integers (minor units) or a decimal package.
- `break` inside a `select` inside a `for` only exits the `select`; use a labeled `break` or `return`.
- JSON: unexported fields are silently skipped; `time.Time` round-trips lose monotonic reading; `any` numbers decode as `float64`. Embedded structs with `MarshalJSON` hijack the parent's marshalling.
- `http.Handler`s finish all writes and all use of the request's `ctx` (including by spawned goroutines) before returning, and always check `r.Context()` for cancellation on long operations.

## 5. Production services (Jeffery, Butcher et al.)

- **Graceful shutdown is mandatory:** catch `SIGINT`/`SIGTERM` via `signal.NotifyContext`, stop accepting new work, `srv.Shutdown(ctx)` with a deadline, wait for in-flight goroutines, then exit. Every long-running component exposes a `Close`/`Shutdown` that is idempotent.
- Configuration via flags or env at startup, validated once, passed down explicitly — always through arguments, keeping it out of package-level globals.
- **Structured logging is `go.uber.org/zap`, and zap only** — it replaces `log`, `fmt.Print*`, and `log/slog`. Rules:
  - One `*zap.Logger` is built in `main` (`zap.NewProduction()` by default, `zap.NewDevelopment()` behind a `-dev` flag) and passed down explicitly as a constructor argument. That explicitly passed logger is the only one; package-level loggers, `zap.L()`/`zap.S()` globals, and `zap.ReplaceGlobals` stay out of the codebase.
  - Use the typed `*zap.Logger` API (`logger.Info("msg", zap.String("k", v), zap.Error(err))`) on every hot or concurrent path; it is allocation-free. `*zap.SugaredLogger` (`Infow`, `Infof`) is allowed only in cold startup/shutdown code; handlers, workers, and loops always stay on the typed API.
  - Messages are short, static, lowercase phrases; everything variable goes in fields, keeping the message free of `fmt.Sprintf`. Field keys are `snake_case` and stable (`request_id`, `user_id`, `duration_ms`); keys are a contract with the log pipeline.
  - `zap.Error(err)` for errors (key `error`), `zap.Duration`, `zap.Int64`, etc. over `zap.Any`; `zap.Any` uses reflection and is a hot-path allocation.
  - Derive child loggers with `logger.With(...)` for request/trace IDs and component names (`logger.Named("ingester")`), so each field is added once rather than on every call.
  - Log at the edge (handlers, workers, `main`), return errors from the middle. Each error is handled exactly once: either logged or returned. Levels: `Debug` diagnostic, `Info` state changes, `Warn` recoverable/degraded, `Error` needs a human; `Fatal`/`Panic` only in `main` before the server is up.
  - `defer logger.Sync()` in `main`; ignoring its error is acceptable there and only there (stdout/stderr return `EINVAL` on sync).
  - Echo: log requests with `middleware.RequestLoggerWithConfig` feeding the zap logger via `LogValuesFunc`, in place of `middleware.Logger()`.
  - Tests: `zap.NewNop()` by default; `zaptest.NewLogger(t)` when you want output on failure; `zap/zaptest/observer` to assert on emitted entries.
- Every external call has a timeout, a retry policy with exponential backoff and jitter (idempotent operations only), and a circuit/limit so failures don't cascade.
- Expose health checks and metrics (`/metrics` via Prometheus, `pprof`) on the internal `-pprof` listener only, kept off the public listener. HTTP metrics come from `handler.Metrics` (OTel semconv names, labelled by route pattern, never raw URI); Go runtime metrics from the Prometheus Go collector with all `runtime/metrics` enabled. Grafana dashboards are provisioned from `infra/grafana/dashboards/*.json`.
- Tests for concurrent code follow the **Testing** section: deterministic, event-synchronized, `testing/synctest` for anything time-based, run under `-race` (`make test`; use `-count=N` locally to shake out flakiness).

---

## General style

- `gofmt` only. Doc comments on all exported identifiers, starting with the name.
- **Early return over nesting.** Handle the error/edge case and `return` (or `continue` in loops) immediately; keep the happy path at the lowest indentation. After a branch that returns, continue at the outer level, dropping the `else`.
- **Program to levels of abstraction.** Low-level mechanics (I/O, parsing, direct socket/stream handling) live in a dedicated package or type that encapsulates them; the rest of the application calls a clean, high-level API and works with domain concepts, leaving raw bytes, wire formats, and connection details to that dedicated layer. A function stays at one level of abstraction, keeping raw `net.Conn` reads out of business-decision code.
- Standard library first; third-party only with a stated reason. Pre-approved: `github.com/labstack/echo/v4` (HTTP), `go.uber.org/zap` (logging), `go.uber.org/goleak` (tests only), `golang.org/x/sync`, `golang.org/x/time/rate`, `github.com/prometheus/client_golang` (metrics).
- Errors are values: return them, wrap with context (`fmt.Errorf("loading config: %w", err)`), handle once.
- Keep all mutable state locally owned; dependencies passed explicitly.
- Write for humans, not as an AI: plain, direct prose where every sentence carries content, free of conversational filler, verbose preambles, and generic pleasantries.
- Keep code comments, commit messages, and PR/issue comments concise, objective, and focused on technical realities.
- Tests: see **Testing** below. Bug fixes ship with a failing-before test.

## Testing

- **Native Go only.** Tests import the standard library and packages from this module, nothing else: `testing`, `net/http/httptest`, `testing/synctest`, `reflect.DeepEqual`/`slices.Equal`/`maps.Equal`, `errors.Is`/`errors.As`, `bytes`, `strings`, `io`, `os`. The standard library is the only test and assertion tooling — this rules out `testify`, `gomock`, `gomega`, `go-cmp`, and every other third-party test or assertion library, even one already in `go.sum` transitively. Two exceptions: (1) constructing the thing under test with its own dependencies (e.g. `echo.New()` to build a `Context`, `zap.NewNop()` for a logger) — only dependencies of the code under test, as opposed to libraries whose purpose is the test itself; (2) **`go.uber.org/goleak`** for goroutine-leak detection, via `goleak.VerifyNone(t)` (or `defer goleak.VerifyNone(t)`) in tests that start goroutines, and `goleak.VerifyTestMain(m)` in a package `TestMain` when most of the package is concurrent. Pass `goleak.IgnoreCurrent()`/`IgnoreTopFunction` only for goroutines provably owned by a dependency, with a comment naming it. Need a fake? Write a small struct that satisfies the interface, in the `_test.go` file.
- Tests live next to the code (`foo_test.go`), use table-driven cases with `t.Run`, `t.Helper()` in helpers, `t.TempDir()` for files, `t.Context()` for contexts, `t.Cleanup` for teardown, `t.Parallel()` where the test is independent. Assert with plain `if got != want { t.Fatalf("got %v, want %v", got, want) }`.
- **Tests are event-driven: synchronize on events, keeping sleeps, hardcoded timing, and the wall clock out entirely.** A test that passes because the machine was fast enough is a flaky test, and CI runners are slower, noisier, and more parallel than your laptop. Always replace with the event-based alternatives below: `time.Sleep` to "wait for the goroutine", `time.After`/`time.Tick` in assertions, polling loops with a fixed delay, asserting on measured durations (`elapsed < 100ms`), comparing against `time.Now()`, and `-timeout`-dependent behavior.
  - Synchronize on events, not time: unbuffered channels, `sync.WaitGroup`, `errgroup.Wait`, `<-done`, or `t.Context()` cancellation. If you need to know a goroutine reached a point, have it signal on a channel.
  - Time-dependent logic (timeouts, tickers, retries with backoff, expiry) is tested with `testing/synctest` (`synctest.Test(t, func(t *testing.T) { ... })`, then `synctest.Wait()`), where the fake clock advances instantly and deterministically. Inside a bubble use `synctest.Sleep(d)` (Go 1.27+: `time.Sleep` + `synctest.Wait` in one call) to advance the clock and settle goroutines. HTTP code under `synctest` uses `httptest.NewTestServer` (Go 1.27+), which serves over an in-memory network so no real port or wall clock is involved; `httptest.NewServer` is for tests outside a bubble. Use `synctest` first, before adding any knob to production code.
  - If `synctest` does not fit, inject the clock: the component takes a `now func() time.Time` or a small `Clock` interface, and the test passes a fake that advances only when the test says so. Hardcoded `time.Second` sleeps inside production code are a smell for the same reason.
  - Anything that must wait on an external process (a server starting, a file appearing) waits on the event itself (listener ready channel, `net.Listen` before `Serve`, `httptest.NewServer` which is ready on return), rather than on a sleep.
  - A test that needs a deadline as a last resort against hangs uses `t.Context()` with `context.WithTimeout` measured in seconds, and fails with a clear message; it exists solely as a hang guard, while assertions pass on events.
- Concurrency tests must be deterministic and are run under the race detector (`make test`). Every goroutine a test starts is waited on before the test returns and the test ends with `goleak.VerifyNone(t)`; reserve `t.Parallel()` for tests that leave shared package state untouched (and note `goleak.VerifyNone` requires serial tests; use `VerifyTestMain` for parallel packages).
- Tests are fully self-contained and independent of execution order, map iteration order, random seeds (`rand.New(rand.NewSource(1))` if randomness is needed), environment variables, the working directory, network access, and the local time zone/locale. Tests that need infra are guarded so `make test` still passes without `make infra-up` (`t.Skip` when the dependency is unreachable, stated in the skip message).
- Stick to plain `testing` assertions and untagged test files; golden files only with a stated reason. If a test needs more than a screen of setup, the code under test is the problem.

## Standing rules

- Let `go mod tidy` manage `go.sum`; keep binaries, `vendor/`, and IDE files out of commits; run `go get -u ./...` only when explicitly asked.
- Start a goroutine only when you can name its stop condition.
- Add a mutex, channel, buffer, pool, or "optimization" only with a measurement or a stated invariant that requires it.
- Ignore an error with `_` only alongside a comment saying why.

## Commits / PRs

- Small, focused commits, imperative subject (`Add bounded worker pool to ingester`).
- Concurrency changes state the invariant protected and how goroutines terminate; performance changes include before/after `benchstat` output.
- Mention any new dependency and why it was needed.
