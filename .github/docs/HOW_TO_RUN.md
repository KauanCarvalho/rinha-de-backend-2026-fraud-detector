# How to run

## Docker (the full stack)

```sh
make docker-up      # builds the image (builds the index from the vendored dataset) and starts the stack
curl -X POST localhost:9999/fraud-score -d @resources/example-payloads.json  # (send one at a time; see the API docs)
make docker-down
```

The reference dataset (`resources/references.json.gz`) is vendored and
committed, same as `loadtest/fixtures/test-data.json` — the build never
needs network access to fetch it. See
[`INFRASTRUCTURE.md`](INFRASTRUCTURE.md) for the full Dockerfile rationale.

Want to run it on another machine using **only the published image** — no
clone, no build, exactly what the challenge's own `submission` branch does?
See [`RUNNING_FROM_GHCR.md`](RUNNING_FROM_GHCR.md).

## Without Docker, for local development

```sh
make index          # builds ./data/index.bin from resources/references.json.gz
make run            # starts the API on :9999 with INDEX_PATH=data/index.bin
```

## Tests

```sh
make test           # unit tests, table-driven, testify assertions, -race -cover
make docker-up
make test-e2e       # Go tests against the real running stack
make lint           # golangci-lint
```

`make ci` runs the full local quality gate (fmt, vet, lint, test) in one
go — the same checks `.github/workflows/ci.yml` runs on every push.

## Load testing (official k6 scripts, vendored under `loadtest/`)

```sh
make docker-up
make load-smoke     # official smoke.js — quick sanity check
make load-test      # official test.js — ramps to 1200 req/s over 120s, writes loadtest/test/results.json
```

`stress-test` is an alias for `load-test`, if that's the name your muscle
memory reaches for.

Note: `resources/references.json.gz` is the real 3,000,000-vector official
dataset — its checksum matches `loadtest/fixtures/test-data.json`'s
`references_checksum_sha256` exactly. (The file published on the challenge
repo's `main` branch is a smaller 1M-vector one that doesn't match; the
correct 3M file lives on that repo's `tmp` branch instead. See
[`RESULTS.md`](RESULTS.md#the-dataset-it-really-is-3000000-vectors) for how
that was found.) With the right dataset, `results.json`'s detection numbers
reflect actual k-NN search quality, not an answer-key mismatch.

## Configuration (env vars, see `internal/config`)

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `9999` | HTTP listen port |
| `INDEX_PATH` | `data/index.bin` | path to the pre-built IVF index |
| `LOG_LEVEL` | `info` | `debug`\|`info`\|`warn`\|`error` |
| `KNN_NPROBE` | `8` | number of IVF clusters probed per search — see [`RESULTS.md`](RESULTS.md#replacing-the-k-d-tree-with-ivf) |
| `READ_TIMEOUT` / `WRITE_TIMEOUT` / `SHUTDOWN_TIMEOUT` | `5s` each | HTTP server timeouts |

Every default is sane enough that the container runs correctly with **no
environment variables set at all** — see the Dockerfile section of
[`INFRASTRUCTURE.md`](INFRASTRUCTURE.md).
