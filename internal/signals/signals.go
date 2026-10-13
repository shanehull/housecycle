// Package signals turns stored series into the housing buy signals: valuation
// (price to income and the detrended Q ratio), volume, price stability, credit
// conditions, and the real cost of capital.
package signals

import (
	"context"
	"fmt"

	"github.com/shanehull/housecycle/internal/analytics"
	"github.com/shanehull/housecycle/internal/model"
)

// Status is how close an indicator is to a buy signal.
type Status int

const (
	Unknown Status = iota
	Far
	Approaching
	Buy
)

func (s Status) String() string {
	switch s {
	case Far:
		return "far"
	case Approaching:
		return "approaching"
	case Buy:
		return "buy"
	default:
		return "unknown"
	}
}

// Thresholds encode the buy rules. Tune here.
const (
	cheapPct            = 33.0 // % percentile below which a valuation measure is cheap
	volumeLowPct        = 25.0 // % percentile below which buying volume is at a cyclical low
	creditSpreadWidePct = 66.0 // % percentile above which credit risk is priced rather than complacent
	debtLowPct          = 33.0 // % percentile below which household leverage is back in its low range
	ppiFlatQoQ          = 1.0  // % q/q construction cost growth considered near zero
	rateHaltMonths      = 3    // months without a cash-rate move considered on hold
	realRateLowPct      = 40.0 // % real cash rate below this percentile is cheap money
)

// Loader retrieves a stored series by ID.
type Loader interface {
	Series(ctx context.Context, id string) (model.Series, error)
}

// Condition is one testable rule within an indicator.
type Condition struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Target string `json:"target"`
	Met    bool   `json:"met"`
}

// Ref is a horizontal reference line drawn on a chart.
type Ref struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Kind  string  `json:"kind"` // "parity" | "quartile"
}

// Chart is a series to plot for an indicator.
type Chart struct {
	ID     string        `json:"id"`
	Label  string        `json:"label"`
	Unit   string        `json:"unit"`
	Freq   string        `json:"freq"`
	Points []model.Point `json:"points"`
	Refs   []Ref         `json:"refs,omitempty"`
}

// Indicator is an evaluated buy signal.
type Indicator struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Blurb      string      `json:"blurb"`
	Status     Status      `json:"status"`
	Score      float64     `json:"score"`
	Conditions []Condition `json:"conditions"`
	Charts     []Chart     `json:"charts"`
	Note       string      `json:"note"`
}

// Dashboard is the full evaluated view.
type Dashboard struct {
	Indicators []Indicator `json:"indicators"`
	SignalsMet int         `json:"signalsMet"`
	Evaluated  int         `json:"evaluated"`
	Score      float64     `json:"score"`
}

// Evaluate builds the dashboard.
func Evaluate(ctx context.Context, loader Loader) *Dashboard {
	d := &Dashboard{
		Indicators: []Indicator{
			evalValuation(ctx, loader),
			evalVolume(ctx, loader),
			evalPrices(ctx, loader),
			evalCreditConditions(ctx, loader),
			evalRates(ctx, loader),
		},
	}
	for _, ind := range d.Indicators {
		if ind.Status == Unknown {
			continue
		}
		d.Evaluated++
		if ind.Status == Buy {
			d.SignalsMet++
		}
	}
	if d.Evaluated > 0 {
		d.Score = float64(d.SignalsMet) / float64(d.Evaluated)
	}
	return d
}

// --- Valuation: price to income and the detrended Q ratio ---

