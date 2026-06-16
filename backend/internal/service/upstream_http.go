package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
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
	// 基于 DefaultTransport 克隆而非零值 Transport:零值会丢失 TLS 握手超时(10s)、
	// dial 超时/keep-alive、空闲连接回收与环境代理(http_proxy/https_proxy)。
	// 其中"TLS 握手无超时"在出口拥挤/直连受限网络下会让单页请求挂满整个页超时,
	// 是日志翻页偶发卡死的主因;环境代理则保证本地开发走系统代理(生产无代理不受影响)。
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// 强制 HTTP/1.1:Go h2 客户端经代理/CF 偶发流挂死(项目内 GATEWAY_OPENAI_HTTP2
	// 已有同类 fallback 机制),低频轮询场景 h1 足够且行为可预期。
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	if p.ProxyURL != "" {
		// 统一经 proxyurl.Parse 校验(scheme 白名单 + socks5→socks5h 防 DNS 泄漏);
		// 额外把用户常写的 socks:// 当作 socks5:// 别名(见 normalizeProxyAlias)。
		_, proxyURL, err := proxyurl.Parse(normalizeProxyAlias(p.ProxyURL))
		if err != nil {
			return nil, fmt.Errorf("代理地址非法: %v", err) // proxyurl 错误已脱敏,不用 %w
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
