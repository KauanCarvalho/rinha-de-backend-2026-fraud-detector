# How to run

## Docker (the full stack)

```sh
make docker-up      # builds the image (builds the index from the vendored dataset) and starts the stack
curl -X POST localhost:9999/fraud-score -d @resources/example-payloads.json  # (send one at a time; see the API docs)
make docker-down
```

The reference dataset (`resources/references.json.gz`) is vendored and
committed, same as `loadtest/fixtures/test-data.json` — the build never
needs network access to fetch it (`make update-dataset` refreshes it from
upstream if it ever changes). See
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
make load-data      # optional: refresh loadtest/fixtures/test-data.json from upstream
```

`stress-test` is an alias for `load-test`, if that's the name your muscle
memory reaches for.

Note: `loadtest/fixtures/test-data.json` embeds a
`references_checksum_sha256` for the dataset its expected answers were
computed against, and it does not match the dataset currently published at
`resources/references.json.gz` (confirmed by checksum — the upstream
dataset was almost certainly reduced after the edition closed). Expect the
`results.json` detection score to reflect that mismatch rather than a flaw
in this project's k-NN search; the latency/throughput numbers are
unaffected by it. See [`RESULTS.md`](RESULTS.md) for what these actually
measured.

## Configuration (env vars, see `internal/config`)

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `9999` | HTTP listen port |
| `INDEX_PATH` | `data/index.bin` | path to the pre-built k-d tree |
| `LOG_LEVEL` | `info` | `debug`\|`info`\|`warn`\|`error` |
| `KNN_MAX_EXTRA_LEAVES` | `2000` | k-d tree backtracking budget (0 = unbounded/exact) |
| `READ_TIMEOUT` / `WRITE_TIMEOUT` / `SHUTDOWN_TIMEOUT` | `5s` each | HTTP server timeouts |

Every default is sane enough that the container runs correctly with **no
environment variables set at all** — see the Dockerfile section of
[`INFRASTRUCTURE.md`](INFRASTRUCTURE.md).
