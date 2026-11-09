package identity

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"measix/platform/ent"
	"measix/platform/ent/deployment"
	"measix/platform/ent/portalsession"
)

type DeploymentSettingsView struct {
	DeploymentID string
	Name         string
	Timezone     string
	PublicOrigin string
	UpdatedAt    time.Time
}

func (s *Service) DeploymentSettings(ctx context.Context) (DeploymentSettingsView, error) {
	row, err := s.Client.Deployment.Query().Only(ctx)
	if err != nil {
		return DeploymentSettingsView{}, err
	}
	return DeploymentSettingsView{DeploymentID: row.ID, Name: row.Name, Timezone: row.Timezone, PublicOrigin: row.PublicOrigin, UpdatedAt: row.UpdatedAt}, nil
}

// UpdateDeploymentSettings changes the display profile and the canonical
// public address projected to clients. Listener, storage, keys, Portal source
// and the fixed budget timezone remain startup-owned settings.
func (s *Service) UpdateDeploymentSettings(ctx context.Context, name, publicOrigin string, expectedUpdatedAt time.Time, actorUserID string) (DeploymentSettingsView, error) {
	// Keep the committed row and the process-local origin projection in the
	// same update order. The database CAS alone cannot prevent a later commit
	// from being followed by an earlier request's delayed SetPublicOrigin.
	s.deploymentSettingsMu.Lock()
	defer s.deploymentSettingsMu.Unlock()

	name = strings.TrimSpace(name)
	canonicalOrigin, originErr := CanonicalPublicOrigin(publicOrigin)
	if name == "" || utf8.RuneCountInString(name) > 120 || originErr != nil || expectedUpdatedAt.IsZero() || actorUserID == "" {
		return DeploymentSettingsView{}, ErrInvalidInput
	}
	publicOrigin = canonicalOrigin
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return DeploymentSettingsView{}, err
	}
	rollback := func(cause error) (DeploymentSettingsView, error) {
		_ = tx.Rollback()
		return DeploymentSettingsView{}, cause
	}
	current, err := tx.Deployment.Query().Only(ctx)
	if err != nil {
		return rollback(err)
	}
	if !current.UpdatedAt.Equal(expectedUpdatedAt) {
		return rollback(ErrConflict)
	}
	if current.Name == name && current.PublicOrigin == publicOrigin {
		if err := tx.Commit(); err != nil {
			return DeploymentSettingsView{}, err
		}
		return DeploymentSettingsView{DeploymentID: current.ID, Name: current.Name, Timezone: current.Timezone, PublicOrigin: current.PublicOrigin, UpdatedAt: current.UpdatedAt}, nil
	}
	now := s.Now().UTC()
	updated, err := tx.Deployment.UpdateOneID(current.ID).
		Where(deployment.UpdatedAtEQ(expectedUpdatedAt)).
		SetName(name).
		SetPublicOrigin(publicOrigin).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return rollback(ErrConflict)
		}
		return rollback(err)
	}
	if _, err = tx.DeploymentSettingAudit.Create().
		SetDeploymentID(current.ID).
		SetActorUserID(actorUserID).
		SetOldName(current.Name).
		SetNewName(name).
		SetOldPublicOrigin(current.PublicOrigin).
		SetNewPublicOrigin(publicOrigin).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if current.PublicOrigin != publicOrigin {
		if _, err = tx.PortalSession.Update().Where(portalsession.RevokedEQ(false)).SetRevoked(true).Save(ctx); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return DeploymentSettingsView{}, err
	}
	s.SetPublicOrigin(updated.PublicOrigin)
	return DeploymentSettingsView{DeploymentID: updated.ID, Name: updated.Name, Timezone: updated.Timezone, PublicOrigin: updated.PublicOrigin, UpdatedAt: updated.UpdatedAt}, nil
}
