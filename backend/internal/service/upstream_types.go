package service

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// UpstreamProvider 上游站点业务对象(spec §4.1)。
type UpstreamProvider struct {
	ID                     int64
	Name                   string
	Type                   string // domain.UpstreamTypeSub2API | domain.UpstreamTypeNewAPI
	SiteURL                string
	APIBaseURL             string // 空 = 同 SiteURL
	Status                 string
	Credentials            map[string]any
	ProxyID                *int64
	ProxyURL               string // repo 加载时按 ProxyID 解析,不落库
	BalanceThreshold       *float64
	NotifyOnPriceChange    bool
	RefreshIntervalMinutes int
	LatestSnapshot         *domain.UpstreamSnapshot
	LastRefreshedAt        *time.Time
	LastError              string
	ConsecutiveFailures    int
	BalanceAlerted         bool
	RefreshStartedAt       *time.Time
	Remark                 string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// EffectiveAPIBaseURL 有效 API 地址(spec §2 术语):api_base_url 非空取之,否则 site_url。
func (p *UpstreamProvider) EffectiveAPIBaseURL() string {
	if p.APIBaseURL != "" {
		return p.APIBaseURL
	}
	return p.SiteURL
}

// UpstreamChangeEvent 变更事件业务对象(spec §4.2)。
type UpstreamChangeEvent struct {
	ID         int64
	ProviderID int64
	Type       string
	Summary    string
	Detail     map[string]any
	Notified   bool
	CreatedAt  time.Time
}

// UpstreamChangeEventDraft diff 产物,提交时落库。
type UpstreamChangeEventDraft struct {
	Type    string
	Summary string
	Detail  map[string]any
	// Notify 该事件是否需要发通知(按 spec §7.2 矩阵在 diff 时判定)
	Notify bool
}

// UpstreamToken 上游 token/key 统一表示(两种上游字段差异收敛到最小集 + Raw)。
type UpstreamToken struct {
	ID        any            `json:"id"`
	Name      string         `json:"name"`
	Key       string         `json:"key,omitempty"` // 仅创建响应含明文
	Group     string         `json:"group,omitempty"`
	ExpiresAt *time.Time     `json:"expires_at,omitempty"`
	Raw       map[string]any `json:"raw,omitempty"`
}

// CreateUpstreamTokenRequest 创建上游 token 请求(spec §9)。
type CreateUpstreamTokenRequest struct {
	Name  string `json:"name"`
	Group string `json:"group,omitempty"` // newapi 分组;sub2api 忽略
}

// UpstreamAdapter 上游适配器(spec §5.1)。
type UpstreamAdapter interface {
	// Login 用账密换长期凭证,返回需合并进 credentials 的键值。
	// turnstileToken 由浏览器层按需注入(无则空串)。
	Login(ctx context.Context, p *UpstreamProvider, turnstileToken string) (map[string]any, error)
	FetchSnapshot(ctx context.Context, p *UpstreamProvider) (*domain.UpstreamSnapshot, error)
	ListTokens(ctx context.Context, p *UpstreamProvider) ([]UpstreamToken, error)
	CreateToken(ctx context.Context, p *UpstreamProvider, req CreateUpstreamTokenRequest) (*UpstreamToken, error)
}

// BrowserSolver CloakBrowser 过盾/自动登录(spec §6)。chromedp 实现见 upstream_browser.go。
type BrowserSolver interface {
	Enabled(ctx context.Context) bool
	// SolveCFClearance 打开 site_url 等待 cf_clearance,返回需合并进 credentials 的键值
	// (cf_clearance / cf_user_agent / cf_expires_at)。
	SolveCFClearance(ctx context.Context, p *UpstreamProvider) (map[string]any, error)
	// AutoLogin 浏览器自动登录(humanize + Turnstile),返回会话产物。
	AutoLogin(ctx context.Context, p *UpstreamProvider) (map[string]any, error)
}

// ClaimedRefresh 抢占成功的刷新句柄(spec §7.1)。
type ClaimedRefresh struct {
	ProviderID int64
	ClaimedAt  time.Time
}

// CommitRefreshInput 一次刷新结果的原子提交(单事务:事件插入 + provider 更新,
// 条件 WHERE refresh_started_at = ClaimedAt,被抢走则整体丢弃返回 false)。
type CommitRefreshInput struct {
	Claim          ClaimedRefresh
	Snapshot       *domain.UpstreamSnapshot // nil = 失败路径不更新快照
	Status         string
	LastError      string
	ConsecFailures int
	BalanceAlerted bool
	Events         []UpstreamChangeEventDraft
	RefreshedAt    time.Time
}

// UpstreamProviderRepository 存储接口。
type UpstreamProviderRepository interface {
	Create(ctx context.Context, p *UpstreamProvider) (*UpstreamProvider, error)
	GetByID(ctx context.Context, id int64) (*UpstreamProvider, error)
	List(ctx context.Context) ([]*UpstreamProvider, error)
	Update(ctx context.Context, p *UpstreamProvider) (*UpstreamProvider, error)
	// Delete 事务内级联删除事件(诊断截图由 service 层清理)。
	Delete(ctx context.Context, id int64) error
	// ListDue 返回到期应刷新的 provider id(status != disabled 且到期)。
	ListDue(ctx context.Context, now time.Time) ([]int64, error)
	// ClaimRefresh 条件更新抢占刷新锁;ok=false 表示已有刷新进行中。
	ClaimRefresh(ctx context.Context, id int64, now time.Time, staleBefore time.Time) (ClaimedRefresh, bool, error)
	// CommitRefresh 原子提交;committed=false 表示锁已被他人抢走,结果整体丢弃。
	CommitRefresh(ctx context.Context, in CommitRefreshInput) (committed bool, insertedEvents []UpstreamChangeEvent, err error)
	// UpdateCredentials 仅更新凭证(登录续期后回写;不触碰刷新锁)。
	UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error
	// MarkEventsNotified 通知发送后标记。
	MarkEventsNotified(ctx context.Context, eventIDs []int64) error
}

// UpstreamChangeEventFilter 游标分页(按 created_at DESC, id DESC)。
type UpstreamChangeEventFilter struct {
	Limit           int
	BeforeCreatedAt *time.Time
	BeforeID        *int64
}

// UpstreamChangeEventRepository 事件存储接口。
type UpstreamChangeEventRepository interface {
	Insert(ctx context.Context, providerID int64, drafts []UpstreamChangeEventDraft) ([]UpstreamChangeEvent, error)
	ListByProvider(ctx context.Context, providerID int64, filter UpstreamChangeEventFilter) ([]UpstreamChangeEvent, error)
}

// 哨兵错误
var (
	ErrUpstreamProviderNotFound  = infraerrors.NotFound("UPSTREAM_PROVIDER_NOT_FOUND", "upstream provider not found")
	ErrUpstreamRefreshInProgress = errors.New("upstream refresh in progress")
	// errUpstreamAuth 适配器内部:token 失效(401/403 非 CF)
	errUpstreamAuth = errors.New("upstream auth failed")
	// errUpstreamCFChallenge 适配器内部:命中 Cloudflare challenge
	errUpstreamCFChallenge = errors.New("upstream cloudflare challenge")
	// errUpstream2FA sub2api 登录要求 2FA
	errUpstream2FA = errors.New("upstream requires 2fa")
	// errUpstreamTurnstile newapi 登录要求 Turnstile token
	errUpstreamTurnstile = errors.New("upstream requires turnstile")
)

// 确保包级 var 被引用(避免 lint 误报;实际在适配器实现中使用)。
// errUpstreamAuth/errUpstreamCFChallenge/errUpstreamTurnstile 已在适配器中真正使用,无需 lint 白名单。
var _ = errUpstream2FA
