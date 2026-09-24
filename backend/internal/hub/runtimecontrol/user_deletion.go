package runtimecontrol

import (
	"context"

	"measix/platform/ent"
	"measix/platform/ent/activation"
	"measix/platform/ent/budgetallocation"
	"measix/platform/ent/budgetaudit"
	"measix/platform/ent/budgetbucket"
	"measix/platform/ent/budgetlimit"
	"measix/platform/ent/budgetreconciliation"
	"measix/platform/ent/budgetrequest"
	"measix/platform/ent/budgetsettlement"
	"measix/platform/ent/budgettemplate"
	"measix/platform/ent/budgettemplateassignment"
	"measix/platform/ent/budgettemplateaudit"
	"measix/platform/ent/device"
	"measix/platform/ent/enrollment"
	"measix/platform/ent/enterpriseupdate"
	"measix/platform/ent/idempotencyrecord"
	"measix/platform/ent/manageddraft"
	"measix/platform/ent/managedrelease"
	"measix/platform/ent/portalsession"
	"measix/platform/ent/requestusage"
	"measix/platform/ent/secretversion"
	"measix/platform/ent/semanticusage"
	"measix/platform/ent/session"
	"measix/platform/ent/upstreamconfigrevision"
	"measix/platform/ent/usagedetail"
	"measix/platform/ent/usageevent"
	"measix/platform/ent/userbudget"
)

const deletedActorID = "deleted_principal"