func evalValuation(ctx context.Context, loader Loader) Indicator {
	ind := Indicator{
		ID:    "valuation",
		Name:  "Valuation",
		Blurb: "Australian house prices measured against household income, and the detrended Q ratio: the dwelling stock net of its housing debt, over the cost of replacing it, scaled by its running geometric mean. Cheap on both is the strongest buy signal.",
	}

	hpi := loadSeries(ctx, loader, "oecd.au.hpi")
	awe := loadSeries(ctx, loader, "abs.au.awe")

	var conditions []Condition
	var charts []Chart

	// Price to income.
	if hpi != nil && awe != nil {
		pti := analytics.RatioSeries(hpi.Points, awe.Points)
		if len(pti) >= 8 {
			last, _ := analytics.Last(pti)
			pct := analytics.PercentileRank(pti, last.Value)
			conditions = append(conditions, Condition{
				Label:  "Price-to-income cheap",
				Value:  pctile(pct),
				Target: fmt.Sprintf("<=%s", ord(int(cheapPct))),
				Met:    pct <= cheapPct,
			})
			charts = append(charts, Chart{ID: "pti", Label: "Price-to-income", Unit: "ratio", Freq: "Q", Points: pti, Refs: quartileRefs(pti)})
		}
	}

	// Detrended Q ratio: the market value of the dwelling stock less the housing
	// credit against it, over what the structure would cost to rebuild, then
	// divided by that ratio's running geometric mean so the oscillator is
	// time-invariant and mean-reverting. Market value and replacement cost are
	// the ABS household balance sheet; housing credit is RBA D2.
	value := loadSeries(ctx, loader, "abs.au.housing_value")
	replacement := loadSeries(ctx, loader, "abs.au.dwelling_replacement")
	debt := loadSeries(ctx, loader, "rba.housing_credit")
	if value != nil && replacement != nil && debt != nil {
		var equity []model.Point
		for _, p := range value.Points {
			d, ok := analytics.AsOf(debt.Points, p.Date)
			if !ok {
				continue
			}
			equity = append(equity, model.Point{Date: p.Date, Value: p.Value - d})
		}
		ratio := analytics.RatioSeries(equity, replacement.Points)
		q := analytics.Detrend(ratio)
		if len(q) >= 8 {
			last, _ := analytics.Last(q)
			pct := analytics.PercentileRank(q, last.Value)
			conditions = append(conditions, Condition{
				Label:  "Detrended Q ratio cheap",
				Value:  fmt.Sprintf("%.2f (%s)", last.Value, pctile(pct)),
				Target: fmt.Sprintf("<=%s", ord(int(cheapPct))),
				Met:    pct <= cheapPct,
			})
			charts = append(charts, Chart{
				ID: "q", Label: "Detrended Q ratio", Unit: "index", Freq: "Q",
				Points: q, Refs: quartileRefs(q),
			})
			ind.Note = fmt.Sprintf("Residential land and dwellings less housing credit, over dwellings at replacement cost, scaled by its running geometric mean, from %s.", ratio[0].Date.Format("Jan 2006"))
		}
	}

	finish(&ind, conditions, charts)
	return ind
}

// --- Volume (housing turnover) --------------------------------------------------

func evalVolume(ctx context.Context, loader Loader) Indicator {
	ind := Indicator{
		ID:    "volume",
		Name:  "Volume",
		Blurb: "Housing turnover: dwelling transfers (property sales) per resident, all buyers, scaled to population so it stays comparable across decades. Bears die on low volume.",
	}

	transfers := loadSeries(ctx, loader, "abs.au.dwelling_transfers")
	pop := loadSeries(ctx, loader, "abs.au.population")
	if transfers == nil || pop == nil {
		finish(&ind, nil, nil)
		return ind
	}
	rate := scale(analytics.RatioSeries(transfers.Points, pop.Points), 1000)
	if len(rate) < 8 {
		finish(&ind, nil, nil)
		return ind
	}
	last, _ := analytics.Last(rate)
	pct := analytics.PercentileRank(rate, last.Value)
	ind.Note = fmt.Sprintf("Dwelling transfers per 1,000 residents, from %s. Established houses and attached dwellings, all buyers.",
		rate[0].Date.Format("Jan 2006"))

	finish(&ind, []Condition{{
		Label:  "Dwelling transfers at a cyclical low",
		Value:  pctile(pct),
		Target: fmt.Sprintf("<=%s", ord(int(volumeLowPct))),
		Met:    pct <= volumeLowPct,
	}}, []Chart{{
		ID: "transfers_per_capita", Label: "Dwelling transfers per 1,000 residents", Unit: "per 1,000",
		Freq: "Q", Points: rate, Refs: quartileRefs(rate),
	}})
	return ind
}

