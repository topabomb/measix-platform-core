package budget

import (
	"testing"
	"time"
)

func TestNaturalPeriodWindowsUseDeploymentTimezoneAndHalfOpenBounds(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	scopeStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day, err := windowFor(PeriodDay, time.Date(2026, 3, 8, 7, 30, 0, 0, time.UTC), scopeStart, newYork)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC); !day.Start.Equal(want) {
		t.Fatalf("DST day start = %s, want %s", day.Start, want)
	}
	if want := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC); day.End == nil || !day.End.Equal(want) {
		t.Fatalf("DST day end = %v, want %s", day.End, want)
	}
	atBoundary, err := windowFor(PeriodDay, *day.End, scopeStart, newYork)
	if err != nil {
		t.Fatal(err)
	}
	if !atBoundary.Start.Equal(*day.End) {
		t.Fatalf("half-open boundary selected start %s, want %s", atBoundary.Start, *day.End)
	}

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	admitted := time.Date(2026, 9, 16, 3, 0, 0, 0, time.UTC)
	week, err := windowFor(PeriodWeek, admitted, scopeStart, shanghai)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 9, 13, 16, 0, 0, 0, time.UTC); !week.Start.Equal(want) {
		t.Fatalf("week start = %s, want Monday %s", week.Start, want)
	}
	if want := time.Date(2026, 9, 20, 16, 0, 0, 0, time.UTC); week.End == nil || !week.End.Equal(want) {
		t.Fatalf("week end = %v, want %s", week.End, want)
	}
	month, err := windowFor(PeriodMonth, admitted, scopeStart, shanghai)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 8, 31, 16, 0, 0, 0, time.UTC); !month.Start.Equal(want) {
		t.Fatalf("month start = %s, want %s", month.Start, want)
	}
	lifetime, err := windowFor(PeriodLifetime, admitted, scopeStart, shanghai)
	if err != nil {
		t.Fatal(err)
	}
	if !lifetime.Start.Equal(scopeStart) || lifetime.End != nil {
		t.Fatalf("lifetime window = %+v", lifetime)
	}
}

func TestNewScopeStartsAtActivationWithoutRetroactiveNaturalPeriodUsage(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	activation := time.Date(2026, 9, 20, 3, 15, 0, 0, time.UTC)
	window, err := windowFor(PeriodMonth, activation.Add(time.Minute), activation, location)
	if err != nil {
		t.Fatal(err)
	}
	if !window.Start.Equal(activation) {
		t.Fatalf("new scope start = %s, want activation %s", window.Start, activation)
	}
}
