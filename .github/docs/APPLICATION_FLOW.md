# How the application works, end to end

This walks through **everything** that happens — from a raw file of past
transactions to a single `approved: true/false` answer — assuming no prior
knowledge of vector search, k-d trees, or any of that. If you can read a
flowchart, you can follow this.

## The short version

This app answers one question: *"is this new transaction shaped like past
fraud, or like a normal purchase?"* It answers it by comparing the new
transaction against a huge pile of **already-labeled** past transactions,
and copying whatever the 5 most similar ones were labeled. No AI model, no
training — just very fast comparison-by-similarity. That single idea is the
entire product; everything below is just the plumbing to make that
comparison happen accurately and in under a millisecond.

## The two lives of this application

Everything below splits cleanly into two phases that never overlap:

```mermaid
flowchart LR
    subgraph BUILD["Build time — happens once, before anything runs"]
        direction LR
        A[references.json.gz<br/>1M past transactions,<br/>already labeled] --> B[cmd/indexbuilder] --> C[(index.bin<br/>~39MB file)]
    end
    subgraph RUNTIME["Runtime — happens forever after, per request"]
        direction LR
        D[New transaction<br/>arrives as JSON] --> E[cmd/api] --> F[approved: true/false]
    end
    C -.->|"loaded into memory<br/>when the container starts"| E
```

**Build time** turns a big file of history into a data structure optimized
for one operation: "find the 5 entries most similar to this one, fast."
**Runtime** is the actual web server, answering requests using that
structure. Build time happens inside `docker build`, once; runtime happens
inside the running container, millions of times.

---

## Part 1 — Build time: from a spreadsheet of history to a fast lookup table

### The raw material

[`resources/references.json.gz`](../../resources/references.json.gz) is a
compressed file with 1,000,000 entries that looks like this:

```json
{ "vector": [0.01, 0.0833, 0.05, ..., 0.0416], "label": "legit" }
{ "vector": [0.58, 0.92, 1.0, ..., 0.0032], "label": "fraud" }
```

Think of it as a **ledger of already-solved cases**: 1 million past
transactions, each one already boiled down to 14 numbers (more on that
below) and already stamped "this one was fraud" or "this one was legit."
Nobody needs to figure out those labels — they came pre-labeled from the
challenge organizers, based on real (synthetic) outcomes. This app never
invents or re-labels anything; it only ever *compares against* this fixed
ledger.

### The build pipeline

[`cmd/indexbuilder`](../../cmd/indexbuilder/main.go) runs exactly once, at
Docker build time (see [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md) for why),
and does three things to that ledger:

```mermaid
flowchart TD
    A[references.json.gz<br/>16.7MB compressed] -->|"1: decompress + read,<br/>one entry at a time<br/>(internal/dataset)"| B["1,000,000 vectors<br/>+ 1,000,000 labels<br/>in memory"]
    B -->|"2: quantize<br/>(internal/knn.Quantize)"| C["same numbers,<br/>compressed to int16<br/>~90MB instead of ~280MB"]
    C -->|"3: build k-d tree<br/>(internal/knn.Build)"| D["index.bin<br/>~39MB, organized<br/>for fast search"]
```

