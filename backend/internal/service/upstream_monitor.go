package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

const (
	refreshStaleAfter        = 5 * time.Minute // 抢占锁 stale(spec §7.1)
	unreachableFailThreshold = 3               // 连续失败阈值(spec §7.2)
	monitorTickInterval      = time.Minute
	monitorConcurrency       = 3
)

// notifyDispatcher 窄接口(NotifyDispatcher 满足)。
type notifyDispatcher interface {
	Dispatch(ctx context.Context, ev NotifyEvent)
}

// UpstreamMonitorService 刷新编排(spec §5.4/§7):
// 锁 → 采集(401 续期重试 1 次 / CF 过盾)→ diff → 事务提交 → 通知。
type UpstreamMonitorService struct {
	repo       UpstreamProviderRepository
	adapters   map[string]UpstreamAdapter
	dispatcher notifyDispatcher
	browser    BrowserSolver
	proxies    upstreamProxyResolver
}

// NewUpstreamMonitorService 构造 UpstreamMonitorService。
func NewUpstreamMonitorService(
	repo UpstreamProviderRepository,
	adapters map[string]UpstreamAdapter,
	dispatcher notifyDispatcher,
	browser BrowserSolver,
	proxies upstreamProxyResolver,
) *UpstreamMonitorService {
	return &UpstreamMonitorService{
		repo: repo, adapters: adapters,
		dispatcher: dispatcher, browser: browser, proxies: proxies,
	}
}

// RefreshProvider 手动(manual=true,绕过 disabled 不允许)与定时共用入口(spec §7)。
func (m *UpstreamMonitorService) RefreshProvider(ctx context.Context, id int64, manual bool) error {
	p, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.Status == domain.UpstreamStatusDisabled && !manual {
		return errors.New("上游已停用")
	}
	if p.Status == domain.UpstreamStatusDisabled && manual {
		return errors.New("上游已停用,请先启用")
	}
	now := time.Now().UTC()
	claim, ok, err := m.repo.ClaimRefresh(ctx, id, now, now.Add(-refreshStaleAfter))
	if err != nil {
		return err
	}
	if !ok {
		return ErrUpstreamRefreshInProgress
	}
	if m.proxies != nil && p.ProxyID != nil {
		if u, err := m.proxies.GetURL(ctx, *p.ProxyID); err == nil {
			p.ProxyURL = u
		}
	}

	snapshot, fetchErr := m.fetchWithRecovery(ctx, p)
	if fetchErr != nil {
		return m.commitFailure(ctx, p, claim, fetchErr)
	}
	return m.commitSuccess(ctx, p, claim, snapshot)
}

// fetchWithRecovery 凭证生命周期(spec §5.4):
// 401 → 有账密则 Login(纯 HTTP)→ 需浏览器则 BrowserSolver → 重试采集(最多 1 次);
// CF challenge → 过盾 → 重试。
func (m *UpstreamMonitorService) fetchWithRecovery(ctx context.Context, p *UpstreamProvider) (*domain.UpstreamSnapshot, error) {
	adapter, ok := m.adapters[p.Type]
	if !ok {
		return nil, fmt.Errorf("未知上游类型: %s", p.Type)
	}
	snap, err := adapter.FetchSnapshot(ctx, p)
	if err == nil {
		return snap, nil
	}

	switch {
	case errors.Is(err, errUpstreamCFChallenge):
		if m.browser == nil || !m.browser.Enabled(ctx) {
			return nil, fmt.Errorf("站点有 Cloudflare 盾且未配置 CloakBrowser: %w", err)
		}
		cfCreds, cfErr := m.browser.SolveCFClearance(ctx, p)
		if cfErr != nil {
			return nil, fmt.Errorf("过盾失败: %w", cfErr)
		}
		p.Credentials = MergeUpstreamCredentials(p.Credentials, cfCreds)
		if updateErr := m.repo.UpdateCredentials(ctx, p.ID, p.Credentials); updateErr != nil {
			return nil, updateErr
		}
		return adapter.FetchSnapshot(ctx, p) // 重试 1 次

	case errors.Is(err, errUpstreamAuth):
		username, _ := p.Credentials["username"].(string)
		password, _ := p.Credentials["password"].(string)
		if username == "" || password == "" {
			return nil, err // 无账密 → credential_error
		}
		newCreds, loginErr := adapter.Login(ctx, p, "")
		if loginErr != nil {
			// 需要 Turnstile / 被 CF 拦 → 浏览器自动登录(spec §5.4)
			if errors.Is(loginErr, errUpstreamTurnstile) || errors.Is(loginErr, errUpstreamCFChallenge) {
				if m.browser == nil || !m.browser.Enabled(ctx) {
					return nil, fmt.Errorf("登录需浏览器且未配置 CloakBrowser: %w", loginErr)
				}
				artifacts, bErr := m.browser.AutoLogin(ctx, p)
				if bErr != nil {
					return nil, fmt.Errorf("浏览器自动登录失败: %w", bErr)
				}
				newCreds = artifacts
			} else {
				return nil, loginErr // 2FA / 账密错误 → credential_error
			}
		}
		p.Credentials = MergeUpstreamCredentials(p.Credentials, newCreds)
		if updateErr := m.repo.UpdateCredentials(ctx, p.ID, p.Credentials); updateErr != nil {
			return nil, updateErr
		}
		return adapter.FetchSnapshot(ctx, p) // 续期重试上限 1 次(spec §5.4)

	default:
		return nil, err
	}
}

