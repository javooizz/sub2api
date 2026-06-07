package service

import (
	"context"
	"log/slog"
	"time"
)

// NotifyDispatcher 按 scope+enabled+events 过滤渠道并逐个发送(spec §8)。
// 单渠道失败仅记 last_error + 日志,不影响其他渠道与调用方主流程。
type NotifyDispatcher struct {
	repo    NotifyChannelRepository
	senders map[string]NotifySender // type -> sender
}

// NewNotifyDispatcher 构造 NotifyDispatcher。
// senders 的 key 为渠道类型字符串(domain.NotifyChannelTypeEmail 等)。
func NewNotifyDispatcher(repo NotifyChannelRepository, senders map[string]NotifySender) *NotifyDispatcher {
	return &NotifyDispatcher{repo: repo, senders: senders}
}

// Dispatch 同步发送(调用方已在事务提交后调用,内部超时自护)。
// ev.Detail["event_types"]([]string)用于渠道订阅过滤；缺失视为不限。
func (d *NotifyDispatcher) Dispatch(ctx context.Context, ev NotifyEvent) {
	channels, err := d.repo.ListByScope(ctx, ev.Source)
	if err != nil {
		slog.Error("notify: 列出通知渠道失败", "scope", ev.Source, "error", err)
		return
	}

	eventTypes := extractEventTypes(ev)

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if !channelSubscribes(ch, eventTypes) {
			continue
		}
		sender, ok := d.senders[ch.Type]
		if !ok {
			slog.Warn("notify: 未知渠道类型,跳过", "channel_id", ch.ID, "type", ch.Type)
			continue
		}

		sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		sendErr := sender.Send(sendCtx, ch, ev)
		cancel()

		if sendErr != nil {
			slog.Warn("notify: 渠道发送失败", "channel_id", ch.ID, "type", ch.Type, "error", sendErr)
		}
		if markErr := d.repo.MarkResult(ctx, ch.ID, sendErr); markErr != nil {
			slog.Error("notify: 记录发送结果失败", "channel_id", ch.ID, "error", markErr)
		}
	}
}

// extractEventTypes 从 ev.Detail["event_types"] 提取事件类型列表。
// 支持 []string 和 []any(JSON 反序列化产生的类型)。
func extractEventTypes(ev NotifyEvent) []string {
	raw, ok := ev.Detail["event_types"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// channelSubscribes 判断渠道是否订阅了本次事件。
// 渠道 Events 为空 = 全部订阅；事件类型列表为空 = 不限类型。
// 两者均非空时，有交集即发送。
func channelSubscribes(ch *NotifyChannel, eventTypes []string) bool {
	if len(ch.Events) == 0 || len(eventTypes) == 0 {
		return true
	}
	sub := make(map[string]struct{}, len(ch.Events))
	for _, e := range ch.Events {
		sub[e] = struct{}{}
	}
	for _, e := range eventTypes {
		if _, ok := sub[e]; ok {
			return true
		}
	}
	return false
}
