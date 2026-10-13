package analytics

import (
	"math"
	"testing"
	"time"

	"github.com/shanehull/housecycle/internal/model"
)

func series(vals ...float64) []model.Point {
	pts := make([]model.Point, len(vals))
	for i, v := range vals {
		pts[i] = model.Point{Date: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, i, 0), Value: v}
	}
	return pts
}

func TestLast(t *testing.T) {
	if _, ok := Last(nil); ok {
		t.Fatal("empty series should not have a last point")
	}
	p, ok := Last(series(1, 2, 3))
	if !ok || p.Value != 3 {
		t.Fatalf("want 3, got %v ok=%v", p.Value, ok)
	}
}

func TestPercentileRank(t *testing.T) {
	pts := series(1, 2, 3, 4, 5)
	if got := PercentileRank(pts, 3); got != 60 {
		t.Fatalf("want 60, got %v", got)
	}
	if got := PercentileRank(pts, 99); got != 100 {
		t.Fatalf("want 100, got %v", got)
	}
	if got := PercentileRank(nil, 1); !math.IsNaN(got) {
		t.Fatalf("want NaN, got %v", got)
	}
}

func TestAsOf(t *testing.T) {
	pts := series(10, 20, 30) // Jan, Feb, Mar 2000
	mid := time.Date(2000, 2, 20, 0, 0, 0, 0, time.UTC)
	if v, ok := AsOf(pts, mid); !ok || v != 20 {
		t.Fatalf("want 20, got %v ok=%v", v, ok)
	}
	before := time.Date(1999, 12, 31, 0, 0, 0, 0, time.UTC)
	if _, ok := AsOf(pts, before); ok {
		t.Fatal("expected no value before the series start")
	}
}
