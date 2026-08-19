package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/secretversion"
	"measix/platform/ent/upstreamconfigrevision"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

var (
	ErrNotFound         = errors.New("upstream entity not found")
	ErrRevisionConflict = errors.New("revision conflict")
	ErrInvalidConfig    = errors.New("invalid upstream config")
)

type SecretView struct {
	SecretID      string
	Name          string
	SecretVersion int
}

type UpstreamView struct {
	UpstreamID           string
	Name                 string
	ConfigRevision       int
	ActiveConfigRevision *int
	Status               string
	Config               adminapi.UpstreamConfig
}

type Service struct {
	Client *ent.Client
	Box    *security.SecretBox
	Now    func() time.Time
}

func NewService(client *ent.Client, box *security.SecretBox) *Service {
	return &Service{Client: client, Box: box, Now: time.Now}
}

func (s *Service) CreateSecret(ctx context.Context, createdBy, name, value string) (SecretView, error) {
	name = strings.TrimSpace(name)
	if name == "" || value == "" || s.Box == nil {
		return SecretView{}, fmt.Errorf("invalid secret")
	}
	payload, err := s.Box.Encrypt([]byte(value))
	if err != nil {
		return SecretView{}, err
	}
	now := s.Now().UTC()
	id := platformid.New(platformid.Secret)
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return SecretView{}, err
	}
	rollback := func(cause error) (SecretView, error) {
		_ = tx.Rollback()
		return SecretView{}, cause
	}
	if _, err := tx.Secret.Create().
		SetID(id).
		SetName(name).
		SetLatestSecretVersion(1).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.SecretVersion.Create().
		SetSecretID(id).
		SetSecretVersion(1).
		SetEncryptedPayload(payload).
		SetKeyVersion(s.Box.KeyVersion()).
		SetCreatedByUserID(createdBy).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return SecretView{}, err
	}
	return SecretView{SecretID: id, Name: name, SecretVersion: 1}, nil
}

func (s *Service) ReplaceSecret(ctx context.Context, createdBy, secretID string, expectedVersion int, value string) (SecretView, error) {
	if value == "" || expectedVersion <= 0 || s.Box == nil {
		return SecretView{}, fmt.Errorf("invalid secret replacement")
	}
	payload, err := s.Box.Encrypt([]byte(value))
	if err != nil {
		return SecretView{}, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return SecretView{}, err
	}
	rollback := func(cause error) (SecretView, error) {
		_ = tx.Rollback()
		return SecretView{}, cause
	}
	secret, err := tx.Secret.Get(ctx, secretID)
	if ent.IsNotFound(err) {
		return rollback(ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if secret.LatestSecretVersion != int64(expectedVersion) {
		return rollback(ErrRevisionConflict)
	}
	next := expectedVersion + 1
	now := s.Now().UTC()
	if _, err := tx.SecretVersion.Create().
		SetSecretID(secretID).
		SetSecretVersion(int64(next)).
		SetEncryptedPayload(payload).
		SetKeyVersion(s.Box.KeyVersion()).
		SetCreatedByUserID(createdBy).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.Secret.UpdateOneID(secretID).
		SetLatestSecretVersion(int64(next)).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return SecretView{}, err
	}
	return SecretView{SecretID: secretID, Name: secret.Name, SecretVersion: next}, nil
}

func (s *Service) ResolveSecret(ctx context.Context, secretID string, version int) ([]byte, error) {
	if s.Box == nil || version <= 0 {
		return nil, ErrNotFound
	}
	row, err := s.Client.SecretVersion.Query().Where(
		secretversion.SecretIDEQ(secretID),
		secretversion.SecretVersionEQ(int64(version)),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.Box.Decrypt(row.EncryptedPayload)
}

func (s *Service) CreateUpstream(ctx context.Context, createdBy string, config adminapi.UpstreamConfig) (UpstreamView, error) {
	config.Name = strings.TrimSpace(config.Name)
	if err := ValidateConfig(ctx, s.Client, config); err != nil {
		return UpstreamView{}, err
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return UpstreamView{}, err
	}
	id := platformid.New(platformid.Upstream)
	now := s.Now().UTC()
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return UpstreamView{}, err
	}
	rollback := func(cause error) (UpstreamView, error) {
		_ = tx.Rollback()
		return UpstreamView{}, cause
	}
	if _, err := tx.Upstream.Create().
		SetID(id).
		SetName(config.Name).
		SetConfigRevision(1).
		SetStatus("INACTIVE").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if _, err := tx.UpstreamConfigRevision.Create().
		SetUpstreamID(id).
		SetRevision(1).
		SetConfigJSON(configJSON).
		SetCreatedByUserID(createdBy).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return UpstreamView{}, err
	}
	return UpstreamView{UpstreamID: id, Name: config.Name, ConfigRevision: 1, Status: "INACTIVE", Config: config}, nil
}

