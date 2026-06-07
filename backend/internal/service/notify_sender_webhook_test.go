package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func webhookChannel(url string, extra map[string]any) *NotifyChannel {
	cfg := map[string]any{"url": url}
	for k, v := range extra {
		cfg[k] = v
	}
	return &NotifyChannel{ID: 1, Type: "webhook", Config: cfg}
}

func TestWebhookNotifySender_DefaultJSONBody(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	s := NewWebhookNotifySender(func(ctx context.Context) bool { return true }) // 测试放开私网(httptest 是 127.0.0.1)
	ev := NotifyEvent{Title: "标题", Summary: "摘要", Severity: "warning",
		Source: "upstream", Items: []string{"条目1"}, Link: "/admin/upstream-providers?id=1"}

	if err := s.Send(context.Background(), webhookChannel(srv.URL, nil), ev); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if got["title"] != "标题" || got["severity"] != "warning" {
		t.Fatalf("默认 JSON body 不符: %v", got)
	}
}

func TestWebhookNotifySender_CustomTemplate(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// 钉钉机器人风格模板
	tpl := `{"msgtype":"text","text":{"content":"{{.Title}}: {{join .Items "; "}}"}}`
	s := NewWebhookNotifySender(func(ctx context.Context) bool { return true })
	ev := NotifyEvent{Title: "上游告警", Items: []string{"a", "b"}}

	if err := s.Send(context.Background(), webhookChannel(srv.URL, map[string]any{"body_template": tpl}), ev); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if !strings.Contains(string(raw), "上游告警: a; b") {
		t.Fatalf("模板渲染不符: %s", raw)
	}
}

func TestWebhookNotifySender_RejectsPrivateAddressByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("私网地址不应被请求")
	}))
	defer srv.Close()

	s := NewWebhookNotifySender(func(ctx context.Context) bool { return false }) // 默认禁私网
	err := s.Send(context.Background(), webhookChannel(srv.URL, nil), NotifyEvent{Title: "t"})
	if err == nil {
		t.Fatal("私网 webhook 默认应被拒绝")
	}
}

func TestWebhookNotifySender_RejectsBadScheme(t *testing.T) {
	s := NewWebhookNotifySender(func(ctx context.Context) bool { return true })
	err := s.Send(context.Background(), webhookChannel("ftp://example.com/hook", nil), NotifyEvent{Title: "t"})
	if err == nil {
		t.Fatal("非 http/https scheme 应被拒绝")
	}
}

func TestWebhookNotifySender_RejectsRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
	}))
	defer srv.Close()

	s := NewWebhookNotifySender(func(ctx context.Context) bool { return true })
	err := s.Send(context.Background(), webhookChannel(srv.URL, nil), NotifyEvent{Title: "t"})
	if err == nil {
		t.Fatal("重定向应被拒绝")
	}
}

func TestWebhookNotifySender_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	s := NewWebhookNotifySender(func(ctx context.Context) bool { return true })
	err := s.Send(context.Background(), webhookChannel(srv.URL, nil), NotifyEvent{Title: "t"})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("非 2xx 应返回含状态码错误,实际: %v", err)
	}
}
