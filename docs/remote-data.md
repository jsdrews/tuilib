# Remote data too large for one request

Which component shows it, how it is paged, and why. This is the reasoning
behind CLAUDE.md rule 34. The decisions and their rejected alternatives live
on the wayfinder map "Map: choosing components for large remote data" (#70),
and the build is specified in #81.

> **Status.** Everything here exists: `pkg/eventlog`, `source.Anchored`,
> `source.Model`'s `MaxHeld` / `Follow` / `Find`, `pkg/table`'s span mode
> (`Options.Anchored`), `pkg/resume`, and `logview.Prepend` /
> `AppendMarker`. Each shape has an example: `examples/patterns/eventlog`
> (Seekable, AWX-shaped), `examples/patterns/anchored` (Anchored,
> Elasticsearch-shaped) and `examples/patterns/podlogs` (Streamed, pod logs).

## Classify the data first

Two questions decide everything: what an item *is*, and how an item can be
*reached*. The terms are in `CONTEXT.md`.

**What an item is**

| | Meaning | Examples |
|---|---|---|
| **Record** | fields the user compares across items | Prefect flow runs, Argo apps, most REST lists |
| **Event** | fields in a timeline read in order | AWX job events, Elasticsearch log documents |
| **Line** | raw text read in order | pod logs, Prefect log messages |

**How it is reached**

| | Meaning | Examples |
|---|---|---|
| **Seekable** | item N can be fetched directly; the total is known or grows | offset/limit endpoints, AWX `counter` ranges |
| **Anchored** | only reachable by walking forwards or backwards from an anchor; no reliable total | Elasticsearch `search_after`, Prefect `logs/filter` by timestamp |
| **Streamed** | only the tail, then whatever arrives | Kubernetes pod logs |

Plus one flag, **Growing**: items keep arriving at the end while the user watches.

## Then pick the component

| | Seekable | Anchored | Streamed |
|---|---|---|---|
| **Record** | `table` + `SetWindow` + `source.Model` | `table` span + `source.Anchored` | `table` + `SetKeyedRows` from a watch |
| **Event** | `eventlog` + `inspector` on enter | `eventlog` + `source.Anchored` + `inspector` | `logview` + `inspector` |
| **Line** | `eventlog` | `eventlog` + `source.Anchored` | `logview` + `pkg/resume` |

Three rules sit on top:

- **If it fits in one request, don't page it.** A finished AWX job whose
  output is under 1 MiB goes in a `textview`; a short list goes through
  `SetRows`. Paging machinery earns its keep only when the data doesn't fit.
- **Events never go in a table.** A timeline is read in order. Sorting and
  comparing across rows are what a table is for, and neither applies. The
  fields are one enter away, in an inspector.
- **Search jumps, filter narrows.** A filter changes the query, so fewer items
  come back. Search finds the next match and moves the view there, with its
  surrounding context intact. Over remote data this matters more, not less:
  a filtered result loses the lines around each hit, which is usually what
  you were looking for.

## The sources

### Prefect: flow runs and logs

- **Flow runs** are Record / Seekable. `POST /flow_runs/filter` takes offset
  and limit, with `/flow_runs/count` and `/flow_runs/paginate` for totals.
  Use `table` + `source.Model` + `pkg/poll`.
  - Keep `PageSize` at or below `PREFECT_SERVER_API_DEFAULT_LIMIT` (200). A
    larger limit is refused with a 422 rather than trimmed.
- **Logs** are Line (or Event) / Anchored / Growing, in an `eventlog` over
  `source.Anchored`.
  - `POST /logs/filter` returns a bare array with no total, sorted by
    timestamp only, with no tiebreaker.
  - Offsets are unstable while a run is live: workers send logs in batches
    about every 2s, stamped when they were created, so a new batch can land
    in the middle of timestamp order and shift every later offset.
  - Anchor at Newest (sort descending, then reverse). Follow by polling
    `timestamp.after_` with a few seconds of overlap, and drop duplicates by
    log id.
  - Keep polling briefly after the run finishes: late batches still arrive.
- **The logs websocket is not a data channel.** `WS /logs/out` is off by
  default, never replays history, and silently drops logs once a
  subscriber's 256-entry queue is full. At most it is a hint to poll sooner
  (`Poke`).

### AWX: job events

- **Job events** are Event / Seekable / Growing, in an `eventlog` over
  `source.Model` with `MaxHeld`.
  - ansible-runner numbers events 1, 2, 3… with no gaps at the source, so
    `order_by=counter&counter__gte=a&counter__lte=b` fetches any range.
  - The total is the highest counter (`order_by=-counter&limit=1`), and it
    grows while the job runs.
- **Holes appear mid-run** because several workers save events out of order.
  - A short reply means "not saved yet", not "the end". Draw holes as
    placeholders and refetch them while Growing.
  - The final count only settles once `event_processing_finished` is true.
- **Always send `order_by=counter`.** The default order (`start_line`) has
  ties.
- **Events carry 0–n output lines.** The item is the event, so windows count
  events and positions stay stable as output expands.
- **The stdout endpoint can't page a large job.** Past 1 MiB, every format
  except `*_download` returns a "too large" message. It suits only a
  finished, small job, in a `textview`.
- **The websocket is a hint.**
  - It carries `counter` in a `job_events` group, but it is lossy (about 30
    messages a second at most), drops events with no output, and can't
    resume after a disconnect.
  - Use it to `Poke`; `jobs-summary` carries `final_counter`.

### Elasticsearch (ECK): log documents

- **Log documents** are Event / Anchored / Growing, in an `eventlog` over
  `source.Anchored`.
  - `from`/`size` stops at `index.max_result_window` (10k). Beyond that the
    only way in is `search_after` with the full sort values.
  - Always sort with a unique tiebreaker. Under a point in time (PIT),
    `_shard_doc` is added automatically.
  - To page backwards, reverse every sort, pass the first row's sort values,
    and reverse the hits client-side.
  - The cursor is the sort values encoded as JSON, and it is opaque to the
    library.
- **Totals are an exact count or "at least N".** `track_total_hits` counts to
  10k by default. Most log views turn totals off.
- **A PIT stabilises scroll-back but cannot tail.**
  - It is a snapshot, so extend `keep_alive` per page and `DELETE` it when
    done.
  - Tailing is polling `search_after` forward without a PIT, rewinding for
    documents that arrive late (refresh is about 1s) and dropping duplicates
    by `_id`.
- **The gutter shows `@timestamp`**, the server's time for each document,
  as `Item.Mark` — the same column Kibana's Discover leads with, and
  stable as pages arrive above it (there are no positions to show). It is
  never the time the TUI fetched anything. If your messages carry the
  application's own timestamp, leave `Mark` empty rather than show two
  times for one event; and note `@timestamp` may be ingestion time
  unless the pipeline parses the app's time into it.
- **ECK changes only the connection.**
  - The service is `<name>-es-http:9200`, with TLS by default.
  - The CA is in `<name>-es-http-certs-public` (`tls.crt`), and the elastic
    user's password is in `<name>-es-elastic-user`.

### Kubernetes: pod logs

- **Pod logs** are Line / Streamed / Growing, in a `logview` with `pkg/resume`.
- **There is no pagination.** The endpoint streams the newest log file, about
  10 MiB with the kubelet defaults.
  - "Load older" means asking again with a larger `tailLines`, which
    re-downloads every newer line. So it is an explicit action, never
    triggered by scrolling.
- **Resume is by `sinceTime`, truncated to the second**, so a naive reconnect
  repeats lines or loses them.
  - stern resumes exactly by always requesting timestamps, remembering the
    last second and how many lines it held, and skipping that many.
  - `pkg/resume` is that algorithm, generic and without I/O.
- **Rotation loss is undetectable.** A resume that lands after the file
  rotated returns nothing for what went into the rotated file, and a quiet
  container looks the same. Mark every reconnect in the log instead of
  claiming there was or wasn't loss.
- **Containers and runs.**
  - Follow ends when the container stops; watch the pod to pick up the next
    run.
  - `previous=true` reaches back one run, shown as its own view, never
    interleaved.
  - Several containers means one stream each, tagged with pod and container.

### Generic REST

- **Most list endpoints** are Record / Seekable: `table` + `source.Model`, as
  in rule 29.
- **Cursor-paginated lists** that can only go forward are Record /
  Seekable-by-cursor (`ByCursor`, which accumulates a growing window).
- **Lists that page both ways from a token** are Record / Anchored: a
  `table` span over `source.Anchored`.

## What carries over from tables

Everything built for slow remote tables (#69, rule 29) applies to the
eventlog and to anchored spans, because they share a windowing core with
`pkg/table`:

- **Waiting:** stale rows dimmed under a `showing X · ⠙ loading Y` suffix
  while a query is in flight, and Loading only before the first answer.
- **Staying cheap:** scroll requests wait for the viewport to settle, a
  superseded request is cancelled through `Query.Ctx`, and nothing is paged
  while the committed query is unanswered.
- **Reporting:** the query history (answered, failed, cancelled) goes to the
  output console, and the border shows `⠙ loading rows …` or
  `loading older… / newer…` while data is missing.

What is new for timelines:

- **Follow:** pinned at the newest item. New items are counted, not shown,
  once the user moves away (`↓ 42 new`). The source polls; when not
  following, a poll is a **probe** that counts without fetching.
- **Late items:** merged by key where the page puts them, without moving the
  view.
- **Remote find:** `n`/`N` search resident items first, then ask the server
  for one match past the edge and jump or re-anchor there.
- **Spans:** Anchored data is held as a span grown at either edge, trimmed at
  the end furthest from the viewport past 5,000 items.

## Research

Each source's findings are summarised in the resolution comment of its
research ticket, with primary-source links:

- AWX job events and stdout (#71)
- Elasticsearch paging and tailing (#72)
- Prefect logs and flow runs (#73)
- Kubernetes pod logs (#74)
