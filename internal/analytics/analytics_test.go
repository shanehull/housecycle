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
