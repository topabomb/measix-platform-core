package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"measix/platform/ent"
	"measix/platform/ent/activation"
	"measix/platform/ent/managedrelease"
	"measix/platform/ent/upstreamconfigrevision"
	"measix/platform/internal/hub/identity"
)

var (
	ErrInUse                = errors.New("upstream is referenced")
	ErrActivationInProgress = errors.New("runtime activation is in progress")
)

// DeleteUpstream uses existing rows and SQLite's write transaction. Secrets,
// usage, immutable Releases and terminal control facts retain their bytes.
func (s *Service) DeleteUpstream(ctx context.Context, actorID, id string, expectedRevision int) error {
	if expectedRevision < 1 {
		return ErrInvalidConfig
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := identity.RequireActiveAdmin(ctx, tx, actorID); err != nil {
		return err
	}
	row, err := tx.Upstream.Get(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if row.ConfigRevision != int64(expectedRevision) {
		return ErrRevisionConflict
	}
	busy, err := tx.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).Exist(ctx)
	if err != nil {
		return err
	}
	if busy {
		return ErrActivationInProgress
	}
	drafts, err := tx.ManagedDraft.Query().All(ctx)
	if err != nil {
		return err
	}
	for _, draft := range drafts {
		if err := rejectUpstreamReference(draft.ContentJSON, id); err != nil {
			return err
		}
	}
	// Retained releases can be re-published; current ACTIVE alone is insufficient.
	after := ""
	for {
		releases, err := tx.ManagedRelease.Query().Where(managedrelease.IDGT(after)).Order(ent.Asc(managedrelease.FieldID)).Limit(16).Select(managedrelease.FieldID, managedrelease.FieldReleaseContentJSON).All(ctx)
		if err != nil {
			return err
		}
		for _, release := range releases {
			if err := rejectUpstreamReference(release.ReleaseContentJSON, id); err != nil {
				return err
			}
		}
		if len(releases) < 16 {
			break
		}
		after = releases[len(releases)-1].ID
	}
	if _, err := tx.UpstreamConfigRevision.Delete().Where(upstreamconfigrevision.UpstreamIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	if err := tx.Upstream.DeleteOneID(id).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func rejectUpstreamReference(raw []byte, id string) error {
	var content struct {
		Bindings json.RawMessage `json:"bindings"`
	}
	if json.Unmarshal(raw, &content) != nil || len(content.Bindings) == 0 || bytes.Equal(bytes.TrimSpace(content.Bindings), []byte("null")) {
		return ErrInvalidConfig
	}
	var bindings []struct {
		UpstreamID string `json:"upstreamId"`
		TargetKind string `json:"targetKind"`
	}
	if json.Unmarshal(content.Bindings, &bindings) != nil {
		return ErrInvalidConfig
	}
	for _, binding := range bindings {
		if binding.UpstreamID == id {
			return ErrInUse
		}
		if binding.UpstreamID == "" && binding.TargetKind != "REMOTE_WORKSPACE" {
			return ErrInvalidConfig
		}
	}
	return nil
}
