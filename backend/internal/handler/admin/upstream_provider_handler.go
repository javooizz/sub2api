package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UpstreamProviderHandler 上游站点管理 API(spec §9)。
type UpstreamProviderHandler struct {
	svc            *service.UpstreamProviderService
	monitor        *service.UpstreamMonitorService
	settingService *service.SettingService
	dataDir        string
	usage          *service.UpstreamUsageService
	collector      *service.UpstreamUsageCollector
}

// NewUpstreamProviderHandler 构造函数，wire 注入。
func NewUpstreamProviderHandler(
	svc *service.UpstreamProviderService,
	monitor *service.UpstreamMonitorService,
	settingService *service.SettingService,
	cfg *config.Config,
	usage *service.UpstreamUsageService,
	collector *service.UpstreamUsageCollector,
) *UpstreamProviderHandler {
	return &UpstreamProviderHandler{
		svc:            svc,
		monitor:        monitor,
		settingService: settingService,
		dataDir:        cfg.Pricing.DataDir,
		usage:          usage,
		collector:      collector,
	}
}

// ─────────────────────────────── 请求/响应 DTO ───────────────────────────────

type upstreamProviderRequest struct {
	Name                   string         `json:"name" binding:"required"`
	Type                   string         `json:"type" binding:"required"`
	SiteURL                string         `json:"site_url" binding:"required"`
	APIBaseURL             string         `json:"api_base_url"`
	Credentials            map[string]any `json:"credentials"`
	BalanceThreshold       *float64       `json:"balance_threshold"`
	NotifyOnPriceChange    *bool          `json:"notify_on_price_change"`
	RefreshIntervalMinutes int            `json:"refresh_interval_minutes"`
	RechargeRatio          *float64       `json:"recharge_ratio"`
	Remark                 string         `json:"remark"`
}

func (r *upstreamProviderRequest) toService() *service.UpstreamProvider {
	notifyPrice := true
	if r.NotifyOnPriceChange != nil {
		notifyPrice = *r.NotifyOnPriceChange
	}
	ratio := 1.0
	if r.RechargeRatio != nil {
		ratio = *r.RechargeRatio
	}
	creds := r.Credentials
	if creds == nil {
		creds = map[string]any{}
	}
	return &service.UpstreamProvider{
		Name: r.Name, Type: r.Type,
		SiteURL: r.SiteURL, APIBaseURL: r.APIBaseURL,
		Credentials: creds,
		BalanceThreshold:       r.BalanceThreshold,
		NotifyOnPriceChange:    notifyPrice,
		RefreshIntervalMinutes: r.RefreshIntervalMinutes,
		RechargeRatio:          ratio,
		Remark:                 r.Remark,
	}
}

type upstreamProviderResponse struct {
	ID                     int64                            `json:"id"`
	Name                   string                           `json:"name"`
	Type                   string                           `json:"type"`
	SiteURL                string                           `json:"site_url"`
	APIBaseURL             string                           `json:"api_base_url"`
	EffectiveAPIBaseURL    string                           `json:"effective_api_base_url"`
	Status                 string                           `json:"status"`
	Credentials            map[string]any                   `json:"credentials"` // 已脱敏
	CredentialStatus       service.UpstreamCredentialStatus `json:"credential_status"`
	BalanceThreshold       *float64                         `json:"balance_threshold"`
	NotifyOnPriceChange    bool                             `json:"notify_on_price_change"`
	RefreshIntervalMinutes int                              `json:"refresh_interval_minutes"`
	LatestSnapshot         *domain.UpstreamSnapshot         `json:"latest_snapshot,omitempty"`
	LastRefreshedAt        *time.Time                       `json:"last_refreshed_at"`
	LastError              string                           `json:"last_error"`
	ConsecutiveFailures    int                              `json:"consecutive_failures"`
	Remark                 string                           `json:"remark"`
	RechargeRatio          float64                          `json:"recharge_ratio"`
	UsageSummary           *service.UsageSummary            `json:"usage_summary,omitempty"`
	CreatedAt              time.Time                        `json:"created_at"`
}

