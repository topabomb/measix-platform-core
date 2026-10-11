package usage

import (
	"context"

	"measix/platform/ent"
	"measix/platform/ent/agentspace"
	"measix/platform/internal/wire/usageingestapi"
)

func statisticsOnly(event usageingestapi.UsageSettlement) bool {
	return event.StatisticsOnly != nil && *event.StatisticsOnly
}

func observeDAVCredential(ctx context.Context, tx *ent.Tx, event usageingestapi.UsageSettlement) error {
	fact := event.Request
	if fact.UpstreamHttpStatus == nil || *fact.UpstreamHttpStatus != 401 || fact.DavSecretRef == nil || fact.WorkspaceTarget == nil {
		return nil
	}
	target := fact.WorkspaceTarget
	ref := fact.DavSecretRef
	service, err := tx.WorkspaceService.Get(ctx, target.WorkspaceServiceId)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if service.DavResourceID != fact.ResourceId {
		return nil
	}
	_, err = tx.AgentSpace.Update().Where(agentspace.IDEQ(fact.UserId), agentspace.WorkspaceServiceIDEQ(target.WorkspaceServiceId), agentspace.AgentSpaceIDEQ(target.AgentSpaceId), agentspace.RemoteUsernameEQ(target.RemoteUsername), agentspace.DavBindingRevisionEQ(int64(target.BindingRevision)), agentspace.DavSecretIDEQ(ref.SecretId), agentspace.DavSecretVersionEQ(int64(ref.SecretVersion)), agentspace.FileAccessIntentEQ("AUTHORIZED")).SetDavConfirmed(false).SetDiagnosticCode("dav_credential_unavailable").Save(ctx)
	return err
}
