package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// UpstreamLinkedAccount 关联帐号视图(spec §9)。
type UpstreamLinkedAccount struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Status   string `json:"status"`
	BaseURL  string `json:"base_url"`
}

// UpstreamAccountLister 对账号体系的窄依赖(R2.3:导出接口):列出 type=upstream 的账号及其 base_url。
type UpstreamAccountLister interface {
	ListUpstreamTypeAccounts(ctx context.Context) ([]UpstreamLinkedAccount, error)
}

// UpstreamProviderService 上游站点管理(spec §9 除刷新外的全部端点)。
type UpstreamProviderService struct {
	repo     UpstreamProviderRepository
	events   UpstreamChangeEventRepository
	accounts UpstreamAccountLister
	settings upstreamSettingsReader
	adapters map[string]UpstreamAdapter
	dataDir  string
}

// NewUpstreamProviderService 构造 UpstreamProviderService。
func NewUpstreamProviderService(
	repo UpstreamProviderRepository,
	events UpstreamChangeEventRepository,
	accounts UpstreamAccountLister,
	settings upstreamSettingsReader,
	adapters map[string]UpstreamAdapter,
	dataDir string,
) *UpstreamProviderService {
	return &UpstreamProviderService{
		repo:     repo,
		events:   events,
		accounts: accounts,
		settings: settings,
		adapters: adapters,
		dataDir:  dataDir,
	}
}

// adapterFor 按 Type 路由适配器；未知 Type 报错。
func (s *UpstreamProviderService) adapterFor(p *UpstreamProvider) (UpstreamAdapter, error) {
	a, ok := s.adapters[p.Type]
	if !ok {
		return nil, fmt.Errorf("未知上游类型: %s", p.Type)
	}
	return a, nil
}

// resolveProxy 从「采集设置」加载全局代理 URL 进 p.ProxyURL(HTTP 采集用;浏览器过盾在 cdpURLFor 内读取)。
func (s *UpstreamProviderService) resolveProxy(ctx context.Context, p *UpstreamProvider) {
	if s.settings == nil {
		return
	}
	p.ProxyURL = s.settings.GetUpstreamManagementRuntime(ctx).ProxyURL
}

// validate 校验名称、类型、凭证（账密或 access_token 至少一组）。
func (s *UpstreamProviderService) validate(p *UpstreamProvider) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("名称不能为空")
	}
	if p.Type != domain.UpstreamTypeSub2API && p.Type != domain.UpstreamTypeNewAPI {
		// 允许测试用的自定义 type 直通（adapters map 会做最终校验）
		if _, ok := s.adapters[p.Type]; !ok {
			return fmt.Errorf("type 仅支持 sub2api | newapi，不支持: %s", p.Type)
		}
	}
	hasPassword := false
	if u, _ := p.Credentials["username"].(string); u != "" {
		if pw, _ := p.Credentials["password"].(string); pw != "" {
			hasPassword = true
		}
	}
	hasToken := false
	if t, _ := p.Credentials["access_token"].(string); t != "" {
		hasToken = true
	}
	if !hasPassword && !hasToken {
		return errors.New("账密与 access token 至少填一组")
	}
	return nil
}

// normalizeURLs 归一化 SiteURL（必填）和 APIBaseURL（可空）。
func (s *UpstreamProviderService) normalizeURLs(p *UpstreamProvider) error {
	site, err := NormalizeUpstreamURL(p.SiteURL)
	if err != nil {
		return fmt.Errorf("官网地址非法: %w", err)
	}
	p.SiteURL = site
	if strings.TrimSpace(p.APIBaseURL) != "" {
		api, err := NormalizeUpstreamURL(p.APIBaseURL)
		if err != nil {
			return fmt.Errorf("API 地址非法: %w", err)
		}
		p.APIBaseURL = api
	} else {
		p.APIBaseURL = ""
	}
	return nil
}

// ─────────────────────────────────── CRUD ───────────────────────────────────

