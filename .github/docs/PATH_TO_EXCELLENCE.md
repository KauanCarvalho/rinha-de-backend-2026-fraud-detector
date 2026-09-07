# Path to a 6000-point score

[`RESULTS.md`](RESULTS.md#the-6000-point-ceiling)
shows the scoring formula and where this project actually lands
(`final_score` in the **+660 to +1497** range depending on host
contention outside this project's own containers — the spread is
measurement noise, not index nondeterminism; see RESULTS.md for why).
Detection is no longer the bottleneck; p99 is. This document is the
follow-up to the obvious next question: **what would it actually take to
close the rest of the gap?** Not as a to-do list this project intends to
execute — that would contradict the whole premise in
[`INFRASTRUCTURE.md`](INFRASTRUCTURE.md#design-philosophy)
— but as an honest, itemized answer, so the gap isn't left as a vague
hand-wave.

## The math, restated

```
score_p99 = 1000 · log10(1000 / max(p99, 1ms))     ceiling +3000 at p99 ≤ 1ms
score_det = 1000 · log10(1/ε) − 300·log10(1+E)      ceiling ~+3000 at E = 0
final_score = score_p99 + score_det                 range [-6000, +6000]
```

Measured today: p99 around 1.3s → `score_p99` around −130. To reach the
+3000 ceiling requires `p99 ≤ 1ms` — still roughly **three orders of
magnitude** faster, sustained at 1200 req/s on 0.475 vCPU per replica.
That budget leaves well under a millisecond of wall-clock time per request
for *everything*: accepting the TCP connection, parsing HTTP, parsing
JSON, searching 3,000,000 vectors, serializing the response, and writing
it back. Every layer this project uses for maintainability (`net/http`,
Go's GC, `encoding/json`) has overhead that is individually tiny but
collectively larger than the whole budget — and that gap exists
*regardless* of how good the search structure underneath is, which is
exactly what the notes below found out empirically.

Below is what's left on the list, in order of the return it buys.

> [!NOTE]
> An earlier version of this list had "stop allocating on the hot path"
> here — pre-rendered response bodies, a `sync.Pool`'d request buffer, and
> `debug.SetGCPercent(-1)`. That one turned out not to require the
> maintainability trade-off the rest of this document is about: this API's
> response space is exactly 6 fixed JSON bodies, so pre-rendering them is a
> free win, not a readability sacrifice. All three are implemented — see
> [`RESULTS.md`](RESULTS.md#low-risk-optimizations-pre-rendered-responses-buffer-pooling-gc-tuning)
> for what they measurably bought (p99 −7.8%, http_errors −46%).

> [!NOTE]
> Categorical-tag partitioning is also implemented
> (`internal/knn.Tag`/`PartitionedIndex`) — reference vectors are split
> into up to 16 buckets by four already-boolean dimensions
> (card_present, is_online, unknown_merchant, has-last-transaction) before
> any distance-based search runs, so a query only ever searches its own
> bucket. Pure data partitioning, no assembly, no custom event loop. See
> [`RESULTS.md`](RESULTS.md#categorical-tag-partitioning) for the numbers.

> [!TIP]
> What used to be item 3 here — replacing the k-d tree with an IVF index —
> is also done, and turned out to be the single biggest win in this
> project's entire history: `final_score` went from the low hundreds to
> four figures. `internal/knn` no longer contains a k-d tree at all — see
> [`RESULTS.md`](RESULTS.md#replacing-the-k-d-tree-with-ivf) for the full
> story, including two other ideas (search escalation, partition
> rebalancing) that looked promising and made things *worse* in practice
> before IVF was tried.

## 1. Bypass `net/http`

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
low-level investment on the list. With detection quality now solved, this
is also the most obviously impactful item left: p99 is the entire
remaining gap, and this is the layer between the NIC and the search that
costs the most.

## 2. SIMD the distance computation

**What it means:** computing a Euclidean distance between two 14-dimension
vectors is 14 multiplications and 14 additions. A CPU's SIMD instructions
(AVX2 on x86-64) do 4-8 of those multiply-adds in a single instruction
instead of one at a time. Over millions of comparisons — now mostly
against IVF cluster centroids and a handful of clusters' worth of vectors,
rather than every reference vector — that difference compounds directly
into search latency.

**What to do:** Go 1.26+ ships an experimental `GOEXPERIMENT=simd` mode
(the `archsimd` package) that exposes these intrinsics from Go without
dropping to assembly or cgo. `sqDist` and the centroid-distance loop in
`internal/knn` would be rewritten to operate on 4 or 8 `int16` lanes at
once instead of a plain `for` loop over 14 elements.

**Cost:** the experimental SIMD API is not yet a stable Go API — it can
change between minor releases, and code that uses it needs a fallback path
for CPUs without AVX2. It also only helps the actual distance-computation
inner loop; most of p99 is elsewhere (scheduling, allocation, HTTP), so on
its own this buys less than #1.

## 3. A leaner load balancer

**What it means:** HAProxy is general-purpose — TLS termination, HTTP
parsing, ACLs, and a config language this project never uses beyond plain
round-robin. A purpose-built proxy in C or Rust that does only `accept()`
→ pick a backend → forward bytes — optionally using `SCM_RIGHTS` to hand
an accepted file descriptor directly to a worker process instead of
proxying every byte through userspace — would shave off that overhead.

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
(`net/http`'s protocol handling, a stable public API, a battle-tested
proxy) for latency headroom this challenge's scoring formula rewards on a
logarithmic curve. That trade is legitimate engineering, and the reasoning
above should make clear it is not "cheating" or unfair, just a different
set of priorities. This project's stated goal from the start was a
production-shaped, maintainable Go service, not a maximum-score entry in a
closed competition — see [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md#design-philosophy).
The gap between four figures and `+6000` is now fully measured and
explained rather than mysterious — and, notably, it is now a *latency*
gap, not a *detection* gap: closing it further is a rewrite into a
fundamentally different architecture, not another algorithm swap.
