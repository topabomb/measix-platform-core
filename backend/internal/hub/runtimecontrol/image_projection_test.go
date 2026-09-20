package runtimecontrol_test

import (
	"context"
	"reflect"
	"testing"

	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestPublishProjectsImageGenerationRuntimeProfile(t *testing.T) {
	ctx := context.Background()
	_, service, relay, server, _, adminID, upstreamID, _ := newRuntimeControlEnv(t)
	defer server.Close()
	draft, err := service.Capability.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	imageID := platformid.New(platformid.ImageGeneration)
	images := []adminapi.ImageGenerationDefinition{{
		ImageId: imageID, DisplayName: "Image", ClientProtocol: adminapi.ImageGenerationDefinitionClientProtocolOPENAIIMAGESGENERATIONS,
		UpstreamModelKey: "gpt-image-1", RuntimePath: "/v1/images/generations", MaxImagesPerRequest: 3,
		AllowedSizes: []string{"1536x1024", "1024x1024"}, Enabled: true,
	}}
	draft.Content.ImageGenerators = &images
	draft.Content.Policy.DefaultImageGenerationId = &imageID
	draft.Content.Bindings = append(draft.Content.Bindings, adminapi.RuntimeBindingDefinition{
		RuntimeRouteId: platformid.New(platformid.Route), ResourceId: imageID, UpstreamId: upstreamID,
		AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/images/generations"}, TransportPolicy: adminapi.RuntimeBindingDefinitionTransportPolicyHTTPREQUESTRESPONSE,
	})
	draft, err = service.Capability.PutDraft(ctx, adminID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	publishAndFinalize(t, service, adminID, draft.DraftRevision)
	resource, ok := relay.Current().Resources[imageID]
	if !ok {
		t.Fatal("published relay state omitted image resource")
	}
	if resource.Kind != "IMAGE_GENERATION" || resource.ClientProtocol != "OPENAI_IMAGES_GENERATIONS" || resource.ImageProfile == nil {
		t.Fatalf("image runtime resource = %+v", resource)
	}
	if resource.ImageProfile.MaxImagesPerRequest != 3 || !reflect.DeepEqual(resource.ImageProfile.AllowedSizes, []string{"1024x1024", "1536x1024"}) {
		t.Fatalf("image profile = %+v", resource.ImageProfile)
	}
}
