package signals

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shanehull/housecycle/internal/model"
)

type stubLoader map[string][]model.Point

func (s stubLoader) Series(_ context.Context, id string) (model.Series, error) {
	pts, ok := s[id]
	if !ok {
		return model.Series{}, errors.New("not found")
	}
	return model.Series{ID: id, Points: pts}, nil
}

func monthly(y int, start time.Month, n int, base, step float64) []model.Point {
	pts := make([]model.Point, n)
	for i := range pts {
		pts[i] = model.Point{Date: time.Date(y, start, 1, 0, 0, 0, 0, time.UTC).AddDate(0, i, 0), Value: base + float64(i)*step}
	}
	return pts
}

func quarterly(y int, start time.Month, n int, base, step float64) []model.Point {
	pts := make([]model.Point, n)
	for i := range pts {
		pts[i] = model.Point{Date: time.Date(y, start, 1, 0, 0, 0, 0, time.UTC).AddDate(0, i*3, 0), Value: base + float64(i)*step}
	}
	return pts
}

func TestClassify(t *testing.T) {
	if classify(1.0) != Buy {
		t.Fatal("1.0 should be buy")
	}
	if classify(0.5) != Approaching {
		t.Fatal("0.5 should be approaching")
	}
	if classify(0.25) != Far {
		t.Fatal("0.25 should be far")
	}
}

func TestEvalCreditConditionsBuy(t *testing.T) {
	// Falling leverage and a wide risk spread: a washed-out, sober market.
	loader := stubLoader{
		"rba.debt_to_income": quarterly(1988, 1, 120, 160, -1),
		"rba.corp_yield_5y":  monthly(2002, 7, 120, 4, 0.05),
		"rba.ags_5y":         monthly(2002, 7, 120, 3, 0),
	}
	ind := evalCreditConditions(context.Background(), loader)
	if ind.Status != Buy {
		t.Fatalf("want buy, got %v (%+v)", ind.Status, ind.Conditions)
	}
}

func TestEvalCreditConditionsUnknownWithoutData(t *testing.T) {
	ind := evalCreditConditions(context.Background(), stubLoader{})
	if ind.Status != Unknown {
		t.Fatalf("want unknown, got %v", ind.Status)
	}
}

func TestEvalValuationCheap(t *testing.T) {
	// Prices flat while earnings rise => price-to-income falls to a low percentile.
	awe := quarterly(1995, 1, 40, 2000, 40)
	hpi := quarterly(1995, 1, 40, 100, 0)
	loader := stubLoader{"abs.au.awe": awe, "oecd.au.hpi": hpi}
	ind := evalValuation(context.Background(), loader)
	if ind.Status == Unknown {
		t.Fatal("expected an evaluation")
	}
	if len(ind.Charts) != 1 {
		t.Fatalf("want a chart, got %d", len(ind.Charts))
	}
}

func TestEvalValuationDetrendedQ(t *testing.T) {
	loader := stubLoader{
		"abs.au.housing_value":        quarterly(1990, 1, 120, 1000, 5),
		"abs.au.dwelling_replacement": quarterly(1990, 1, 120, 800, 0),
		"rba.housing_credit":          monthly(1990, 1, 360, 200, 0),
	}
	ind := evalValuation(context.Background(), loader)
	if ind.Status == Unknown {
		t.Fatal("expected an evaluation")
	}
	if len(ind.Charts) != 1 || ind.Charts[0].ID != "q" {
		t.Fatalf("want the detrended Q chart, got %+v", ind.Charts)
	}
}

func TestEvalRates(t *testing.T) {
	cash := monthly(2020, 1, 36, 4.0, -0.05)
	cpi := quarterly(2020, 1, 12, 3.0, -0.1)
	loader := stubLoader{"rba.cash_rate": cash, "rba.cpi_yoy": cpi}
	ind := evalRates(context.Background(), loader)
	if ind.Status == Unknown {
		t.Fatal("expected an evaluation")
	}
	if len(ind.Charts) != 1 || len(ind.Charts[0].Refs) == 0 {
		t.Fatal("expected a chart with a reference line")
	}
}

func TestEvalPrices(t *testing.T) {
	// Build cost growth collapses at the end, so the latest year-ended rate is the lowest on record.
	pts := quarterly(2000, 1, 39, 100, 2)
	pts = append(pts, model.Point{Date: time.Date(2009, time.October, 1, 0, 0, 0, 0, time.UTC), Value: 100})
	loader := stubLoader{"abs.au.house_construction": pts}
	ind := evalPrices(context.Background(), loader)
	if ind.Status != Buy {
		t.Fatalf("want buy, got %v (%+v)", ind.Status, ind.Conditions)
	}
	if len(ind.Charts) != 1 {
		t.Fatalf("want a chart, got %d", len(ind.Charts))
	}
}

func TestEvalVolume(t *testing.T) {
	// Transfers fall while the population is flat: a cyclical low per resident.
	loader := stubLoader{
		"abs.au.dwelling_transfers": monthly(2002, 7, 120, 40000, -100),
		"abs.au.population":         monthly(2002, 7, 120, 20000000, 0),
	}
	ind := evalVolume(context.Background(), loader)
	if ind.Status != Buy {
		t.Fatalf("want buy, got %v (%+v)", ind.Status, ind.Conditions)
	}
}

func TestEvaluateCounts(t *testing.T) {
	loader := stubLoader{
		"oecd.au.hpi":               quarterly(1996, 1, 60, 60, 1),
		"abs.au.awe":                quarterly(1996, 1, 60, 1000, 30),
		"abs.au.house_construction": quarterly(1996, 1, 60, 60, 1.2),
		"abs.au.dwelling_transfers": monthly(2002, 7, 120, 40000, -100),
		"abs.au.population":         monthly(2002, 7, 120, 20000000, 0),
		"rba.debt_to_income":        quarterly(1988, 1, 120, 160, -1),
		"rba.corp_yield_5y":         monthly(2002, 7, 120, 4, 0.05),
		"rba.ags_5y":                monthly(2002, 7, 120, 3, 0),
		"rba.cash_rate":             monthly(2020, 1, 36, 4.0, -0.05),
		"rba.cpi_yoy":               quarterly(2020, 1, 12, 3.0, -0.1),
	}
	d := Evaluate(context.Background(), loader)
	if d.Evaluated != 5 {
		t.Fatalf("want 5 evaluated, got %d", d.Evaluated)
	}
	if d.Score < 0 || d.Score > 1 {
		t.Fatalf("score out of range: %v", d.Score)
	}
}
