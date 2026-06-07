package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
)

// emailDelivery 窄接口：EmailService.SendEmail 天然满足。
type emailDelivery interface {
	SendEmail(ctx context.Context, to, subject, body string) error
}

// EmailNotifySender email 渠道实现（spec §8 P0）。
// 借用现有 EmailService 的 SMTP 发信能力，不进 NotificationEmailService 模板/退订体系。
type EmailNotifySender struct {
	delivery emailDelivery
}

// NewEmailNotifySender 创建 email 通知发送器，delivery 通常传入 *EmailService。
func NewEmailNotifySender(delivery emailDelivery) *EmailNotifySender {
	return &EmailNotifySender{delivery: delivery}
}

// Send 向渠道配置的所有收件人发送告警邮件。
// 遍历全部收件人，首次失败记录后继续发送；全部发完后返回第一个错误。
func (s *EmailNotifySender) Send(ctx context.Context, ch *NotifyChannel, ev NotifyEvent) error {
	recipients := stringSliceFromConfig(ch.Config, "recipients")
	if len(recipients) == 0 {
		return errors.New("通知渠道未配置收件人")
	}
	body := buildNotifyEmailBody(ev)
	var firstErr error
	for _, to := range recipients {
		if err := s.delivery.SendEmail(ctx, to, ev.Title, body); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("发送至 %s 失败: %w", to, err)
		}
	}
	return firstErr
}

// buildNotifyEmailBody 内置简洁告警模板：标题 + 摘要 + 变更条目列表 + 跳转链接。
func buildNotifyEmailBody(ev NotifyEvent) string {
	var b strings.Builder
	b.WriteString("<div style=\"font-family:sans-serif;max-width:560px\">")
	b.WriteString("<h2 style=\"margin:0 0 8px\">" + html.EscapeString(ev.Title) + "</h2>")
	if ev.Summary != "" {
		b.WriteString("<p style=\"margin:0 0 12px;color:#444\">" + html.EscapeString(ev.Summary) + "</p>")
	}
	if len(ev.Items) > 0 {
		b.WriteString("<ul style=\"margin:0 0 12px;padding-left:20px\">")
		for _, item := range ev.Items {
			b.WriteString("<li style=\"margin:4px 0\">" + html.EscapeString(item) + "</li>")
		}
		b.WriteString("</ul>")
	}
	if ev.Link != "" {
		b.WriteString("<p style=\"margin:12px 0 0\"><a href=\"" + html.EscapeString(ev.Link) + "\">查看详情</a></p>")
	}
	b.WriteString("</div>")
	return b.String()
}

// stringSliceFromConfig 从 JSONB config 取字符串数组（兼容 []string 与 []any）。
// 空字符串和纯空白项会被过滤。
func stringSliceFromConfig(cfg map[string]any, key string) []string {
	raw, ok := cfg[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}
	return nil
}