// toUpstreamProviderResponse 构造脱敏响应。
// withSnapshot=false 用于列表（瘦身：去掉 ModelPricing 与 UserInfo，保留 Groups 完整）；
// withSnapshot=true 用于详情（全量 LatestSnapshot）。
func toUpstreamProviderResponse(p *service.UpstreamProvider, withSnapshot bool) upstreamProviderResponse {
	redacted, status := service.RedactUpstreamCredentials(p.Credentials)
	resp := upstreamProviderResponse{
		ID:                     p.ID,
		Name:                   p.Name,
		Type:                   p.Type,
		SiteURL:                p.SiteURL,
		APIBaseURL:             p.APIBaseURL,
		EffectiveAPIBaseURL:    p.EffectiveAPIBaseURL(),
		Status:                 p.Status,
		Credentials:            redacted,
		CredentialStatus:       status,
		BalanceThreshold:       p.BalanceThreshold,
		NotifyOnPriceChange:    p.NotifyOnPriceChange,
		RefreshIntervalMinutes: p.RefreshIntervalMinutes,
		LastRefreshedAt:        p.LastRefreshedAt,
		LastError:              p.LastError,
		ConsecutiveFailures:    p.ConsecutiveFailures,
		Remark:                 p.Remark,
		RechargeRatio:          p.RechargeRatio,
		CreatedAt:              p.CreatedAt,
	}
	if withSnapshot {
		// 详情：全量 snapshot
		resp.LatestSnapshot = p.LatestSnapshot
	} else if p.LatestSnapshot != nil {
		// 列表：只去掉 ModelPricing 与 UserInfo，保留 Groups 完整（含 Models，前端计算 modelCount）
		snap := p.LatestSnapshot
		thin := &domain.UpstreamSnapshot{
			FetchedAt: snap.FetchedAt,
			Balance:   snap.Balance,
			Currency:  snap.Currency,
			Partial:   snap.Partial,
			Groups:    snap.Groups, // Groups 完整保留（Name/Ratio/Description/Models 全留）
			// UserInfo 和 ModelPricing 不包含（瘦身目标是巨大的 pricing 表）
		}
		resp.LatestSnapshot = thin
	}
	return resp
}

// upstreamChangeEventDTO Events 端点专用 DTO（snake_case，R2.4）。
type upstreamChangeEventDTO struct {
	ID         int64          `json:"id"`
	ProviderID int64          `json:"provider_id"`
	Type       string         `json:"type"`
	Summary    string         `json:"summary"`
	Detail     map[string]any `json:"detail"`
	Notified   bool           `json:"notified"`
	CreatedAt  time.Time      `json:"created_at"`
}

func toEventDTO(e service.UpstreamChangeEvent) upstreamChangeEventDTO {
	return upstreamChangeEventDTO{
		ID:         e.ID,
		ProviderID: e.ProviderID,
		Type:       e.Type,
		Summary:    e.Summary,
		Detail:     e.Detail,
		Notified:   e.Notified,
		CreatedAt:  e.CreatedAt,
	}
}

// ─────────────────────────────── 端点处理器 ──────────────────────────────────

// List GET /admin/upstream-providers
func (h *UpstreamProviderHandler) List(c *gin.Context) {
	providers, err := h.svc.List(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	summaries, _ := h.usage.SummariesBatch(c.Request.Context(), providers)
	out := make([]upstreamProviderResponse, 0, len(providers))
	for _, p := range providers {
		r := toUpstreamProviderResponse(p, false)
		if s, ok := summaries[p.ID]; ok {
			sc := s
			r.UsageSummary = &sc
		}
		out = append(out, r)
	}
	response.Success(c, out)
}

// Get GET /admin/upstream-providers/:id
func (h *UpstreamProviderHandler) Get(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	p, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	r := toUpstreamProviderResponse(p, true)
	if summaries, err := h.usage.SummariesBatch(c.Request.Context(), []*service.UpstreamProvider{p}); err == nil {
		if s, ok := summaries[p.ID]; ok {
			sc := s
			r.UsageSummary = &sc
		}
	}
	response.Success(c, r)
}

// Create POST /admin/upstream-providers
func (h *UpstreamProviderHandler) Create(c *gin.Context) {
	var req upstreamProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RechargeRatio != nil && *req.RechargeRatio <= 0 {
		response.BadRequest(c, "recharge_ratio must be > 0")
		return
	}
	created, err := h.svc.Create(c.Request.Context(), req.toService())
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, toUpstreamProviderResponse(created, false))
}

// Update PUT /admin/upstream-providers/:id
func (h *UpstreamProviderHandler) Update(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req upstreamProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RechargeRatio != nil && *req.RechargeRatio <= 0 {
		response.BadRequest(c, "recharge_ratio must be > 0")
		return
	}
	updated, err := h.svc.Update(c.Request.Context(), id, req.toService())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toUpstreamProviderResponse(updated, false))
}

