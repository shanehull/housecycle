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

// The ABS 5206.0 household income account is an XLSX data cube. It publishes
// gross disposable income in trend, seasonally adjusted and original form. The
// price-to-income measure uses the seasonally adjusted block.
const absIncomeLatest = "https://www.abs.gov.au/statistics/economy/national-accounts/" +
	"australian-national-accounts-national-income-expenditure-and-product/latest-release"

var incomeHrefRe = regexp.MustCompile(`href="([^"]*5206020_Household_Income\.xlsx)"`)

func (a ABS) fetchHouseholdDisposableIncome(ctx context.Context, cfg Config) ([]model.Series, error) {
	page, err := get(ctx, cfg, absIncomeLatest, "text/html", nil)
	if err != nil {
		return nil, err
	}
	m := incomeHrefRe.FindSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("household income xlsx link not found (page layout changed)")
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

	col := findColumnSA(data, "GROSS DISPOSABLE INCOME")
	if col < 0 {
		return nil, fmt.Errorf("gross disposable income column not found")
	}

	var points []model.Point
	for _, row := range data[dataStart(data):] {
		if len(row) == 0 {
			continue
		}
		t, ok := cellDate(row[0])
		if !ok {
			continue
		}
		if v, ok := cellFloat(row, col); ok {
			points = append(points, model.Point{Date: t, Value: v})
		}
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("no household income rows parsed")
	}

	return []model.Series{
		{ID: "abs.au.household_disposable_income", Label: "Households: gross disposable income (seasonally adjusted)", Unit: "$m", Freq: "Q", Source: "ABS", Points: points},
	}, nil
}

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

func findColumnSA(rows [][]string, title string) int {
	if len(rows) < 3 {
		return -1
	}
	for i, h := range rows[0] {
		if strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(h), ";")) != title {
			continue
		}
		if i < len(rows[2]) && strings.TrimSpace(rows[2][i]) == "Seasonally Adjusted" {
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
