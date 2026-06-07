package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// ──────────────────────────────────────────────────────────────
// fakeUpstreamProviderRepo: 内存版 UpstreamProviderRepository
// 未覆盖的方法统一 panic，以便测试发现意外调用。
// ──────────────────────────────────────────────────────────────

type fakeUpstreamProviderRepo struct {
	UpstreamProviderRepository // 嵌入接口,未覆盖方法 panic 即测试失败
	providers                  map[int64]*UpstreamProvider
	nextID                     int64
}

func newFakeUpstreamRepo() *fakeUpstreamProviderRepo {
	return &fakeUpstreamProviderRepo{providers: map[int64]*UpstreamProvider{}, nextID: 1}
}

func (f *fakeUpstreamProviderRepo) Create(ctx context.Context, p *UpstreamProvider) (*UpstreamProvider, error) {
	cp := *p
	cp.ID = f.nextID
	f.nextID++
	f.providers[cp.ID] = &cp
	return &cp, nil
}

func (f *fakeUpstreamProviderRepo) GetByID(ctx context.Context, id int64) (*UpstreamProvider, error) {
	p, ok := f.providers[id]
	if !ok {
		return nil, ErrUpstreamProviderNotFound
	}
	cp := *p
	return &cp, nil
}

func (f *fakeUpstreamProviderRepo) Update(ctx context.Context, p *UpstreamProvider) (*UpstreamProvider, error) {
	f.providers[p.ID] = p
	return p, nil
}

func (f *fakeUpstreamProviderRepo) Delete(ctx context.Context, id int64) error {
	if _, ok := f.providers[id]; !ok {
		return ErrUpstreamProviderNotFound
	}
	delete(f.providers, id)
	return nil
}

// ──────────────────────────────────────────────────────────────
// fakeAccountLister: 导出接口 UpstreamAccountLister 的内存实现
// ──────────────────────────────────────────────────────────────

type fakeAccountLister struct {
	accounts []UpstreamLinkedAccount
}

func (f *fakeAccountLister) ListUpstreamTypeAccounts(ctx context.Context) ([]UpstreamLinkedAccount, error) {
	return f.accounts, nil
}

// ──────────────────────────────────────────────────────────────
// fakeUpstreamAdapter: 用于 TestConnection R2.5 测试
// ──────────────────────────────────────────────────────────────

type fakeUpstreamAdapter struct {
	fetchSnapshotCalls int
	fetchSnapshotErrs  []error // 按调用顺序返回错误，耗尽后返回 nil
	loginResult        map[string]any
	loginErr           error
	snapshot           *domain.UpstreamSnapshot
}

func (a *fakeUpstreamAdapter) Login(ctx context.Context, p *UpstreamProvider, turnstileToken string) (map[string]any, error) {
	return a.loginResult, a.loginErr
}

func (a *fakeUpstreamAdapter) FetchSnapshot(ctx context.Context, p *UpstreamProvider) (*domain.UpstreamSnapshot, error) {
	idx := a.fetchSnapshotCalls
	a.fetchSnapshotCalls++
	if idx < len(a.fetchSnapshotErrs) {
		return nil, a.fetchSnapshotErrs[idx]
	}
	return a.snapshot, nil
}

func (a *fakeUpstreamAdapter) ListTokens(ctx context.Context, p *UpstreamProvider) ([]UpstreamToken, error) {
	return nil, nil
}

func (a *fakeUpstreamAdapter) CreateToken(ctx context.Context, p *UpstreamProvider, req CreateUpstreamTokenRequest) (*UpstreamToken, error) {
	return nil, nil
}

// ──────────────────────────────────────────────────────────────
// 辅助：构造测试用 UpstreamProviderService（使用默认适配器）
// ──────────────────────────────────────────────────────────────

func newTestProviderService(repo UpstreamProviderRepository, accounts UpstreamAccountLister) *UpstreamProviderService {
	return NewUpstreamProviderService(repo, nil, accounts, nil, map[string]UpstreamAdapter{
		domain.UpstreamTypeNewAPI:  NewNewAPIAdapter(),
		domain.UpstreamTypeSub2API: NewSub2APIAdapter(),
	}, "")
}

// ──────────────────────────────────────────────────────────────
// Test: 创建校验与 URL 归一化
// ──────────────────────────────────────────────────────────────

