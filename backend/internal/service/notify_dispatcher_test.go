package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type fakeNotifyChannelRepo struct {
	channels []*NotifyChannel
	marked   map[int64]error // channelID -> sentErr
}

func (f *fakeNotifyChannelRepo) Create(ctx context.Context, ch *NotifyChannel) (*NotifyChannel, error) {
	return ch, nil
}
func (f *fakeNotifyChannelRepo) GetByID(ctx context.Context, id int64) (*NotifyChannel, error) {
	for _, c := range f.channels {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, ErrNotifyChannelNotFound
}
func (f *fakeNotifyChannelRepo) ListByScope(ctx context.Context, scope string) ([]*NotifyChannel, error) {
	out := []*NotifyChannel{}
	for _, c := range f.channels {
		if c.Scope == scope {
			out = append(out, c)
		}
	}
	return out, nil
}
func (f *fakeNotifyChannelRepo) Update(ctx context.Context, ch *NotifyChannel) (*NotifyChannel, error) {
	return ch, nil
}
func (f *fakeNotifyChannelRepo) Delete(ctx context.Context, id int64) error { return nil }
func (f *fakeNotifyChannelRepo) MarkResult(ctx context.Context, id int64, sentErr error) error {
	if f.marked == nil {
		f.marked = map[int64]error{}
	}
	f.marked[id] = sentErr
	return nil
}

type fakeSender struct {
	sent []int64 // 收到的 channel id
	fail map[int64]error
}

func (f *fakeSender) Send(ctx context.Context, ch *NotifyChannel, ev NotifyEvent) error {
	f.sent = append(f.sent, ch.ID)
	if f.fail != nil {
		if err, ok := f.fail[ch.ID]; ok {
			return err
		}
	}
	return nil
}

func newTestDispatcher(repo NotifyChannelRepository, s NotifySender) *NotifyDispatcher {
	return NewNotifyDispatcher(repo, map[string]NotifySender{
		domain.NotifyChannelTypeEmail:   s,
		domain.NotifyChannelTypeWebhook: s,
	})
}

func TestNotifyDispatcher_FiltersByScopeEnabledEvents(t *testing.T) {
	now := time.Now()
	_ = now
	repo := &fakeNotifyChannelRepo{channels: []*NotifyChannel{
		{ID: 1, Type: domain.NotifyChannelTypeEmail, Scope: domain.NotifyScopeUpstream, Enabled: true},                                                         // 全订阅 → 发
		{ID: 2, Type: domain.NotifyChannelTypeEmail, Scope: domain.NotifyScopeUpstream, Enabled: false},                                                        // 停用 → 不发
		{ID: 3, Type: domain.NotifyChannelTypeEmail, Scope: "other", Enabled: true},                                                                            // scope 不符 → 不发
		{ID: 4, Type: domain.NotifyChannelTypeEmail, Scope: domain.NotifyScopeUpstream, Enabled: true, Events: []string{domain.UpstreamEventBalanceLow}},        // 订阅含 balance_low → 发
		{ID: 5, Type: domain.NotifyChannelTypeEmail, Scope: domain.NotifyScopeUpstream, Enabled: true, Events: []string{domain.UpstreamEventPriceChanged}},     // 只订价格 → 不发
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(repo, sender)

	d.Dispatch(context.Background(), NotifyEvent{
		Source: domain.NotifyScopeUpstream,
		Title:  "余额不足",
		Detail: map[string]any{"event_types": []string{domain.UpstreamEventBalanceLow}},
	})

	want := map[int64]bool{1: true, 4: true}
	if len(sender.sent) != 2 {
		t.Fatalf("期望发送 2 个渠道,实际 %v", sender.sent)
	}
	for _, id := range sender.sent {
		if !want[id] {
			t.Fatalf("渠道 %d 不应收到通知", id)
		}
	}
}

func TestNotifyDispatcher_SingleChannelFailureIsolated(t *testing.T) {
	repo := &fakeNotifyChannelRepo{channels: []*NotifyChannel{
		{ID: 1, Type: domain.NotifyChannelTypeEmail, Scope: domain.NotifyScopeUpstream, Enabled: true},
		{ID: 2, Type: domain.NotifyChannelTypeEmail, Scope: domain.NotifyScopeUpstream, Enabled: true},
	}}
	sender := &fakeSender{fail: map[int64]error{1: errors.New("smtp down")}}
	d := newTestDispatcher(repo, sender)

	d.Dispatch(context.Background(), NotifyEvent{Source: domain.NotifyScopeUpstream, Title: "t"})

	if len(sender.sent) != 2 {
		t.Fatalf("渠道 1 失败不应阻断渠道 2,实际发送 %v", sender.sent)
	}
	err1, called1 := repo.marked[1]
	if !called1 {
		t.Fatal("渠道 1 应调用 MarkResult")
	}
	if err1 == nil {
		t.Fatal("渠道 1 应记录 last_error")
	}
	err2, called2 := repo.marked[2]
	if !called2 {
		t.Fatal("渠道 2 应调用 MarkResult")
	}
	if err2 != nil {
		t.Fatalf("渠道 2 应记录成功,实际 %v", err2)
	}
}

func TestNotifyDispatcher_UnknownSenderTypeSkipped(t *testing.T) {
	repo := &fakeNotifyChannelRepo{channels: []*NotifyChannel{
		{ID: 1, Type: "carrier-pigeon", Scope: domain.NotifyScopeUpstream, Enabled: true},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(repo, sender) // map 中没有 carrier-pigeon

	d.Dispatch(context.Background(), NotifyEvent{Source: domain.NotifyScopeUpstream, Title: "t"})

	if len(sender.sent) != 0 {
		t.Fatalf("未知渠道类型应跳过,实际 %v", sender.sent)
	}
}