// Delete DELETE /admin/upstream-providers/:id
func (h *UpstreamProviderHandler) Delete(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// Refresh POST /admin/upstream-providers/:id/refresh（同步执行，manual=true）
// 409 表示正在刷新中（R2.7）。
func (h *UpstreamProviderHandler) Refresh(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	err := h.monitor.RefreshProvider(c.Request.Context(), id, true)
	if err != nil {
		// R2.7: RefreshInProgress → 409
		if isRefreshInProgress(err) {
			response.Error(c, http.StatusConflict, "正在刷新中，请稍候")
			return
		}
		// provider 不存在 → 404；其余(采集/过盾/凭证失败)→ 400 + 友好信息
		// (失败状态与 last_error 已在 commitFailure 持久化,前端从详情可见)
		if errors.Is(err, service.ErrUpstreamProviderNotFound) {
			response.ErrorFrom(c, err)
			return
		}
		response.BadRequest(c, err.Error())
		return
	}
	if h.collector != nil {
		if err := h.collector.CollectProvider(c.Request.Context(), id); err != nil {
			slog.Warn("upstream: 手动刷新后采集消耗失败", "provider_id", id, "error", err)
		}
	}
	p, gerr := h.svc.Get(c.Request.Context(), id)
	if gerr != nil {
		response.ErrorFrom(c, gerr)
		return
	}
	response.Success(c, toUpstreamProviderResponse(p, true))
}

// Relogin POST /admin/upstream-providers/:id/relogin
// 剥离 access_token/cf_* → 强制走自动登录路径（R2.7 修订：用 svc.ResetTokens，不走 svc.Update）。
func (h *UpstreamProviderHandler) Relogin(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.ResetTokens(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.Refresh(c)
}

// TestConnection POST /admin/upstream-providers/test
func (h *UpstreamProviderHandler) TestConnection(c *gin.Context) {
	var req struct {
		upstreamProviderRequest
		ProviderID int64 `json:"provider_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	snap, err := h.svc.TestConnection(c.Request.Context(), req.toService(), req.ProviderID)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, snap)
}

// LinkedAccounts GET /admin/upstream-providers/:id/linked-accounts
func (h *UpstreamProviderHandler) LinkedAccounts(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	accounts, err := h.svc.LinkedAccounts(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, accounts)
}

// ListTokens GET /admin/upstream-providers/:id/tokens
func (h *UpstreamProviderHandler) ListTokens(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	tokens, err := h.svc.ListTokens(c.Request.Context(), id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, tokens)
}

// CreateToken POST /admin/upstream-providers/:id/tokens（key 明文仅本次返回）
func (h *UpstreamProviderHandler) CreateToken(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req service.CreateUpstreamTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	tok, apiBaseURL, err := h.svc.CreateToken(c.Request.Context(), id, req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"token": tok, "api_base_url": apiBaseURL})
}

// Events GET /admin/upstream-providers/:id/events?limit=&before_created_at=&before_id=
// 返回专用 DTO（snake_case，R2.4）。
func (h *UpstreamProviderHandler) Events(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	filter := service.UpstreamChangeEventFilter{}
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := c.Query("before_created_at"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.BeforeCreatedAt = &t
		}
	}
	if v := c.Query("before_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.BeforeID = &n
		}
	}
	events, err := h.svc.Events(c.Request.Context(), id, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	dtos := make([]upstreamChangeEventDTO, 0, len(events))
	for _, e := range events {
		dtos = append(dtos, toEventDTO(e))
	}
	response.Success(c, dtos)
}

// Usage GET /admin/upstream-providers/:id/usage?scope=&window=
func (h *UpstreamProviderHandler) Usage(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	scope := c.DefaultQuery("scope", service.UsageScopeKey)
	window := c.DefaultQuery("window", "month")
	switch scope {
	case service.UsageScopeKey, service.UsageScopeGroup, service.UsageScopeModel:
	default:
		response.BadRequest(c, "invalid scope")
		return
	}
	bd, err := h.usage.Breakdown(c.Request.Context(), id, scope, window)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, bd)
}

// ─────────────────── 诊断截图 ───────────────────

// diagnosticsFileRe 合法诊断文件名：14 位数字 + .png（防路径穿越）。
var diagnosticsFileRe = regexp.MustCompile(`^[0-9]{14}\.png$`)

func validDiagnosticsFile(name string) bool {
	return diagnosticsFileRe.MatchString(name)
}

// Diagnostics GET /admin/upstream-providers/:id/diagnostics/:file（spec §6）
func (h *UpstreamProviderHandler) Diagnostics(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	file := c.Param("file")
	if !validDiagnosticsFile(file) {
		response.BadRequest(c, "非法文件名")
		return
	}
	path := filepath.Join(h.dataDir, "upstream-diagnostics", strconv.FormatInt(id, 10), file)
	c.File(path)
}

// ─────────────────── 专项设置 ───────────────────

// GetSettings GET /admin/settings/upstream-management
func (h *UpstreamProviderHandler) GetSettings(c *gin.Context) {
	rt := h.settingService.GetUpstreamManagementRuntime(c.Request.Context())
	rt.ProxyURL = service.MaskUpstreamProxyURL(rt.ProxyURL) // 响应脱敏:隐藏代理密码段
	response.Success(c, rt)
}

// UpdateSettings PUT /admin/settings/upstream-management
func (h *UpstreamProviderHandler) UpdateSettings(c *gin.Context) {
	var req service.UpstreamManagementRuntime
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// 代理 URL 合并:前端回传含 *** 的脱敏占位时保留旧值,避免把真实代理覆盖成掩码串。
	existing := h.settingService.GetUpstreamManagementRuntime(c.Request.Context())
	req.ProxyURL = service.MergeUpstreamProxyURL(existing.ProxyURL, req.ProxyURL)
	if err := h.settingService.SetUpstreamManagementSettings(c.Request.Context(), req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, h.settingService.GetUpstreamManagementRuntime(c.Request.Context()))
}

// ─────────────────── 辅助函数 ───────────────────

// isRefreshInProgress 判断是否是"正在刷新"哨兵错误（R2.7）。
func isRefreshInProgress(err error) bool {
	return errors.Is(err, service.ErrUpstreamRefreshInProgress)
}
