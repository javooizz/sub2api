package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"
)

const webhookResponseLimit = 64 << 10 // 64KB，spec §8

// allowPrivateWebhookFunc 运行时读取设置项 upstream_notify_allow_private_webhook。
// Task 6 会将 SettingService.AllowPrivateWebhook 传入此处。
type allowPrivateWebhookFunc func(ctx context.Context) bool

// WebhookNotifySender 通用 webhook 渠道（spec §8 P1）。
//
// SSRF 防护策略：
//   - scheme 校验：仅允许 http / https
//   - 连接前 DNS 解析 + resolved-IP 校验（防 DNS rebinding）
//   - 禁止跟随重定向（CheckRedirect 返回错误）
//   - 响应体读取上限 64KB，避免慢速响应阻塞
//   - 凭证/响应原文严禁进日志（仅截取 200 字节进 error message）
type WebhookNotifySender struct {
	allowPrivate allowPrivateWebhookFunc
}

// NewWebhookNotifySender 构造 WebhookNotifySender。
// allowPrivate 为 nil 时视作永远禁止私网（与 false 等价）。
func NewWebhookNotifySender(allowPrivate allowPrivateWebhookFunc) *WebhookNotifySender {
	return &WebhookNotifySender{allowPrivate: allowPrivate}
}

// Send 实现 NotifySender 接口。
func (s *WebhookNotifySender) Send(ctx context.Context, ch *NotifyChannel, ev NotifyEvent) error {
	rawURL, _ := ch.Config["url"].(string)
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("webhook URL 非法：仅支持 http/https")
	}

	body, contentType, err := s.buildBody(ch, ev)
	if err != nil {
		return fmt.Errorf("渲染 webhook body 失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)

	// 支持自定义请求头（如钉钉 sign、企业微信 key 等）
	if headers, ok := ch.Config["headers"].(map[string]any); ok {
		for k, v := range headers {
			if sv, ok := v.(string); ok {
				req.Header.Set(k, sv)
			}
		}
	}

	client := s.newClient(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook 请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 仅读 200 字节用于错误诊断，避免响应原文（可能含敏感信息）大量进日志
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("webhook 响应 %d: %s", resp.StatusCode, string(snippet))
	}
	// 主动消耗响应体以便连接复用，但限制读取量
	_, _ = io.CopyN(io.Discard, resp.Body, webhookResponseLimit)
	return nil
}

// buildBody 构造请求 body。
// 若 Config["body_template"] 为空，使用默认标准 JSON（含 title/summary/severity/source/items/link）；
// 否则将其作为 Go text/template 渲染，FuncMap 注册 join / json 两个辅助函数。
func (s *WebhookNotifySender) buildBody(ch *NotifyChannel, ev NotifyEvent) ([]byte, string, error) {
	tplStr, _ := ch.Config["body_template"].(string)
	if strings.TrimSpace(tplStr) == "" {
		// 默认标准 JSON 结构
		payload := map[string]any{
			"title":    ev.Title,
			"summary":  ev.Summary,
			"severity": ev.Severity,
			"source":   ev.Source,
			"items":    ev.Items,
			"link":     ev.Link,
		}
		b, err := json.Marshal(payload)
		return b, "application/json", err
	}

	tpl, err := template.New("webhook").Funcs(template.FuncMap{
		// join 用于拼接字符串切片，如 {{join .Items "; "}}
		"join": strings.Join,
		// json 将任意值序列化为 JSON 字符串，用于嵌入复杂字段
		"json": func(v any) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
	}).Parse(tplStr)
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ev); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "application/json", nil
}

// newClient 构造带 SSRF 防护的 http.Client。
// 防护要点：
//  1. DialContext 中先 DNS 解析，用 isPrivateIP（同包）校验每个 resolved IP；
//     命中私网则跳过，用第一个通过校验的 IP 字面量直连（防 DNS rebinding）。
//  2. 逐 IP 尝试，全部被拒绝时返回最后一个错误。
//  3. CheckRedirect 直接拒绝，避免重定向到内网地址。
func (s *WebhookNotifySender) newClient(ctx context.Context) *http.Client {
	// 计算一次，避免在 DialContext 闭包中重复调用
	allowPrivate := s.allowPrivate != nil && s.allowPrivate(ctx)

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(dctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(dctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("无法解析 %s", host)
			}
			var lastErr error
			for _, a := range ips {
				if !allowPrivate && isPrivateIP(a.IP) {
					lastErr = fmt.Errorf("webhook 目标地址 %s 被 SSRF 策略拒绝(可在采集设置中放开私网)", a.IP)
					continue
				}
				conn, derr := dialer.DialContext(dctx, network, net.JoinHostPort(a.IP.String(), port))
				if derr != nil {
					lastErr = derr
					continue
				}
				return conn, nil
			}
			return nil, lastErr
		},
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
		// 拒绝任何重定向——重定向目标可能指向内网，是典型 SSRF 攻击向量
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errors.New("webhook 不允许重定向")
		},
	}
}
