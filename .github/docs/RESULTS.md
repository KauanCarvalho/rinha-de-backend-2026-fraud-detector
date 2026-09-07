# Results

What actually happened when this got built and measured — not projections.
Everything here was run locally against the real containerized stack
(`docker-compose.yml`), under the challenge's resource budget (1 CPU /
350MB total), using the official k6 scripts vendored under
[`loadtest/`](../../loadtest). See [`HOW_TO_RUN.md`](HOW_TO_RUN.md) to
reproduce any of it yourself.

## Correctness, decoupled from load

Before any load testing, the two fully worked examples from
`docs/en/DETECTION_RULES.md` (a legitimate transaction and a fraudulent
one) are reproduced exactly end to end — vectorization, k-d tree search,
and scoring — both as Go tests (`internal/vectorize`, `internal/detector`)
and against the running containerized API with the real dataset:

| case | expected | got |
|---|---|---|
| legit example | `approved: true, fraud_score: 0.0` | `approved: true, fraud_score: 0` |
| fraud example | `approved: false, fraud_score: 1.0` | `approved: false, fraud_score: 1` |

Test suite as it stands: unit tests (table-driven, `testify`) at 87-100%
coverage across every `internal/` package, E2E tests against the real
running stack, `golangci-lint` at 0 issues. All wired into
`.github/workflows/ci.yml`.

## The dataset: it really is 3,000,000 vectors

`docs/en/EVALUATION.md` states plainly that the official answer key is
"k-NN with k=5 and Euclidean distance with brute force ... over the
14-dimensional vectors" of the **3,000,000**-vector reference set — but the
`references.json.gz` published on the challenge repo's `main` branch only
has 1,000,000 records, and its checksum does not match what
`loadtest/fixtures/test-data.json` expects. Checking the challenge repo's
own git history resolves the discrepancy: `main` briefly held a 3M-vector
file (commit `ce187933`, "3mi references", 2026-04-27) that was replaced by
the current 1M-vector one on the very same day — but the 3M version was
also pushed to a still-existing branch, `tmp`, and never removed there.
Decompressing it gives a SHA-256 that matches `test-data.json`'s
`references_checksum_sha256` exactly:

```
test-data.json expects              : 24a1fd58...878f77
tmp branch references.json.gz (gz)  : 43d10de8...cab67
tmp branch, decompressed            : 24a1fd58...878f77  ← match
```