func (s *Service) ListUpstreams(ctx context.Context) ([]UpstreamView, error) {
	rows, err := s.Client.Upstream.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]UpstreamView, 0, len(rows))
	for _, row := range rows {
		config, err := LoadCandidateConfig(ctx, s.Client, row.ID)
		if err != nil {
			return nil, err
		}
		views = append(views, upstreamView(row.ID, row.Name, row.ConfigRevision, row.ActiveConfigRevision, row.Status, config))
	}
	return views, nil
}

func (s *Service) GetUpstream(ctx context.Context, upstreamID string) (UpstreamView, error) {
	row, err := s.Client.Upstream.Get(ctx, upstreamID)
	if ent.IsNotFound(err) {
		return UpstreamView{}, ErrNotFound
	}
	if err != nil {
		return UpstreamView{}, err
	}
	config, err := LoadCandidateConfig(ctx, s.Client, upstreamID)
	if err != nil {
		return UpstreamView{}, err
	}
	return upstreamView(row.ID, row.Name, row.ConfigRevision, row.ActiveConfigRevision, row.Status, config), nil
}

func (s *Service) UpdateUpstream(ctx context.Context, createdBy, upstreamID string, expectedRevision int, config adminapi.UpstreamConfig) (UpstreamView, error) {
	config.Name = strings.TrimSpace(config.Name)
	if err := ValidateConfig(ctx, s.Client, config); err != nil {
		return UpstreamView{}, err
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return UpstreamView{}, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return UpstreamView{}, err
	}
	rollback := func(cause error) (UpstreamView, error) {
		_ = tx.Rollback()
		return UpstreamView{}, cause
	}
	row, err := tx.Upstream.Get(ctx, upstreamID)
	if ent.IsNotFound(err) {
		return rollback(ErrNotFound)
	}
	if err != nil {
		return rollback(err)
	}
	if row.ConfigRevision != int64(expectedRevision) {
		return rollback(ErrRevisionConflict)
	}
	next := expectedRevision + 1
	now := s.Now().UTC()
	if _, err := tx.UpstreamConfigRevision.Create().
		SetUpstreamID(upstreamID).
		SetRevision(int64(next)).
		SetConfigJSON(configJSON).
		SetCreatedByUserID(createdBy).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return rollback(err)
	}
	updated, err := tx.Upstream.UpdateOneID(upstreamID).
		SetName(config.Name).
		SetConfigRevision(int64(next)).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return UpstreamView{}, err
	}
	return upstreamView(updated.ID, updated.Name, updated.ConfigRevision, updated.ActiveConfigRevision, updated.Status, config), nil
}

func LoadCandidateConfig(ctx context.Context, client *ent.Client, upstreamID string) (adminapi.UpstreamConfig, error) {
	row, err := client.Upstream.Get(ctx, upstreamID)
	if ent.IsNotFound(err) {
		return adminapi.UpstreamConfig{}, ErrNotFound
	}
	if err != nil {
		return adminapi.UpstreamConfig{}, err
	}
	revision, err := client.UpstreamConfigRevision.Query().Where(
		upstreamconfigrevision.UpstreamIDEQ(upstreamID),
		upstreamconfigrevision.RevisionEQ(row.ConfigRevision),
	).Only(ctx)
	if err != nil {
		return adminapi.UpstreamConfig{}, err
	}
	var config adminapi.UpstreamConfig
	if err := json.Unmarshal(revision.ConfigJSON, &config); err != nil {
		return adminapi.UpstreamConfig{}, err
	}
	return config, nil
}

