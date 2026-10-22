// Package repository reads tenant aggregates directly from MySQL.
package repository

import (
	"context"
	"fmt"
	"time"

	dbgen "gopanel/internal/db/generated"
)

type Day struct {
	Date    time.Time
	Total   int64
	Percent int
}
type Snapshot struct {
	ActiveCustomers, ActiveProducts, RecentCustomers int64
	Days                                             []Day
	Start, AsOf                                      time.Time
}
type Repository struct{ q *dbgen.Queries }

func New(db dbgen.DBTX) *Repository { return &Repository{q: dbgen.New(db)} }
func Window(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -29)
}
func (r *Repository) Read(ctx context.Context, tenant string, now time.Time) (result Snapshot, err error) {
	if tenant == "" {
		return result, fmt.Errorf("dashboard tenant required")
	}
	result.AsOf = now.UTC()
	result.Start = Window(now)
	if result.ActiveCustomers, err = r.q.DashboardActiveCustomers(ctx, tenant); err != nil {
		return result, fmt.Errorf("count active customers: %w", err)
	}
	if result.ActiveProducts, err = r.q.DashboardActiveProducts(ctx, tenant); err != nil {
		return result, fmt.Errorf("count active products: %w", err)
	}
	rows, err := r.q.DashboardCustomerDays(ctx, dbgen.DashboardCustomerDaysParams{TenantID: tenant, WindowStart: result.Start, WindowEnd: result.AsOf})
	if err != nil {
		return result, fmt.Errorf("count customer days: %w", err)
	}
	counts := map[string]int64{}
	var maximum int64 = 1
	for _, row := range rows {
		counts[row.Day.Format("2006-01-02")] = row.Total
		result.RecentCustomers += row.Total
		if row.Total > maximum {
			maximum = row.Total
		}
	}
	for date := result.Start; len(result.Days) < 30; date = date.AddDate(0, 0, 1) {
		total := counts[date.Format("2006-01-02")]
		percent := int(total * 100 / maximum)
		if total > 0 && percent == 0 {
			percent = 1
		}
		result.Days = append(result.Days, Day{Date: date, Total: total, Percent: percent})
	}
	return result, nil
}
