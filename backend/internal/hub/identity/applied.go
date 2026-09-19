package identity

import (
	"context"
	"errors"

	"measix/platform/ent"
	"measix/platform/ent/managedrelease"
)

var (
	ErrAppliedRelease    = errors.New("application report does not match a published release")
	ErrAppliedRegression = errors.New("application report is older than the current session report")
)

// ReportManagedApplied records a client assertion only. It grants no runtime
// permission and does not renew the session. Keeping the report on Session
// prevents a re-enrolled installation from inheriting an old acknowledgement.
func (s *Service) ReportManagedApplied(ctx context.Context, token string, generation int, hash string) error {
	p, err := s.AuthenticateAccess(ctx, token)
	if err != nil {
		return err
	}
	if generation < 1 || hash == "" {
		return ErrInvalidInput
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	exists, err := tx.ManagedRelease.Query().Where(managedrelease.ManagedGenerationEQ(int64(generation)), managedrelease.SnapshotHashEQ(hash), managedrelease.StatusIn("ACTIVE", "SUPERSEDED")).Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return ErrAppliedRelease
	}
	current, err := tx.Session.Get(ctx, p.SessionID)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrRevoked
		}
		return err
	}
	if current.Status != "ACTIVE" || !current.ExpiresAt.After(s.Now()) {
		return ErrRevoked
	}
	if current.AppliedManagedGeneration != nil && *current.AppliedManagedGeneration > int64(generation) {
		return ErrAppliedRegression
	}
	_, err = tx.Session.UpdateOneID(p.SessionID).SetAppliedManagedGeneration(int64(generation)).SetAppliedSnapshotHash(hash).SetAppliedReportedAt(s.Now().UTC()).Save(ctx)
	if err != nil {
		return err
	}
	return tx.Commit()
}
