package capability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/activation"
	"measix/platform/ent/managedrelease"
	"measix/platform/ent/releasehistoryaudit"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
)

var (
	ErrCleanupSelection  = errors.New("invalid release cleanup selection")
	ErrCleanupStale      = errors.New("release cleanup preview is stale")
	ErrRetentionRevision = errors.New("release retention revision conflict")
	ErrCleanupBusy       = errors.New("runtime activation is in progress")
)

func retention(row *ent.Deployment) (adminapi.ReleaseRetention, error) {
	n, d := 10, 30
	rule := adminapi.ReleaseRetentionRule{Enabled: false, KeepLast: &n, KeepDays: &d}
	if len(row.ReleaseRetentionJSON) > 0 {
		rule = adminapi.ReleaseRetentionRule{}
		if err := json.Unmarshal(row.ReleaseRetentionJSON, &rule); err != nil {
			return adminapi.ReleaseRetention{}, err
		}
	}
	return adminapi.ReleaseRetention{Revision: int(row.ReleaseRetentionRevision), Rule: rule, LastCleanupAt: row.ReleaseCleanupAt}, nil
}
func validRule(rule adminapi.ReleaseRetentionRule) bool {
	return (rule.KeepLast != nil || rule.KeepDays != nil) &&
		(rule.KeepLast == nil || (*rule.KeepLast >= 1 && *rule.KeepLast <= 1000)) &&
		(rule.KeepDays == nil || (*rule.KeepDays >= 1 && *rule.KeepDays <= 36500))
}
func (s *Service) GetReleaseRetention(ctx context.Context) (adminapi.ReleaseRetention, error) {
	row, err := s.Client.Deployment.Query().Only(ctx)
	if err != nil {
		return adminapi.ReleaseRetention{}, err
	}
	return retention(row)
}
func (s *Service) UpdateReleaseRetention(ctx context.Context, actor string, req adminapi.UpdateReleaseRetentionRequest) (adminapi.ReleaseRetention, error) {
	if !validRule(req.Rule) {
		return adminapi.ReleaseRetention{}, ErrCleanupSelection
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return adminapi.ReleaseRetention{}, err
	}
	defer tx.Rollback()
	if err = identity.RequireActiveAdmin(ctx, tx, actor); err != nil {
		return adminapi.ReleaseRetention{}, err
	}
	row, err := tx.Deployment.Query().Only(ctx)
	if err != nil {
		return adminapi.ReleaseRetention{}, err
	}
	if int(row.ReleaseRetentionRevision) != req.ExpectedRevision {
		return adminapi.ReleaseRetention{}, ErrRetentionRevision
	}
	raw, _ := json.Marshal(req.Rule)
	row, err = tx.Deployment.UpdateOneID(row.ID).SetReleaseRetentionJSON(raw).SetReleaseRetentionRevision(row.ReleaseRetentionRevision + 1).ClearReleaseCleanupAt().Save(ctx)
	if err != nil {
		return adminapi.ReleaseRetention{}, err
	}
	if _, err = historyAudit(ctx, tx.Client(), actor, "RULE_UPDATED", nil, 0, &req.Rule, s.Now().UTC()); err != nil {
		return adminapi.ReleaseRetention{}, err
	}
	if err = tx.Commit(); err != nil {
		return adminapi.ReleaseRetention{}, err
	}
	return retention(row)
}

func validSelection(sel adminapi.ReleaseCleanupSelection) bool {
	modes := 0
	if sel.ReleaseIds != nil {
		modes++
		if len(*sel.ReleaseIds) < 1 || len(*sel.ReleaseIds) > 200 {
			return false
		}
		seen := map[string]bool{}
		for _, id := range *sel.ReleaseIds {
			if id == "" || seen[id] {
				return false
			}
			seen[id] = true
		}
	}
	if sel.PublishedBefore != nil {
		modes++
	}
	if sel.UseRetentionRule != nil && *sel.UseRetentionRule {
		modes++
	}
	if sel.PublishedAfter != nil && (sel.PublishedBefore == nil || !sel.PublishedAfter.Before(*sel.PublishedBefore)) {
		return false
	}
	return modes == 1
}

