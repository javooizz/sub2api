package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/upstreamchangeevent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type upstreamChangeEventRepository struct {
	client *dbent.Client
}

// NewUpstreamChangeEventRepository 构造变更事件 Repository。
func NewUpstreamChangeEventRepository(client *dbent.Client) service.UpstreamChangeEventRepository {
	return &upstreamChangeEventRepository{client: client}
}

// Insert 批量插入变更事件草稿。不使用批量 CreateBulk 以便逐条处理错误。
func (r *upstreamChangeEventRepository) Insert(ctx context.Context, providerID int64, drafts []service.UpstreamChangeEventDraft) ([]service.UpstreamChangeEvent, error) {
	out := make([]service.UpstreamChangeEvent, 0, len(drafts))
	for _, d := range drafts {
		e, err := r.client.UpstreamChangeEvent.Create().
			SetProviderID(providerID).
			SetType(d.Type).
			SetSummary(d.Summary).
			SetDetail(d.Detail).
			SetNotified(false).
			Save(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, service.UpstreamChangeEvent{
			ID:         e.ID,
			ProviderID: e.ProviderID,
			Type:       e.Type,
			Summary:    e.Summary,
			Detail:     e.Detail,
			Notified:   e.Notified,
			CreatedAt:  e.CreatedAt,
		})
	}
	return out, nil
}

// ListByProvider 游标分页查询变更事件(created_at DESC, id DESC)。
// 游标条件:WHERE (created_at < before) OR (created_at = before AND id < beforeID)。
// limit 默认 20,上限 100。
func (r *upstreamChangeEventRepository) ListByProvider(ctx context.Context, providerID int64, filter service.UpstreamChangeEventFilter) ([]service.UpstreamChangeEvent, error) {
	q := r.client.UpstreamChangeEvent.Query().
		Where(upstreamchangeevent.ProviderIDEQ(providerID))

	// 游标过滤:仅在两个游标字段均非 nil 时生效
	if filter.BeforeCreatedAt != nil && filter.BeforeID != nil {
		q = q.Where(
			upstreamchangeevent.Or(
				upstreamchangeevent.CreatedAtLT(*filter.BeforeCreatedAt),
				upstreamchangeevent.And(
					upstreamchangeevent.CreatedAtEQ(*filter.BeforeCreatedAt),
					upstreamchangeevent.IDLT(*filter.BeforeID),
				),
			),
		)
	}

	// limit 范围:默认 20,上限 100
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := q.
		Order(
			dbent.Desc(upstreamchangeevent.FieldCreatedAt),
			dbent.Desc(upstreamchangeevent.FieldID),
		).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]service.UpstreamChangeEvent, 0, len(rows))
	for _, e := range rows {
		out = append(out, service.UpstreamChangeEvent{
			ID:         e.ID,
			ProviderID: e.ProviderID,
			Type:       e.Type,
			Summary:    e.Summary,
			Detail:     e.Detail,
			Notified:   e.Notified,
			CreatedAt:  e.CreatedAt,
		})
	}
	return out, nil
}
