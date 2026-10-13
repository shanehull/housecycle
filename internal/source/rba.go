package source

import (
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"github.com/shanehull/housecycle/internal/model"
)

// RBA fetches the cash rate target (F1.1), year-ended CPI inflation (G1), and
// the credit-conditions series: household debt-to-income (E2) and the corporate
// and government bond yields behind the credit spread (F3, F2.1).
type RBA struct{}

func (RBA) Name() string { return "RBA" }

const (
	rbaF11 = "https://www.rba.gov.au/statistics/tables/csv/f1.1-data.csv"
	rbaG1  = "https://www.rba.gov.au/statistics/tables/csv/g1-data.csv"
	rbaF21 = "https://www.rba.gov.au/statistics/tables/csv/f2.1-data.csv"
	rbaF3  = "https://www.rba.gov.au/statistics/tables/csv/f3-data.csv"
	rbaE2  = "https://www.rba.gov.au/statistics/tables/csv/e2-data.csv"
)

func (RBA) Fetch(ctx context.Context, cfg Config) ([]model.Series, error) {
	column := func(url, title, id, label, freq string) func(context.Context) ([]model.Series, error) {
		return func(ctx context.Context) ([]model.Series, error) {
			body, err := get(ctx, cfg, url, "text/csv", nil)
			if err != nil {
				return nil, fmt.Errorf("rba %s: %w", id, err)
			}
			points, err := parseRBAColumn(body, title)
			if err != nil {
				return nil, fmt.Errorf("rba %s: %w", id, err)
			}
			return []model.Series{
				{ID: id, Label: label, Unit: "%", Freq: freq, Source: "RBA", Points: points},
			}, nil
		}
	}

	return fetchAll(ctx,
		column(rbaF11, "Cash Rate Target", "rba.cash_rate", "RBA cash rate target (monthly average)", "M"),
		column(rbaG1, "Year-ended inflation", "rba.cpi_yoy", "CPI inflation, year-ended", "Q"),
		column(rbaF3, "Non-financial corporate A-rated bonds – Yield – 5 year target tenor", "rba.corp_yield_5y", "Non-financial corporate A-rated bond yield, 5 year", "M"),
		column(rbaF21, "Australian Government 5 year bond", "rba.ags_5y", "Australian Government 5 year bond yield", "M"),
		column(rbaE2, "Household debt to income", "rba.debt_to_income", "Household debt to income", "Q"),
	)
}

// parseRBAColumn reads the column named title from an RBA statistics CSV body.
func parseRBAColumn(body []byte, title string) ([]model.Point, error) {
	lines := strings.Split(string(body), "\n")

	headerAt, dataAt := -1, -1
	for i, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if headerAt < 0 && strings.HasPrefix(trimmed, "Title,") {
			headerAt = i
		}
		if strings.HasPrefix(trimmed, "Series ID,") {
			dataAt = i + 1
			break
		}
	}
	if headerAt < 0 || dataAt < 0 || dataAt >= len(lines) {
		return nil, fmt.Errorf("could not locate %q in RBA table", title)
	}

	titles, err := csv.NewReader(strings.NewReader(lines[headerAt])).Read()
	if err != nil {
		return nil, err
	}
	col := -1
	for i, t := range titles {
		if strings.TrimSpace(t) == title {
			col = i
			break
		}
	}
	if col < 0 {
		return nil, fmt.Errorf("column %q not found in header", title)
	}

	r := csv.NewReader(strings.NewReader(strings.Join(lines[dataAt:], "\n")))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var points []model.Point
	for _, row := range rows {
		if col >= len(row) {
			continue
		}
		t, err := parsePeriod(strings.TrimSpace(row[0]))
		if err != nil {
			continue
		}
		raw := strings.TrimSpace(row[col])
		if raw == "" {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		points = append(points, model.Point{Date: t, Value: v})
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("no observations for %q", title)
	}
	return points, nil
}