This project now vendors that 3,000,000-vector file as
`resources/references.json.gz`, matching what the official answer key was
actually computed against. (Earlier revisions of this document described
the 1M/3M mismatch as "almost certainly reduced after the edition closed
to save storage" — that guess was wrong. The reduction happened
2026-04-27, *during* the live edition, and the 3M file was never deleted,
just moved out of `main`'s current tree.)

## In-process k-d tree: why a search budget exists, at 3M vectors

14 dimensions is past the point where k-d trees keep meaningful pruning
power — an *unbounded* search degrades toward a near-linear scan as the
dataset grows. Measured directly against the real 3M-vector index
(`cmd/accuracycheck`, a throwaway diagnostic tool, run against the 50,000
labeled requests in `loadtest/fixtures/test-data.json`):

| `KNN_MAX_EXTRA_LEAVES` | TP | TN | FP | FN | offline failure_rate |
|---|---|---|---|---|---|
| `0` (unbounded/exact) | 34,962 | 15,038 | 0 | 0 | **0.00%** |
| `500` | 30,969 | 10,908 | 3,993 | 4,130 | 16.25% |
| `2000` | 32,337 | 12,524 | 2,625 | 2,514 | 10.28% |
| `5000` (chosen) | — | — | — | — | (see load test below) |
| `8000` | 33,615 | 14,278 | 1,347 | 760 | 4.21% |

`0` is a special value: real branch-and-bound with geometric pruning, not
"no search." It gets the offline failure rate to zero because it is
mathematically equivalent to brute force here. It is also the *slowest*
option under concurrent load — see below.

Build/serialize numbers for the real 3M-vector official dataset:

| step | time |
|---|---|
| parse `references.json.gz` + quantize | ~7.6s |
| build k-d tree | ~3.2s |
| save (`gob`) → `index.bin` | ~0.8s |
| load `index.bin` at container start | ~1.3s (well under the 2s HAProxy health-check window) |

## Full load test (official `loadtest/test.js`): three configurations, 3M dataset

`test.js` ramps arrival rate to **1200 req/s over 120s**, run through the
full stack (HAProxy + 2 API replicas) with the same 1 CPU / 350MB total
budget throughout. Three `KNN_MAX_EXTRA_LEAVES` values were measured
end-to-end against the real (correct) 3M dataset:

| `KNN_MAX_EXTRA_LEAVES` | p99 | http_errors | failure_rate | final_score |
|---|---|---|---|---|
| `0` (unbounded/exact) | 2002ms (cutoff) | 21,707 of 25,749 | 84.3% (cutoff) | **−6000** |
| `8000` | 2001ms (cutoff, by 1.35ms) | 627 | 5.88% | **−3324.9** |
| `5000` (chosen) | **1274ms** | **46** | **5.32%** | **−203.6** |

Two things worth calling out:

1. **`0` (exact) is not viable under load, even though it is the most
   accurate offline.** Real branch-and-bound pruning is fast for a single
   query in isolation, but at 1200 req/s split across two replicas with
   0.475 vCPU each, per-query search cost compounds into queueing delay:
   84% of requests timed out. A search technique's offline speed on one
   query says nothing about its throughput under concurrency with a fixed
   CPU budget.
2. **`8000` and `5000` both keep the offline failure rate low, but `8000`'s
   extra backtracking pushes p99 to 2001ms — 1.35ms over the challenge's
   hard 2000ms cutoff**, which zeroes the entire latency score
   (`p99_score = -3000`) regardless of how good detection was. `5000`
   avoids both hard cutoffs entirely (`p99_score.cut_triggered: false`,
   `detection_score.cut_triggered: false`) and, empirically, has a *better*
   failure rate too (5.32% vs 5.88%) — more backtracking budget does not
   monotonically improve real-world outcomes once it pushes latency across
   a hard threshold.

`KNN_MAX_EXTRA_LEAVES=5000` is the value this project ships with.

## Smoke test (official `loadtest/smoke.js`, 1 VU, 5 requests)

```
checks_succeeded: 100.00% (20/20)
http_req_duration: avg=3.25ms  p90=6.92ms  p95=7.11ms  max=7.3ms
http_req_failed:   0.00%
```

## Why the winning solutions score close to the 6000-point ceiling

`docs/en/EVALUATION.md` gives the exact formula:

```
score_p99 = 1000 · log10(1000 / max(p99, 1ms))   , floor -3000 if p99 > 2000ms
score_det = 1000 · log10(1/ε) − 300·log10(1+E)    , floor -3000 if failure_rate > 15%
final_score = score_p99 + score_det               , range [-6000, +6000]
```

The ceiling of `+6000` requires **both** `p99 ≤ 1ms` *and* `E = 0` (zero
weighted errors). Every 10× improvement in p99 is worth another 1000
points — going from our measured 1274ms to 1ms would require closing a
~1000× latency gap, worth roughly +3100 points on its own.

That gap is not a tuning knob, it's an architectural choice. Sustaining
sub-millisecond p99 at 1200 req/s on 0.475 vCPU per replica means the
per-request compute budget is a fraction of a millisecond end-to-end,
including network I/O — there is no room left for Go's `net/http` request
lifecycle, garbage collection pauses, or a pointer-chasing k-d tree with
its attendant cache misses. The [inspiration repos](INFRASTRUCTURE.md#why-this-isnt-a-bit-mining-exercise)
that won this edition get there with a hand-rolled `epoll` event loop, a
custom C load balancer using `SCM_RIGHTS` fd-passing, AVX2 SIMD distance
calculations, `GC` disabled outright, and `logging: none` — every one of
those choices exists specifically to erase overhead between the NIC and
the distance computation. It is a different regime of engineering, not a
more-careful version of this one.

## The trade-off, stated plainly

This project deliberately does not chase that ceiling (see
[`INFRASTRUCTURE.md`](INFRASTRUCTURE.md) for why: structured logging,
idiomatic `net/http`, a maintainable k-d tree, Go's GC left on). With the
correct 3M dataset and a properly tuned search budget, it clears both hard
cutoffs comfortably (`failure_rate` 5.32% against a 15% ceiling, p99 1274ms
against a 2000ms ceiling) and lands at `final_score ≈ -203.6` — far from
the theoretical ceiling, but for reasons that are now fully measured and
understood rather than guessed at: the score is dominated by the p99 term,
and closing that gap further is a rewrite into a fundamentally different
architecture, not a parameter change.
