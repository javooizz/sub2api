package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// ErrBrowserDisabled 未配置 CloakBrowser CDP 地址(spec §6:降级、人工兜底)。
var ErrBrowserDisabled = errors.New("CloakBrowser 未配置")

const (
	browserCFTimeout    = 60 * time.Second // 过盾上限(spec §6)
	browserLoginTimeout = 90 * time.Second
	diagnosticsKeep     = 5 // 每 provider 滚动保留(spec §6)
)

// upstreamSettingsReader BrowserSolver 对设置的窄依赖。
type upstreamSettingsReader interface {
	GetUpstreamManagementRuntime(ctx context.Context) UpstreamManagementRuntime
}

// ChromedpBrowserSolver 经 CDP 控制 CloakBrowser(spec §6)。
// 每个任务开独立 browser context;指纹种子按 provider id 派生(CDP query 参数,
// CloakBrowser cloakserve 支持 per-connection seed),站点眼中身份稳定。
type ChromedpBrowserSolver struct {
	settings upstreamSettingsReader
	dataDir  string // 截图根目录(config DataDir)
}

// NewChromedpBrowserSolver 构造 ChromedpBrowserSolver。
func NewChromedpBrowserSolver(settings upstreamSettingsReader, dataDir string) *ChromedpBrowserSolver {
	return &ChromedpBrowserSolver{settings: settings, dataDir: dataDir}
}

// Enabled 报告是否配置了 CDP URL。
func (s *ChromedpBrowserSolver) Enabled(ctx context.Context) bool {
	return s.settings.GetUpstreamManagementRuntime(ctx).BrowserCDPURL != ""
}

// cdpURLFor 注入 per-provider 指纹种子与代理(CloakBrowser CDP query 参数约定)。
func (s *ChromedpBrowserSolver) cdpURLFor(ctx context.Context, p *UpstreamProvider) string {
	base := s.settings.GetUpstreamManagementRuntime(ctx).BrowserCDPURL
	sep := "?"
	for i := 0; i < len(base); i++ {
		if base[i] == '?' {
			sep = "&"
			break
		}
	}
	u := base + sep + "fingerprint-seed=upstream-" + strconv.FormatInt(p.ID, 10)
	if p.ProxyURL != "" {
		u += "&proxy=" + p.ProxyURL
	}
	return u
}

// SolveCFClearance 打开 site_url 轮询等待 cf_clearance(spec §6 任务 1)。
func (s *ChromedpBrowserSolver) SolveCFClearance(ctx context.Context, p *UpstreamProvider) (map[string]any, error) {
	if !s.Enabled(ctx) {
		return nil, ErrBrowserDisabled
	}
	ctx, cancel := context.WithTimeout(ctx, browserCFTimeout)
	defer cancel()
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, s.cdpURLFor(ctx, p))
	defer cancelAlloc()
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	if err := chromedp.Run(taskCtx, chromedp.Navigate(p.SiteURL)); err != nil {
		return nil, s.failWithScreenshot(taskCtx, p, fmt.Errorf("打开站点失败: %w", err))
	}
	var ua string
	_ = chromedp.Run(taskCtx, chromedp.Evaluate("navigator.userAgent", &ua))

	deadline := time.Now().Add(browserCFTimeout)
	for time.Now().Before(deadline) {
		if cf := readCookie(taskCtx, "cf_clearance"); cf != "" {
			return map[string]any{
				"cf_clearance":  cf,
				"cf_user_agent": ua,
				// cf_clearance 实际有效期由 CF 决定,这里记录保守的 30 分钟提示值
				"cf_expires_at": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
			}, nil
		}
		select {
		case <-taskCtx.Done():
			return nil, s.failWithScreenshot(taskCtx, p, errors.New("等待 cf_clearance 超时"))
		case <-time.After(2 * time.Second):
		}
	}
	return nil, s.failWithScreenshot(taskCtx, p, errors.New("等待 cf_clearance 超时"))
}

