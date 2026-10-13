// Package model defines the core data types shared across the packages.
package model

import "time"

// Point is a single observation.
type Point struct {
	Date  time.Time `json:"date"`
	Value float64   `json:"value"`
}

// Series is a named, dated sequence of observations from one source.
type Series struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Unit   string  `json:"unit"`
	Freq   string  `json:"freq"`   // Q, M, W, D
	Source string  `json:"source"` // ABS, OECD, RBA
	Points []Point `json:"points"`
}
