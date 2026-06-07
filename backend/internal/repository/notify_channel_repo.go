package repository

import (
	"context"
	"time"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/notifychannel"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// notifyChannelRepository 是 NotifyChannelRepository 的 ent 实现。
type notifyChannelRepository struct {
	client *dbent.Client
}

// NewNotifyChannelRepository 构造（wire 注入）。
func NewNotifyChannelRepository(client *dbent.Client) service.NotifyChannelRepository {
	return &notifyChannelRepository{client: client}
}

// toServiceNotifyChannel 将 ent 实体转换为 service 业务对象。
func toServiceNotifyChannel(e *dbent.NotifyChannel) *service.NotifyChannel {
	if e == nil {
		return nil
	}
	return &service.NotifyChannel{
		ID:         e.ID,
		Name:       e.Name,
		Type:       e.Type,
		Scope:      e.Scope,
		Enabled:    e.Enabled,
		Events:     e.Events,
		Config:     e.Config,
		LastSentAt: e.LastSentAt,
		LastError:  e.LastError,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
}

// Create 新建通知渠道。
func (r *notifyChannelRepository) Create(ctx context.Context, ch *service.NotifyChannel) (*service.NotifyChannel, error) {
	client := clientFromContext(ctx, r.client)
	created, err := client.NotifyChannel.Create().
		SetName(ch.Name).
		SetType(ch.Type).
		SetScope(ch.Scope).
		SetEnabled(ch.Enabled).
		SetEvents(ch.Events).
		SetConfig(ch.Config).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return toServiceNotifyChannel(created), nil
}

// GetByID 按 ID 查询通知渠道；不存在返回 ErrNotifyChannelNotFound。
func (r *notifyChannelRepository) GetByID(ctx context.Context, id int64) (*service.NotifyChannel, error) {
	client := clientFromContext(ctx, r.client)
	e, err := client.NotifyChannel.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrNotifyChannelNotFound
		}
		return nil, err
	}
	return toServiceNotifyChannel(e), nil
}

// ListByScope 返回指定 scope 下所有通知渠道，按 ID 升序排列。
func (r *notifyChannelRepository) ListByScope(ctx context.Context, scope string) ([]*service.NotifyChannel, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.NotifyChannel.Query().
		Where(notifychannel.ScopeEQ(scope)).
		Order(notifychannel.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.NotifyChannel, 0, len(rows))
	for _, e := range rows {
		out = append(out, toServiceNotifyChannel(e))
	}
	return out, nil
}

// Update 更新通知渠道可变字段；不存在返回 ErrNotifyChannelNotFound。
// Scope 业务固定,不可更新。
func (r *notifyChannelRepository) Update(ctx context.Context, ch *service.NotifyChannel) (*service.NotifyChannel, error) {
	client := clientFromContext(ctx, r.client)
	updated, err := client.NotifyChannel.UpdateOneID(ch.ID).
		SetName(ch.Name).
		SetType(ch.Type).
		SetEnabled(ch.Enabled).
		SetEvents(ch.Events).
		SetConfig(ch.Config).
		Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrNotifyChannelNotFound
		}
		return nil, err
	}
	return toServiceNotifyChannel(updated), nil
}

// Delete 删除通知渠道；不存在返回 ErrNotifyChannelNotFound。
func (r *notifyChannelRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	err := client.NotifyChannel.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrNotifyChannelNotFound
		}
		return err
	}
	return nil
}

// MarkResult 记录发送结果。
//
//   - sentErr == nil：更新 last_sent_at 为当前时间，并清空 last_error。
//   - sentErr != nil：仅写入 last_error（截断至 500 字节，UTF-8 安全），不更新 last_sent_at。
func (r *notifyChannelRepository) MarkResult(ctx context.Context, id int64, sentErr error) error {
	client := clientFromContext(ctx, r.client)
	upd := client.NotifyChannel.UpdateOneID(id)
	if sentErr == nil {
		now := time.Now()
		upd.SetLastSentAt(now).SetLastError("")
	} else {
		msg := sentErr.Error()
		if len(msg) > 500 {
			msg = msg[:500]
			for len(msg) > 0 && !utf8.ValidString(msg) {
				msg = msg[:len(msg)-1]
			}
		}
		upd.SetLastError(msg)
	}
	return upd.Exec(ctx)
}
