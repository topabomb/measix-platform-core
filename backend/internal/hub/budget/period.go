package budget

import (
	"fmt"
	"time"
)

type periodWindow struct {
	Start time.Time
	End   *time.Time
}

func windowFor(period Period, admittedAt, scopeStart time.Time, location *time.Location) (periodWindow, error) {
	if admittedAt.IsZero() || scopeStart.IsZero() || location == nil || admittedAt.Before(scopeStart) {
		return periodWindow{}, ErrInvalidConfiguration
	}
	local := admittedAt.In(location)
	var start, end time.Time
	switch period {
	case PeriodDay:
		start = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
		end = start.AddDate(0, 0, 1)
	case PeriodWeek:
		start = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
		daysSinceMonday := (int(start.Weekday()) + 6) % 7
		start = start.AddDate(0, 0, -daysSinceMonday)
		end = start.AddDate(0, 0, 7)
	case PeriodMonth:
		start = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location)
		end = start.AddDate(0, 1, 0)
	case PeriodLifetime:
		return periodWindow{Start: scopeStart.UTC()}, nil
	default:
		return periodWindow{}, fmt.Errorf("%w: unknown period %q", ErrInvalidConfiguration, period)
	}
	start = start.UTC()
	end = end.UTC()
	if scopeStart.After(start) {
		start = scopeStart.UTC()
	}
	return periodWindow{Start: start, End: &end}, nil
}