// purgeUserData runs only after Relay has durably acknowledged the deleted
// principal tombstone. It removes user-owned identity, credential, usage and
// budget state in one transaction. Shared configuration survives, but any
// historical actor attribution is anonymized so it no longer retains the
// deleted principal identifier.
func purgeUserData(ctx context.Context, tx *ent.Tx, userID, deletionActivationID string) error {
	active, err := tx.BudgetRequest.Query().Where(
		budgetrequest.UserIDEQ(userID),
		budgetrequest.StateIn(budgetrequest.StateADMITTED, budgetrequest.StateSTARTED, budgetrequest.StateRECONCILIATION),
	).Exist(ctx)
	if err != nil {
		return err
	}
	if active {
		return ErrDeleteInFlight
	}
	usageRequestIDs, err := tx.RequestUsage.Query().Where(requestusage.UserIDEQ(userID)).Select(requestusage.FieldRequestID).Strings(ctx)
	if err != nil {
		return err
	}
	if len(usageRequestIDs) > 0 {
		if _, err = tx.UsageDetail.Delete().Where(usagedetail.RequestIDIn(usageRequestIDs...)).Exec(ctx); err != nil {
			return err
		}
		if _, err = tx.SemanticUsage.Delete().Where(semanticusage.RequestIDIn(usageRequestIDs...)).Exec(ctx); err != nil {
			return err
		}
		if _, err = tx.UsageEvent.Delete().Where(usageevent.RequestIDIn(usageRequestIDs...)).Exec(ctx); err != nil {
			return err
		}
	}
	if _, err = tx.RequestUsage.Delete().Where(requestusage.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}

	budgetRequestIDs, err := tx.BudgetRequest.Query().Where(budgetrequest.UserIDEQ(userID)).Select(budgetrequest.FieldID).Strings(ctx)
	if err != nil {
		return err
	}
	if len(budgetRequestIDs) > 0 {
		if _, err = tx.BudgetAudit.Delete().Where(budgetaudit.RequestIDIn(budgetRequestIDs...)).Exec(ctx); err != nil {
			return err
		}
		if _, err = tx.BudgetReconciliation.Delete().Where(budgetreconciliation.RequestIDIn(budgetRequestIDs...)).Exec(ctx); err != nil {
			return err
		}
		if _, err = tx.BudgetSettlement.Delete().Where(budgetsettlement.RequestIDIn(budgetRequestIDs...)).Exec(ctx); err != nil {
			return err
		}
		if _, err = tx.BudgetAllocation.Delete().Where(budgetallocation.RequestIDIn(budgetRequestIDs...)).Exec(ctx); err != nil {
			return err
		}
		if _, err = tx.BudgetRequest.Delete().Where(budgetrequest.IDIn(budgetRequestIDs...)).Exec(ctx); err != nil {
			return err
		}
	}

	budgetIDs, err := tx.UserBudget.Query().Where(userbudget.UserIDEQ(userID)).Select(userbudget.FieldID).Ints(ctx)
	if err != nil {
		return err
	}
	if len(budgetIDs) > 0 {
		scopeKeys, err := tx.BudgetLimit.Query().Where(budgetlimit.UserBudgetIDIn(budgetIDs...)).Select(budgetlimit.FieldScopeKey).Strings(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.BudgetLimit.Delete().Where(budgetlimit.UserBudgetIDIn(budgetIDs...)).Exec(ctx); err != nil {
			return err
		}
		if len(scopeKeys) > 0 {
			if _, err = tx.BudgetBucket.Delete().Where(budgetbucket.ScopeKeyIn(scopeKeys...)).Exec(ctx); err != nil {
				return err
			}
		}
	}
	if _, err = tx.BudgetAudit.Delete().Where(budgetaudit.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}
	if _, err = tx.UserBudget.Delete().Where(userbudget.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}
	if _, err = tx.BudgetTemplateAssignment.Delete().Where(budgettemplateassignment.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}
	if _, err = tx.BudgetTemplateAudit.Delete().Where(budgettemplateaudit.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}

	sessionIDs, err := tx.Session.Query().Where(session.UserIDEQ(userID)).Select(session.FieldID).Strings(ctx)
	if err != nil {
		return err
	}
	if len(sessionIDs) > 0 {
		if _, err = tx.PortalSession.Delete().Where(portalsession.SessionIDIn(sessionIDs...)).Exec(ctx); err != nil {
			return err
		}
	}
	if _, err = tx.Session.Delete().Where(session.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}
	if _, err = tx.Enrollment.Delete().Where(enrollment.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}
	if _, err = tx.Device.Delete().Where(device.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}

	activationIDs, err := tx.Activation.Query().Where(activation.SubjectIDEQ(userID), activation.IDNEQ(deletionActivationID)).Select(activation.FieldID).Strings(ctx)
	if err != nil {
		return err
	}
	if len(activationIDs) > 0 {
		if _, err = tx.IdempotencyRecord.Delete().Where(idempotencyrecord.ActivationIDIn(activationIDs...)).Exec(ctx); err != nil {
			return err
		}
		if _, err = tx.Activation.Delete().Where(activation.IDIn(activationIDs...)).Exec(ctx); err != nil {
			return err
		}
	}
	if _, err = tx.IdempotencyRecord.Delete().Where(idempotencyrecord.AdminUserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}

	if _, err = tx.Activation.Update().Where(activation.CreatedByUserIDEQ(userID)).SetCreatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.BudgetAudit.Update().Where(budgetaudit.ActorUserIDEQ(userID)).SetActorUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.BudgetLimit.Update().Where(budgetlimit.CreatedByUserIDEQ(userID)).SetCreatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.UserBudget.Update().Where(userbudget.UpdatedByUserIDEQ(userID)).SetUpdatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.BudgetTemplate.Update().Where(budgettemplate.CreatedByUserIDEQ(userID)).SetCreatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.BudgetTemplate.Update().Where(budgettemplate.UpdatedByUserIDEQ(userID)).SetUpdatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.BudgetTemplateAssignment.Update().Where(budgettemplateassignment.UpdatedByUserIDEQ(userID)).SetUpdatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.BudgetTemplateAudit.Update().Where(budgettemplateaudit.ActorUserIDEQ(userID)).SetActorUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.Enrollment.Update().Where(enrollment.CreatedByUserIDEQ(userID)).SetCreatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.EnterpriseUpdate.Update().Where(enterpriseupdate.CreatedByUserIDEQ(userID)).SetCreatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.ManagedDraft.Update().Where(manageddraft.UpdatedByUserIDEQ(userID)).SetUpdatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.ManagedRelease.Update().Where(managedrelease.CreatedByUserIDEQ(userID)).SetCreatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.SecretVersion.Update().Where(secretversion.CreatedByUserIDEQ(userID)).SetCreatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}
	if _, err = tx.UpstreamConfigRevision.Update().Where(upstreamconfigrevision.CreatedByUserIDEQ(userID)).SetCreatedByUserID(deletedActorID).Save(ctx); err != nil {
		return err
	}

	return tx.User.DeleteOneID(userID).Exec(ctx)
}
