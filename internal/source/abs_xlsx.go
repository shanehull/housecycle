package source

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shanehull/housecycle/internal/model"
	"github.com/shanehull/housecycle/internal/xlsx"
)

// The ABS household balance sheet (Table 35, 5232.0) is published as an XLSX
// data cube. It values dwellings at replacement cost and land at market value,
// from 1988, giving the Q ratio its replacement-cost denominator.
const absHouseholdLatest = "https://www.abs.gov.au/statistics/economy/national-accounts/" +
	"australian-national-accounts-finance-and-wealth/latest-release"

var xlsxHrefRe = regexp.MustCompile(`href="([^"]*5232035\.xlsx)"`)

func (a ABS) fetchHouseholdBalanceSheet(ctx context.Context, cfg Config) ([]model.Series, error) {
	page, err := get(ctx, cfg, absHouseholdLatest, "text/html", nil)
	if err != nil {
		return nil, err
	}
	m := xlsxHrefRe.FindSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("household balance sheet xlsx link not found (page layout changed)")
	}

	body, err := get(ctx, cfg, "https://www.abs.gov.au"+string(m[1]),
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", nil)
	if err != nil {
		return nil, err
	}
	sheets, err := xlsx.Read(body)
	if err != nil {
		return nil, err
	}
	data := sheets["Data1"]
	if len(data) == 0 {
		return nil, fmt.Errorf("Data1 sheet missing")
	}

	housingCol := findColumn(data[0], "Residential land and dwellings")
	replacementCol := findColumn(data[0], "Non-financial - Produced - Fixed assets - Dwellings")
	if housingCol < 0 || replacementCol < 0 {
		return nil, fmt.Errorf("expected columns not found in ABS household balance sheet")
	}

	var housing, replacement []model.Point
	for _, row := range data[dataStart(data):] {
		if len(row) == 0 {
			continue
		}
		t, ok := cellDate(row[0])
		if !ok {
			continue
		}
		if v, ok := cellFloat(row, housingCol); ok {
			housing = append(housing, model.Point{Date: t, Value: v})
		}
		if v, ok := cellFloat(row, replacementCol); ok {
			replacement = append(replacement, model.Point{Date: t, Value: v})
		}
	}
	if len(housing) == 0 || len(replacement) == 0 {
		return nil, fmt.Errorf("no household balance sheet rows parsed")
	}

	return []model.Series{
		{ID: "abs.au.housing_value", Label: "Households: residential land and dwellings", Unit: "$b", Freq: "Q", Source: "ABS", Points: housing},
		{ID: "abs.au.dwelling_replacement", Label: "Households: dwellings at replacement cost", Unit: "$b", Freq: "Q", Source: "ABS", Points: replacement},
	}, nil
}

// dataStart finds the first row whose first cell is a date.
func dataStart(rows [][]string) int {
	for i, row := range rows {
		if len(row) > 0 {
			if _, ok := cellDate(row[0]); ok {
				return i
			}
		}
	}
	return len(rows)
}

// cellDate parses either an ISO date string or an Excel serial number (1900 system).
func cellDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 10 {
		if t, err := time.Parse("2006-01-02", raw[:10]); err == nil {
			return t, true
		}
	}
	if serial, err := strconv.ParseFloat(raw, 64); err == nil {
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(serial)), true
	}
	return time.Time{}, false
}

func findColumn(header []string, title string) int {
	for i, h := range header {
		if strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(h), ";")) == title {
			return i
		}
	}
	return -1
}

func cellFloat(row []string, col int) (float64, bool) {
	if col < 0 || col >= len(row) {
		return 0, false
	}
	raw := strings.ReplaceAll(strings.TrimSpace(row[col]), ",", "")
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
