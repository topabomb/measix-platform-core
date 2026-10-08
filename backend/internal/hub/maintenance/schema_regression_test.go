package maintenance_test

import (
	"context"
	"measix/platform/internal/hub/maintenance"
	"measix/platform/internal/hub/testutil"
	"testing"
)

func TestCheckRejectsMissingCurrentColumn(t *testing.T) {
	st := testutil.OpenStore(t)
	if _, err := st.DB.Exec("ALTER TABLE sessions DROP COLUMN refresh_response_ciphertext"); err != nil {
		t.Fatal(err)
	}
	if _, err := maintenance.Check(context.Background(), st.DB); err == nil {
		t.Fatal("current schema check accepted a missing session column")
	}
}