**Step 1 — read.** Each of the 1M lines is parsed one at a time (never all
loaded as raw JSON text at once — that would need way more memory than this
challenge's 350MB budget allows). Every entry becomes 14 floating-point
numbers plus a fraud/legit flag.

**Step 2 — quantize** (a fancy word for "round to less precision, on
purpose"). Each of those 14 numbers lives between -1 and 1. Storing them as
full 64-bit decimals is more precision than this problem needs, so each one
gets rescaled and rounded into a 16-bit whole number instead (multiply by
10,000, round). This alone shrinks the dataset from ~280MB to ~90MB — the
kind of thing that matters a lot when the entire app, including the load
balancer, has to fit in 350MB total.

**Step 3 — organize for search.** This is the important one, explained
below with a real analogy, because it's the one non-obvious idea in the
whole app.

### Why not just compare against all 1 million, every time?

That's the naive approach — and it works! For each new transaction, compute
the distance to all 1,000,000 stored ones, sort, take the 5 closest. It's
also too slow: measured on this exact dataset, that "check everyone"
approach takes ~14-29 milliseconds *per request* — and the scoring rules
for this challenge want answers in around 1 millisecond. Something faster
was needed.

**The analogy: finding a word in a dictionary.** If a word is somewhere in
a 1,000-page dictionary and you flip through page by page, that's slow. But
because the dictionary is *alphabetically organized*, you can jump straight
to roughly the right area and narrow down from there — a handful of
comparisons instead of a thousand. A **k-d tree** does the same trick, but
for 14-dimensional points instead of alphabetical words: it recursively
splits the 1 million points into smaller and smaller groups (imagine
repeatedly asking "is this point's 3rd number bigger or smaller than X?"
and going left or right), until each final group has only ~32 points left
in it. Searching means walking down that structure instead of checking
every point — dramatically fewer comparisons for the typical case.

```mermaid
flowchart TD
    Root["All 1,000,000 points"] --> L["~500,000 points<br/>(one side of a split)"]
    Root --> R["~500,000 points<br/>(other side)"]
    L --> LL["...keeps splitting..."]
    L --> LR["...keeps splitting..."]
    R --> RL["...keeps splitting..."]
    R --> RR["...keeps splitting..."]
    LL --> Leaf["🍃 leaf: ~32 points<br/>(final answer lives<br/>near here)"]
```

One honest caveat, explained in plain terms: this trick works best when
there are few "dimensions" (few numbers per point). This app's points have
**14** numbers each, which is enough that the dictionary trick stops being
perfectly efficient — it still massively narrows things down, but the code
also puts a hard cap on how much extra digging it's allowed to do per
search (`KNN_MAX_EXTRA_LEAVES`, default 2000), so that even a hard case
never blows past a predictable time budget. It's the difference between "I
will spend as long as it takes to be 100% certain" and "I will spend at
most this long, and take the best answer I've found by then." In practice
that difference is basically never wrong — see
[`RESULTS.md`](RESULTS.md) for the actual measured numbers.

The output of all three steps is a single file, `index.bin`, containing the
whole organized structure. That file gets baked directly into the Docker
image (see [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md)) — the running
application never touches `references.json.gz` at all.

---

## Part 2 — Starting the application

When a container starts, [`cmd/api`](../../cmd/api/main.go) runs through a
short, strict startup sequence before it will accept a single request:

```mermaid
flowchart TD
    A([Container starts]) --> B["Read configuration<br/>from environment variables<br/>(internal/config)"]
    B --> C["Load index.bin into memory<br/>(~0.5s, ~90MB)"]
    C -->|"fails?"| C1["log the error, exit 1<br/>(container restarts)"]
    C -->|"succeeds"| D["Ask the OS to reclaim<br/>scratch memory used<br/>while loading"]
    D --> E["Wire up the HTTP routes<br/>and start listening on :9999"]
    E --> F(["Ready — GET /ready<br/>now returns 200"])
```

This matters for one reason worth spelling out: **the 1-million-entry index
is loaded exactly once, when the container boots**, and then lives in RAM
for as long as the container runs. A request never triggers any file
reading or parsing — it only ever reads a structure that was already
sitting ready in memory. That's *why* answering a request can be this fast:
the expensive work already happened, once, well before the first customer's
transaction ever arrived.

---

## Part 3 — A request's full journey

The application exposes exactly two HTTP routes, both on port `9999`:

| Route | Purpose |
|---|---|
| `GET /ready` | "Are you alive and ready for traffic?" — used by HAProxy and by whoever is orchestrating containers. |
| `POST /fraud-score` | The actual product: "is this transaction fraud?" |

### `GET /ready` — the simple one

By the time the HTTP server is even listening, the index has already
loaded successfully (see Part 2) — so this route has nothing left to check.
It just answers `200 OK` immediately. No body, no logic.

### `POST /fraud-score` — the real one, step by step

Here is one transaction's complete trip through the system, from the
moment a client sends it to the moment it gets an answer:

```mermaid
sequenceDiagram
    participant C as Client
    participant LB as HAProxy<br/>(round-robin)
    participant API as API instance<br/>(1 of 2)
    participant IDX as In-memory index<br/>(index.bin, loaded at boot)

    C->>LB: POST /fraud-score<br/>{transaction, customer, merchant, ...}
    LB->>API: forwards as-is<br/>(no inspection, no logic)
    Note over API: 1. Parse the JSON body
    Note over API: 2. Vectorize:<br/>turn the transaction into 14 numbers
    Note over API: 3. Quantize:<br/>same rounding as the reference data
    API->>IDX: 4. Search: find the 5 most similar<br/>past transactions
    IDX-->>API: 5 labels (fraud/legit)
    Note over API: 5. Score: count how many<br/>of the 5 were "fraud"
    Note over API: 6. Decide: score < 0.6 ? approve : deny
    API-->>LB: {"approved": bool, "fraud_score": number}
    LB-->>C: same response, forwarded
```

Each numbered step, in detail:

#### Step 1 — Parse

The request body is JSON describing one transaction — amount, time, the
cardholder's spending history, the merchant, the terminal, and (maybe) the
previous transaction. If it doesn't parse as valid JSON, the app answers
`400 Bad Request` immediately and none of the steps below happen.

#### Step 2 — Vectorize: turning a transaction into 14 numbers

This is the step that actually needs explaining, because "vector" sounds
scarier than what it is: it's just **the same transaction, described as a
list of 14 numbers instead of a paragraph**, each one scaled to fit between
0 and 1 (so that "amount" and "hour of day" and "distance in km" — wildly
different units — become directly comparable). This happens in
[`internal/vectorize`](../../internal/vectorize/vectorize.go), following a
fixed formula for each position:

| # | What it captures, in plain words | How it's phrased as a 0–1 number |
|---|---|---|
| 0 | How large is this purchase? | amount ÷ R$10,000 (capped at 1.0) |
| 1 | How many installments? | installments ÷ 12 |
| 2 | Is this unusually large *for this specific customer*? | (amount ÷ their usual average) ÷ 10 |
| 3 | What time of day? | hour ÷ 23 (midnight=0, 11pm≈1) |
| 4 | What day of the week? | Monday=0 ... Sunday=1 |
| 5 | How long since this customer's last purchase? | minutes ÷ 1 day — or **-1** if there's no previous purchase on record at all |
| 6 | How far is this from their *last* purchase? | km ÷ 1,000 — or **-1** if there's no previous purchase |
| 7 | How far is this from the cardholder's home address? | km ÷ 1,000 |
| 8 | How active has this customer been in the last 24h? | transaction count ÷ 20 |
| 9 | Was this an online purchase? | 1 if yes, 0 if in-person |
| 10 | Was the physical card actually there? | 1 if yes, 0 if not |
| 11 | Has this customer ever shopped at this merchant before? | 1 if it's a **new/unknown** merchant, 0 if familiar |
| 12 | How risky is this *type* of store, historically? | a fixed table by store category (e.g. gambling-adjacent categories score high, groceries score low; unlisted categories default to 0.5) |
| 13 | How big is this merchant's typical sale? | merchant's average ticket ÷ R$10,000 |

Two details worth calling out because they trip people up:

- **The `-1` values (dimensions 5 and 6) are deliberate, not an error.**
  When a customer has no purchase history at all, there is nothing to
  measure "minutes since" or "km from" against. Rather than guessing a
  number, the app writes `-1` — a value that can never occur naturally
  (every real measurement lands between 0 and 1) — so that "no history"
  cases end up clustered together with other "no history" cases when the
  search runs, instead of being confused with some *specific* small
  distance or short time gap.
- **Every number is capped between 0 and 1** ("clamped," in the code). A
  R$50,000 purchase doesn't get a score of 5.0 — it gets capped at 1.0,
  same as a R$10,000 one. Past a certain point, "how far over the line"
  stops mattering; only "over the line or not" does.

The result of this step is one list of 14 numbers — the exact same shape as
every entry in the 1-million-row reference file from Part 1.

#### Step 3 — Quantize

The freshly computed 14 numbers get rounded into the same compact int16
format the reference data was stored in (Part 1, step 2). This isn't
optional — the search in the next step is comparing numbers directly, so
both sides need to speak the same "precision dialect."

#### Step 4 — Search: find the 5 most similar past cases

This is where the k-d tree from Part 1 actually gets used. The app hands it
the new transaction's 14 numbers and asks: *"of the 1,000,000 entries you
hold, which 5 are numerically closest to this one?"* "Closest" here means
literal straight-line distance across all 14 numbers at once (the same
math as distance on a map, just extended from 2 coordinates to 14). The
search returns those 5 entries — specifically, their fraud/legit labels;
the actual historical transaction details are never even loaded, only the
label.

**Why 5, and why those exact 5?** Because the pattern "things that look
alike tend to behave alike" is the entire premise. If 4 of the 5 most
similar past transactions the system has ever seen were fraud, that's a
strong signal this one probably is too — not because of any rule anyone
wrote down, but because fraud tends to have a recognizable "shape" (unusual
amount, odd hour, unfamiliar merchant, no purchase history, far from home
— several of those at once), and this comparison surfaces that shape
automatically.

#### Step 5 — Score

Count how many of those 5 labels say `"fraud"`. Divide by 5. That's the
`fraud_score` — always one of `0.0, 0.2, 0.4, 0.6, 0.8, 1.0`, nothing else
is possible with 5 neighbors.

#### Step 6 — Decide

`approved = fraud_score < 0.6`. This threshold is fixed by the challenge
rules, not configurable: 3 or more "fraud" votes out of 5 denies the
transaction; 2 or fewer approves it. ([`internal/scoring`](../../internal/scoring/scoring.go)
is the entire implementation of this step — it's intentionally that small.)

#### The response

```json
{ "approved": false, "fraud_score": 0.6 }
```

Sent back exactly as-is through HAProxy to the original client. The whole
round trip, in the numbers actually measured for this project, lands well
under a millisecond in the common case — see
[`RESULTS.md`](RESULTS.md).

---

## Putting it all together, with the real topology

The diagrams above showed one API instance for clarity. The real deployment
always runs two, behind a load balancer that only ever does plain
round-robin — see [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md) for why that
split exists and what each piece is allowed (and not allowed) to do:

```mermaid
flowchart LR
    Client -->|":9999"| LB["HAProxy<br/>round-robin only,<br/>zero business logic"]
    LB --> API1["API instance 1<br/>own copy of index.bin<br/>in memory"]
    LB --> API2["API instance 2<br/>own copy of index.bin<br/>in memory"]
```

Both instances are byte-for-byte identical processes, each with its own
independent copy of the 90MB index loaded into its own memory — there is no
shared database and no cross-instance coordination of any kind. This is
deliberate: since the reference data never changes during a run (Part 1
happens once, at build time), there's nothing to keep in sync. Either
instance can answer any request, entirely on its own.

---

## Glossary, for the truly unfamiliar

- **Vector** — a fixed-length list of numbers describing something. Here,
  14 numbers describe one transaction. Two vectors are "similar" if their
  numbers are close to each other, position by position.
- **Nearest neighbor** — given a new vector, the stored vector(s) closest
  to it. "5 nearest neighbors" = the 5 closest matches.
- **k-d tree** — a way of pre-organizing a big pile of vectors so that
  finding nearest neighbors doesn't require checking every single one (the
  "dictionary" analogy above).
- **Quantize** — rounding numbers to a less precise, more compact format on
  purpose, to save memory, when that precision isn't actually needed.
- **Label** — the known, correct answer attached to a historical record
  ("fraud" or "legit"), used only for comparison, never re-derived.
