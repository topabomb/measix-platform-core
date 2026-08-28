package enterpriseupdate

import (
	"context"
	"errors"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/enterpriseupdate"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

var (
	ErrNotFound      = errors.New("enterprise update not found")
	ErrInvalidStatus = errors.New("invalid status transition")
	ErrInvalidLimit  = errors.New("limit must be between 1 and 20")
)

type Service struct {
	Client *ent.Client
	Now    func() time.Time
}

func NewService(client *ent.Client) *Service {
	return &Service{Client: client, Now: time.Now}
}

type UpdateView struct {
	ID           string
	Title        string
	Content      string
	Status       string
	PublishedAt  *time.Time
	FeedRevision int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (s *Service) nextFeedRevision(ctx context.Context) (int64, error) {
	row, err := s.Client.EnterpriseUpdate.Query().Order(ent.Desc(enterpriseupdate.FieldFeedRevision)).First(ctx)
	if ent.IsNotFound(err) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return row.FeedRevision + 1, nil
}

func (s *Service) Create(ctx context.Context, createdBy string, title, content string) (UpdateView, error) {
	now := s.Now().UTC()
	rev, err := s.nextFeedRevision(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	row, err := s.Client.EnterpriseUpdate.Create().
		SetID(platformid.New(platformid.EntUpdate)).
		SetTitle(title).
		SetContent(content).
		SetStatus("DRAFT").
		SetFeedRevision(rev).
		SetCreatedByUserID(createdBy).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	return toView(row), nil
}

func (s *Service) Get(ctx context.Context, id string) (UpdateView, error) {
	row, err := s.Client.EnterpriseUpdate.Get(ctx, id)
	if ent.IsNotFound(err) {
		return UpdateView{}, ErrNotFound
	}
	if err != nil {
		return UpdateView{}, err
	}
	return toView(row), nil
}

func (s *Service) List(ctx context.Context, limit int) ([]UpdateView, int64, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.Client.EnterpriseUpdate.Query().Order(ent.Desc(enterpriseupdate.FieldCreatedAt)).Limit(limit).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	views := make([]UpdateView, 0, len(rows))
	for _, row := range rows {
		views = append(views, toView(row))
	}
	latestRev, err := s.Client.EnterpriseUpdate.Query().Order(ent.Desc(enterpriseupdate.FieldFeedRevision)).First(ctx)
	if ent.IsNotFound(err) {
		return views, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return views, latestRev.FeedRevision, nil
}

func (s *Service) Update(ctx context.Context, id, title, content string) (UpdateView, error) {
	row, err := s.Client.EnterpriseUpdate.Get(ctx, id)
	if ent.IsNotFound(err) {
		return UpdateView{}, ErrNotFound
	}
	if err != nil {
		return UpdateView{}, err
	}
	if row.Status != "DRAFT" {
		return UpdateView{}, ErrInvalidStatus
	}
	now := s.Now().UTC()
	updated, err := s.Client.EnterpriseUpdate.UpdateOne(row).
		SetTitle(title).
		SetContent(content).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	return toView(updated), nil
}

func (s *Service) Publish(ctx context.Context, id string) (UpdateView, error) {
	row, err := s.Client.EnterpriseUpdate.Get(ctx, id)
	if ent.IsNotFound(err) {
		return UpdateView{}, ErrNotFound
	}
	if err != nil {
		return UpdateView{}, err
	}
	if row.Status != "DRAFT" {
		return UpdateView{}, ErrInvalidStatus
	}
	now := s.Now().UTC()
	rev, err := s.nextFeedRevision(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	updated, err := s.Client.EnterpriseUpdate.UpdateOne(row).
		SetStatus("PUBLISHED").
		SetPublishedAt(now).
		SetFeedRevision(rev).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	return toView(updated), nil
}

func (s *Service) Withdraw(ctx context.Context, id string) (UpdateView, error) {
	row, err := s.Client.EnterpriseUpdate.Get(ctx, id)
	if ent.IsNotFound(err) {
		return UpdateView{}, ErrNotFound
	}
	if err != nil {
		return UpdateView{}, err
	}
	if row.Status != "PUBLISHED" {
		return UpdateView{}, ErrInvalidStatus
	}
	now := s.Now().UTC()
	rev, err := s.nextFeedRevision(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	updated, err := s.Client.EnterpriseUpdate.UpdateOne(row).
		SetStatus("WITHDRAWN").
		SetFeedRevision(rev).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	return toView(updated), nil
}

// ListPublished returns published updates ordered by publishedAt desc.
// It supports date filtering and limit as defined by the S0.2 contract.
func (s *Service) ListPublished(ctx context.Context, startDate, endDate *time.Time, limit int) ([]UpdateView, bool, error) {
	if limit < 1 || limit > 20 {
		return nil, false, ErrInvalidLimit
	}
	query := s.Client.EnterpriseUpdate.Query().Where(enterpriseupdate.StatusEQ("PUBLISHED"))
	if startDate != nil {
		query = query.Where(enterpriseupdate.PublishedAtGTE(*startDate))
	}
	if endDate != nil {
		// inclusive: end of day = endDate + 24h - 1ns
		endOfDay := endDate.Add(24*time.Hour - time.Nanosecond)
		query = query.Where(enterpriseupdate.PublishedAtLTE(endOfDay))
	}
	rows, err := query.Order(ent.Desc(enterpriseupdate.FieldPublishedAt)).Limit(limit + 1).All(ctx)
	if err != nil {
		return nil, false, err
	}
	truncated := len(rows) > limit
	if truncated {
		rows = rows[:limit]
	}
	views := make([]UpdateView, 0, len(rows))
	for _, row := range rows {
		views = append(views, toView(row))
	}
	return views, truncated, nil
}

func toView(row *ent.EnterpriseUpdate) UpdateView {
	return UpdateView{
		ID:           row.ID,
		Title:        row.Title,
		Content:      row.Content,
		Status:       row.Status,
		PublishedAt:  row.PublishedAt,
		FeedRevision: row.FeedRevision,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func ToAdminWire(v UpdateView) adminapi.EnterpriseUpdate {
	status := adminapi.EnterpriseUpdateStatus(v.Status)
	result := adminapi.EnterpriseUpdate{
		EnterpriseUpdateId: adminapi.EnterpriseUpdateId(v.ID),
		Title:              v.Title,
		Content:            v.Content,
		Status:             status,
		FeedRevision:       int(v.FeedRevision),
		CreatedAt:          v.CreatedAt,
		UpdatedAt:          v.UpdatedAt,
	}
	if v.PublishedAt != nil {
		result.PublishedAt = *v.PublishedAt
	}
	return result
}
