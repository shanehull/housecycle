# housecycle

Australian housing buy-signal monitor. One Go binary reads long-run macro series and renders a single dashboard, at housecycle.shanehull.com.

A bit of fun for a long-horizon buyer, not market timing: no one can call a housing top or bottom.

The best signal stays off the dashboard: it is when friends stop talking about buying houses at barbecues.

## Signals

| Signal               | What it measures                                                                                                                                                          | Source    | History |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- | ------- |
| Valuation            | House prices relative to income, and the Misesian Stationarity Index (household land and dwellings ÷ dwellings at replacement cost) scaled by its running geometric mean. | OECD, ABS | 1988 →  |
| Volume               | Dwelling transfers, all buyers, per resident, so the rate stays comparable as the population drifts.                                                                      | ABS       | 2002 →  |
| Price stability      | Construction cost growth near zero and the cash rate on hold.                                                                                                             | ABS, RBA  | 1996 →  |
| Credit conditions    | The flow and price of credit: housing and investor credit growth, and the corporate credit spread. Contrarian: tight, sober credit is the entry, not loose and cheap.     | RBA       | 1977 →  |
| Real cost of capital | Cash rate minus year-ended CPI inflation. Housing is the most rate-sensitive asset; a low real rate drives affordability.                                                 | RBA       | 1990 →  |

Each signal evaluates one or more rules and returns a status: not yet, forming, or buy conditions met. The header score is how many of the five are at buy.

Series IDs: `oecd.au.hpi`, `abs.au.awe`, `abs.au.house_construction`, `abs.au.dwelling_transfers`, `abs.au.population`, `abs.au.housing_value`, `abs.au.dwelling_replacement`, `rba.cash_rate`, `rba.cpi_yoy`, `rba.housing_credit_growth`, `rba.investor_credit_growth`, `rba.corp_yield_5y`, `rba.ags_5y`.

## How it works

```
OECD / ABS / RBA  ->  ingest  ->  in-memory cache  ->  signals  ->  Chart.js
```

- `internal/source`: one adapter per provider, all emitting `model.Series`. OECD is CSV; ABS is SDMX-JSON; RBA parses named columns from its statistics CSVs. Every source is public and keyless.
- `internal/cache`: an in-memory cache of fetched series, no database or disk. The app refetches on startup.
- `internal/analytics`: the pure maths (percentile rank, changes, forward-fill, the MS index).
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
- Volume is transfers per resident, not the raw count: over decades the count drifts with population, so the per-capita rate is the comparable measure. The ABS dwelling-stock count starts only in 2011, so population (ABS estimated resident population, quarterly, from 1990) is the scale that spans the full series.
- The ABS household balance sheet (Table 35, 5232.0) is published only as an XLSX data cube, not through the SDMX API, so `internal/source/abs_xlsx.go` scrapes it (a small stdlib XLSX reader lives in `internal/xlsx`). It values dwellings at replacement cost and land at market, from 1988, giving the MS index a market numerator and a replacement-cost denominator.
- The MS index denominator must be a replacement cost, not a second market value: a market denominator (for example, value less debt) also rises with prices, which inverts the signal.
- Credit conditions measure the flow and the price of credit, not the stock. Housing and investor credit growth (RBA D1) are rates, and the corporate credit spread (RBA F3 and F2.1) is a ratio, so they stay comparable across decades where a debt-to-income level would not. The securities spread only starts in 2013, so that leg is short.

## Disclaimer

A research tool, not financial advice. Market data is published by OECD, ABS, and the RBA; their terms govern use.

## License

MIT. See [LICENSE](LICENSE).