// commitSuccess diff → 事务提交 → 通知(spec §7.1/§7.2)。
func (m *UpstreamMonitorService) commitSuccess(ctx context.Context, p *UpstreamProvider, claim ClaimedRefresh, snapshot *domain.UpstreamSnapshot) error {
	diff := DiffUpstreamSnapshots(DiffInput{
		Old: p.LatestSnapshot, New: snapshot,
		BalanceThreshold: p.BalanceThreshold, BalanceAlerted: p.BalanceAlerted,
		NotifyOnPriceChange: p.NotifyOnPriceChange,
	})
	committed, inserted, err := m.repo.CommitRefresh(ctx, CommitRefreshInput{
		Claim: claim, Snapshot: snapshot,
		Status: domain.UpstreamStatusActive, LastError: "",
		ConsecFailures: 0, BalanceAlerted: diff.BalanceAlerted,
		Events: diff.Events, RefreshedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	if !committed {
		slog.Warn("upstream: 刷新结果被丢弃(锁已易主)", "provider_id", p.ID)
		return nil
	}
	m.notifyEvents(ctx, p, diff.Events, inserted)
	return nil
}

// commitFailure 失败路径:计数、状态机、按矩阵生成事件(spec §7.2)。
func (m *UpstreamMonitorService) commitFailure(ctx context.Context, p *UpstreamProvider, claim ClaimedRefresh, fetchErr error) error {
	failures := p.ConsecutiveFailures + 1
	status := p.Status
	var drafts []UpstreamChangeEventDraft

	isCredential := errors.Is(fetchErr, errUpstreamAuth) || errors.Is(fetchErr, errUpstream2FA) ||
		errors.Is(fetchErr, errUpstreamTurnstile) || errors.Is(fetchErr, ErrBrowserDisabled)
	if isCredential {
		if p.Status != domain.UpstreamStatusCredentialError { // 状态未变化不重复(spec §7.2)
			drafts = append(drafts, UpstreamChangeEventDraft{
				Type:    domain.UpstreamEventCredentialError,
				Summary: "凭证失效且自动续期失败",
				Detail:  map[string]any{"category": credentialErrorCategory(fetchErr)},
				Notify:  true,
			})
		}
		status = domain.UpstreamStatusCredentialError
	} else {
		if failures >= unreachableFailThreshold && p.Status != domain.UpstreamStatusUnreachable {
			drafts = append(drafts, UpstreamChangeEventDraft{
				Type:    domain.UpstreamEventRefreshFailed,
				Summary: fmt.Sprintf("连续 %d 次刷新失败", failures),
				Detail:  map[string]any{"failures": failures},
				Notify:  true,
			})
			status = domain.UpstreamStatusUnreachable
		} else if failures >= unreachableFailThreshold {
			status = domain.UpstreamStatusUnreachable
		}
		// failures < threshold: status 不变(保持 active 或上一状态)
	}

	lastError := sanitizeUpstreamError(fetchErr)
	committed, inserted, err := m.repo.CommitRefresh(ctx, CommitRefreshInput{
		Claim: claim, Snapshot: nil, // 失败不动快照
		Status: status, LastError: lastError,
		ConsecFailures: failures, BalanceAlerted: p.BalanceAlerted,
		Events: drafts, RefreshedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	if committed {
		m.notifyEvents(ctx, p, drafts, inserted)
	}
	return fmt.Errorf("刷新失败: %s", lastError)
}

// credentialErrorCategory 事件 detail 只存错误类别(spec §4.1.1:不存原文)。
func credentialErrorCategory(err error) string {
	switch {
	case errors.Is(err, errUpstream2FA):
		return "2fa_required"
	case errors.Is(err, errUpstreamTurnstile):
		return "turnstile_required"
	case errors.Is(err, ErrBrowserDisabled):
		return "browser_disabled"
	default:
		return "auth_failed"
	}
}

// sanitizeUpstreamError last_error 截断(错误链本身不含凭证——上游 HTTP 层已保证)。
func sanitizeUpstreamError(err error) string {
	msg := err.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
}

// notifyEvents 合并需通知的事件为一个 NotifyEvent(spec §7.2)并标记 notified。
func (m *UpstreamMonitorService) notifyEvents(ctx context.Context, p *UpstreamProvider, drafts []UpstreamChangeEventDraft, inserted []UpstreamChangeEvent) {
	var items []string
	var types []string
	var notifyIDs []int64
	for i, d := range drafts {
		if !d.Notify {
			continue
		}
		items = append(items, d.Summary)
		types = append(types, d.Type)
		if i < len(inserted) {
			notifyIDs = append(notifyIDs, inserted[i].ID)
		}
	}
	if len(items) == 0 {
		return
	}
	m.dispatcher.Dispatch(ctx, NotifyEvent{
		Source: domain.NotifyScopeUpstream, Severity: "warning",
		Title:   "上游告警:" + p.Name,
		Summary: fmt.Sprintf("%d 项变更需要关注", len(items)),
		Items:   items,
		Link:    fmt.Sprintf("/admin/upstream-providers?id=%d", p.ID),
		Detail:  map[string]any{"event_types": types, "provider_id": p.ID},
	})
	if err := m.repo.MarkEventsNotified(ctx, notifyIDs); err != nil {
		slog.Error("upstream: 标记事件已通知失败", "provider_id", p.ID, "error", err)
	}
}

// ---- 定时 runner(spec §7:每实例 ticker + DB 锁互斥,不做 leader)----

// UpstreamMonitorRunner 后台扫描(结构参照 channel_monitor_runner)。
type UpstreamMonitorRunner struct {
	svc    *UpstreamMonitorService
	repo   UpstreamProviderRepository
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewUpstreamMonitorRunner 构造 UpstreamMonitorRunner。
func NewUpstreamMonitorRunner(svc *UpstreamMonitorService, repo UpstreamProviderRepository) *UpstreamMonitorRunner {
	return &UpstreamMonitorRunner{svc: svc, repo: repo}
}

// Start 启动定时 ticker 轮询(进程生命周期内运行)。
// 注意:runner 随进程退出,刷新中断由抢占锁 5 分钟 stale 重抢兜底(R1.7)。
func (r *UpstreamMonitorRunner) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(monitorTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.tick(ctx)
			}
		}
	}()
	slog.Info("upstream: monitor runner started")
}

// Stop 等待 goroutine 退出。
func (r *UpstreamMonitorRunner) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

// tick 扫描到期 provider 并并发刷新(最大并发度 monitorConcurrency)。
func (r *UpstreamMonitorRunner) tick(ctx context.Context) {
	due, err := r.repo.ListDue(ctx, time.Now().UTC())
	if err != nil {
		slog.Error("upstream: 扫描到期上游失败", "error", err)
		return
	}
	if len(due) == 0 {
		return
	}
	sem := make(chan struct{}, monitorConcurrency)
	var wg sync.WaitGroup
	for _, id := range due {
		select {
		case <-ctx.Done():
			return
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(pid int64) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := r.svc.RefreshProvider(ctx, pid, false); err != nil &&
				!errors.Is(err, ErrUpstreamRefreshInProgress) {
				slog.Warn("upstream: 定时刷新失败", "provider_id", pid, "error", err)
			}
		}(id)
	}
	wg.Wait()
}

// ProvideUpstreamMonitor wire 组装并启动 runner(进程生命周期内单例)。
// 注意:runner 不注册 shutdown cleanup(R1.7),刷新中断由抢占锁 5min stale 重抢兜底。
func ProvideUpstreamMonitor(
	repo UpstreamProviderRepository,
	adapters map[string]UpstreamAdapter,
	dispatcher *NotifyDispatcher,
	browser BrowserSolver,
	proxyService *ProxyService,
) *UpstreamMonitorService {
	svc := NewUpstreamMonitorService(repo, adapters, dispatcher, browser, proxyService)
	runner := NewUpstreamMonitorRunner(svc, repo)
	runner.Start()
	return svc
}
