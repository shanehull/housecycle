package source

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"github.com/shanehull/housecycle/internal/model"
)

// The current RBA tables carry bond yields from September 2013 only. Earlier
// history lives in the historical workbook (xls-hist/f02histhist.xls, F2
// Capital Market Yields - Government Bonds), a binary XLS the stdlib cannot
// read. This is that workbook's 5-year Australian Government column extracted
// to CSV, monthly, January 1972 to May 2013.
//
//go:embed data/ags_5y_monthly.csv
var ags5yHistoryCSV []byte

var ags5yHistory = mustParseHistory(ags5yHistoryCSV)

func mustParseHistory(body []byte) []model.Point {
	points, err := parseHistory(body)
	if err != nil {
		panic(fmt.Sprintf("source: embedded 5-year bond history: %v", err))
	}
	return points
}

func parseHistory(body []byte) ([]model.Point, error) {
	rows, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		return nil, err
	}
	var points []model.Point
	for i, row := range rows {
		if len(row) < 2 {
			return nil, fmt.Errorf("row %d: want date and value", i+1)
		}
		date, value := strings.TrimSpace(row[0]), strings.TrimSpace(row[1])
		if i == 0 || date == "date" {
			continue
		}
		t, err := parsePeriod(date)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		points = append(points, model.Point{Date: t, Value: v})
	}
	return points, nil
}

// extendSeries prepends history that predates the live series. The live table
// wins wherever the two overlap.
func extendSeries(live, history []model.Point) []model.Point {
	if len(live) == 0 || len(history) == 0 {
		return live
	}
	first := live[0].Date
	out := make([]model.Point, 0, len(live)+len(history))
	for _, p := range history {
		if p.Date.Before(first) {
			out = append(out, p)
		}
	}
	return append(out, live...)
}
