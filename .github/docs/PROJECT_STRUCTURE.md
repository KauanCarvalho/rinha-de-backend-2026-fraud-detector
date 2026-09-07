# Project structure

What each top-level directory and file is for, and why it's shaped this way.

```
.
├── cmd/
│   ├── api/
│   └── indexbuilder/
├── internal/
│   ├── config/
│   ├── domain/
│   ├── vectorize/
│   ├── knn/
│   ├── scoring/
│   ├── detector/
│   ├── dataset/
│   ├── httpapi/
│   └── observability/
├── e2e/
├── loadtest/
│   └── fixtures/
├── resources/
├── deployments/
├── data/                (gitignored, generated locally)
├── .github/
│   ├── README.md
│   ├── docs/
│   └── workflows/
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── .golangci.yml
├── go.mod / go.sum
├── info.json
└── LICENSE
```

## `cmd/`

Go convention: every `main` package lives under `cmd/<binary-name>/`. This
repo has two binaries, and they run at very different times:

- **`cmd/api`** — the actual runtime service. Loads config, loads the
  pre-built index from disk, starts the HTTP server, handles graceful
  shutdown. This is what `ENTRYPOINT` in the `Dockerfile` runs.
- **`cmd/indexbuilder`** — a build-time tool, not a server. It reads the
  official `references.json.gz`, quantizes and indexes every vector into a
  k-d tree, and serializes the result to `index.bin`. It runs once inside
  the Docker build (see the `indexer` stage in `Dockerfile`) or manually via
  `make index` for local development. `cmd/api` never parses the raw
  dataset itself — it only ever loads the binary this tool produces.

## `internal/`

Everything business-logic-shaped. `internal/` is a Go compiler-enforced
boundary: nothing outside this module can import these packages, which
keeps the actual API surface small and intentional. Each package has one
job:

| package | responsibility |
|---|---|
| `domain` | wire types for the `/fraud-score` request/response, matching the challenge's API spec exactly |
| `vectorize` | turns a request into the 14-dimensional normalized vector (`docs/en/DETECTION_RULES.md`'s formulas) |
| `knn` | the k-d tree itself: quantization, build (with quickselect), bounded search, gob (de)serialization — knows nothing about fraud, just nearest-neighbor search over fixed-size vectors |
| `scoring` | the tiny, pure `fraud_score`/`approved` decision from a vote count |
| `detector` | wires the three packages above into the one decision the HTTP layer needs — the seam that lets `vectorize`/`knn`/`scoring` be unit-tested in isolation and `detector` be tested as the full pipeline without touching HTTP |
| `dataset` | streaming parser for the `{"vector": [...], "label": "..."}` format shared by `references.json(.gz)` — used by both `cmd/indexbuilder` and tests |
| `httpapi` | routes (`GET /ready`, `POST /fraud-score`), logging/recover middleware, server wiring |
| `config` | env-var configuration with defaults — no config file, no flags library |
| `observability` | the one place `log/slog` gets configured |

## `e2e/`

Go tests (`//go:build e2e` tag, excluded from `go test ./...`) that hit a
**real, already-running** `docker compose` stack over the network — the LB,
both API replicas, the real built index. Run them with `make test-e2e`
after `make docker-up`. These catch anything a unit test can't: wiring
mistakes in `docker-compose.yml`, the HAProxy config, or the Dockerfile
itself.

## `loadtest/`

The official k6 scripts from `zanfranceschi/rinha-de-backend-2026`
(`test.js`, `smoke.js`, `k6-summary.js`, `docker-compose.yml`), vendored
here so performance can be measured locally without hunting down the
upstream repo. `loadtest/fixtures/test-data.json` is the official
pre-labeled test payload set (~25MB) — committed on purpose, so the repo is
self-contained.

## `resources/`

Official, static reference files, vendored and committed so the build
never depends on the network to fetch them (the same reasoning as
`loadtest/fixtures/`):

- **`references.json.gz`** — the actual reference dataset `cmd/indexbuilder`
  builds the k-d tree from.
- **`example-payloads.json`** — used by manual testing and `e2e/`.
- **`example-references.json`** — a small excerpt of the reference dataset
  format, per `docs/en/DATASET.md`, handy for quick inspection.

The two files `vectorize` actually depends on at build time
(`mcc_risk.json`, `normalization.json`) live under
`internal/vectorize/resources/` instead, `go:embed`-ded straight into the
binary — see that package's doc comment for why.

## `deployments/`

Infra config that isn't Go code and isn't the top-level compose file:
currently just `haproxy.cfg`, the plain round-robin, no-business-logic
load balancer config required by `docs/en/ARCHITECTURE.md`.

## `data/` (gitignored)

Where `make index` writes the built `index.bin` from
`resources/references.json.gz`, for local (non-Docker) development. Never
committed — it's regenerated on demand, and the Docker image builds its own
copy independently in the `indexer` stage.

## `.github/`

- **`.github/README.md`** — the repo's main page. `.github/`, the repo
  root, and `docs/` are the three locations GitHub recognizes for a
  repository's README, so this renders on the repo homepage exactly as if
  it were at the root — it just lives here instead, keeping the root
  listing to code and config only.
- **`.github/docs/`** — everything the README links out to instead of
  carrying itself: [`APPLICATION_FLOW.md`](APPLICATION_FLOW.md)
  (the full request/build journey, diagrammed, for non-experts),
  [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md)
  (architecture, resource budget, the Dockerfile explained),
  [`HOW_TO_RUN.md`](HOW_TO_RUN.md), [`RUNNING_FROM_GHCR.md`](RUNNING_FROM_GHCR.md),
  [`RESULTS.md`](RESULTS.md), and this file.
- **`.github/workflows/`** — `ci.yml` (vet, lint, unit tests, then a real
  containerized build + official smoke test) and `release.yml` (publishes
  the image to GHCR on a version tag).

## Root files

- **`Dockerfile`** — multi-stage: build the Go binaries, build the index
  from the vendored dataset (`resources/references.json.gz`, no network
  needed), then copy just the static binary and `index.bin` into a minimal
  `distroless` final image.
- **`docker-compose.yml`** — the full topology (HAProxy + 2 API replicas)
  within the challenge's 1 CPU / 350MB aggregate budget.
- **`Makefile`** — the single entry point for every workflow (`make help`
  lists them): build, test, lint, run, dataset/index generation, Docker,
  and load testing.
- **`.golangci.yml`** — the lint rule set (`golangci-lint run`, wired into
  CI).
- **`info.json`** / **`LICENSE`** — required by `docs/en/SUBMISSION.md` for
  participation (MIT license, participant metadata).
