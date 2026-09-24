package capability_test

import (
	"context"
	"encoding/json"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"os"
	"strings"
	"testing"
)

func TestSharedClientReferenceCases(t *testing.T) {
	raw, err := os.ReadFile("../../../../api/fixtures/client-integration/reference-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, ExpectedCode string
		Content            adminapi.ManagedDraftContent
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			store, boot, _ := bootstrapI2(t)
			svc := capability.NewService(store.Client)
			ctx := context.Background()
			draft, err := svc.GetDraft(ctx)
			if err != nil {
				t.Fatal(err)
			}
			updated, err := svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, c.Content)
			if err != nil {
				t.Fatal(err)
			}
			result, err := svc.ValidateDraft(ctx, updated.DraftRevision)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, issue := range result.Errors {
				if issue.Code == c.ExpectedCode {
					found = true
				}
				// Projection recipes omit private bindings; only reference issues are
				// tested here. Actual publish validation is exercised by runtimecontrol.
				if c.ExpectedCode == "" && (strings.HasSuffix(issue.Code, "_ref") || strings.HasPrefix(issue.Code, "invalid_default_") || issue.Code == "missing_provider") {
					t.Fatalf("valid reference rejected: %+v", issue)
				}
			}
			if c.ExpectedCode != "" && !found {
				t.Fatalf("missing %s: %+v", c.ExpectedCode, result.Errors)
			}
		})
	}
}
