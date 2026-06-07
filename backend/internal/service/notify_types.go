package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// NotifyChannel 通知渠道业务对象（spec §4.3）。
type NotifyChannel struct {
	ID         int64
	Name       string
	Type       string     // domain.NotifyChannelTypeEmail | domain.NotifyChannelTypeWebhook
	Scope      string     // 目前固定 domain.NotifyScopeUpstream
	Enabled    bool
	Events     []string   // 订阅事件类型；空 = 全部
	Config     map[string]any
	LastSentAt *time.Time
	LastError  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NotifyEvent 一次通知载荷（一次刷新的多条变更合并为一个事件，spec §8）。
type NotifyEvent struct {
	Source   string         // "upstream"
	Severity string         // info | warning | error
	Title    string
	Summary  string
	Items    []string       // 各变更摘要行
	Link     string         // 跳回详情页的相对路径，如 /admin/upstream-providers?id=3
	Detail   map[string]any
}

// NotifySender 每种渠道类型一个实现（spec §8）。
type NotifySender interface {
	Send(ctx context.Context, ch *NotifyChannel, ev NotifyEvent) error
}

// NotifyChannelRepository 通知渠道存储接口，repository 层实现。
type NotifyChannelRepository interface {
	Create(ctx context.Context, ch *NotifyChannel) (*NotifyChannel, error)
	GetByID(ctx context.Context, id int64) (*NotifyChannel, error)
	ListByScope(ctx context.Context, scope string) ([]*NotifyChannel, error)
	Update(ctx context.Context, ch *NotifyChannel) (*NotifyChannel, error)
	Delete(ctx context.Context, id int64) error
	// MarkResult 记录发送结果：sentErr=nil 时更新 last_sent_at 并清空 last_error，
	// 否则只写 last_error（不更新 last_sent_at）。
	MarkResult(ctx context.Context, id int64, sentErr error) error
}

// ErrNotifyChannelNotFound 通知渠道不存在时返回，映射为 HTTP 404。
var ErrNotifyChannelNotFound = infraerrors.NotFound("NOTIFY_CHANNEL_NOT_FOUND", "notify channel not found")
