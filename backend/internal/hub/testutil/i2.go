package testutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/store"
	"measix/platform/pkg/platformid"
)

type StoreHandle = store.Store

func OpenStoreHandle(t *testing.T) *StoreHandle {
	return OpenStore(t)
}

func NewIdentityService(t *testing.T, st *StoreHandle, now time.Time) *identity.Service {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deploymentID := platformid.New(platformid.Deployment)
	signer, err := security.NewAccessSigner(privateKey, deploymentID, "test-key", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	service := identity.New(st.Client, signer, []byte("01234567890123456789012345678901"))
	service.Now = func() time.Time { return now }
	signer.Now = service.Now
	return service
}
