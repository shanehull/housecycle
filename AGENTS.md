# AGENTS.md — housecycle

Australian housing buy-signal monitor. Read [README.md](README.md) for what it does and how it is wired.

It is a bit of fun. Do not write copy that implies it can time a market: no one can call a top or a bottom, and the tool does not try to.

## Commands

```bash
go run ./cmd/housecycle    # serve the dashboard on :8080
go build ./...             # builds clean (no dependencies)
go vet ./...               # always clean
go test ./...              # unit tests, offline, no network
golangci-lint run ./...    # house lint
```

## Conventions

Mirror the house style established in lodestar, go-nem, and go-jev.

- All Go passes `golangci-lint run ./...` with the default config, and `gofmt` before commit.
- No `any` outside genuine decode boundaries (JSON, SDMX, heterogeneous payloads). Provider responses are typed structs with JSON tags.
- Prefer the standard library. Keep dependencies minimal. There are no third-party dependencies.
- Every public method takes `ctx context.Context` first.
- The HTTP server sets read, write, and idle timeouts, logs every request, and drains in-flight requests on SIGINT and SIGTERM.
- Structured logging with `slog`. Prose in docs is not hard-wrapped: one line per paragraph.
- Commit messages follow conventional commits. release-please generates the changelog. Do not commit unless the user asks; the user commits.

## Layout

```
cmd/housecycle        binary and wiring
cmd/sri               print and verify CDN script SRI hashes
internal/model        shared series and dashboard types
internal/source       providers -> model.Series (one adapter each)
internal/cache        in-memory cache of series (no database)
internal/analytics    pure maths, no I/O
internal/signals      signal definitions and evaluation
internal/ingest       fetch -> cache orchestration
internal/web          templates, static assets, HTTP server
internal/xlsx         stdlib XLSX reader for ABS data cubes
```

## Working on indicators

The metrics live in `internal/signals/signals.go`. Thresholds are constants at the top of that file; tune them there. To add an indicator: write one `evalX` function returning an `Indicator`, append it in `Evaluate`, and add any new series to the relevant source adapter in `internal/source`.

Sources must degrade gracefully. A fetch error for one source never fails the page; the indicator simply evaluates to `Unknown` and renders as no data.

Every fetch replaces the series in the in-memory cache, so repeated runs are idempotent. There is no persistence; history comes from the providers, which return full series each time.

## Testing

`go test ./...` runs offline with no network: `internal/analytics`, `internal/source` parsers (feed fixtures), and `internal/signals` thresholds.

A live tier against the real providers is intended, gated by an opt-in environment variable and skipped by default. It is not wired yet.

## Data sources

All free and keyless. ABS via SDMX-JSON for the main series, plus the household balance sheet (5232.0 Table 35) scraped from its XLSX data cube because it is not in the API (`internal/xlsx` is a small stdlib reader). OECD via CSV, RBA via its statistics CSVs (columns selected by name). If a series is needed but no decades-long source exists, say so rather than substituting a short one.

## Frontend

Go `html/template` plus Chart.js, one binary, no node step. Third-party scripts load from jsDelivr (Chart.js), matching shanehull.com. Local CSS and JS are embedded with `embed.FS` and served from `/static/`.

Security headers are set for every response, including a Content-Security-Policy with a per-request script nonce and style nonce. There are no inline `style` attributes: dynamic values (the ring offset and meter widths) are CSS custom properties set through a nonce-authorised `<style>` block. Third-party scripts are pinned by SRI. Their URLs and sha384 hashes live in `internal/web/cdn.go` as the single source of truth, feeding both the `integrity` attributes and the CSP. After bumping a script version, run `go run ./cmd/sri` to print the new hashes (it flags drift), then update that file.

`static/og.png` is the social card, baked rather than generated at runtime. Its subtitle lists the signal names, so it goes stale whenever a signal is renamed or dropped.

To rebuild it: write an HTML page matching the card's CSS at `1200x630`, screenshot it with headless Chrome (`"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=2 --window-size=1200,630 --screenshot=og-2x.png "file://$PWD/og.html"`), resize to exact pixels with `sips -z 630 1200 og-2x.png --out og.png`, then copy it into `internal/web/static/`.
