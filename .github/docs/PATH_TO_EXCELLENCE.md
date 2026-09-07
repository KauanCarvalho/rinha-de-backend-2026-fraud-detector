# Path to a 6000-point score

[`RESULTS.md`](RESULTS.md#why-the-winning-solutions-score-close-to-the-6000-point-ceiling)
shows the scoring formula and where this project actually lands
(`final_score ≈ -203.6`, dominated by the p99 term). This document is the
follow-up to the obvious next question: **what would it actually take to
close that gap?** Not as a to-do list this project intends to execute —
that would contradict the whole premise in
[`INFRASTRUCTURE.md`](INFRASTRUCTURE.md#why-this-isnt-a-bit-mining-exercise)
— but as an honest, itemized answer, so "the winners over-engineered it"
isn't left as a vague hand-wave.

## The math, restated

```
score_p99 = 1000 · log10(1000 / max(p99, 1ms))     ceiling +3000 at p99 ≤ 1ms
score_det = 1000 · log10(1/ε) − 300·log10(1+E)      ceiling ~+3000 at E = 0
final_score = score_p99 + score_det                 range [-6000, +6000]
```

Measured today: `p99 = 1274ms` → `score_p99 = -105.2`. To reach the +3000
ceiling requires `p99 ≤ 1ms` — roughly **three orders of magnitude** faster,
sustained at 1200 req/s on 0.475 vCPU per replica. That budget leaves well
under a millisecond of wall-clock time per request for *everything*:
accepting the TCP connection, parsing HTTP, parsing JSON, searching
3,000,000 vectors, serializing the response, and writing it back. Every
layer this project uses for maintainability (`net/http`, Go's GC,
`encoding/json`, a pointer-based k-d tree) has overhead that is individually
tiny but collectively larger than the whole budget.

Below is what each of those layers would be replaced with, in order of the
return it buys.

## 1. Stop allocating on the hot path

**What it means:** every `make([]byte, ...)`, every `fmt.Sprintf`, every
interface boxing, every slice that grows past its capacity, hands work to
Go's garbage collector. Under sustained load, GC pauses (even sub-millisecond
ones) show up directly in the p99 tail, because p99 is exactly the
percentile GC pauses land in.

**What to do:**
- Pre-allocate and reuse buffers with `sync.Pool` for request/response
  bodies instead of allocating per request.
- Replace `encoding/json` (which allocates heavily via reflection) with a
  hand-written encoder/decoder for the two fixed message shapes this API
  has, or a code-generated one (`easyjson`, `ffjson`).
- Pre-render the static parts of the response (`{"approved":`) as a byte
  literal and only format the two variable fields.
- `debug.SetGCPercent(-1)` to disable the garbage collector outright, paired
  with `debug.SetMemoryLimit(...)` as a backstop so it still collects before
  hitting the container's memory limit. This is what "no GC" means in
  practice — not literally never collecting, but only collecting under
  memory pressure instead of on a percentage-growth schedule.

**Cost:** loses most of the readability and safety `encoding/json` and
ordinary GC-managed code give for free. Every buffer's lifetime becomes the
programmer's responsibility to reason about instead of the runtime's.

## 2. Bypass `net/http`

**What it means:** Go's `net/http` server is general-purpose — it supports
HTTP/1.1 and HTTP/2, chunked transfer, arbitrary headers, keep-alive
management, TLS, and more. All of that machinery runs on every request,
whether the caller needs it or not. A `epoll`-based event loop written by
hand (Linux syscalls directly, one goroutine or OS thread managing many
sockets) only implements exactly the request shape this API actually
receives: a fixed-format `POST` with a JSON body, always over plain HTTP,
always short-lived.

**What to do:** read raw bytes off the socket with `epoll_wait`, parse just
enough of the HTTP request line and the JSON body to extract the 14
fields the vectorizer needs (skip building a full `http.Request` struct),
and write the response directly as bytes.

**Cost:** every protocol edge case `net/http` handles for you (malformed
requests, slow-loris clients, header size limits, timeouts) becomes code
you have to write and maintain yourself. This is the single largest
"bit-mining" investment on the list — most of the winning repos' custom
epoll loops live here.

## 3. SIMD the distance computation

**What it means:** computing a Euclidean distance between two 14-dimension
vectors is 14 multiplications and 14 additions. A CPU's SIMD instructions
(AVX2 on x86-64) do 4-8 of those multiply-adds in a single instruction
instead of one at a time. Over millions of comparisons, that difference
compounds directly into search latency.

**What to do:** Go 1.26+ ships an experimental `GOEXPERIMENT=simd` mode
(the `archsimd` package) that exposes these intrinsics from Go without
dropping to assembly or cgo. The distance loop in `internal/knn` would be
rewritten to operate on 4 or 8 `int16` lanes at once instead of a plain
`for` loop over 14 elements.

**Cost:** the experimental SIMD API is not yet a stable Go API — it can
change between minor releases, and code that uses it needs a fallback path
for CPUs without AVX2. It also only helps the actual distance-computation
inner loop; for this project's data shape, most p99 cost is elsewhere
(scheduling, allocation, HTTP), so on its own this buys less than #1 or #2.

## 4. A search structure built for this dimensionality

**What it means:** [`RESULTS.md`](RESULTS.md#in-process-k-d-tree-why-a-search-budget-exists-at-3m-vectors)
already shows the k-d tree's pruning power degrades at 14 dimensions —
that's why `KNN_MAX_EXTRA_LEAVES` exists at all, trading recall for a
bounded worst case. An **IVF (Inverted File) index** clusters the
3,000,000 reference vectors ahead of time (k-means, built once at index
time) and, at query time, only searches the handful of clusters nearest to
the query — trading a small, tunable amount of recall for a much smaller
candidate set per query, with more predictable cost than backtracking
budget on a tree. A **VP-tree** is an alternative that stays exact
(unlike IVF) but partitions by distance rather than by axis, which
degrades more gracefully than a k-d tree in higher dimensions — the
official `docs/en/EVALUATION.md` mentions it by name as worth
considering.

**What to do:** replace `internal/knn`'s k-d tree with an IVF index (build
step: run k-means over the 3M vectors to produce e.g. 1,000 centroids;
search step: find the nearest few centroids to the query, then brute-force
only the vectors assigned to those clusters) or a VP-tree.

**Cost:** meaningfully more index-building complexity (k-means has its own
convergence and initialization concerns), and IVF trades away the
guarantee of exact nearest neighbors that the k-d tree with a large enough
budget still gives.

## 5. A leaner load balancer

**What it means:** HAProxy is general-purpose — TLS termination, HTTP
parsing, ACLs, and a config language this project never uses beyond plain
round-robin. Some winning solutions replace it with a purpose-built proxy
in C or Rust that does only `accept()` → pick a backend → forward bytes,
sometimes using `SCM_RIGHTS` to hand an accepted file descriptor directly
to a worker process instead of proxying every byte through userspace.

**What to do, if pursued:** a minimal round-robin dispatcher using raw
sockets and `SCM_RIGHTS` fd-passing.

**Cost:** the challenge's own rules require the load balancer to stay
"dumb" (no business logic) — HAProxy already satisfies that with zero
custom code to maintain. This is the lowest-ROI item on the list for this
project specifically: HAProxy's own overhead is a small fraction of the
measured p99, and the code that would replace it is pure infrastructure
risk with no functional upside beyond shaving microseconds.

## Why this project stops here

Every item above trades a specific piece of Go's ordinary safety net
(GC, `net/http`'s protocol handling, exact search, a battle-tested proxy)
for latency headroom this challenge's scoring formula rewards on a
logarithmic curve. That trade is legitimate engineering — it is exactly
what the winning repos did, and the reasoning above should make clear it
is not "cheating" or unfair, just a different set of priorities. This
project's stated goal from the start was a production-shaped,
maintainable Go service, not a maximum-score entry in a closed
competition — see [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md#why-this-isnt-a-bit-mining-exercise).
The gap between `-203.6` and `+6000` is now fully measured and explained
rather than mysterious; closing it is a rewrite into a different kind of
project, not a backlog for this one.
