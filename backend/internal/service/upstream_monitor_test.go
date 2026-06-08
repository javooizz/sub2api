package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// fakeMonitorRepo 实现刷新编排所需的 repo 方法。
type fakeMonitorRepo struct {
	UpstreamProviderRepository
	provider     *UpstreamProvider
	claimOK      bool
	committed    *CommitRefreshInput
	commitResult bool
	credsUpdated map[string]any
}

func (f *fakeMonitorRepo) GetByID(ctx context.Context, id int64) (*UpstreamProvider, error) {
	cp := *f.provider
	return &cp, nil
}
func (f *fakeMonitorRepo) ClaimRefresh(ctx context.Context, id int64, now, staleBefore time.Time) (ClaimedRefresh, bool, error) {
	if !f.claimOK {
		return ClaimedRefresh{}, false, nil
	}
	return ClaimedRefresh{ProviderID: id, ClaimedAt: now}, true, nil
}
func (f *fakeMonitorRepo) CommitRefresh(ctx context.Context, in CommitRefreshInput) (bool, []UpstreamChangeEvent, error) {
	f.committed = &in
	events := make([]UpstreamChangeEvent, 0, len(in.Events))
	for i, d := range in.Events {
		events = append(events, UpstreamChangeEvent{ID: int64(i + 1), Type: d.Type, Summary: d.Summary})
	}
	return f.commitResult, events, nil
}
func (f *fakeMonitorRepo) UpdateCredentials(ctx context.Context, id int64, creds map[string]any) error {
	f.credsUpdated = creds
	return nil
}
func (f *fakeMonitorRepo) MarkEventsNotified(ctx context.Context, ids []int64) error { return nil }

// monFakeAdapter 可编程行为(加 mon 前缀避免与其他 test 文件冲突)。
type monFakeAdapter struct {
	snapshots   []func() (*domain.UpstreamSnapshot, error) // 依次出队
	loginResult map[string]any
	loginErr    error
	loginCalls  int
}

func (f *monFakeAdapter) Login(ctx context.Context, p *UpstreamProvider, turnstile string) (map[string]any, error) {
	f.loginCalls++
	return f.loginResult, f.loginErr
}
func (f *monFakeAdapter) FetchSnapshot(ctx context.Context, p *UpstreamProvider) (*domain.UpstreamSnapshot, error) {
	if len(f.snapshots) == 0 {
		return nil, errors.New("no more snapshots")
	}
	fn := f.snapshots[0]
	f.snapshots = f.snapshots[1:]
	return fn()
}
func (f *monFakeAdapter) ListTokens(ctx context.Context, p *UpstreamProvider) ([]UpstreamToken, error) {
	return nil, nil
}
func (f *monFakeAdapter) CreateToken(ctx context.Context, p *UpstreamProvider, req CreateUpstreamTokenRequest) (*UpstreamToken, error) {
	return nil, nil
}

type monFakeDispatcher struct{ events []NotifyEvent }

func (f *monFakeDispatcher) Dispatch(ctx context.Context, ev NotifyEvent) {
	f.events = append(f.events, ev)
}

type monFakeBrowser struct{ enabled bool }

func (f *monFakeBrowser) Enabled(ctx context.Context) bool { return f.enabled }
func (f *monFakeBrowser) SolveCFClearance(ctx context.Context, p *UpstreamProvider) (map[string]any, error) {
	return map[string]any{"cf_clearance": "cf-new"}, nil
}
func (f *monFakeBrowser) AutoLogin(ctx context.Context, p *UpstreamProvider) (map[string]any, error) {
	return map[string]any{"access_token": "browser-token"}, nil
}

func baseProvider() *UpstreamProvider {
	return &UpstreamProvider{
		ID: 1, Name: "demo", Type: domain.UpstreamTypeNewAPI,
		SiteURL: "https://a.com", Status: domain.UpstreamStatusActive,
		Credentials:            map[string]any{"access_token": "tok", "username": "u", "password": "p"},
		RefreshIntervalMinutes: 60,
		NotifyOnPriceChange:    true,
	}
}