// Metadata is read first. Full immutable contents are fetched only one record
// at a time, and every mutation uses the same plan inside a write transaction.
func cleanupPlan(ctx context.Context, c *ent.Client, sel adminapi.ReleaseCleanupSelection, now time.Time) (adminapi.ReleaseCleanupPreview, error) {
	p := adminapi.ReleaseCleanupPreview{Candidates: []adminapi.ReleaseCleanupItem{}, Protected: []adminapi.ReleaseCleanupItem{}, ReleasedUpstreamIds: []string{}}
	if !validSelection(sel) {
		return p, ErrCleanupSelection
	}
	dep, err := c.Deployment.Query().Only(ctx)
	if err != nil {
		return p, err
	}
	policy, err := retention(dep)
	if err != nil {
		return p, err
	}
	if sel.UseRetentionRule != nil && *sel.UseRetentionRule && (!policy.Rule.Enabled || !validRule(policy.Rule)) {
		return p, ErrCleanupSelection
	}
	state, err := c.ManagedState.Get(ctx, "current")
	if err != nil {
		return p, err
	}
	busy, err := c.Activation.Query().Where(activation.StateIn("APPLYING", "UNKNOWN")).Exist(ctx)
	if err != nil {
		return p, err
	}
	p.ActivationBlocked = busy || state.RuntimeStatus != "READY"
	controls, err := c.Activation.Query().Where(activation.ControlRevisionEQ(state.DesiredControlRevision)).Select(activation.FieldTargetGeneration).All(ctx)
	if err != nil {
		return p, err
	}
	protectedGen := map[int64]bool{}
	for _, a := range controls {
		if a.TargetGeneration != nil {
			protectedGen[*a.TargetGeneration] = true
		}
	}
	rows, err := c.ManagedRelease.Query().Order(ent.Desc(managedrelease.FieldManagedGeneration)).Select(managedrelease.FieldID, managedrelease.FieldManagedGeneration, managedrelease.FieldStatus, managedrelease.FieldCreatedAt, managedrelease.FieldSnapshotHash).All(ctx)
	if err != nil {
		return p, err
	}
	wanted := map[string]bool{}
	if sel.ReleaseIds != nil {
		for _, id := range *sel.ReleaseIds {
			wanted[id] = true
		}
	}
	removed := map[string]bool{}
	upstreams := map[string]bool{}
	for i, row := range rows {
		selected := wanted[row.ID]
		if sel.PublishedBefore != nil {
			selected = row.CreatedAt.Before(sel.PublishedBefore.UTC()) && (sel.PublishedAfter == nil || !row.CreatedAt.Before(sel.PublishedAfter.UTC()))
		}
		if sel.UseRetentionRule != nil && *sel.UseRetentionRule {
			selected = (policy.Rule.KeepLast == nil || i >= *policy.Rule.KeepLast) && (policy.Rule.KeepDays == nil || row.CreatedAt.Before(now.AddDate(0, 0, -*policy.Rule.KeepDays)))
		}
		if !selected {
			continue
		}
		delete(wanted, row.ID)
		item := adminapi.ReleaseCleanupItem{ReleaseId: row.ID, ManagedGeneration: int(row.ManagedGeneration), Status: row.Status}
		switch {
		case row.Status == "ACTIVE" || row.ManagedGeneration == state.ActiveManagedGeneration || (state.ActiveReleaseID != nil && row.ID == *state.ActiveReleaseID):
			item.Reason = "active"
		case row.Status == "STAGED":
			item.Reason = "staged"
		case protectedGen[row.ManagedGeneration]:
			item.Reason = "control_target"
		case row.Status != "SUPERSEDED" && row.Status != "ACTIVATION_FAILED":
			item.Reason = "ineligible"
		}
		if item.Reason != "" {
			p.Protected = append(p.Protected, item)
			continue
		}
		if len(p.Candidates) == 200 {
			p.HasMore = true
			continue
		}
		full, err := c.ManagedRelease.Get(ctx, row.ID)
		if err != nil {
			return p, err
		}
		ids, err := upstream.ReferencedUpstreams(full.ReleaseContentJSON)
		if err != nil {
			return p, err
		}
		for _, id := range ids {
			upstreams[id] = true
		}
		item.Bytes = int64(len(full.ReleaseContentJSON) + len(full.SnapshotJSON))
		p.Candidates = append(p.Candidates, item)
		p.ReclaimableBytes += item.Bytes
		removed[row.ID] = true
	}
	for id := range wanted {
		p.Protected = append(p.Protected, adminapi.ReleaseCleanupItem{ReleaseId: id, Reason: "not_found"})
	}
	sort.Slice(p.Protected, func(i, j int) bool { return p.Protected[i].ReleaseId < p.Protected[j].ReleaseId })
	// Only connections which lose their last retained/draft reference are shown.
	drafts, err := c.ManagedDraft.Query().All(ctx)
	if err != nil {
		return p, err
	}
	for _, d := range drafts {
		ids, e := upstream.ReferencedUpstreams(d.ContentJSON)
		if e != nil {
			return p, e
		}
		for _, id := range ids {
			delete(upstreams, id)
		}
	}
	for _, row := range rows {
		if removed[row.ID] || len(upstreams) == 0 {
			continue
		}
		full, e := c.ManagedRelease.Get(ctx, row.ID)
		if e != nil {
			return p, e
		}
		ids, e := upstream.ReferencedUpstreams(full.ReleaseContentJSON)
		if e != nil {
			return p, e
		}
		for _, id := range ids {
			delete(upstreams, id)
		}
	}
	for id := range upstreams {
		p.ReleasedUpstreamIds = append(p.ReleasedUpstreamIds, id)
	}
	sort.Strings(p.ReleasedUpstreamIds)
	raw, _ := json.Marshal(struct {
		Selection      adminapi.ReleaseCleanupSelection
		PolicyRevision int
		StateRevision  int64
		Preview        adminapi.ReleaseCleanupPreview
	}{sel, policy.Revision, state.ManagedStateRevision, p})
	hash := sha256.Sum256(raw)
	p.PreviewHash = "sha256:" + hex.EncodeToString(hash[:])
	return p, nil
}
func (s *Service) PreviewReleaseCleanup(ctx context.Context, sel adminapi.ReleaseCleanupSelection) (adminapi.ReleaseCleanupPreview, error) {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return adminapi.ReleaseCleanupPreview{}, err
	}
	defer tx.Rollback()
	return cleanupPlan(ctx, tx.Client(), sel, s.Now().UTC())
}
func (s *Service) ExecuteReleaseCleanup(ctx context.Context, actor string, req adminapi.ExecuteReleaseCleanupRequest) (adminapi.ReleaseCleanupResult, error) {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return adminapi.ReleaseCleanupResult{}, err
	}
	defer tx.Rollback()
	if err = identity.RequireActiveAdmin(ctx, tx, actor); err != nil {
		return adminapi.ReleaseCleanupResult{}, err
	}
	p, err := cleanupPlan(ctx, tx.Client(), req.Selection, s.Now().UTC())
	if err != nil {
		return adminapi.ReleaseCleanupResult{}, err
	}
	if p.ActivationBlocked {
		return adminapi.ReleaseCleanupResult{}, ErrCleanupBusy
	}
	if p.PreviewHash != req.PreviewHash {
		return adminapi.ReleaseCleanupResult{}, ErrCleanupStale
	}
	result, err := deleteHistory(ctx, tx.Client(), actor, "MANUAL_CLEANUP", p, s.Now().UTC())
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func deleteHistory(ctx context.Context, c *ent.Client, actor, kind string, p adminapi.ReleaseCleanupPreview, now time.Time) (adminapi.ReleaseCleanupResult, error) {
	ids := make([]string, 0, len(p.Candidates))
	gens := make([]int, 0, len(p.Candidates))
	for _, item := range p.Candidates {
		ids = append(ids, item.ReleaseId)
		gens = append(gens, item.ManagedGeneration)
	}
	if len(ids) == 0 {
		return adminapi.ReleaseCleanupResult{}, ErrCleanupSelection
	}
	// Freeze each retained successor's original comparison before removing a
	// predecessor. No retained snapshot or published content is rewritten.
	rows, err := c.ManagedRelease.Query().Order(ent.Asc(managedrelease.FieldManagedGeneration)).Select(managedrelease.FieldID, managedrelease.FieldDiffSummaryJSON).All(ctx)
	if err != nil {
		return adminapi.ReleaseCleanupResult{}, err
	}
	deleted := map[string]bool{}
	for _, id := range ids {
		deleted[id] = true
	}
	for i, row := range rows {
		if deleted[row.ID] || len(row.DiffSummaryJSON) > 0 {
			continue
		}
		if i == 0 || !deleted[rows[i-1].ID] {
			continue
		}
		current, e := c.ManagedRelease.Get(ctx, row.ID)
		if e != nil {
			return adminapi.ReleaseCleanupResult{}, e
		}
		prev, e := c.ManagedRelease.Get(ctx, rows[i-1].ID)
		if e != nil {
			return adminapi.ReleaseCleanupResult{}, e
		}
		summary, _, e := releaseSummary(current, prev)
		if e != nil {
			return adminapi.ReleaseCleanupResult{}, e
		}
		raw, _ := json.Marshal(summary)
		if _, e = c.ManagedRelease.UpdateOneID(row.ID).SetDiffSummaryJSON(raw).Save(ctx); e != nil {
			return adminapi.ReleaseCleanupResult{}, e
		}
	}
	if _, err = c.ManagedRelease.Delete().Where(managedrelease.IDIn(ids...)).Exec(ctx); err != nil {
		return adminapi.ReleaseCleanupResult{}, err
	}
	auditID, err := historyAudit(ctx, c, actor, kind, gens, p.ReclaimableBytes, nil, now)
	return adminapi.ReleaseCleanupResult{DeletedCount: len(ids), ReclaimedBytes: p.ReclaimableBytes, AuditId: auditID}, err
}
func historyAudit(ctx context.Context, c *ent.Client, actor, kind string, gens []int, n int64, rule *adminapi.ReleaseRetentionRule, now time.Time) (int, error) {
	if gens == nil {
		gens = []int{}
	}
	details := adminapi.ReleaseHistoryAudit{ActorUserId: "", Kind: kind, CreatedAt: now, ReleaseGenerations: gens, ReclaimedBytes: n, Rule: rule}
	raw, _ := json.Marshal(details)
	row, err := c.ReleaseHistoryAudit.Create().SetActorUserID(actor).SetKind(kind).SetDetailsJSON(raw).SetCreatedAt(now).Save(ctx)
	if err != nil {
		return 0, err
	}
	threshold, err := c.ReleaseHistoryAudit.Query().Order(ent.Desc(releasehistoryaudit.FieldID)).Offset(199).First(ctx)
	if err == nil {
		_, err = c.ReleaseHistoryAudit.Delete().Where(releasehistoryaudit.IDLT(threshold.ID)).Exec(ctx)
	} else if ent.IsNotFound(err) {
		err = nil
	}
	return row.ID, err
}
func (s *Service) ListReleaseHistoryAudit(ctx context.Context) (adminapi.ReleaseHistoryAuditPage, error) {
	rows, err := s.Client.ReleaseHistoryAudit.Query().Order(ent.Desc(releasehistoryaudit.FieldID)).Limit(200).All(ctx)
	out := adminapi.ReleaseHistoryAuditPage{Items: []adminapi.ReleaseHistoryAudit{}}
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		var item adminapi.ReleaseHistoryAudit
		if err = json.Unmarshal(row.DetailsJSON, &item); err != nil {
			return out, err
		}
		item.AuditId = row.ID
		item.ActorUserId = row.ActorUserID
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// The existing Hub reconciler drives this bounded hourly sweep. There is no
// second timer, worker or retry queue, and disabled rules do not mutate history.
func (s *Service) SweepReleaseRetention(ctx context.Context) error {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	dep, err := tx.Deployment.Query().Only(ctx)
	if err != nil {
		return err
	}
	policy, err := retention(dep)
	if err != nil {
		return err
	}
	now := s.Now().UTC()
	if !policy.Rule.Enabled || (dep.ReleaseCleanupAt != nil && now.Sub(*dep.ReleaseCleanupAt) < time.Hour) {
		return nil
	}
	enabled := true
	p, err := cleanupPlan(ctx, tx.Client(), adminapi.ReleaseCleanupSelection{UseRetentionRule: &enabled}, now)
	if err != nil {
		return err
	}
	if p.ActivationBlocked {
		return nil
	}
	if len(p.Candidates) > 0 {
		if _, err = deleteHistory(ctx, tx.Client(), "system", "AUTOMATIC_CLEANUP", p, now); err != nil {
			return err
		}
	}
	if _, err = tx.Deployment.UpdateOneID(dep.ID).SetReleaseCleanupAt(now).Save(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

// NextGeneration uses the durable high-water mark even after failed releases
// have been purged; the remaining maximum supports older seeded test databases.
func NextGeneration(ctx context.Context, c *ent.Client) (int, error) {
	state, err := c.ManagedState.Get(ctx, "current")
	if err != nil {
		return 0, err
	}
	last := state.LastAssignedGeneration
	row, err := c.ManagedRelease.Query().Order(ent.Desc(managedrelease.FieldManagedGeneration)).First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return 0, err
	}
	if row != nil && row.ManagedGeneration > last {
		last = row.ManagedGeneration
	}
	if state.ActiveManagedGeneration > last {
		last = state.ActiveManagedGeneration
	}
	return int(last) + 1, nil
}