// Create 校验 + 归一化 + 状态 active + interval 默认 60。
func (s *UpstreamProviderService) Create(ctx context.Context, p *UpstreamProvider) (*UpstreamProvider, error) {
	if err := s.validate(p); err != nil {
		return nil, err
	}
	if err := s.normalizeURLs(p); err != nil {
		return nil, err
	}
	p.Status = domain.UpstreamStatusActive
	if p.RefreshIntervalMinutes <= 0 {
		p.RefreshIntervalMinutes = 60
	}
	return s.repo.Create(ctx, p)
}

// Get 按 ID 查询。
func (s *UpstreamProviderService) Get(ctx context.Context, id int64) (*UpstreamProvider, error) {
	return s.repo.GetByID(ctx, id)
}

// List 返回全部上游站点。
func (s *UpstreamProviderService) List(ctx context.Context) ([]*UpstreamProvider, error) {
	return s.repo.List(ctx)
}

// Update 凭证按敏感键合并（spec §9: 凭证字段缺省 = 不修改）；
// status 由刷新流程管理（不被 PUT 覆盖）；interval<=0 用 existing。
func (s *UpstreamProviderService) Update(ctx context.Context, id int64, in *UpstreamProvider) (*UpstreamProvider, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	in.ID = id
	in.Credentials = MergeUpstreamCredentials(existing.Credentials, in.Credentials)
	if err := s.validate(in); err != nil {
		return nil, err
	}
	if err := s.normalizeURLs(in); err != nil {
		return nil, err
	}
	// 保护运行时字段：status 不被 PUT 覆盖
	in.Status = existing.Status
	if in.RefreshIntervalMinutes <= 0 {
		in.RefreshIntervalMinutes = existing.RefreshIntervalMinutes
	}
	return s.repo.Update(ctx, in)
}

// SetDisabled 手动启停（disabled ↔ active；spec §11 状态机的手动维度）。
func (s *UpstreamProviderService) SetDisabled(ctx context.Context, id int64, disabled bool) (*UpstreamProvider, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if disabled {
		existing.Status = domain.UpstreamStatusDisabled
	} else {
		existing.Status = domain.UpstreamStatusActive
	}
	return s.repo.Update(ctx, existing)
}

// Delete 删除 provider + 级联清理截图（spec §6）。
func (s *UpstreamProviderService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	RemoveDiagnostics(s.dataDir, id) // 级联清理截图（spec §6）
	return nil
}

// ────────────────────────────── 关联帐号 ──────────────────────────────

// LinkedAccounts 关联帐号：有效 API 地址按 §9 精确规则匹配。
func (s *UpstreamProviderService) LinkedAccounts(ctx context.Context, id int64) ([]UpstreamLinkedAccount, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	all, err := s.accounts.ListUpstreamTypeAccounts(ctx)
	if err != nil {
		return nil, err
	}
	effective := p.EffectiveAPIBaseURL()
	out := []UpstreamLinkedAccount{}
	for _, acc := range all {
		if UpstreamURLMatches(effective, acc.BaseURL) {
			out = append(out, acc)
		}
	}
	return out, nil
}

// ─────────────────────────────── Token ──────────────────────────────────

// ListTokens 透传适配器（spec §9）。
func (s *UpstreamProviderService) ListTokens(ctx context.Context, id int64) ([]UpstreamToken, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.resolveProxy(ctx, p)
	a, err := s.adapterFor(p)
	if err != nil {
		return nil, err
	}
	return a.ListTokens(ctx, p)
}

// CreateToken 透传适配器，返回 tok + EffectiveAPIBaseURL（spec §2 快速加号）。
func (s *UpstreamProviderService) CreateToken(ctx context.Context, id int64, req CreateUpstreamTokenRequest) (*UpstreamToken, string, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	s.resolveProxy(ctx, p)
	a, err := s.adapterFor(p)
	if err != nil {
		return nil, "", err
	}
	tok, err := a.CreateToken(ctx, p, req)
	if err != nil {
		return nil, "", err
	}
	// 返回有效 API 地址供前端一并展示（spec §2 快速加号）
	return tok, p.EffectiveAPIBaseURL(), nil
}

// ─────────────────────────── TestConnection ─────────────────────────────

