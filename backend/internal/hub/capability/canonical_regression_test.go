package capability_test

import (
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
	"reflect"
	"testing"
	"time"
)

func TestSnapshotPreservesSeedOrderAndCanonicalStarterIDs(t *testing.T) {
	content := validDraft(platformid.New(platformid.Upstream))
	aid := platformid.New(platformid.Assistant)
	content.Assistants = &[]adminapi.ManagedAssistantDefinition{{AssistantDefinitionId: aid, ModelId: content.Models[0].ModelId, MemorySeed: []string{" z first ", "a second"}, Enabled: true}}
	content.Starters = &[]adminapi.AssistantStarterDefinition{
		{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: aid, SortOrder: 0},
		{StarterId: platformid.New(platformid.Starter), AssistantDefinitionId: aid, SortOrder: 9},
	}
	if (*content.Starters)[0].StarterId < (*content.Starters)[1].StarterId {
		(*content.Starters)[0].StarterId, (*content.Starters)[1].StarterId = (*content.Starters)[1].StarterId, (*content.Starters)[0].StarterId
	}
	input := capability.SnapshotInput{DeploymentID: platformid.New(platformid.Deployment), ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, PublishedAt: time.Now(), Content: content}
	s := capability.NewService(nil)
	snapshot, hash, err := s.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.Assistants[0].MemorySeed, []string{"z first", "a second"}) {
		t.Fatalf("authored seed order changed: %v", snapshot.Assistants[0].MemorySeed)
	}
	if snapshot.Starters[0].StarterId > snapshot.Starters[1].StarterId {
		t.Fatal("wire starters are not ordered by stable ID")
	}
	(*content.Starters)[0], (*content.Starters)[1] = (*content.Starters)[1], (*content.Starters)[0]
	input.Content = content
	_, otherHash, err := s.CompileSnapshot(input)
	if err != nil || hash != otherHash {
		t.Fatalf("array permutation changed hash: %v", err)
	}
}
