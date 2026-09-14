package enterpriseupdate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"measix/platform/ent"
	"measix/platform/ent/enterpriseupdate"
	"measix/platform/pkg/platformid"
)

var (
	ErrInvalidInput     = errors.New("invalid enterprise update")
	ErrNotFound         = errors.New("enterprise update not found")
	ErrInvalidStatus    = errors.New("invalid status transition")
	ErrInvalidLimit     = errors.New("limit must be between 1 and 20")
	ErrInvalidDateRange = errors.New("startDate must not be after endDate")
)

type Service struct {
	Client *ent.Client
	Now    func() time.Time
}

func NewService(client *ent.Client) *Service {
	return &Service{Client: client, Now: time.Now}
}

type UpdateView struct {
	ID            string
	Title         string
	Content       string
	ContentFormat string
	Category      string
	Severity      string
	Status        string
	PublishedAt   *time.Time
	FeedRevision  int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func validateInput(title, content, format, category, severity string) error {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(content) == "" ||
		(format != "PLAIN" && format != "MARKDOWN") ||
		(category != "NOTICE" && category != "ANNOUNCEMENT" && category != "MAINTENANCE") ||
		(severity != "INFO" && severity != "WARNING" && severity != "CRITICAL") {
		return ErrInvalidInput
	}
	return nil
}

func (s *Service) Create(ctx context.Context, createdBy, title, content, contentFormat, category, severity string) (UpdateView, error) {
	if err := validateInput(title, content, contentFormat, category, severity); err != nil {
		return UpdateView{}, err
	}
	now := s.Now().UTC()
	row, err := s.Client.EnterpriseUpdate.Create().
		SetID(platformid.New(platformid.EntUpdate)).
		SetTitle(title).
		SetContent(content).
		SetContentFormat(contentFormat).
		SetCategory(category).
		SetSeverity(severity).
		SetStatus("DRAFT").
		SetFeedRevision(0).
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

func (s *Service) List(ctx context.Context, limit int, after string) ([]UpdateView, int64, error) {
	if limit < 1 || limit > 201 {
		limit = 50
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	deployment, err := tx.Deployment.Query().Only(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := tx.EnterpriseUpdate.Query().Where(enterpriseupdate.IDGT(after)).Order(ent.Asc(enterpriseupdate.FieldID)).Limit(limit).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	views := make([]UpdateView, 0, len(rows))
	for _, row := range rows {
		views = append(views, toView(row))
	}
	return views, deployment.FeedRevision, nil
}

func (s *Service) Update(ctx context.Context, id, title, content, contentFormat, category, severity string) (UpdateView, error) {
	if err := validateInput(title, content, contentFormat, category, severity); err != nil {
		return UpdateView{}, err
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	defer tx.Rollback()
	row, err := tx.EnterpriseUpdate.Get(ctx, id)
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
	updated, err := tx.EnterpriseUpdate.UpdateOne(row).
		SetTitle(title).
		SetContent(content).
		SetContentFormat(contentFormat).
		SetCategory(category).
		SetSeverity(severity).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	if err := tx.Commit(); err != nil {
		return UpdateView{}, err
	}
	return toView(updated), nil
}

func (s *Service) Publish(ctx context.Context, id string) (UpdateView, error) {
	return s.transition(ctx, id, "DRAFT", "PUBLISHED")
}
func (s *Service) Withdraw(ctx context.Context, id string) (UpdateView, error) {
	return s.transition(ctx, id, "PUBLISHED", "WITHDRAWN")
}
func (s *Service) transition(ctx context.Context, id, from, to string) (UpdateView, error) {
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	defer tx.Rollback()
	row, err := tx.EnterpriseUpdate.Get(ctx, id)
	if ent.IsNotFound(err) {
		return UpdateView{}, ErrNotFound
	}
	if err != nil {
		return UpdateView{}, err
	}
	if row.Status != from {
		return UpdateView{}, ErrInvalidStatus
	}
	if to == "PUBLISHED" {
		if err := validateInput(row.Title, row.Content, row.ContentFormat, row.Category, row.Severity); err != nil {
			return UpdateView{}, err
		}
	}
	deployment, err := tx.Deployment.Query().Only(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	deployment, err = tx.Deployment.UpdateOneID(deployment.ID).AddFeedRevision(1).Save(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	now := s.Now().UTC()
	update := tx.EnterpriseUpdate.UpdateOneID(id).SetStatus(to).SetFeedRevision(deployment.FeedRevision).SetUpdatedAt(now)
	if to == "PUBLISHED" {
		update.SetPublishedAt(now)
	}
	row, err = update.Save(ctx)
	if err != nil {
		return UpdateView{}, err
	}
	if err := tx.Commit(); err != nil {
		return UpdateView{}, err
	}
	return toView(row), nil
}

type FeedMetadata struct {
	Timezone string
	Revision int64
	ETag     string
}

// ListPublished captures the query, revision and items in one transaction.
// Inputs denote calendar dates, not UTC instants. AddDate handles DST boundaries.
func (s *Service) ListPublished(ctx context.Context, startDate, endDate *time.Time, limit int) ([]UpdateView, bool, FeedMetadata, error) {
	if limit < 1 || limit > 20 {
		return nil, false, FeedMetadata{}, ErrInvalidLimit
	}
	tx, err := s.Client.Tx(ctx)
	if err != nil {
		return nil, false, FeedMetadata{}, err
	}
	defer tx.Rollback()
	deployment, err := tx.Deployment.Query().Only(ctx)
	if err != nil {
		return nil, false, FeedMetadata{}, err
	}
	location, err := time.LoadLocation(deployment.Timezone)
	if err != nil {
		return nil, false, FeedMetadata{}, err
	}
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, location) }
	var start, end *time.Time
	if startDate != nil {
		v := day(*startDate)
		start = &v
	}
	if endDate != nil {
		v := day(*endDate)
		end = &v
	} else if start != nil {
		v := day(s.Now().In(location))
		end = &v
	}
	if start != nil && end != nil && start.After(*end) {
		return nil, false, FeedMetadata{}, ErrInvalidDateRange
	}
	query := tx.EnterpriseUpdate.Query().Where(enterpriseupdate.StatusEQ("PUBLISHED"))
	if start != nil {
		query = query.Where(enterpriseupdate.PublishedAtGTE(start.UTC()))
	}
	if end != nil {
		query = query.Where(enterpriseupdate.PublishedAtLT(end.AddDate(0, 0, 1).UTC()))
	}
	rows, err := query.Order(ent.Desc(enterpriseupdate.FieldPublishedAt), ent.Asc(enterpriseupdate.FieldID)).Limit(limit + 1).All(ctx)
	if err != nil {
		return nil, false, FeedMetadata{}, err
	}
	truncated := len(rows) > limit
	if truncated {
		rows = rows[:limit]
	}
	views := make([]UpdateView, 0, len(rows))
	for _, row := range rows {
		views = append(views, toView(row))
	}
	// Include normalized query and represented content, not a global revision alone.
	payload, err := json.Marshal(struct {
		Revision   int64
		Timezone   string
		Start, End *time.Time
		Limit      int
		Items      []UpdateView
		Truncated  bool
	}{deployment.FeedRevision, deployment.Timezone, start, end, limit, views, truncated})
	if err != nil {
		return nil, false, FeedMetadata{}, err
	}
	etag := fmt.Sprintf("\"feed-%x\"", sha256.Sum256(payload))
	return views, truncated, FeedMetadata{Timezone: deployment.Timezone, Revision: deployment.FeedRevision, ETag: etag}, nil
}

// The deployment owns the monotonic public revision; drafts do not advance it.
func (s *Service) LatestFeedRevision(ctx context.Context) (int64, error) {
	deployment, err := s.Client.Deployment.Query().Only(ctx)
	if err != nil {
		return 0, err
	}
	return deployment.FeedRevision, nil
}

func toView(row *ent.EnterpriseUpdate) UpdateView {
	return UpdateView{
		ID:            row.ID,
		Title:         row.Title,
		Content:       row.Content,
		ContentFormat: row.ContentFormat,
		Category:      row.Category,
		Severity:      row.Severity,
		Status:        row.Status,
		PublishedAt:   row.PublishedAt,
		FeedRevision:  row.FeedRevision,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
