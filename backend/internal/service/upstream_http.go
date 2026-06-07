package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const (
	upstreamRequestTimeout  = 15 * time.Second // 单接口(spec §5.1)
	upstreamSnapshotTimeout = 60 * time.Second // 整次快照
)

// upstreamHTTPClient 每 provider 构造:代理 + cf cookie + 配套 UA(spec §5.1)。
// 低频轮询场景,不进连接池体系。
type upstreamHTTPClient struct {
	base    string // 归一化 site_url
	client  *http.Client
	headers map[string]string // 每请求附加头(含 Cookie/UA)
}

func newUpstreamHTTPClient(p *UpstreamProvider) (*upstreamHTTPClient, error) {
	base, err := NormalizeUpstreamURL(p.SiteURL)
	if err != nil {
		return nil, fmt.Errorf("site_url 非法: %w", err)
	}
	transport := &http.Transport{}
	if p.ProxyURL != "" {
		proxyURL, err := url.Parse(p.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("代理地址非法: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	headers := map[string]string{}
	if cf, _ := p.Credentials["cf_clearance"].(string); cf != "" {
		headers["Cookie"] = "cf_clearance=" + cf
		if ua, _ := p.Credentials["cf_user_agent"].(string); ua != "" {
			headers["User-Agent"] = ua // 必须与 cf_clearance 配套(spec §4.1.1)
		}
	}
	return &upstreamHTTPClient{
		base:    base,
		client:  &http.Client{Timeout: upstreamRequestTimeout, Transport: transport},
		headers: headers,
	}, nil
}

// doJSON 发送请求并解析 JSON;返回 HTTP 状态码供调用方分类。
// 错误分类:401/403 + CF 特征 → errUpstreamCFChallenge;401/403 其他 → errUpstreamAuth。
func (c *upstreamHTTPClient) doJSON(ctx context.Context, method, path string, body any, extraHeaders map[string]string, out any) (int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reqBody = bytes.NewReader(b)
	}
	u, err := url.JoinPath(c.base, path)
	if err != nil {
		return 0, err
	}
	// JoinPath 会转义 query,path 带 query 时拆开处理
	if i := strings.Index(path, "?"); i >= 0 {
		u, err = url.JoinPath(c.base, path[:i])
		if err != nil {
			return 0, err
		}
		u += path[i:]
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4MB 上限
	if err != nil {
		return resp.StatusCode, err
	}
	if isCFChallenge(resp, data) {
		return resp.StatusCode, errUpstreamCFChallenge
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return resp.StatusCode, errUpstreamAuth
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 注意:不把响应原文放进错误(可能含敏感信息),只给状态码(spec §4.1.1)
		return resp.StatusCode, fmt.Errorf("上游响应 %d", resp.StatusCode)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("解析上游响应失败: %w", err)
		}
	}
	return resp.StatusCode, nil
}

// isCFChallenge Cloudflare challenge 特征:403/503 + cf-ray/cf-mitigated 头或 challenge 页面标记。
func isCFChallenge(resp *http.Response, body []byte) bool {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusServiceUnavailable {
		return false
	}
	if resp.Header.Get("cf-mitigated") == "challenge" {
		return true
	}
	if resp.Header.Get("cf-ray") != "" {
		lower := bytes.ToLower(body)
		if bytes.Contains(lower, []byte("challenge")) || bytes.Contains(lower, []byte("cloudflare")) {
			return true
		}
	}
	return false
}

// newCookieJar 登录流程用(session cookie)。
func newCookieJar() http.CookieJar {
	jar, _ := cookiejar.New(nil)
	return jar
}
