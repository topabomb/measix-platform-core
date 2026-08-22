package usage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/requestusage"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

var ErrInvalidBatch = errors.New("invalid request usage batch")

type Service struct {
	Client *ent.Client
	Now    func() time.Time
}

func NewService(client *ent.Client) *Service {
	return &Service{Client: client, Now: time.Now}
}

func (s *Service) Ingest(ctx context.Context, batch usageingestapi.UsageBatch) (usageingestapi.UsageBatchAck, error) {
	if s.Client == nil || len(batch.Events) < 1 || len(batch.Events) > 200 {
		return usageingestapi.UsageBatchAck{}, ErrInvalidBatch
	}
	for i := range batch.Events {
		if err := validateRequestUsage(batch.Events[i]); err != nil {
			return usageingestapi.UsageBatchAck{}, fmt.Errorf("%w: event[%d]: %v", ErrInvalidBatch, i, err)
		}
	}

	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return usageingestapi.UsageBatchAck{}, err
	}
	rollback := func(cause error) (usageingestapi.UsageBatchAck, error) {
		_ = tx.Rollback()
		return usageingestapi.UsageBatchAck{}, cause
	}

	ack := usageingestapi.UsageBatchAck{}
	ingestedAt := s.Now().UTC()
	for _, event := range batch.Events {
		exists, err := tx.RequestUsage.Query().Where(requestusage.RequestIDEQ(event.RequestId)).Exist(ctx)
		if err != nil {
			return rollback(err)
		}
		if exists {
			ack.DuplicateCount++
			continue
		}
		_, err = tx.RequestUsage.Create().
			SetRequestID(event.RequestId).
			SetNillableInteractionID(event.InteractionId).
			SetDeploymentID(event.DeploymentId).
			SetUserID(event.UserId).
			SetNillableDeviceID(event.DeviceId).
			SetResourceID(event.ResourceId).
			SetRuntimeRouteID(event.RuntimeRouteId).
			SetUpstreamID(event.UpstreamId).
			SetManagedGeneration(int64(event.ManagedGeneration)).
			SetControlRevision(int64(event.ControlRevision)).
			SetStartedAt(event.StartedAt.UTC()).
			SetCompletedAt(event.CompletedAt.UTC()).
			SetForwarded(event.Forwarded).
			SetHTTPStatus(event.HttpStatus).
			SetNillableUpstreamHTTPStatus(event.UpstreamHttpStatus).
			SetRequestBytes(int64(event.RequestBytes)).
			SetResponseBytes(int64(event.ResponseBytes)).
			SetDurationMs(int64(event.DurationMs)).
			SetNillableErrorClass(event.ErrorClass).
			SetIngestedAt(ingestedAt).
			Save(ctx)
		if err != nil {
			return rollback(err)
		}
		ack.AcceptedCount++
	}
	if err := tx.Commit(); err != nil {
		return usageingestapi.UsageBatchAck{}, err
	}
	return ack, nil
}

func validateRequestUsage(event usageingestapi.RequestUsageEvent) error {
	checks := []struct {
		kind  platformid.Kind
		value string
	}{
		{platformid.Request, event.RequestId},
		{platformid.Deployment, event.DeploymentId},
		{platformid.User, event.UserId},
		{platformid.Route, event.RuntimeRouteId},
		{platformid.Upstream, event.UpstreamId},
	}
	for _, check := range checks {
		if err := platformid.Validate(check.kind, check.value); err != nil {
			return err
		}
	}
	if event.InteractionId != nil && platformid.Validate(platformid.Interaction, *event.InteractionId) != nil {
		return fmt.Errorf("invalid interactionId")
	}
	if event.DeviceId != nil && platformid.Validate(platformid.Device, *event.DeviceId) != nil {
		return fmt.Errorf("invalid deviceId")
	}
	kind, err := platformid.KindOf(event.ResourceId)
	if err != nil || (kind != platformid.Model && kind != platformid.TTS && kind != platformid.ASR && kind != platformid.MCP) {
		return fmt.Errorf("invalid resourceId")
	}
	if event.ManagedGeneration < 0 || event.ControlRevision < 0 || event.HttpStatus < 100 || event.HttpStatus > 599 || event.RequestBytes < 0 || event.ResponseBytes < 0 || event.DurationMs < 0 {
		return fmt.Errorf("invalid counters or status")
	}
	if event.UpstreamHttpStatus != nil && (*event.UpstreamHttpStatus < 100 || *event.UpstreamHttpStatus > 599) {
		return fmt.Errorf("invalid upstream status")
	}
	if event.StartedAt.IsZero() || event.CompletedAt.IsZero() || event.CompletedAt.Before(event.StartedAt) {
		return fmt.Errorf("invalid event time range")
	}
	if !event.Forwarded && event.UpstreamHttpStatus != nil {
		return fmt.Errorf("unforwarded request cannot have upstream status")
	}
	return nil
}
