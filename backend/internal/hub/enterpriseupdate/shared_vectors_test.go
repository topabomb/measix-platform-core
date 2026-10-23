package enterpriseupdate_test

import (
	"encoding/json"
	"errors"
	"measix/platform/internal/hub/enterpriseupdate"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestSharedLocalAndRemoteFeedVectors(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../api/fixtures/portal/feed-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Suites []struct {
			Name, Timezone, Now string
			Items               []struct{ Key, PublishedAt, Status string }
			Queries             []struct {
				Name, StartDate, EndDate, Error string
				Limit                           int
				Expected                        []string
				Truncated                       bool
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	parse := func(raw string) time.Time {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	for _, suite := range fixture.Suites {
		t.Run(suite.Name, func(t *testing.T) {
			s, ctx, admin := setupService(t)
			deployment, err := s.Client.Deployment.Query().Only(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Client.Deployment.UpdateOneID(deployment.ID).SetTimezone(suite.Timezone).Save(ctx); err != nil {
				t.Fatal(err)
			}
			for _, item := range suite.Items {
				at := parse(item.PublishedAt)
				s.Now = func() time.Time { return at }
				row, err := s.Create(ctx, admin, item.Key, "Fixture body", "PLAIN", "NOTICE", "INFO")
				if err != nil {
					t.Fatal(err)
				}
				if item.Status == "PUBLISHED" {
					if _, err = s.Publish(ctx, row.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
			now := parse(suite.Now)
			s.Now = func() time.Time { return now }
			for _, query := range suite.Queries {
				t.Run(query.Name, func(t *testing.T) {
					date := func(raw string) *time.Time {
						if raw == "" {
							return nil
						}
						value, err := time.Parse("2006-01-02", raw)
						if err != nil {
							t.Fatal(err)
						}
						return &value
					}
					start, end := date(query.StartDate), date(query.EndDate)
					rows, truncated, meta, err := s.ListPublished(ctx, start, end, query.Limit)
					if query.Error != "" {
						if !errors.Is(err, enterpriseupdate.ErrInvalidDateRange) {
							t.Fatalf("want invalid range, got %v", err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					keys := []string{}
					for _, row := range rows {
						keys = append(keys, row.Title)
					}
					if !reflect.DeepEqual(keys, query.Expected) || truncated != query.Truncated || meta.Timezone != suite.Timezone {
						t.Fatalf("unexpected query: %v truncated=%t timezone=%s", keys, truncated, meta.Timezone)
					}
					_, _, repeat, err := s.ListPublished(ctx, start, end, query.Limit)
					if err != nil || repeat.ETag != meta.ETag {
						t.Fatal("unstable query ETag")
					}
				})
			}
		})
	}
}
