package source

import (
	"testing"
	"time"

	"github.com/shanehull/housecycle/internal/model"
)

func TestParsePeriod(t *testing.T) {
	cases := map[string]time.Time{
		"2026-Q2":    time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		"1970-Q1":    time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		"1994-S2":    time.Date(1994, 11, 1, 0, 0, 0, 0, time.UTC),
		"1994-S1":    time.Date(1994, 5, 1, 0, 0, 0, 0, time.UTC),
		"2026-06":    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		"2026-06-01": time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		"11/10/2026": time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC),
	}
	for in, want := range cases {
		got, err := parsePeriod(in)
		if err != nil {
			t.Fatalf("parsePeriod(%q): %v", in, err)
		}
		if !got.Equal(want) {
			t.Fatalf("parsePeriod(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := parsePeriod("nonsense"); err == nil {
		t.Fatal("expected error for nonsense period")
	}
}

func TestParsePeriodValueCSV(t *testing.T) {
	body := []byte("FREQ,REF_AREA,TIME_PERIOD,OBS_VALUE\n" +
		"Q,AU,1970-Q1,3.1274\n" +
		"Q,AU,1970-Q2,3.1846\n" +
		"Q,AU,1970-Q3,\n")
	pts, err := parsePeriodValueCSV(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 2 {
		t.Fatalf("want 2 points, got %d", len(pts))
	}
	if pts[0].Value != 3.1274 || pts[1].Value != 3.1846 {
		t.Fatalf("unexpected values: %+v", pts)
	}
}

const aweFixture = `{
  "data": {
    "structures": [{
      "dimensions": {
        "series": [
          {"id":"MEASURE","values":[{"id":"1","name":"All employees"}]},
          {"id":"SEX","values":[{"id":"3","name":"Persons"}]},
          {"id":"FREQ","values":[{"id":"S","name":"Half-yearly"}]}
        ],
        "observation": [
          {"id":"TIME_PERIOD","values":[{"id":"2024-S1"},{"id":"2024-S2"}]}
        ]
      }
    }],
    "dataSets": [{
      "series": {
        "0:0:0": {"observations": {"0":[1600.0],"1":[1650.0]}}
      }
    }]
  }
}`

func TestParseSDMX(t *testing.T) {
	ds, err := parseSDMX([]byte(aweFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Series) != 1 {
		t.Fatalf("want 1 series, got %d", len(ds.Series))
	}
	pts := toPoints(&ds.Series[0])
	if len(pts) != 2 || pts[1].Value != 1650 {
		t.Fatalf("unexpected points: %+v", pts)
	}
}

const rbaE2Fixture = `E2 HOUSEHOLD FINANCES – SELECTED RATIOS
Title,Household debt to assets,Household debt to income
Description,Ratio of household debt to household assets,Ratio of household debt to annualised household disposable income
Frequency,Quarterly,Quarterly
Type,Original,Original
Units,Per cent,Per cent

Source,ABS,ABS
Publication date,25-Sep-2026,25-Sep-2026
Series ID,BHFDA,BHFDDIT
31/03/1988,100.0,60.5
30/06/1988,101.0,61.5
`

func TestParseRBAColumn(t *testing.T) {
	pts, err := parseRBAColumn([]byte(rbaE2Fixture), "Household debt to income")
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 2 || pts[0].Value != 60.5 || pts[1].Value != 61.5 {
		t.Fatalf("unexpected points: %+v", pts)
	}
	if got := pts[0].Date.Format("2006-01-02"); got != "1988-03-31" {
		t.Fatalf("want 1988-03-31, got %s", got)
	}
	if _, err := parseRBAColumn([]byte(rbaE2Fixture), "No such column"); err == nil {
		t.Fatal("expected error for a missing column")
	}
}

func TestCellDate(t *testing.T) {
	for in, want := range map[string]string{
		"2026-06-01":          "2026-06-01",
		"2026-06-01 00:00:00": "2026-06-01",
	} {
		got, ok := cellDate(in)
		if !ok || got.Format("2006-01-02") != want {
			t.Fatalf("cellDate(%q) = %v %v, want %s", in, got, ok, want)
		}
	}
	if _, ok := cellDate("nonsense"); ok {
		t.Fatal("expected not ok for nonsense")
	}
}

func TestFindColumnSA(t *testing.T) {
	rows := [][]string{
		{"Date", "GROSS DISPOSABLE INCOME ;", "GROSS DISPOSABLE INCOME ;"},
		{"Unit", "$ Millions", "$ Millions"},
		{"Series Type", "Trend", "Seasonally Adjusted"},
	}
	if got := findColumnSA(rows, "GROSS DISPOSABLE INCOME"); got != 2 {
		t.Fatalf("findColumnSA = %d, want 2", got)
	}
	if got := findColumnSA(rows, "Missing"); got != -1 {
		t.Fatalf("findColumnSA missing = %d, want -1", got)
	}
}

func TestExtendSeriesPrefersLive(t *testing.T) {
	history := []model.Point{
		{Date: time.Date(2013, time.March, 1, 0, 0, 0, 0, time.UTC), Value: 3},
		{Date: time.Date(2013, time.April, 1, 0, 0, 0, 0, time.UTC), Value: 2.9},
	}
	live := []model.Point{
		{Date: time.Date(2013, time.April, 1, 0, 0, 0, 0, time.UTC), Value: 2.5},
		{Date: time.Date(2013, time.May, 1, 0, 0, 0, 0, time.UTC), Value: 2.4},
	}
	got := extendSeries(live, history)
	if len(got) != 3 {
		t.Fatalf("want 3 points, got %d: %+v", len(got), got)
	}
	if got[0].Value != 3 || got[1].Value != 2.5 || got[2].Value != 2.4 {
		t.Fatalf("unexpected merge: %+v", got)
	}
}

func TestEmbeddedBondHistory(t *testing.T) {
	if len(ags5yHistory) < 400 {
		t.Fatalf("want the full monthly history, got %d points", len(ags5yHistory))
	}
	first, last := ags5yHistory[0], ags5yHistory[len(ags5yHistory)-1]
	if first.Date.Year() != 1972 || last.Date.Year() != 2013 {
		t.Fatalf("unexpected span: %v to %v", first.Date, last.Date)
	}
	if last.Value < 2 || last.Value > 4 {
		t.Fatalf("implausible last yield: %v", last.Value)
	}
}
