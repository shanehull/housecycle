# housecycle

Australian housing buy-signal monitor. One Go binary reads long-run macro series and renders a single dashboard, at housecycle.shanehull.com.

A bit of fun for a long-horizon buyer, not market timing: no one can call a housing top or bottom.

The best signal stays off the dashboard: it is when friends stop talking about buying houses at barbecues.

## Signals

| Signal               | What it measures                                                                                                                                                                      | Source         | History |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------- | ------- |
| Valuation            | House prices relative to income, and the detrended Q ratio: the dwelling stock net of its debt over replacement cost, scaled by its running geometric mean.                           | OECD, ABS, RBA | 1990 →  |
| Volume               | Dwelling transfers, all buyers, per resident, so the rate stays comparable as the population drifts.                                                                                  | ABS            | 2002 →  |
| Construction costs   | House construction cost growth ranked against its own history. Low growth marks a building sector nobody is bidding up, and it has marked the strongest gains over five to ten years. | ABS            | 1996 →  |
| Credit conditions    | Household leverage (debt-to-income against its own history) and the price of credit risk (the corporate spread). Tight, sober credit is the entry.                                    | RBA            | 1988 →  |

Each signal evaluates one or more rules and returns a status: not yet, forming, or buy conditions met. The header score is how many of the four are at buy.

Series IDs: `oecd.au.hpi`, `abs.au.awe`, `abs.au.house_construction`, `abs.au.dwelling_transfers`, `abs.au.population`, `abs.au.housing_value`, `abs.au.dwelling_replacement`, `rba.housing_credit`, `rba.debt_to_income`, `rba.corp_yield_5y`, `rba.ags_5y`.

## How it works

```
OECD / ABS / RBA  ->  ingest  ->  in-memory cache  ->  signals  ->  Chart.js
```

- `internal/source`: one adapter per provider, all emitting `model.Series`. OECD is CSV; ABS is SDMX-JSON; RBA parses named columns from its statistics CSVs. Every source is public and keyless.
- `internal/cache`: an in-memory cache of fetched series, no database or disk. The app refetches on startup.
- `internal/analytics`: the pure maths (percentile rank, changes, forward-fill, the detrended Q ratio).
- `internal/signals`: the signal definitions. Thresholds sit at the top of `signals.go`.
- `internal/web`: the dashboard, with chart data embedded as JSON for Chart.js.

## Quick start

Requires Go 1.26.5, nothing else: no database, no third-party dependencies.

```bash
go run ./cmd/housecycle
```

Then open http://localhost:8080. The first run fetches everything.

## Configuration

| Flag    | Env               | Default |
| ------- | ----------------- | ------- |
| `-addr` | `HOUSECYCLE_ADDR` | `:8080` |

## Development

```bash
air                     # hot reload
golangci-lint run ./...
go test ./...
```

`mise` pins Go, golangci-lint, and air. The server sets read, write and idle timeouts, logs every request with `slog`, and drains in-flight requests on SIGINT and SIGTERM.

## Data notes

- No clean decades-long purchase-volume series is free: ABS total housing loan commitments only start in 2019, when the investor split begins, so purchase volume uses ABS dwelling transfer counts (established and attached, all buyers) from 2002, summed across capital cities and rest of state.
- Volume is transfers per resident, not the raw count: over decades the count drifts with population, so the per-capita rate is the comparable measure. The ABS dwelling-stock count starts only in 2011, so population (ABS estimated resident population, quarterly, from 1981) is the scale that spans the full series.
- Construction cost growth is the ABS house construction price index, year-ended, ranked against its own history. Low growth is the signal: nobody is bidding up the cost of building, so the sector is cold rather than hot. The returns back it up. Since 1997 the bottom tercile of build-cost growth was followed by the strongest house price gains at every horizon: +9.0% over one year against +4.6% for the top tercile, +48.5% over five years against +35.2%, and +96.8% over ten years against +80.6%. The low bucket wins at each horizon, though the middle and top terciles are not ordered beyond one year.
- The ABS household balance sheet (Table 35, 5232.0) is published as an XLSX data cube, so `internal/source/abs_xlsx.go` scrapes it (a small stdlib XLSX reader lives in `internal/xlsx`). It values dwellings at replacement cost and land at market, from 1988, which gives the Q ratio its replacement-cost denominator.
- The Q ratio numerator is the dwelling stock net of the housing credit outstanding against it (RBA D2, owner-occupier and investor, from 1990), so it measures equity. The denominator is the replacement cost of the structure, which tracks building costs. The ratio is divided by its running geometric mean, which flattens the multi-decade trend into a time-invariant, mean-reverting oscillator for spotting short-term valuation anomalies.
- Credit conditions score household leverage and the price of credit risk. Leverage is household debt-to-income (RBA E2, quarterly, 1988 →) ranked against its own history. It is structural and trends up, so it reads stretched and only trips after a real deleveraging; the buy zone is the pre-2001 range, where it met continuously from 1988 to 2000. The price of credit risk is the corporate spread over government borrowing (RBA F3 and F2.1, from 2013).

## Disclaimer

A research tool, not financial advice. Market data is published by OECD, ABS, and the RBA; their terms govern use.

## License

MIT. See [LICENSE](LICENSE).