// --- Price stability ------------------------------------------------------------

func evalPrices(ctx context.Context, loader Loader) Indicator {
	ind := Indicator{
		ID:    "prices",
		Name:  "Price stability",
		Blurb: "The input side of the housing market: construction cost growth has stopped and the cash rate is on hold, so building costs and mortgage rates are not pushing prices higher.",
	}

	var conditions []Condition
	var charts []Chart

	if cost := loadSeries(ctx, loader, "abs.au.house_construction"); cost != nil {
		_, qoq, ok := analytics.Change(cost.Points, 1)
		met := ok && abs(qoq) <= ppiFlatQoQ
		conditions = append(conditions, Condition{
			Label:  "Construction cost growth near zero",
			Value:  fmt.Sprintf("%+.2f%% q/q", qoq),
			Target: fmt.Sprintf("±%.1f%% q/q", ppiFlatQoQ),
			Met:    met,
		})
		charts = append(charts, Chart{
			ID: "cost_qoq", Label: "House construction cost, % change q/q", Unit: "% q/q",
			Freq: "Q", Points: analytics.PctChange(cost.Points, 1),
			Refs: []Ref{{Label: "0%", Value: 0, Kind: "parity"}},
		})
	}

	if cash := loadSeries(ctx, loader, "rba.cash_rate"); cash != nil {
		months := analytics.MonthsSinceChange(cash.Points)
		conditions = append(conditions, Condition{
			Label:  "Cash rate on hold",
			Value:  fmt.Sprintf("%dmo flat", months),
			Target: fmt.Sprintf(">=%d months", rateHaltMonths),
			Met:    months >= rateHaltMonths,
		})
	}

	finish(&ind, conditions, charts)
	return ind
}

// --- Credit conditions ----------------------------------------------------------

func evalCreditConditions(ctx context.Context, loader Loader) Indicator {
	ind := Indicator{
		ID:    "credit",
		Name:  "Credit conditions",
		Blurb: "Credit conditions: household leverage (debt-to-income against its own history) and the price of credit risk (the corporate spread over government borrowing). Buy when leverage is back in its low range and credit risk is priced.",
	}

	debt := loadSeries(ctx, loader, "rba.debt_to_income")
	corp := loadSeries(ctx, loader, "rba.corp_yield_5y")
	ags := loadSeries(ctx, loader, "rba.ags_5y")

	var conditions []Condition
	var charts []Chart

	if debt != nil {
		last, _ := analytics.Last(debt.Points)
		pct := analytics.PercentileRank(debt.Points, last.Value)
		conditions = append(conditions, Condition{
			Label:  "Household debt-to-income low",
			Value:  fmt.Sprintf("%.1f%% (%s)", last.Value, pctile(pct)),
			Target: fmt.Sprintf("<=%s", ord(int(debtLowPct))),
			Met:    pct <= debtLowPct,
		})
		charts = append(charts, Chart{
			ID: "debt_to_income", Label: "Household debt to income", Unit: "%", Freq: "Q",
			Points: debt.Points, Refs: quartileRefs(debt.Points),
		})
	}

	if corp != nil && ags != nil {
		var spread []model.Point
		for _, p := range corp.Points {
			g, ok := analytics.AsOf(ags.Points, p.Date)
			if !ok {
				continue
			}
			spread = append(spread, model.Point{Date: p.Date, Value: p.Value - g})
		}
		if len(spread) >= 8 {
			last, _ := analytics.Last(spread)
			pct := analytics.PercentileRank(spread, last.Value)
			conditions = append(conditions, Condition{
				Label:  "Credit risk priced",
				Value:  fmt.Sprintf("%.2fpp (%s)", last.Value, pctile(pct)),
				Target: fmt.Sprintf(">=%s", ord(int(creditSpreadWidePct))),
				Met:    pct >= creditSpreadWidePct,
			})
			charts = append(charts, Chart{
				ID: "credit_spread", Label: "Corporate A-rated spread, 5-year", Unit: "pp", Freq: "M",
				Points: spread, Refs: quartileRefs(spread),
			})
		}
	}

	finish(&ind, conditions, charts)
	return ind
}

