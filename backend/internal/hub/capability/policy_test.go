package capability_test

import (
	"context"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/pkg/platformid"
)

func TestCurrentPolicyDefaultsAndCompiler(t *testing.T) {
	st, _, _ := bootstrapI2(t)
	service := capability.NewService(st.Client)
	draft, err := service.GetDraft(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := draft.Content.Policy
	if p.AllowLocalProviders || p.AllowLocalTts || p.AllowLocalAsr || p.AllowLocalMcp || p.AllowLocalAssistants {
		t.Fatal("new policy must deny all five resource types")
	}
	for _, allow := range []bool{false, true} {
		content := validDraft(platformid.New(platformid.Upstream))
		content.Policy.AllowLocalAssistants = allow
		snapshot, _, err := service.CompileSnapshot(capability.SnapshotInput{DeploymentID: platformid.New(platformid.Deployment), ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, PublishedAt: time.Now(), Content: content})
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.SchemaVersion != 4 || snapshot.Policy.AllowLocalAssistants != allow {
			t.Fatal("current policy not preserved")
		}
	}
}
