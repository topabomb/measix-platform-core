package identity_test

import (
	"context"
	"testing"
	"time"

	"measix/platform/pkg/platformid"
)

// HUB-ID-004: device installation metadata must not duplicate `dev_*`
// authorization identity. A device created from an enrollment must have
// a unique dev_ ID that is separate from the user's usr_ ID. The installation
// ID is metadata, not an authorization identity.
func TestHUBID004DeviceInstallationDoesNotDuplicateAuthorizationIdentity(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	boot, err := s.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.CreateUser(ctx, "member1", "Member", "MEMBER")
	if err != nil {
		t.Fatal(err)
	}

	grant, err := s.CreateEnrollment(ctx, member.ID, boot.AdminUserID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	installationID := platformid.New(platformid.Installation)
	exchange, err := s.ExchangeEnrollment(ctx, grant.Code, installationID, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	// Device ID must have dev_ prefix
	if exchange.DeviceID == "" || exchange.DeviceID[:4] != "dev_" {
		t.Fatalf("device ID does not have dev_ prefix: %s", exchange.DeviceID)
	}
	// User ID must have usr_ prefix
	if exchange.UserID != member.ID || member.ID[:4] != "usr_" {
		t.Fatalf("user ID mismatch: exchange=%s member=%s", exchange.UserID, member.ID)
	}
	// Device ID must differ from User ID
	if exchange.DeviceID == exchange.UserID {
		t.Fatal("device ID equals user ID — authorization identity duplicated")
	}
	// Device ID must differ from Session ID
	if exchange.DeviceID == exchange.SessionID {
		t.Fatal("device ID equals session ID")
	}
	// Installation ID must differ from Device ID (installation is metadata, not auth)
	if installationID == exchange.DeviceID {
		t.Fatal("installation ID equals device ID — metadata became authorization identity")
	}
	// Installation ID must differ from User ID
	if installationID == exchange.UserID {
		t.Fatal("installation ID equals user ID")
	}

	// Verify device is linked to user, not the other way around
	devices, err := s.ListDevices(ctx, member.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ID != exchange.DeviceID {
		t.Fatalf("device not linked to user: %+v", devices)
	}
	if devices[0].InstallationID == nil || *devices[0].InstallationID != installationID {
		t.Fatalf("installation ID not stored as metadata: %v", devices[0].InstallationID)
	}
}