func monSnapOK() *domain.UpstreamSnapshot {
	b := 10.0
	return &domain.UpstreamSnapshot{Balance: &b, Currency: "USD"}
}

func newMonitor(repo UpstreamProviderRepository, adapter UpstreamAdapter, d notifyDispatcher, b BrowserSolver) *UpstreamMonitorService {
	return NewUpstreamMonitorService(repo, map[string]UpstreamAdapter{domain.UpstreamTypeNewAPI: adapter}, d, b, nil)
}

func TestMonitor_SuccessfulRefreshCommitsAndNotifies(t *testing.T) {
	repo := &fakeMonitorRepo{provider: baseProvider(), claimOK: true, commitResult: true}
	// 旧快照有分组,新快照倍率变化 → price_changed 通知
	old := &domain.UpstreamSnapshot{Groups: []domain.UpstreamGroupSnapshot{{Name: "g", Ratio: f64(1.0)}}}
	repo.provider.LatestSnapshot = old
	newSnap := &domain.UpstreamSnapshot{Groups: []domain.UpstreamGroupSnapshot{{Name: "g", Ratio: f64(2.0)}}}
	adapter := &monFakeAdapter{snapshots: []func() (*domain.UpstreamSnapshot, error){
		func() (*domain.UpstreamSnapshot, error) { return newSnap, nil },
	}}
	dispatcher := &monFakeDispatcher{}
	m := newMonitor(repo, adapter, dispatcher, &monFakeBrowser{})

	if err := m.RefreshProvider(context.Background(), 1, true); err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
	if repo.committed == nil || repo.committed.Status != domain.UpstreamStatusActive {
		t.Fatalf("应提交 active 状态: %+v", repo.committed)
	}
	if repo.committed.ConsecFailures != 0 {
		t.Fatal("成功应清零失败计数")
	}
	if len(repo.committed.Events) != 1 || repo.committed.Events[0].Type != domain.UpstreamEventPriceChanged {
		t.Fatalf("应有 price_changed 事件: %+v", repo.committed.Events)
	}
	if len(dispatcher.events) != 1 {
		t.Fatalf("应发一次合并通知: %d", len(dispatcher.events))
	}
}

func TestMonitor_ClaimFailReturnsInProgress(t *testing.T) {
	repo := &fakeMonitorRepo{provider: baseProvider(), claimOK: false}
	m := newMonitor(repo, &monFakeAdapter{}, &monFakeDispatcher{}, &monFakeBrowser{})
	err := m.RefreshProvider(context.Background(), 1, true)
	if !errors.Is(err, ErrUpstreamRefreshInProgress) {
		t.Fatalf("抢锁失败应返回 ErrUpstreamRefreshInProgress: %v", err)
	}
}

func TestMonitor_AuthFailureRetriesLoginOnce(t *testing.T) {
	repo := &fakeMonitorRepo{provider: baseProvider(), claimOK: true, commitResult: true}
	adapter := &monFakeAdapter{
		snapshots: []func() (*domain.UpstreamSnapshot, error){
			func() (*domain.UpstreamSnapshot, error) { return nil, errUpstreamAuth }, // 第一次 401
			func() (*domain.UpstreamSnapshot, error) { return monSnapOK(), nil },     // 续期后成功
		},
		loginResult: map[string]any{"access_token": "fresh"},
	}
	m := newMonitor(repo, adapter, &monFakeDispatcher{}, &monFakeBrowser{})

	if err := m.RefreshProvider(context.Background(), 1, false); err != nil {
		t.Fatalf("续期后应成功: %v", err)
	}
	if adapter.loginCalls != 1 {
		t.Fatalf("应恰好登录一次: %d", adapter.loginCalls)
	}
	if repo.credsUpdated["access_token"] != "fresh" {
		t.Fatal("新凭证应回写")
	}
	// 根因守护:续期产物 {access_token} 不含 username,合并回写不得把身份键 username 抹掉,
	// 否则下一次续期会因缺 username 而永久 credential_error。
	if repo.credsUpdated["username"] != "u" {
		t.Fatalf("续期回写不得丢失 username,实得: %v", repo.credsUpdated["username"])
	}
	if repo.committed.Status != domain.UpstreamStatusActive {
		t.Fatal("最终状态应 active")
	}
}

