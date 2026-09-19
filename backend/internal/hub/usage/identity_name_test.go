package usage

import (
	"context"
	"testing"
	"time"

	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func seedUsageDevice(t *testing.T, st *testutil.StoreHandle, userID, name string, now time.Time) string {
	t.Helper()
	deviceID := platformid.New(platformid.Device)
	if _, err := st.Client.Device.Create().
		SetID(deviceID).
		SetUserID(userID).
		SetName(name).
		SetStatus("ACTIVE").
		SetCreatedAt(now).
		Save(context.Background()); err != nil {
		t.Fatal(err)
	}
	return deviceID
}

// A request list is read to find out who sent what and from where, so each row
// carries both display identities instead of making the reader resolve
// identifiers by hand. The detail view must agree with the list about the same
// request, which is why both go through the same enrichment.
func TestUsageRequestCarriesUserAndDeviceDisplayNames(t *testing.T) {
	st := testutil.OpenStore(t)
	ctx, now := context.Background(), time.Now().UTC()
	userID, upstreamID := seedUsageParents(t, st.Client, now)
	deviceID := seedUsageDevice(t, st, userID, "Pixel 8", now)
	service := NewService(st.Client)

	event := validRequestUsageEvent(now, userID, upstreamID)
	event.DeviceId = &deviceID
	if _, err := service.Ingest(ctx, usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{event}}); err != nil {
		t.Fatal(err)
	}

	views, err := service.ListRequests(ctx, Filter{}, 50)
	if err != nil || len(views) != 1 {
		t.Fatalf("views=%d err=%v", len(views), err)
	}
	detail, err := service.GetRequest(ctx, event.RequestId)
	if err != nil {
		t.Fatal(err)
	}
	for name, view := range map[string]RequestView{"list": views[0], "detail": detail} {
		if view.UserDisplayName != "Usage Test" {
			t.Fatalf("%s userDisplayName=%q want %q", name, view.UserDisplayName, "Usage Test")
		}
		if view.DeviceName != "Pixel 8" {
			t.Fatalf("%s deviceName=%q want %q", name, view.DeviceName, "Pixel 8")
		}
	}
}

// Traffic that carries no device is recorded without one, and its row must not
// borrow a device name from any other request.
func TestUsageRequestWithoutDeviceHasNoDeviceName(t *testing.T) {
	st := testutil.OpenStore(t)
	ctx, now := context.Background(), time.Now().UTC()
	userID, upstreamID := seedUsageParents(t, st.Client, now)
	named := seedUsageDevice(t, st, userID, "Pixel 8", now)
	service := NewService(st.Client)

	withDevice := validRequestUsageEvent(now, userID, upstreamID)
	withDevice.DeviceId = &named
	withoutDevice := validRequestUsageEvent(now, userID, upstreamID)
	if withoutDevice.DeviceId != nil {
		t.Fatal("fixture unexpectedly carries a device")
	}
	if _, err := service.Ingest(ctx, usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{withDevice, withoutDevice}}); err != nil {
		t.Fatal(err)
	}

	views, err := service.ListRequests(ctx, Filter{}, 50)
	if err != nil || len(views) != 2 {
		t.Fatalf("views=%d err=%v", len(views), err)
	}
	seen := 0
	for _, view := range views {
		want := "Pixel 8"
		if view.DeviceID == nil {
			want = ""
			seen++
		}
		if view.DeviceName != want {
			t.Fatalf("deviceId=%v deviceName=%q want=%q", view.DeviceID, view.DeviceName, want)
		}
		if view.UserDisplayName != "Usage Test" {
			t.Fatalf("userDisplayName=%q want %q", view.UserDisplayName, "Usage Test")
		}
	}
	if seen != 1 {
		t.Fatalf("expected exactly one device-less row, matched %d", seen)
	}
}