// AutoLogin 浏览器自动登录(spec §6 任务 2):填账密 → 等 Turnstile → 提交 → 提取产物。
// 选择器为上游默认前端形状,上游改版时调整常量即可。
func (s *ChromedpBrowserSolver) AutoLogin(ctx context.Context, p *UpstreamProvider) (map[string]any, error) {
	if !s.Enabled(ctx) {
		return nil, ErrBrowserDisabled
	}
	username, _ := p.Credentials["username"].(string)
	password, _ := p.Credentials["password"].(string)
	if username == "" || password == "" {
		return nil, errors.New("缺少账密,无法自动登录")
	}
	ctx, cancel := context.WithTimeout(ctx, browserLoginTimeout)
	defer cancel()
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, s.cdpURLFor(ctx, p))
	defer cancelAlloc()
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	loginURL := p.SiteURL + "/login"
	sel := loginSelectorsFor(p.Type)
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(loginURL),
		chromedp.WaitVisible(sel.username, chromedp.ByQuery),
		chromedp.SendKeys(sel.username, username, chromedp.ByQuery),
		chromedp.SendKeys(sel.password, password, chromedp.ByQuery),
	)
	if err != nil {
		return nil, s.failWithScreenshot(taskCtx, p, fmt.Errorf("登录表单交互失败: %w", err))
	}
	// CloakBrowser 自动过 Turnstile:轮询等待 widget 完成(token 注入隐藏 input)
	waitTurnstile(taskCtx, 30*time.Second)
	if err := chromedp.Run(taskCtx, chromedp.Click(sel.submit, chromedp.ByQuery)); err != nil {
		return nil, s.failWithScreenshot(taskCtx, p, fmt.Errorf("提交登录失败: %w", err))
	}

	// 等待登录态产物:sub2api → localStorage JWT;newapi → session cookie
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		switch p.Type {
		case "sub2api":
			var token string
			_ = chromedp.Run(taskCtx, chromedp.Evaluate(
				`localStorage.getItem('token') || ''`, &token))
			if token != "" {
				return map[string]any{"access_token": token}, nil
			}
		case "newapi":
			if sess := readCookie(taskCtx, "session"); sess != "" {
				return map[string]any{"session_cookie": sess}, nil
			}
		}
		select {
		case <-taskCtx.Done():
			return nil, s.failWithScreenshot(taskCtx, p, errors.New("等待登录态超时"))
		case <-time.After(1 * time.Second):
		}
	}
	return nil, s.failWithScreenshot(taskCtx, p, errors.New("等待登录态超时"))
}

type loginSelectors struct{ username, password, submit string }

func loginSelectorsFor(providerType string) loginSelectors {
	if providerType == "newapi" {
		return loginSelectors{
			username: `input[name="username"]`,
			password: `input[name="password"]`,
			submit:   `button[type="submit"]`,
		}
	}
	// sub2api 登录页
	return loginSelectors{
		username: `input[type="email"], input[name="email"]`,
		password: `input[type="password"]`,
		submit:   `button[type="submit"]`,
	}
}

// waitTurnstile 轮询 Turnstile token(无 widget 时立即返回)。
func waitTurnstile(ctx context.Context, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var state string
		_ = chromedp.Run(ctx, chromedp.Evaluate(`(() => {
			const el = document.querySelector('input[name="cf-turnstile-response"]');
			if (!el) return 'absent';
			return el.value ? 'done' : 'pending';
		})()`, &state))
		if state == "absent" || state == "done" {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

// readCookie 经 CDP 读取指定 cookie。
func readCookie(ctx context.Context, name string) string {
	var value string
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := chromedpCookies(ctx)
		if err != nil {
			return err
		}
		for _, c := range cookies {
			if c.Name == name {
				value = c.Value
				return nil
			}
		}
		return nil
	}))
	return value
}

// failWithScreenshot 截图存 data 目录(spec §6 失败诊断),返回包装错误。
// 错误信息不含凭证与页面内容,仅含截图相对文件名。
func (s *ChromedpBrowserSolver) failWithScreenshot(ctx context.Context, p *UpstreamProvider, cause error) error {
	var shot []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&shot)); err != nil || len(shot) == 0 {
		return cause
	}
	dir := filepath.Join(s.dataDir, "upstream-diagnostics", strconv.FormatInt(p.ID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return cause
	}
	name := time.Now().UTC().Format("20060102150405") + ".png"
	if err := os.WriteFile(filepath.Join(dir, name), shot, 0o644); err != nil {
		return cause
	}
	if err := pruneDiagnostics(s.dataDir, p.ID, diagnosticsKeep); err != nil {
		slog.Warn("upstream: 清理诊断截图失败", "provider_id", p.ID, "error", err)
	}
	return fmt.Errorf("%w(诊断截图: %s)", cause, name)
}

// pruneDiagnostics 按文件名(时间戳)倒序保留最新 keep 张。
func pruneDiagnostics(dataDir string, providerID int64, keep int) error {
	dir := filepath.Join(dataDir, "upstream-diagnostics", strconv.FormatInt(providerID, 10))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) <= keep {
		return nil
	}
	sort.Strings(names) // 时间戳命名,字典序=时间序
	for _, name := range names[:len(names)-keep] {
		_ = os.Remove(filepath.Join(dir, name))
	}
	return nil
}

// RemoveDiagnostics 删除 provider 的全部诊断截图(删除 provider 时调用,spec §6)。
func RemoveDiagnostics(dataDir string, providerID int64) {
	_ = os.RemoveAll(filepath.Join(dataDir, "upstream-diagnostics", strconv.FormatInt(providerID, 10)))
}

// chromedpCookies 经 CDP 获取当前页面所有 cookie 的薄封装。
func chromedpCookies(ctx context.Context) ([]*network.Cookie, error) {
	return network.GetCookies().Do(ctx)
}
