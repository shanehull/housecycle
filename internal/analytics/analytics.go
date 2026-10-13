// Package analytics provides the pure computations used to turn raw series into signals.
package analytics

import (
	"math"
	"slices"
	"sort"
	"time"

	"github.com/shanehull/housecycle/internal/model"
)

// Last returns the most recent point, or false when the series is empty.
func Last(points []model.Point) (model.Point, bool) {
	if len(points) == 0 {
		return model.Point{}, false
	}
	return points[len(points)-1], true
}

// PercentileRank returns the share of observations at or below value, in 0..100.
// It answers "where does the current reading sit in its own history?" — the basis
// for spotting cyclical lows.
func PercentileRank(points []model.Point, value float64) float64 {
	if len(points) == 0 {
		return math.NaN()
	}
	n := 0
	for _, p := range points {
		if p.Value <= value {
			n++
		}
	}
	return float64(n) / float64(len(points)) * 100
}

// AsOf returns the value of the most recent point at or before t, forward-filling
// a slower series (e.g. quarterly inflation) onto a faster timeline.
func AsOf(points []model.Point, t time.Time) (float64, bool) {
	idx := sort.Search(len(points), func(i int) bool { return points[i].Date.After(t) })
	if idx == 0 {
		return 0, false
	}
	return points[idx-1].Value, true
}

// Change returns the change over the last n observations (n back from the end).
func Change(points []model.Point, n int) (abs, pctChange float64, ok bool) {
	if len(points) <= n {
		return 0, 0, false
	}
	cur := points[len(points)-1].Value
	prev := points[len(points)-1-n].Value
	if prev == 0 {
		return cur - prev, 0, false
	}
	return cur - prev, (cur - prev) / prev * 100, true
}

// PctChange returns the period-on-period percentage change with the given lag,
// dated at the later point. It turns a trending index into a stationary series.
func PctChange(points []model.Point, lag int) []model.Point {
	if lag < 1 {
		lag = 1
	}
	out := make([]model.Point, 0, len(points))
	for i := lag; i < len(points); i++ {
		prev := points[i-lag].Value
		if prev == 0 {
			continue
		}
		out = append(out, model.Point{Date: points[i].Date, Value: (points[i].Value/prev - 1) * 100})
	}
	return out
}

// MonthsSinceChange returns how many periods since the series last changed value.
func MonthsSinceChange(points []model.Point) int {
	if len(points) < 2 {
		return 0
	}
	last := points[len(points)-1].Value
	for i := len(points) - 2; i >= 0; i-- {
		if points[i].Value != last {
			return len(points) - 1 - i
		}
	}
	return len(points) - 1
}

// RatioSeries returns fast/slow with slow forward-filled onto each fast date.
func RatioSeries(fast, slow []model.Point) []model.Point {
	out := make([]model.Point, 0, len(fast))
	for _, p := range fast {
		s, ok := AsOf(slow, p.Date)
		if !ok || s == 0 {
			continue
		}
		out = append(out, model.Point{Date: p.Date, Value: p.Value / s})
	}
	return out
}

// Quantile returns the p-quantile (0..1) of the point values by linear interpolation.
func Quantile(points []model.Point, p float64) (float64, bool) {
	if len(points) == 0 {
		return 0, false
	}
	vals := make([]float64, len(points))
	for i, pt := range points {
		vals[i] = pt.Value
	}
	slices.Sort(vals)
	idx := p * float64(len(vals)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo < 0 {
		lo = 0
	}
	if hi >= len(vals) {
		hi = len(vals) - 1
	}
	frac := idx - float64(lo)
	return vals[lo]*(1-frac) + vals[hi]*frac, true
}

// Detrend divides each ratio by the cumulative geometric mean of all ratios up
// to that point, giving a time-invariant, mean-reverting oscillator centred on
// 1.0 that isolates short-term valuation anomalies.
func Detrend(ratio []model.Point) []model.Point {
	out := make([]model.Point, 0, len(ratio))
	product := 1.0
	for i, p := range ratio {
		if p.Value == 0 {
			continue
		}
		product *= p.Value
		geoMean := math.Pow(product, 1.0/float64(i+1))
		if geoMean == 0 {
			continue
		}
		out = append(out, model.Point{Date: p.Date, Value: p.Value / geoMean})
	}
	return out
}