// --- Real cost of capital -------------------------------------------------------

func evalRates(ctx context.Context, loader Loader) Indicator {
	ind := Indicator{
		ID:    "rates",
		Name:  "Real cost of capital",
		Blurb: "The real cost of housing finance: the cash rate minus year-ended CPI inflation. Housing is the most rate-sensitive asset, and a low real rate is what turns affordability.",
	}

	cash := loadSeries(ctx, loader, "rba.cash_rate")
	cpi := loadSeries(ctx, loader, "rba.cpi_yoy")
	if cash == nil || cpi == nil {
		finish(&ind, nil, nil)
		return ind
	}

	var real []model.Point
	for _, p := range cash.Points {
		c, ok := analytics.AsOf(cpi.Points, p.Date)
		if !ok {
			continue
		}
		real = append(real, model.Point{Date: p.Date, Value: p.Value - c})
	}
	if len(real) < 8 {
		finish(&ind, nil, nil)
		return ind
	}

	last, _ := analytics.Last(real)
	pct := analytics.PercentileRank(real, last.Value)
	finish(&ind, []Condition{{
		Label:  "Real cash rate low",
		Value:  fmt.Sprintf("%+.2f%% (%s)", last.Value, pctile(pct)),
		Target: fmt.Sprintf("<=%s", ord(int(realRateLowPct))),
		Met:    pct <= realRateLowPct,
	}}, []Chart{{
		ID: "real_rate", Label: "Real cash rate (cash rate minus CPI)", Unit: "%",
		Freq: "M", Points: real,
		Refs: append(quartileRefs(real), Ref{Label: "0%", Value: 0, Kind: "parity"}),
	}})
	return ind
}

// --- helpers -------------------------------------------------------------------

func finish(ind *Indicator, conditions []Condition, charts []Chart) {
	if conditions == nil {
		conditions = []Condition{}
	}
	if charts == nil {
		charts = []Chart{}
	}
	ind.Conditions = conditions
	ind.Charts = charts
	if len(conditions) == 0 {
		ind.Status = Unknown
		return
	}
	met := 0
	for _, c := range conditions {
		if c.Met {
			met++
		}
	}
	ind.Score = float64(met) / float64(len(conditions))
	ind.Status = classify(ind.Score)
}

func classify(score float64) Status {
	switch {
	case score >= 0.999:
		return Buy
	case score >= 0.5:
		return Approaching
	default:
		return Far
	}
}

// loadSeries returns the series, or nil when it is absent or empty.
func loadSeries(ctx context.Context, loader Loader, id string) *model.Series {
	ser, err := loader.Series(ctx, id)
	if err != nil || len(ser.Points) == 0 {
		return nil
	}
	return &ser
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// scale multiplies every point value, used to express a ratio in readable units.
func scale(points []model.Point, f float64) []model.Point {
	out := make([]model.Point, len(points))
	for i, p := range points {
		out[i] = model.Point{Date: p.Date, Value: p.Value * f}
	}
	return out
}

// quartileRefs returns Q1 and Q3 reference lines for a series.
func quartileRefs(points []model.Point) []Ref {
	q1, ok := analytics.Quantile(points, 0.25)
	if !ok {
		return nil
	}
	q3, _ := analytics.Quantile(points, 0.75)
	return []Ref{
		{Label: "Q1", Value: q1, Kind: "quartile"},
		{Label: "Q3", Value: q3, Kind: "quartile"},
	}
}

// ord renders an ordinal like 1st, 2nd, 3rd, 33rd, 111th.
func ord(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

func pctile(v float64) string { return fmt.Sprintf("%s pctile", ord(int(v))) }
