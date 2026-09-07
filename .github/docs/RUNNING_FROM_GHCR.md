# Running the stack from the published image only

This shows how to run the full stack (HAProxy + 2 API replicas) on any
machine with just Docker installed — **no `git clone`, no `go build`, no
Dockerfile** — pulling everything from the publicly published image on
GHCR. This is deliberately the same shape as the challenge's own
`submission` branch (`docs/en/SUBMISSION.md`): only a `docker-compose.yml`
that references a public image, nothing else — useful here for running it
somewhere that isn't this git repository at all.

## Prerequisites

- Docker + Docker Compose. That's it.
- The image must actually be public — see the note at the bottom before
  assuming a `denied`/`401` pull error means something is broken on *your*
  end.

## 1. Get the image reference

Published by `.github/workflows/release.yml` on every version tag:

```
ghcr.io/kauancarvalho/rinha-de-backend-2026-fraud-detector:latest
```

Prefer pinning a specific tag (or better, the immutable manifest digest —
`docker buildx imagetools inspect <ref>` prints it) over `:latest` for
anything you'd want to reproduce later.

## 2. Save these two files in an empty directory

**`docker-compose.yml`** — identical topology and resource budget as the
real one, just `image:` instead of `build:`:

```yaml
services:
  haproxy:
    image: haproxy:3.0-alpine
    ports:
      - "9999:9999"
    volumes:
      - ./haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro
    depends_on:
      - api1
      - api2
    ulimits:
      nofile:
        soft: 65535
        hard: 65535
    deploy:
      resources:
        limits:
          cpus: "0.05"
          memory: "25MB"

  api1:
    image: ghcr.io/kauancarvalho/rinha-de-backend-2026-fraud-detector:latest
    environment:
      PORT: "9999"
      LOG_LEVEL: "info"
      KNN_MAX_EXTRA_LEAVES: "2000"
      GOMEMLIMIT: "145MiB"
    ulimits:
      nofile:
        soft: 65535
        hard: 65535
    deploy:
      resources:
        limits:
          cpus: "0.475"
          memory: "160MB"

  api2:
    image: ghcr.io/kauancarvalho/rinha-de-backend-2026-fraud-detector:latest
    environment:
      PORT: "9999"
      LOG_LEVEL: "info"
      KNN_MAX_EXTRA_LEAVES: "2000"
      GOMEMLIMIT: "145MiB"
    ulimits:
      nofile:
        soft: 65535
        hard: 65535
    deploy:
      resources:
        limits:
          cpus: "0.475"
          memory: "160MB"
```

**`haproxy.cfg`** — plain round-robin, no request inspection (copied as-is
from [`deployments/haproxy.cfg`](../../deployments/haproxy.cfg)):

```
global
    maxconn 4096

defaults
    mode http
    timeout connect 3s
    timeout client  5s
    timeout server  5s
    option httplog

frontend fraud_score_front
    bind *:9999
    default_backend fraud_score_back

backend fraud_score_back
    balance roundrobin
    option httpchk GET /ready
    server api1 api1:9999 check inter 2s fall 3 rise 2
    server api2 api2:9999 check inter 2s fall 3 rise 2
```

## 3. Run it

```sh
docker compose up -d
curl -s http://localhost:9999/ready
curl -s -X POST http://localhost:9999/fraud-score \
  -H 'Content-Type: application/json' \
  -d '{
        "id": "tx-1329056812",
        "transaction": {"amount": 41.12, "installments": 2, "requested_at": "2026-03-11T18:45:53Z"},
        "customer": {"avg_amount": 82.24, "tx_count_24h": 3, "known_merchants": ["MERC-003", "MERC-016"]},
        "merchant": {"id": "MERC-016", "mcc": "5411", "avg_amount": 60.25},
        "terminal": {"is_online": false, "card_present": true, "km_from_home": 29.23},
        "last_transaction": null
      }'
docker compose down
```

No Go toolchain, no dataset download, no build step on this machine — the
image already has `index.bin` baked in from when it was built.

## If the pull fails

GHCR packages pushed with a workflow's default `GITHUB_TOKEN` are created
**private** even when the source repository is public — a genuinely common
gotcha (the challenge's own `docs/en/FAQ.md` lists "leaving the Docker
image referenced in your `docker-compose.yml` private" as a common
mistake). If `docker pull`/`docker compose up` fails with `denied` or
`401`, the fix lives on the publisher's side: go to the package's GitHub
settings and set visibility to **Public**. See the note at the top of
[`release.yml`](../workflows/release.yml) for the exact URL.