func TestMonitor_AuthFailureNoPasswordIsCredentialError(t *testing.T) {
	p := baseProvider()
	delete(p.Credentials, "username")
	delete(p.Credentials, "password")
	repo := &fakeMonitorRepo{provider: p, claimOK: true, commitResult: true}
	adapter := &monFakeAdapter{snapshots: []func() (*domain.UpstreamSnapshot, error){
		func() (*domain.UpstreamSnapshot, error) { return nil, errUpstreamAuth },
	}}
	dispatcher := &monFakeDispatcher{}
	m := newMonitor(repo, adapter, dispatcher, &monFakeBrowser{})

	_ = m.RefreshProvider(context.Background(), 1, false)
	if repo.committed.Status != domain.UpstreamStatusCredentialError {
		t.Fatalf("无账密续期应 credential_error: %s", repo.committed.Status)
	}
	if len(repo.committed.Events) != 1 || repo.committed.Events[0].Type != domain.UpstreamEventCredentialError {
		t.Fatalf("应有 credential_error 事件: %+v", repo.committed.Events)
	}
	if len(dispatcher.events) != 1 {
		t.Fatal("凭证失效应通知")
	}
}

func TestMonitor_ConsecutiveFailuresToUnreachable(t *testing.T) {
	p := baseProvider()
	p.ConsecutiveFailures = 2 // 本次失败 → 3 → unreachable + 通知一次
	repo := &fakeMonitorRepo{provider: p, claimOK: true, commitResult: true}
	adapter := &monFakeAdapter{snapshots: []func() (*domain.UpstreamSnapshot, error){
		func() (*domain.UpstreamSnapshot, error) { return nil, errors.New("conn refused") },
	}}
	dispatcher := &monFakeDispatcher{}
	m := newMonitor(repo, adapter, dispatcher, &monFakeBrowser{})

	_ = m.RefreshProvider(context.Background(), 1, false)
	if repo.committed.Status != domain.UpstreamStatusUnreachable {
		t.Fatalf("连续 3 次失败应 unreachable: %s", repo.committed.Status)
	}
	if repo.committed.ConsecFailures != 3 {
		t.Fatalf("失败计数应为 3: %d", repo.committed.ConsecFailures)
	}
	if len(repo.committed.Events) != 1 || repo.committed.Events[0].Type != domain.UpstreamEventRefreshFailed {
		t.Fatalf("应有 refresh_failed 事件: %+v", repo.committed.Events)
	}
	if len(dispatcher.events) != 1 {
		t.Fatal("跨入 unreachable 应通知一次")
	}
}

func TestMonitor_FailureBelowThresholdNoEvent(t *testing.T) {
	p := baseProvider() // ConsecutiveFailures=0 → 本次 1 < 3
	repo := &fakeMonitorRepo{provider: p, claimOK: true, commitResult: true}
	adapter := &monFakeAdapter{snapshots: []func() (*domain.UpstreamSnapshot, error){
		func() (*domain.UpstreamSnapshot, error) { return nil, errors.New("conn refused") },
	}}
	dispatcher := &monFakeDispatcher{}
	m := newMonitor(repo, adapter, dispatcher, &monFakeBrowser{})

	_ = m.RefreshProvider(context.Background(), 1, false)
	if repo.committed.Status != domain.UpstreamStatusActive {
		t.Fatalf("未达阈值应保持 active: %s", repo.committed.Status)
	}
	if len(repo.committed.Events) != 0 || len(dispatcher.events) != 0 {
		t.Fatal("未达阈值不应有事件与通知")
	}
}

func TestMonitor_DisabledSkipped(t *testing.T) {
	p := baseProvider()
	p.Status = domain.UpstreamStatusDisabled
	repo := &fakeMonitorRepo{provider: p, claimOK: true}
	m := newMonitor(repo, &monFakeAdapter{}, &monFakeDispatcher{}, &monFakeBrowser{})
	err := m.RefreshProvider(context.Background(), 1, false)
	if err == nil {
		t.Fatal("disabled 非手动应拒绝")
	}
}
