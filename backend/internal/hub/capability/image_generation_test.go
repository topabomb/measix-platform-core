package capability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"measix/platform/ent/manageddraft"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestLegacyDraftWithoutImageGeneratorsNormalizesAndNewWriteIsExplicit(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	row, err := st.Client.ManagedDraft.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(row.ContentJSON, &legacy); err != nil {
		t.Fatal(err)
	}
	delete(legacy, "imageGenerators")
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Client.ManagedDraft.UpdateOneID(row.ID).SetContentJSON(payload).Save(ctx); err != nil {
		t.Fatal(err)
	}

	service := capability.NewService(st.Client)
	service.Now = func() time.Time { return now }
	draft, err := service.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Content.ImageGenerators == nil || len(*draft.Content.ImageGenerators) != 0 {
		t.Fatalf("legacy imageGenerators = %#v, want explicit empty list", draft.Content.ImageGenerators)
	}
	updated, err := service.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := st.Client.ManagedDraft.Query().Where(manageddraft.IDEQ(row.ID)).Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stored.ContentJSON, []byte(`"imageGenerators":[]`)) {
		t.Fatalf("new writer omitted imageGenerators: %s", stored.ContentJSON)
	}
	if updated.Content.ImageGenerators == nil {
		t.Fatal("updated draft lost normalized imageGenerators")
	}
}

func TestImageGenerationValidationReferencesBindingAndDefault(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x52}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	upstreams := upstream.NewService(st.Client, box)
	upstreams.Now = func() time.Time { return now }
	secret, err := upstreams.CreateSecret(ctx, boot.AdminUserID, "image-token", "secret")
	if err != nil {
		t.Fatal(err)
	}
	up, err := upstreams.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	service := capability.NewService(st.Client)
	service.Now = func() time.Time { return now }
	draft, err := service.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content := validDraft(up.UpstreamID)
	imageID := platformid.New(platformid.ImageGeneration)
	images := []adminapi.ImageGenerationDefinition{{
		ImageId: imageID, DisplayName: "Image", ClientProtocol: adminapi.ImageGenerationDefinitionClientProtocolOPENAIIMAGESGENERATIONS,
		UpstreamModelKey: "gpt-image-1", RuntimePath: "/v1/images/generations", MaxImagesPerRequest: 4,
		AllowedSizes: []string{"1024x1024", "1536x1024"}, Enabled: true,
	}}
	content.ImageGenerators = &images
	content.Policy.DefaultImageGenerationId = &imageID
	content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{
		RuntimeRouteId: platformid.New(platformid.Route), ResourceId: imageID, UpstreamId: up.UpstreamID,
		AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/images/generations"}, TransportPolicy: adminapi.RuntimeBindingDefinitionTransportPolicyHTTPREQUESTRESPONSE,
	})
	updated, err := service.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ValidateDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("valid image definition rejected: %+v", result.Errors)
	}

	invalid := updated.Content
	badID := platformid.New(platformid.ImageGeneration)
	invalid.Policy.DefaultImageGenerationId = &badID
	invalidImages := append([]adminapi.ImageGenerationDefinition(nil), (*invalid.ImageGenerators)...)
	invalidImages[0].AllowedSizes = []string{"1024x1024", "1024x1024"}
	invalid.ImageGenerators = &invalidImages
	invalidView, err := service.PutDraft(ctx, boot.AdminUserID, updated.DraftRevision, invalid)
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.ValidateDraft(ctx, invalidView.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatal("invalid image default and duplicate size were accepted")
	}
	codes := map[string]bool{}
	for _, issue := range result.Errors {
		codes[issue.Code] = true
	}
	if !codes["invalid_default_image_generation"] || !codes["duplicate_image_size"] {
		t.Fatalf("missing image validation errors: %+v", result.Errors)
	}
}

func TestImageGenerationSnapshotIsCanonicalAndUnsetDefaultStaysUnset(t *testing.T) {
	content := validDraft(platformid.New(platformid.Upstream))
	firstID := platformid.New(platformid.ImageGeneration)
	secondID := platformid.New(platformid.ImageGeneration)
	images := []adminapi.ImageGenerationDefinition{
		{ImageId: firstID, DisplayName: "First", ClientProtocol: adminapi.ImageGenerationDefinitionClientProtocolOPENAIIMAGESGENERATIONS, UpstreamModelKey: "first", RuntimePath: "/v1/images/generations", MaxImagesPerRequest: 2, AllowedSizes: []string{"1536x1024", "1024x1024"}, Enabled: true},
		{ImageId: secondID, DisplayName: "Second", ClientProtocol: adminapi.ImageGenerationDefinitionClientProtocolOPENAIIMAGESGENERATIONS, UpstreamModelKey: "second", RuntimePath: "/v1/images/generations", MaxImagesPerRequest: 1, AllowedSizes: []string{"1024x1024"}, Enabled: true},
	}
	if images[0].ImageId < images[1].ImageId {
		images[0], images[1] = images[1], images[0]
	}
	content.ImageGenerators = &images
	content.Policy.DefaultImageGenerationId = nil
	input := capability.SnapshotInput{DeploymentID: platformid.New(platformid.Deployment), ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, PublishedAt: time.Unix(0, 0).UTC(), Content: content}
	service := capability.NewService(nil)
	snapshot, hash, err := service.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ImageGenerators == nil || len(*snapshot.ImageGenerators) != 2 {
		t.Fatalf("snapshot imageGenerators = %#v", snapshot.ImageGenerators)
	}
	if (*snapshot.ImageGenerators)[0].ImageId > (*snapshot.ImageGenerators)[1].ImageId {
		t.Fatal("image generators are not ordered by stable id")
	}
	var multiSize []string
	for _, image := range *snapshot.ImageGenerators {
		if len(image.AllowedSizes) > 1 {
			multiSize = image.AllowedSizes
		}
	}
	if !reflect.DeepEqual(multiSize, []string{"1024x1024", "1536x1024"}) {
		t.Fatalf("allowed sizes are not canonical: %v", multiSize)
	}
	if snapshot.Policy.DefaultImageGenerationId != nil {
		t.Fatalf("unset default silently changed: %s", *snapshot.Policy.DefaultImageGenerationId)
	}
	images[0], images[1] = images[1], images[0]
	for i := range images {
		if len(images[i].AllowedSizes) > 1 {
			images[i].AllowedSizes[0], images[i].AllowedSizes[1] = images[i].AllowedSizes[1], images[i].AllowedSizes[0]
		}
	}
	content.ImageGenerators = &images
	input.Content = content
	_, reorderedHash, err := service.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	if hash != reorderedHash {
		t.Fatalf("permutation changed snapshot hash: %s != %s", hash, reorderedHash)
	}
	if !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("invalid hash %q", hash)
	}
}