func ValidateConfig(ctx context.Context, client *ent.Client, config adminapi.UpstreamConfig) error {
	if strings.TrimSpace(config.Name) == "" || len(config.TransportCapabilities) == 0 || strings.TrimSpace(config.CorrelationMode) == "" {
		return ErrInvalidConfig
	}
	parsed, err := url.Parse(config.BaseUrl)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ErrInvalidConfig
	}
	if !config.Auth.Type.Valid() || !config.UsageCapabilityLevel.Valid() {
		return ErrInvalidConfig
	}
	if config.TimeoutDefaults.ConnectMs <= 0 || config.TimeoutDefaults.ResponseHeaderMs <= 0 || config.TimeoutDefaults.IdleMs <= 0 {
		return ErrInvalidConfig
	}
	refs, err := SecretRefs(config.Auth)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := platformid.Validate(platformid.Secret, ref.SecretID); err != nil || ref.Version <= 0 {
			return ErrInvalidConfig
		}
		count, err := client.SecretVersion.Query().Where(
			secretversion.SecretIDEQ(ref.SecretID),
			secretversion.SecretVersionEQ(int64(ref.Version)),
		).Count(ctx)
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrInvalidConfig
		}
	}
	return nil
}

type SecretRef struct {
	SecretID string
	Version  int
}

func SecretRefs(auth adminapi.UpstreamConfig_Auth) ([]SecretRef, error) {
	switch auth.Type {
	case adminapi.NONE:
		return nil, nil
	case adminapi.BEARER:
		ref, err := parseSecretRef(auth.AdditionalProperties["secretRef"])
		if err != nil {
			return nil, ErrInvalidConfig
		}
		return []SecretRef{ref}, nil
	case adminapi.STATICHEADER:
		if header, _ := auth.AdditionalProperties["headerName"].(string); strings.TrimSpace(header) == "" {
			return nil, ErrInvalidConfig
		}
		ref, err := parseSecretRef(auth.AdditionalProperties["secretRef"])
		if err != nil {
			return nil, ErrInvalidConfig
		}
		return []SecretRef{ref}, nil
	case adminapi.BASIC:
		if username, _ := auth.AdditionalProperties["username"].(string); strings.TrimSpace(username) == "" {
			return nil, ErrInvalidConfig
		}
		ref, err := parseSecretRef(auth.AdditionalProperties["passwordSecretRef"])
		if err != nil {
			return nil, ErrInvalidConfig
		}
		return []SecretRef{ref}, nil
	default:
		return nil, ErrInvalidConfig
	}
}

func parseSecretRef(value any) (SecretRef, error) {
	object, ok := value.(map[string]interface{})
	if !ok {
		return SecretRef{}, ErrInvalidConfig
	}
	id, ok := object["secretId"].(string)
	if !ok {
		return SecretRef{}, ErrInvalidConfig
	}
	version, ok := numericInt(object["secretVersion"])
	if !ok {
		return SecretRef{}, ErrInvalidConfig
	}
	return SecretRef{SecretID: id, Version: version}, nil
}

func numericInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		if v < 1 || v != float64(int(v)) {
			return 0, false
		}
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	default:
		return 0, false
	}
}

func upstreamView(id, name string, revision int64, active *int64, status string, config adminapi.UpstreamConfig) UpstreamView {
	var activeInt *int
	if active != nil {
		value := int(*active)
		activeInt = &value
	}
	return UpstreamView{UpstreamID: id, Name: name, ConfigRevision: int(revision), ActiveConfigRevision: activeInt, Status: status, Config: config}
}
