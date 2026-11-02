# Development, Verification & Observability

All build, test, lint, and runtime observability workflows in `omnic` are driven exclusively through `make`.

---

## Make Commands Reference

| Target | Description |
| :--- | :--- |
| `make help` | List all available targets with descriptions. |
| `make install-tools`| Download dependencies and compile pinned Go tools (`golangci-lint`, `gofumpt`, `arch-go`, `nilaway`, `dlv`, `sqlc`). |
| `make all` | Compile both emulator server and handset binaries (`build/omnic` and `build/omnic-handset`). |
| `make build` | Compile static release binary `build/omnic` (`CGO_ENABLED=0`). |
| `make build-debug` | Build unoptimized server binary with symbols (`-N -l`) and pinned `dlv` for VS Code debugging. |
| `make build-handset`| Compile static binary `build/omnic-handset`. Alias: `make build-phone`. |
| `make test` | Run complete test suite with Go race detector enabled (`CGO_ENABLED=1 go test -race -count=1 ./...`). |
| `make qa` | Run full hermetic static quality gate in order: `fix` &rarr; `fmt` &rarr; `vet` &rarr; `lint` &rarr; `nilaway` &rarr; `arch-go` &rarr; `loc` &rarr; `fieldalignment`. |
| `make fix` | Modernize Go source with `go fix` modernizers. |
| `make fmt` | Format entire codebase using `gofumpt`. |
| `make lint` | Run `golangci-lint` with strict project rules. |
| `make nilaway` | Run Uber's `nilaway` nil-pointer dereference analysis. |
| `make arch-go` | Enforce hexagonal architecture and complexity caps from `arch-go.yml`. |
| `make loc` | Verify no Go source file exceeds 1,500 lines of code (`scripts/loc.sh`). |
| `make bench` | Run memory and CPU benchmarks (`build/bench/$(NAME).txt`). |
| `make bench-compare`| Compare baseline benchmark against current run via `benchstat`. |
| `make profile` | Profile server under load and capture goroutine/mutex/heap/cpu profiles to `build/profile/`. |
| `make infra-build` | Build local Docker Compose containers with layer caching. |
| `make infra-up` | Spin up local stack: `omnic`, Prometheus, and Grafana in the background. |
| `make infra-down` | Stop local observability stack (preserves volumes). |
| `make clean` | Remove `build/` artifacts. |

---

## Static Quality Pipeline (`make qa`)

The hermetic QA pipeline executes in strict dependency order:

1. **`make fix`**: Runs `go fix` with modernizers bounded by Go 1.27.1.
2. **`make fmt`**: Formats rewritten source via `gofumpt` so line numbers remain deterministic.
3. **`make vet`**: Standard compiler static checks (`go vet`).
4. **`make lint`**: Comprehensive checks via `golangci-lint` (deadcode, error checking, unused variables, styling).
5. **`make nilaway`**: Advanced nil-panic static analysis across all call paths.
6. **`make arch-go`**: Layer boundary enforcement (cmd &rarr; handler &rarr; service &rarr; repository &rarr; domain) and complexity limits (max 5 params, max 3 return values, max 80 lines/function).
7. **`make loc`**: File size guard enforcing $\le 1500$ lines of executable code per file.
8. **`make fieldalignment`**: Struct memory layout optimization to prevent alignment padding waste.

---

## Observability & Local Monitoring Stack

A production-grade observability environment is bundled via Docker Compose:

```bash
# Launch omnic, Prometheus, and Grafana
make infra-up

# Tear down observability stack
make infra-down
```

### Stack Endpoints

| Service | Host URL | Description |
| :--- | :--- | :--- |
| **Omnic API** | `http://localhost:8090` | Container mapped from internal port `:8080`. |
| **Metrics & pprof** | `http://127.0.0.1:6060` | Prometheus `/metrics` and Go `/debug/pprof/`. |
| **Prometheus** | `http://127.0.0.1:9090` | Scrapes `omnic` every 5 seconds. |
| **Grafana** | `http://127.0.0.1:3000` | Pre-provisioned dashboards with anonymous admin access (no login required). |

### Included Grafana Dashboards
- **Go Runtime**: Goroutine counts, memory allocation rate, GC pause distribution, thread allocation.
- **HTTP Transport**: Echo request rates, error codes (4xx/5xx), p95/p99 latency percentiles.

---

## Profiling & Performance Analysis

Capture full pprof profiles under realistic load:

```bash
make profile
```

This captures:
- CPU profile (samples under active load)
- Heap memory allocation & in-use objects
- Mutex contention & block profiles
- Goroutine leak profile (`/debug/pprof/goroutineleak` checking unreachable blocked goroutines)

Profiles and summary reports are persisted to `build/profile/summary.txt`.
