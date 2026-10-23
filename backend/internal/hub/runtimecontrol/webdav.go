package runtimecontrol

import (
	"context"
	"encoding/json"

	"measix/platform/ent"
	"measix/platform/ent/agentspace"
	"measix/platform/ent/workspaceservice"
	"measix/platform/internal/wire/relaycontrolapi"
)

// Confirm only the exact binding carried by the acknowledged descriptor. This
// also initializes migrated bindings when DAV is first published. A late ACK
// cannot confirm a newer permission or credential revision.
func confirmDAVBindings(ctx context.Context, tx *ent.Tx, descriptor []byte) error {
	var state struct {
		DavBindings []relaycontrolapi.UserDAVBinding `json:"davBindings"`
	}
	if err := json.Unmarshal(descriptor, &state); err != nil {
		return err
	}
	for _, b := range state.DavBindings {
		owned, err := tx.WorkspaceService.Query().Where(workspaceservice.IDEQ(b.Target.WorkspaceServiceId), workspaceservice.DavResourceIDEQ(b.ResourceId)).Exist(ctx)
		if err != nil {
			return err
		}
		if !owned {
			continue
		}
		_, err = tx.AgentSpace.Update().Where(
			agentspace.IDEQ(b.UserId), agentspace.WorkspaceServiceIDEQ(b.Target.WorkspaceServiceId), agentspace.AgentSpaceIDEQ(b.Target.AgentSpaceId), agentspace.RemoteUsernameEQ(b.Target.RemoteUsername),
			agentspace.DavBindingRevisionEQ(int64(b.Target.BindingRevision)), agentspace.DavAccessModeEQ(string(b.AccessMode)), agentspace.DavSecretIDEQ(b.SecretRef.SecretId), agentspace.DavSecretVersionEQ(int64(b.SecretRef.SecretVersion)),
			agentspace.FileAccessIntentEQ("AUTHORIZED"), agentspace.DavConfirmedEQ(true), agentspace.RemoteActiveEQ(true), agentspace.StopPendingEQ(false), agentspace.IntentEQ("CONNECTED"),
		).SetDavAppliedRevision(int64(b.Target.BindingRevision)).SetDavAppliedAccessMode(string(b.AccessMode)).Save(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}