func TestProviderService_CreateValidatesAndNormalizes(t *testing.T) {
	repo := newFakeUpstreamRepo()
	svc := newTestProviderService(repo, &fakeAccountLister{})

	// 非法类型
	_, err := svc.Create(context.Background(), &UpstreamProvider{
		Name: "x", Type: "unknown", SiteURL: "https://a.com",
		Credentials: map[string]any{"access_token": "t"},
	})
	if err == nil {
		t.Fatal("非法类型应报错")
	}

	// 凭证两组都缺
	_, err = svc.Create(context.Background(), &UpstreamProvider{
		Name: "x", Type: domain.UpstreamTypeNewAPI, SiteURL: "https://a.com",
		Credentials: map[string]any{},
	})
	if err == nil {
		t.Fatal("账密与 token 至少一组")
	}

	// 正常创建：URL 归一化
	created, err := svc.Create(context.Background(), &UpstreamProvider{
		Name: "ok", Type: domain.UpstreamTypeNewAPI, SiteURL: "HTTPS://A.com/",
		APIBaseURL:  "https://api.a.com:443/v1/",
		Credentials: map[string]any{"access_token": "t", "user_id": float64(1)},
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if created.SiteURL != "https://a.com" || created.APIBaseURL != "https://api.a.com/v1" {
		t.Fatalf("URL 未归一化: %s / %s", created.SiteURL, created.APIBaseURL)
	}
	if created.Status != domain.UpstreamStatusActive {
		t.Fatalf("初始状态应 active: %s", created.Status)
	}
	if created.RefreshIntervalMinutes != 60 {
		t.Fatalf("默认 interval 应为 60，实际 %d", created.RefreshIntervalMinutes)
	}
}

// ──────────────────────────────────────────────────────────────
// Test: 更新时凭证合并（敏感键保留）
// ──────────────────────────────────────────────────────────────

func TestProviderService_UpdateMergesSensitiveCredentials(t *testing.T) {
	repo := newFakeUpstreamRepo()
	svc := newTestProviderService(repo, &fakeAccountLister{})
	created, _ := svc.Create(context.Background(), &UpstreamProvider{
		Name: "a", Type: domain.UpstreamTypeNewAPI, SiteURL: "https://a.com",
		Credentials: map[string]any{"access_token": "secret-1", "user_id": float64(1)},
	})

	// 前端 PUT：凭证只带非敏感键（脱敏回显后 spread）
	updated, err := svc.Update(context.Background(), created.ID, &UpstreamProvider{
		Name: "a2", Type: domain.UpstreamTypeNewAPI, SiteURL: "https://a.com",
		Credentials: map[string]any{"user_id": float64(2)},
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.Credentials["access_token"] != "secret-1" {
		t.Fatal("未提供的敏感键应保留")
	}
	if updated.Credentials["user_id"] != float64(2) {
		t.Fatal("非敏感键应覆盖")
	}
	if updated.Name != "a2" {
		t.Fatal("名称应更新")
	}
}

// ──────────────────────────────────────────────────────────────
// Test: 关联帐号精确匹配（含安全域名过滤）
// ──────────────────────────────────────────────────────────────

func TestProviderService_LinkedAccounts(t *testing.T) {
	repo := newFakeUpstreamRepo()
	accounts := &fakeAccountLister{accounts: []UpstreamLinkedAccount{
		{ID: 1, Name: "acc-match", Platform: "anthropic", BaseURL: "https://a.com/v1"},
		{ID: 2, Name: "acc-other", Platform: "openai", BaseURL: "https://other.com"},
		{ID: 3, Name: "acc-evil", Platform: "openai", BaseURL: "https://a.com.evil/v1"},
	}}
	svc := newTestProviderService(repo, accounts)
	created, _ := svc.Create(context.Background(), &UpstreamProvider{
		Name: "a", Type: domain.UpstreamTypeNewAPI, SiteURL: "https://a.com",
		Credentials: map[string]any{"access_token": "t"},
	})

	linked, err := svc.LinkedAccounts(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(linked) != 1 || linked[0].ID != 1 {
		t.Fatalf("应只匹配 acc-match: %+v", linked)
	}
}

// ──────────────────────────────────────────────────────────────
// Test: adapter 路由（未知类型报错）
// ──────────────────────────────────────────────────────────────

func TestProviderService_AdapterRouting(t *testing.T) {
	svc := newTestProviderService(newFakeUpstreamRepo(), &fakeAccountLister{})
	if _, err := svc.adapterFor(&UpstreamProvider{Type: "unknown"}); err == nil {
		t.Fatal("未知类型应报错")
	}
	if a, _ := svc.adapterFor(&UpstreamProvider{Type: domain.UpstreamTypeNewAPI}); a == nil {
		t.Fatal("newapi 应有适配器")
	}
}

// ──────────────────────────────────────────────────────────────
// Test R2.5: TestConnection 登录续期重试（仅账密无 access_token 时 401 自动 Login 再试）
// ──────────────────────────────────────────────────────────────

func TestProviderService_TestConnectionWithLoginRetry(t *testing.T) {
	fakeAdapter := &fakeUpstreamAdapter{
		// 首次 FetchSnapshot 返回 errUpstreamAuth（模拟 401）
		fetchSnapshotErrs: []error{errUpstreamAuth},
		// Login 成功，返回新 token
		loginResult: map[string]any{"access_token": "new-token"},
		// 二次 FetchSnapshot 成功
		snapshot: func() *domain.UpstreamSnapshot {
			b := float64(100.0)
			return &domain.UpstreamSnapshot{Balance: &b}
		}(),
	}

	svc := NewUpstreamProviderService(
		newFakeUpstreamRepo(), nil, &fakeAccountLister{}, nil,
		map[string]UpstreamAdapter{
			"testtype": fakeAdapter,
		},
		"",
	)

	// 仅账密无 access_token 的 provider
	in := &UpstreamProvider{
		Name: "test", Type: "testtype", SiteURL: "https://a.com",
		Credentials: map[string]any{
			"username": "admin",
			"password": "pass",
		},
	}

	snap, err := svc.TestConnection(context.Background(), in, 0)
	if err != nil {
		t.Fatalf("续期后应成功: %v", err)
	}
	if snap == nil || snap.Balance == nil || *snap.Balance != 100.0 {
		t.Fatalf("快照应有余额: %+v", snap)
	}
	if fakeAdapter.fetchSnapshotCalls != 2 {
		t.Fatalf("应调用 FetchSnapshot 2次，实际 %d", fakeAdapter.fetchSnapshotCalls)
	}
	// 确认 access_token 被合并进凭证（续期后 in.Credentials 含新 token）
	if tok, _ := in.Credentials["access_token"].(string); tok != "new-token" {
		t.Fatalf("续期后凭证应含新 token, 实际: %v", in.Credentials["access_token"])
	}
}

// ──────────────────────────────────────────────────────────────
// Test: TestConnection 无账密且 401 时不续期直接报错
// ──────────────────────────────────────────────────────────────

func TestProviderService_TestConnectionNoRetryWithoutPassword(t *testing.T) {
	fakeAdapter := &fakeUpstreamAdapter{
		// 首次 FetchSnapshot 返回 errUpstreamAuth
		fetchSnapshotErrs: []error{errUpstreamAuth},
		snapshot:          &domain.UpstreamSnapshot{}, // 无账密时不会到二次调用
	}

	svc := NewUpstreamProviderService(
		newFakeUpstreamRepo(), nil, &fakeAccountLister{}, nil,
		map[string]UpstreamAdapter{
			"testtype": fakeAdapter,
		},
		"",
	)

	// 只有 access_token，无账密 → 401 时不重试
	in := &UpstreamProvider{
		Name: "test", Type: "testtype", SiteURL: "https://a.com",
		Credentials: map[string]any{
			"access_token": "expired-token",
		},
	}

	_, err := svc.TestConnection(context.Background(), in, 0)
	if err == nil {
		t.Fatal("没有账密时 401 应直接报错")
	}
	if !errors.Is(err, errUpstreamAuth) {
		t.Fatalf("错误应为 errUpstreamAuth，实际: %v", err)
	}
	if fakeAdapter.fetchSnapshotCalls != 1 {
		t.Fatalf("无账密时只应调用 FetchSnapshot 1次，实际 %d", fakeAdapter.fetchSnapshotCalls)
	}
}

// ──────────────────────────────────────────────────────────────
// Test: ResetTokens 剥离运行时 token 凭证
// ──────────────────────────────────────────────────────────────

func TestProviderService_ResetTokens(t *testing.T) {
	repo := newFakeUpstreamRepo()
	svc := newTestProviderService(repo, &fakeAccountLister{})

	created, _ := svc.Create(context.Background(), &UpstreamProvider{
		Name: "r", Type: domain.UpstreamTypeNewAPI, SiteURL: "https://a.com",
		Credentials: map[string]any{
			"username":         "admin",
			"password":         "pass",
			"access_token":     "tok123",
			"cf_clearance":     "cf_val",
			"cf_user_agent":    "ua",
			"cf_expires_at":    "2026-07-01T00:00:00Z",
			"token_expires_at": "2024-01-01",
		},
	})

	err := svc.ResetTokens(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("ResetTokens 失败: %v", err)
	}

	updated, _ := repo.GetByID(context.Background(), created.ID)
	if _, ok := updated.Credentials["access_token"]; ok {
		t.Fatal("access_token 应被删除")
	}
	if _, ok := updated.Credentials["cf_clearance"]; ok {
		t.Fatal("cf_clearance 应被删除")
	}
	if _, ok := updated.Credentials["cf_user_agent"]; ok {
		t.Fatal("cf_user_agent 应被删除")
	}
	if _, ok := updated.Credentials["cf_expires_at"]; ok {
		t.Fatal("cf_expires_at 应被剥离")
	}
	if _, ok := updated.Credentials["token_expires_at"]; ok {
		t.Fatal("token_expires_at 应被删除")
	}
	// 账密应保留
	if updated.Credentials["username"] != "admin" {
		t.Fatal("username 应保留")
	}
	if updated.Credentials["password"] != "pass" {
		t.Fatal("password 应保留")
	}
}