// TestConnection 不落库验证（spec §9）：providerID>0 且凭证缺省时取存量凭证。
// R2.5: 仅账密无 access_token 时若 FetchSnapshot 返回 401（errUpstreamAuth），
// 自动调 Login 一次 → MergeUpstreamCredentials → 重试 FetchSnapshot 一次（结果不落库）。
func (s *UpstreamProviderService) TestConnection(ctx context.Context, in *UpstreamProvider, providerID int64) (*domain.UpstreamSnapshot, error) {
	if providerID > 0 {
		existing, err := s.repo.GetByID(ctx, providerID)
		if err != nil {
			return nil, err
		}
		in.Credentials = MergeUpstreamCredentials(existing.Credentials, in.Credentials)
	}
	if err := s.validate(in); err != nil {
		return nil, err
	}
	if err := s.normalizeURLs(in); err != nil {
		return nil, err
	}
	s.resolveProxy(ctx, in)
	a, err := s.adapterFor(in)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, upstreamSnapshotTimeout)
	defer cancel()

	snap, err := a.FetchSnapshot(ctx, in)
	if err != nil {
		// R2.5: 401 且凭证含 username+password → 登录续期再重试一次（不落库）
		if errors.Is(err, errUpstreamAuth) && s.hasUsernamePassword(in) {
			loginResult, loginErr := a.Login(ctx, in, "")
			if loginErr == nil && loginResult != nil {
				in.Credentials = MergeUpstreamCredentials(in.Credentials, loginResult)
				snap, err = a.FetchSnapshot(ctx, in)
			}
		}
	}
	return snap, err
}

// hasUsernamePassword 判断凭证是否同时含非空 username 和 password。
func (s *UpstreamProviderService) hasUsernamePassword(p *UpstreamProvider) bool {
	u, _ := p.Credentials["username"].(string)
	pw, _ := p.Credentials["password"].(string)
	return u != "" && pw != ""
}

// ─────────────────────────────── Events ──────────────────────────────────

// Events 变更事件游标分页。
func (s *UpstreamProviderService) Events(ctx context.Context, providerID int64, filter UpstreamChangeEventFilter) ([]UpstreamChangeEvent, error) {
	if _, err := s.repo.GetByID(ctx, providerID); err != nil {
		return nil, err
	}
	return s.events.ListByProvider(ctx, providerID, filter)
}

// ─────────────────────────────── ResetTokens ─────────────────────────────

// resetTokenKeys 剥离的运行时 token 凭证键（Task 16 Relogin 前调用）。
var resetTokenKeys = []string{"access_token", "cf_clearance", "cf_user_agent", "cf_expires_at", "token_expires_at"}

// ResetTokens 剥离 access_token/cf_*/token_expires_at 后整体回写（Task 16 Relogin 用）。
func (s *UpstreamProviderService) ResetTokens(ctx context.Context, id int64) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	// 建立剥离后的凭证 map（删除所有 resetTokenKeys）
	newCreds := make(map[string]any, len(existing.Credentials))
	for k, v := range existing.Credentials {
		newCreds[k] = v
	}
	for _, key := range resetTokenKeys {
		delete(newCreds, key)
	}
	existing.Credentials = newCreds
	_, err = s.repo.Update(ctx, existing)
	return err
}

// ───────────────────────────── wire providers ─────────────────────────────

// ProvideUpstreamAdapters 两种上游适配器（spec §14: 不做插件化，switch 即可）。
func ProvideUpstreamAdapters() map[string]UpstreamAdapter {
	return map[string]UpstreamAdapter{
		domain.UpstreamTypeNewAPI:  NewNewAPIAdapter(),
		domain.UpstreamTypeSub2API: NewSub2APIAdapter(),
	}
}

// ProvideUpstreamProviderService wire 组装（dataDir 取 cfg.Pricing.DataDir，与截图目录一致）。
func ProvideUpstreamProviderService(
	repo UpstreamProviderRepository,
	events UpstreamChangeEventRepository,
	accounts UpstreamAccountLister,
	settingService *SettingService,
	adapters map[string]UpstreamAdapter,
	cfg *config.Config,
) *UpstreamProviderService {
	return NewUpstreamProviderService(repo, events, accounts, settingService, adapters, cfg.Pricing.DataDir)
}
