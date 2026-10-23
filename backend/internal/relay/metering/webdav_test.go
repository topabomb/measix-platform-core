package metering_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"measix/platform/internal/relay/metering"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func TestDAVStatisticsSpoolSurvivesWithoutAdmission(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "relay.db")
	spool, err := metering.OpenSpool(path)
	if err != nil {
		t.Fatal(err)
	}
	recorder := metering.NewRecorder(spool)
	statistics := true
	event := usageingestapi.UsageSettlement{RequestId: platformid.New(platformid.Request), OccurredAt: time.Now(), StatisticsOnly: &statistics, Request: usageingestapi.RequestUsageFact{ResourceKind: "WEBDAV", ClientProtocol: "WEBDAV"}}
	if err = recorder.Record(event); err != nil {
		t.Fatal("stats require admission journal", err)
	}
	if err = recorder.Record(event); err != nil {
		t.Fatal("duplicate statistics", err)
	}
	spool.Close()
	spool, err = metering.OpenSpool(path)
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	rows, err := spool.Pending(ctx, 100)
	if err != nil || len(rows) != 1 {
		t.Fatal("statistics lost", err)
	}
	recovery, err := spool.Recoverable(ctx, 100)
	if err != nil || len(recovery) != 0 {
		t.Fatal("stats enter budget recovery", err)
	}
}
