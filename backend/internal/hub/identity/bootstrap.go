package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"measix/platform/internal/hub/security"
	"measix/platform/pkg/platformid"
)

func (s *Service) Bootstrap(ctx context.Context, deploymentName, adminUsername, adminDisplayName, password string) (BootstrapResult, error) {
	if _, err := time.LoadLocation(s.BootstrapTimezone); err != nil || s.BootstrapTimezone == "" {
		return BootstrapResult{}, ErrInvalidInput
	}
	deploymentName = strings.TrimSpace(deploymentName)
	adminUsername = NormalizeUsername(adminUsername)
	adminDisplayName = strings.TrimSpace(adminDisplayName)
	if deploymentName == "" || adminUsername == "" || adminDisplayName == "" {
		return BootstrapResult{}, fmt.Errorf("invalid bootstrap identity")
	}

	count, err := s.Client.Deployment.Query().Count(ctx)
	if err != nil {
		return BootstrapResult{}, err
	}
	if count != 0 {
		return BootstrapResult{}, fmt.Errorf("deployment already bootstrapped")
	}

	now := s.Now().UTC()
	deploymentID := s.Signer.DeploymentID
	if err := platformid.Validate(platformid.Deployment, deploymentID); err != nil {
		return BootstrapResult{}, err
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return BootstrapResult{}, err
	}

	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return BootstrapResult{}, err
	}
	rollback := func(cause error) (BootstrapResult, error) {
		_ = tx.Rollback()
		return BootstrapResult{}, cause
	}

	if _, err := tx.Deployment.Create().
		SetID(deploymentID).
		SetName(deploymentName).
		SetTimezone(s.BootstrapTimezone).
		SetStatus("ACTIVE").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}

	adminID := platformid.New(platformid.User)
	if _, err := tx.User.Create().
		SetID(adminID).
		SetUsername(adminUsername).
		SetDisplayName(adminDisplayName).
		SetRole("ADMIN").
		SetStatus("ACTIVE").
		SetPasswordHash(passwordHash).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}

	policyID := platformid.New(platformid.Policy)
	content := map[string]any{
		"providers": []any{},
		"models":    []any{},
		"tts":       []any{},
		"asr":       []any{},
		"mcp":       []any{},
		"bindings":  []any{},
		"policy": map[string]any{
			"policyId":            policyID,
			"allowLocalProviders": true,
			"allowLocalTts":       true,
			"allowLocalAsr":       true,
			"allowLocalMcp":       true,
		},
	}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return rollback(err)
	}
	draftID := platformid.New(platformid.Draft)
	if _, err := tx.ManagedDraft.Create().
		SetID(draftID).
		SetDraftRevision(1).
		SetContentJSON(contentJSON).
		SetUpdatedByUserID(adminID).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.ManagedState.Create().
		SetID("current").
		SetActiveManagedGeneration(0).
		SetDesiredControlRevision(0).
		SetManagedStateRevision(1).
		SetRuntimeStatus("READY").
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}

	if err := tx.Commit(); err != nil {
		return BootstrapResult{}, err
	}
	return BootstrapResult{
		DeploymentID: deploymentID,
		AdminUserID:  adminID,
		DraftID:      draftID,
		PolicyID:     policyID,
	}, nil
}
