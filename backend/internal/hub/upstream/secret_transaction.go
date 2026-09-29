package upstream

import (
	"context"
	"fmt"
	"measix/platform/ent"
	"measix/platform/pkg/platformid"
	"strings"
)

// SaveSecretVersionTx lets a domain owner atomically confirm an operation and
// its encrypted credential. The caller owns commit/rollback. An empty ID creates
// a secret; an existing ID appends an immutable version to that same secret.
func (s *Service) SaveSecretVersionTx(ctx context.Context, tx *ent.Tx, actor, id, name, value string) (SecretView, error) {
	if tx == nil || s.Box == nil || strings.TrimSpace(name) == "" || value == "" {
		return SecretView{}, fmt.Errorf("invalid secret")
	}
	payload, err := s.Box.Encrypt([]byte(value))
	if err != nil {
		return SecretView{}, err
	}
	now := s.Now().UTC()
	version := int64(1)
	if id == "" {
		id = platformid.New(platformid.Secret)
		_, err = tx.Secret.Create().SetID(id).SetName(name).SetLatestSecretVersion(version).SetCreatedAt(now).SetUpdatedAt(now).Save(ctx)
	} else {
		row, e := tx.Secret.Get(ctx, id)
		if e != nil {
			return SecretView{}, e
		}
		version = row.LatestSecretVersion + 1
		_, err = tx.Secret.UpdateOneID(id).SetLatestSecretVersion(version).SetUpdatedAt(now).Save(ctx)
	}
	if err != nil {
		return SecretView{}, err
	}
	_, err = tx.SecretVersion.Create().SetSecretID(id).SetSecretVersion(version).SetEncryptedPayload(payload).SetKeyVersion(s.Box.KeyVersion()).SetCreatedByUserID(actor).SetCreatedAt(now).Save(ctx)
	return SecretView{SecretID: id, Name: name, SecretVersion: int(version)}, err
}
