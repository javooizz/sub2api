package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type fakeEmailDelivery struct {
	to       []string
	subjects []string
	bodies   []string
	err      error
}

func (f *fakeEmailDelivery) SendEmail(ctx context.Context, to, subject, body string) error {
	f.to = append(f.to, to)
	f.subjects = append(f.subjects, subject)
	f.bodies = append(f.bodies, body)
	return f.err
}

func TestEmailNotifySender_SendsToAllRecipients(t *testing.T) {
	delivery := &fakeEmailDelivery{}
	s := NewEmailNotifySender(delivery)
	ch := &NotifyChannel{
		ID: 1, Type: domain.NotifyChannelTypeEmail,
		Config: map[string]any{"recipients": []any{"a@x.com", "b@x.com"}},
	}
	ev := NotifyEvent{
		Title: "上游告警:demo", Summary: "余额不足",
		Items: []string{"余额 3.20 低于阈值 5.00", "分组 vip 倍率 1.0 → 1.5"},
		Link:  "/admin/upstream-providers?id=3",
	}

	if err := s.Send(context.Background(), ch, ev); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if len(delivery.to) != 2 || delivery.to[0] != "a@x.com" || delivery.to[1] != "b@x.com" {
		t.Fatalf("收件人不符: %v", delivery.to)
	}
	if !strings.Contains(delivery.bodies[0], "余额 3.20 低于阈值 5.00") {
		t.Fatalf("正文应包含变更条目,实际: %s", delivery.bodies[0])
	}
	if delivery.subjects[0] != "上游告警:demo" {
		t.Fatalf("主题不符: %s", delivery.subjects[0])
	}
}

func TestEmailNotifySender_NoRecipientsIsError(t *testing.T) {
	s := NewEmailNotifySender(&fakeEmailDelivery{})
	ch := &NotifyChannel{ID: 1, Config: map[string]any{}}
	if err := s.Send(context.Background(), ch, NotifyEvent{Title: "t"}); err == nil {
		t.Fatal("无收件人应返回错误")
	}
}

func TestEmailNotifySender_ContinuesOnPartialFailure(t *testing.T) {
	delivery := &fakeEmailDelivery{err: errors.New("smtp error")}
	s := NewEmailNotifySender(delivery)
	ch := &NotifyChannel{Config: map[string]any{"recipients": []any{"a@x.com", "b@x.com"}}}
	err := s.Send(context.Background(), ch, NotifyEvent{Title: "t"})
	if err == nil {
		t.Fatal("有失败应返回错误")
	}
	if len(delivery.to) != 2 {
		t.Fatalf("失败不应中断,两个收件人都应尝试,实际 %d", len(delivery.to))
	}
}
