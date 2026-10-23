package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/testutil"
)

func TestAdminRemovalAfterManyDeletedAccounts(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStoreHandle(t)
	svc := testutil.NewIdentityService(t, st, time.Now().UTC())
	boot, err := svc.Bootstrap(ctx, "Guard regression", "admin", "Admin", "synthetic admin password")
	if err != nil {
		t.Fatal(err)
	}
	target, err := svc.CreateUser(ctx, "target", "Target", "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPassword(ctx, target.ID, "synthetic target password"); err != nil {
		t.Fatal(err)
	}
	// Permanent tombstones can outgrow SQLite's bind-variable limit. Seed them
	// without a variable per row so the regression tests the removal query.
	if _, err := st.DB.ExecContext(ctx, `WITH RECURSIVE n(i) AS (VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i < 33000)
		INSERT INTO deleted_principals(id, deleted_at) SELECT 'retired_' || i, ? FROM n`, svc.Now()); err != nil {
		t.Fatal(err)
	}
	tx, err := st.Client.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	row, err := tx.User.Get(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.GuardAdminRemoval(ctx, tx, boot.AdminUserID, row); err != nil {
		t.Fatalf("another login-capable admin exists: %v", err)
	}
	if _, err := tx.DeletedPrincipal.Create().SetID(boot.AdminUserID).SetDeletedAt(svc.Now()).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if err := identity.GuardAdminRemoval(ctx, tx, boot.AdminUserID, row); !errors.Is(err, identity.ErrLastAdmin) {
		t.Fatalf("tombstoned admins must not be counted: %v", err)
	}
}
