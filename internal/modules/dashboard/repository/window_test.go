package repository

import (
	"testing"
	"time"
)

func TestWindowCalendarUTC(t *testing.T) {
	for _, pair := range [][2]string{{"2026-03-01T00:00:00Z", "2026-01-31"}, {"2024-03-01T00:00:00Z", "2024-02-01"}, {"2026-01-01T01:30:00+03:00", "2025-12-02"}} {
		now, err := time.Parse(time.RFC3339, pair[0])
		if err != nil {
			t.Fatal(err)
		}
		got := Window(now)
		if got.Format("2006-01-02") != pair[1] || got.Hour() != 0 || got.Location() != time.UTC {
			t.Fatal(pair, got)
		}
	}
}
