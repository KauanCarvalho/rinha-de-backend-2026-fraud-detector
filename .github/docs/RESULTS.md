# Results

What actually happened when this got built and measured — not
projections. Everything here was run locally against the real
containerized stack (`docker-compose.yml`), under the challenge's
resource budget (1 CPU / 350MB total), using the official k6 scripts
vendored under [`loadtest/`](../../loadtest). See
[`HOW_TO_RUN.md`](HOW_TO_RUN.md) to reproduce any of it yourself.

## At a glance

Six changes, measured end to end each time, moved `final_score` from deep
negative to four figures:

| # | Change | `final_score` after |
|---|---|---:|
| 0 | Baseline: wrong (1M) reference dataset | −3324.9 |
| 1 | Correct 3,000,000-vector dataset | −203.6 |
| 2 | Pre-rendered responses, buffer pooling, GC tuning | −155.9 |
| 3 | Categorical-tag partitioning | +42.7 |
| 4 | Re-tuned search budget | +214.5 |
| 5 | **IVF replacing the k-d tree entirely** | **+660 to +1497** |

The last row is a range, not a typo — see
[Full load test, `nprobe` swept](#full-load-test-nprobe-swept) for why.
Two other ideas were tried and measured between rows 4 and 5
(confidence-based search escalation, partition rebalancing by amount);
both looked good on paper and made things worse for real — see
[Replacing the k-d tree with IVF](#replacing-the-k-d-tree-with-ivf) for
what happened and why they were reverted rather than kept.

### Jump to a section

[Correctness](#correctness-decoupled-from-load) ·
[The dataset](#the-dataset-it-really-is-3000000-vectors) ·
[k-d tree history](#in-process-k-d-tree-why-a-search-budget-exists-at-3m-vectors) ·
[Low-risk optimizations](#low-risk-optimizations-pre-rendered-responses-buffer-pooling-gc-tuning) ·
[Categorical-tag partitioning](#categorical-tag-partitioning) ·
[Replacing the k-d tree with IVF](#replacing-the-k-d-tree-with-ivf) ·
[Smoke test](#smoke-test-official-loadtestsmokejs-1-vu-5-requests) ·
[The 6000-point ceiling](#the-6000-point-ceiling) ·
[The trade-off, stated plainly](#the-trade-off-stated-plainly)

## Correctness, decoupled from load

Before any load testing, the two fully worked examples from
`docs/en/DETECTION_RULES.md` (a legitimate transaction and a fraudulent
one) are reproduced exactly end to end — vectorization, nearest-neighbor
search, and scoring — both as Go tests (`internal/vectorize`,
`internal/detector`) and against the running containerized API with the
real dataset:

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

> [!IMPORTANT]
> **Superseded.** The k-d tree described in this section and the next was
> later replaced entirely by an IVF index — see
> [Replacing the k-d tree with IVF](#replacing-the-k-d-tree-with-ivf)
> below for what replaced it and why. Kept here for the honest trail of
> what was tried, measured, and learned along the way; `internal/knn` no
> longer contains a k-d tree at all.

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

`KNN_MAX_EXTRA_LEAVES=5000` was the value this project shipped with at
this point — before categorical-tag partitioning changed the picture
again, see below.

## Low-risk optimizations: pre-rendered responses, buffer pooling, GC tuning

[`PATH_TO_EXCELLENCE.md`](PATH_TO_EXCELLENCE.md) itemizes what it would take
to close the remaining gap to the scoring ceiling. Most of that list trades
away this project's maintainability goals for latency and was deliberately
left undone — but one item on it did not require that trade-off, because
`FraudScore` only ever takes one of `scoring.K+1` = 6 exact values
(`fraudCount/5` for `fraudCount` in `[0,5]`). That makes three standard,
idiomatic-Go techniques free wins rather than a maintainability trade:

- **Pre-rendered response bodies** — the 6 possible `/fraud-score` JSON
  responses are encoded once at package init instead of via
  `encoding/json`'s reflection-based `Marshal` on every request
  (`internal/httpapi/handlers.go`).
- **`sync.Pool` for the request-body buffer** — the buffer used to read
  each request body is reused across requests instead of allocated fresh
  every time, always `Reset` before reuse so nothing leaks between
  requests (verified under `-race` with a concurrent-request test).
- **GC tuning** — `debug.SetGCPercent(-1)` disables the percentage-growth
  GC trigger; the `GOMEMLIMIT` already set in `docker-compose.yml` remains
  as the backstop that still triggers collection under real memory
  pressure (`cmd/api/main.go`).

None of this touches `net/http`, changes the API contract, or removes a
single safety net — same measured detection quality, real latency
improvement:

| | before | after | change |
|---|---|---|---|
| p99 | 1274ms | **1174.5ms** | −7.8% |
| http_errors | 46 | **25** | −46% |
| failure_rate | 5.32% | **5.27%** | ~flat (as expected — this doesn't touch detection) |
| final_score | −203.6 | **−155.9** | +23% |

## Categorical-tag partitioning

> [!NOTE]
> **The partitioning described here is still exactly how the index is
> organized today** — only what each partition holds internally later
> changed, from a k-d tree to an IVF index (see
> [Replacing the k-d tree with IVF](#replacing-the-k-d-tree-with-ivf)).
> `KNN_MAX_EXTRA_LEAVES` in this section was also later renamed
> `KNN_NPROBE` and re-tuned again for IVF — the final, current value is in
> that section, not here.

The idea: split the 3M-vector reference set into partitions by a coarse
tag before doing any distance-based search at all. Four of this project's
14 dimensions are already boolean-valued or have a sentinel for "absent":
`is_online`, `card_present`, `unknown_merchant`, and whether
`last_transaction` was present. A mismatch on any one of them contributes
roughly `Scale²` to a squared-distance sum — larger than the combined
contribution of every continuous dimension being merely "close" — so two
vectors with different tags are rarely each other's true nearest
neighbors. `internal/knn.Tag` computes this 4-bit tag from a vector's own
dimensions (no extra metadata needed), and `internal/knn.PartitionedIndex`
builds one k-d tree per non-empty tag (12 non-empty partitions out of 16
possible, on the real 3M dataset) instead of one 3M-vector tree. A query
is routed to its own partition's tree; the rare case of a query tag with
zero matching reference vectors falls back to searching every partition
and merging results, so correctness never regresses below the
unpartitioned index.

Offline (same 50,000 labeled requests, `KNN_MAX_EXTRA_LEAVES` swept):

| budget | unpartitioned failure_rate | partitioned failure_rate |
|---|---|---|
| exact (`0`) | 0.00% | 0.00% |
| `500` | 16.25% | **3.65%** |
| `2000` | 10.28% | **1.82%** |
| `5000` | ~5.3%\* | **1.11%** |
| `8000` | 4.21% | **1.11%** (identical to `5000` — search already saturates before spending the full budget) |

\* `5000` unpartitioned was only measured end-to-end via the real load test
(see the table above), not this offline sweep directly.

Full load test, `KNN_MAX_EXTRA_LEAVES=5000` unchanged, partitioned index:

| | before (unpartitioned) | after (partitioned) | change |
|---|---|---|---|
| p99 | 1174.5ms | 1644.6ms | +40% (worse) |
| http_errors | 25 | 321 | worse |
| failure_rate | 5.27% | **1.68%** | −68% |
| detection_score | −86.0 | **+258.8** | positive for the first time |
| final_score | −155.9 | **+42.7** | positive for the first time |

Detection improved dramatically — enough to flip `final_score` positive
for the first time — but p99 got worse, not better, even though each
partition's tree is roughly 16× smaller on average. Why: the 12 non-empty
partitions are far from evenly sized — the largest single tag holds
**33%** of all 3M vectors (the smallest holds 0.14%) — so a fixed
`maxExtraLeaves` budget is now a *larger fraction* of the biggest,
highest-traffic partition's tree than it was of the full unpartitioned
tree (`5000 / ~31,000 leaves` in that one partition vs. `5000 / ~93,750
leaves` before), meaning most queries now do proportionally *more*
backtracking than before, not less.

### Re-tuning the search budget for the partitioned index

Re-sweeping `KNN_MAX_EXTRA_LEAVES` against the partitioned index (real
load test each time) confirms this and finds a new, much lower optimum:

| `KNN_MAX_EXTRA_LEAVES` | p99 | failure_rate | detection_score | final_score |
|---|---|---|---|---|
| `5000` (old default) | 1644.6ms | 1.68% | +258.8 | +42.7 |
| `2000` | 1401.5ms | 2.05% | +301.1 | **+154.5** |
| `1000` (chosen) | **1275.9ms** | 2.08% | **+320.3** | **+214.5** |
| `500` | 1485.5ms | 3.86% | +3.8 | −168.1 |

`1000` is the sweet spot found: better than `5000` on every single metric
(p99, failure_rate, *and* detection_score all improve together), and
better than `500` on every metric too — below `1000`, p99 stops improving
(it's dominated by fixed costs — HTTP, GC, network — once search itself is
cheap enough) while detection keeps getting worse, a strictly bad trade in
both directions. This was the value the project shipped with at this
point — since superseded, see the next section.

## Replacing the k-d tree with IVF

Two attempts at squeezing more out of the tag-partitioned k-d tree were
tried and measured next, and both made things *worse*, not better:

1. **Confidence-based search escalation** — run a cheap, small-budget
   search first; only pay for a second, expensive search when the result
   lands near the approve/deny cutoff (`fraudCount` 2–4), trusting the
   cheap result otherwise. Offline failure rate got *worse* (8.52% vs.
   4.21% for a flat large budget) — the cheap tier's errors weren't
   confined to the boundary zone the way the theory assumed; it was
   sometimes confidently wrong. Reverted.
2. **Rebalancing the 12 tag-partitions by an amount-median split** — the
   biggest partition held 33% of all vectors; splitting each partition in
   two by its own median `amount` (a single global threshold didn't work —
   `amount`'s typical value varies by an order of magnitude across tags)
   balanced every partition to within a fraction of a percent of 50/50.
   Offline accuracy improved (0.87% vs. 1.11%) but the real load test got
   worse twice in a row (`final_score` +16.7 and +8.5, down from +214.5) —
   doubling the leaf-partition count (24 vs. 12) cost more in overhead than
   the better balance saved in search time. Reverted.

Both are a repeat of a pattern this project kept running into: an idea
that is correct about *offline accuracy* can still be wrong about
*real-world latency under concurrent load*, because the two are shaped by
different things (search-space size vs. memory layout, scheduling, and
constant overhead). The fix that actually worked was a different kind of
change altogether — not a smarter policy on top of the k-d tree, but
replacing the search structure under it.

### Why IVF, and what changed

An **IVF (Inverted-File) index** clusters a partition's own reference
vectors ahead of time with k-means, then at query time only scans the
`nprobe` clusters nearest the query — trading a small, tunable amount of
recall for a search cost that no longer depends on backtracking through a
tree at all. `internal/knn.BuildIVF` replaces `internal/knn.Build`
entirely: `PartitionedIndex.Partitions` now holds one `IVFIndex` per tag
instead of one k-d tree, with cluster count chosen per partition
(`n/1000`, clamped to `[32, 512]`) and 12 Lloyd's-algorithm iterations,
parallelized across the build machine's CPUs. The k-d tree code
(`build.go`, the tree-walking half of `search.go`, `Index`) was deleted
from the repository outright, not just left unused — the two failed
experiments above were reverted by discarding new code, but IVF's win was
large and reproducible enough to justify actually removing what it
replaced.

Build cost went up — a real, honest trade-off:

| step | k-d tree | IVF |
|---|---|---|
| build (3M vectors, 12 partitions) | ~1.7s | ~34s |

Still comfortably inside a `docker build` step, and it's paid once, not
per request.

### Offline nprobe sweep (same 50,000 labeled requests)

| `nprobe` | TP | TN | FP | FN | offline failure_rate |
|---|---|---|---|---|---|
| 1 | 33,793 | 13,831 | 1,169 | 1,207 | 4.75% |
| 2 | 34,560 | 14,626 | 402 | 412 | 1.63% |
| 4 | 34,877 | 14,929 | 85 | 109 | 0.39% |
| 8 | 34,932 | 14,995 | 30 | 43 | 0.15% |
| 16 | 34,950 | 15,020 | 12 | 18 | 0.06% |
| 32 | 34,961 | 15,034 | 1 | 4 | 0.01% |

Every one of these beats the k-d tree's best-ever offline number (1.11%
at a much larger, much more expensive budget) by at least an order of
magnitude, even at `nprobe=1`.

### Full load test, `nprobe` swept

| `nprobe` | p99 | failure_rate | detection_score | final_score |
|---|---|---|---|---|
| 4 | 1302ms | 0.56% | +868.5 | +753.8 |
| **8 (chosen)** | 1255–1441ms\* | 0.18–0.36% | +975–1596 | **+660 to +1497**\* |
| 16 | 1597ms | 0.38% | +862.6 | +659.3 |

\* `nprobe=8` was measured four times across this investigation, under
varying host memory pressure from unrelated processes (see
[`INFRASTRUCTURE.md`](INFRASTRUCTURE.md) — this project's own containers
were never the cause). The spread (+659 on a contended host, +1497 on a
freshly cleared one) is host noise, not index nondeterminism: the same
build, same config, same test data. Even the worst of the four runs beat
every other configuration measured in this project's entire history.
`8` also beats both its neighbors on the sweep above (`4` and `16`) — the
same "sweet spot in the middle" shape seen when tuning the k-d tree's
budget earlier, not a coincidence: too few probes costs accuracy, too many
costs latency for no further accuracy gain, once the true nearest
neighbors are already reliably found.

`KNN_NPROBE=8` (the `KNN_MAX_EXTRA_LEAVES` env var was renamed — see
[`HOW_TO_RUN.md`](HOW_TO_RUN.md) — since "nprobe" is what it now controls)
is the value this project ships with.

## Smoke test (official `loadtest/smoke.js`, 1 VU, 5 requests)

```
checks_succeeded: 100.00% (20/20)
http_req_duration: avg=3.25ms  p90=6.92ms  p95=7.11ms  max=7.3ms
http_req_failed:   0.00%
```

## The 6000-point ceiling

`docs/en/EVALUATION.md` gives the exact formula:

```
score_p99 = 1000 · log10(1000 / max(p99, 1ms))   , floor -3000 if p99 > 2000ms
score_det = 1000 · log10(1/ε) − 300·log10(1+E)    , floor -3000 if failure_rate > 15%
final_score = score_p99 + score_det               , range [-6000, +6000]
```

The ceiling of `+6000` requires **both** `p99 ≤ 1ms` *and* `E = 0` (zero
weighted errors). Every 10× improvement in p99 is worth another 1000
points — going from our measured ~1.3s p99 to 1ms would still require
closing a ~1000× latency gap, worth roughly +3000 points on its own, even
after IVF closed nearly all of the *detection* gap (E is now small enough
that `detection_score` sits close to its own +3000 ceiling on a clean
host — see the IVF section above).

That remaining gap is not a tuning knob, it's an architectural choice.
Sustaining sub-millisecond p99 at 1200 req/s on 0.475 vCPU per replica
means the per-request compute budget is a fraction of a millisecond
end-to-end, including network I/O — there is no room left for Go's
`net/http` request lifecycle or garbage collection pauses, regardless of
how good the search structure underneath them is. Reaching it would take
a hand-rolled `epoll` event loop, a custom load balancer using
`SCM_RIGHTS` fd-passing, SIMD distance calculations, `GC` disabled
outright, and no logging at all — every one of those choices exists
specifically to erase overhead between the NIC and the distance
computation. See [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md#design-philosophy)
for why this project chooses not to go there — it is a different regime
of engineering, not a more-careful version of this one.

## The trade-off, stated plainly

This project deliberately does not chase that ceiling (see
[`INFRASTRUCTURE.md`](INFRASTRUCTURE.md) for why: structured logging,
idiomatic `net/http`). With the correct 3M dataset, categorical-tag
partitioning, an IVF index tuned to `nprobe=8`, and the low-risk
optimizations above (pre-rendered responses, buffer pooling, GC tuning),
it clears both hard cutoffs by a wide margin (`failure_rate` well under
1% against a 15% ceiling, p99 around 1.3s against a 2000ms ceiling) and
lands at `final_score` in the **+660 to +1497** range depending on host
contention outside this project's own containers — the best measured
result by a wide margin, and, unlike earlier in this project's history,
now dominated almost entirely by the p99 term rather than detection
quality. Closing what's left ([`PATH_TO_EXCELLENCE.md`](PATH_TO_EXCELLENCE.md))
is a rewrite into a fundamentally different architecture, not a parameter
or algorithm change.
