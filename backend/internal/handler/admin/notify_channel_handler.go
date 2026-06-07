package admin

import (
	"errors"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NotifyChannelHandler 通知渠道管理(spec §9)。当前 scope 仅接受 "upstream"。
type NotifyChannelHandler struct {
	repo    service.NotifyChannelRepository
	senders map[string]service.NotifySender
}

// NewNotifyChannelHandler 构造(wire 注入)。senders 用于测试发送端点。
func NewNotifyChannelHandler(repo service.NotifyChannelRepository, senders map[string]service.NotifySender) *NotifyChannelHandler {
	return &NotifyChannelHandler{repo: repo, senders: senders}
}

type notifyChannelRequest struct {
	Name    string         `json:"name" binding:"required"`
	Type    string         `json:"type" binding:"required"`
	Scope   string         `json:"scope" binding:"required"`
	Enabled *bool          `json:"enabled"`
	Events  []string       `json:"events"`
	Config  map[string]any `json:"config"`
}

type notifyChannelResponse struct {
	ID         int64          `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Scope      string         `json:"scope"`
	Enabled    bool           `json:"enabled"`
	Events     []string       `json:"events"`
	Config     map[string]any `json:"config"`
	LastSentAt *string        `json:"last_sent_at"`
	LastError  string         `json:"last_error"`
}

func toNotifyChannelResponse(ch *service.NotifyChannel) notifyChannelResponse {
	var lastSent *string
	if ch.LastSentAt != nil {
		s := ch.LastSentAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		lastSent = &s
	}
	events := ch.Events
	if events == nil {
		events = []string{}
	}
	return notifyChannelResponse{
		ID: ch.ID, Name: ch.Name, Type: ch.Type, Scope: ch.Scope,
		Enabled: ch.Enabled, Events: events,
		Config:     redactNotifyChannelConfig(ch.Type, ch.Config),
		LastSentAt: lastSent, LastError: ch.LastError,
	}
}

// redactNotifyChannelConfig webhook headers 值替换为 ***(保留键名),其余原样;返回拷贝。
func redactNotifyChannelConfig(chType string, cfg map[string]any) map[string]any {
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		out[k] = v
	}
	if chType != domain.NotifyChannelTypeWebhook {
		return out
	}
	if headers, ok := cfg["headers"].(map[string]any); ok {
		masked := make(map[string]any, len(headers))
		for k := range headers {
			masked[k] = "***"
		}
		out["headers"] = masked
	}
	return out
}

// mergeNotifyChannelConfig PUT 合并:headers 中值为 "***" 的键保留旧值(脱敏回写防清空)。
func mergeNotifyChannelConfig(existing, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(incoming))
	for k, v := range incoming {
		out[k] = v
	}
	inHeaders, inOK := incoming["headers"].(map[string]any)
	exHeaders, exOK := existing["headers"].(map[string]any)
	if !inOK || !exOK {
		return out
	}
	merged := make(map[string]any, len(inHeaders))
	for k, v := range inHeaders {
		if s, ok := v.(string); ok && s == "***" {
			if old, ok := exHeaders[k]; ok {
				merged[k] = old
				continue
			}
		}
		merged[k] = v
	}
	out["headers"] = merged
	return out
}

// List GET /admin/notify-channels?scope=upstream
func (h *NotifyChannelHandler) List(c *gin.Context) {
	scope := c.DefaultQuery("scope", domain.NotifyScopeUpstream)
	channels, err := h.repo.ListByScope(c.Request.Context(), scope)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]notifyChannelResponse, 0, len(channels))
	for _, ch := range channels {
		out = append(out, toNotifyChannelResponse(ch))
	}
	response.Success(c, out)
}

// Create POST /admin/notify-channels
func (h *NotifyChannelHandler) Create(c *gin.Context) {
	var req notifyChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := validateNotifyChannelRequest(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	created, err := h.repo.Create(c.Request.Context(), &service.NotifyChannel{
		Name: req.Name, Type: req.Type, Scope: req.Scope,
		Enabled: enabled, Events: req.Events, Config: req.Config,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toNotifyChannelResponse(created))
}

// Update PUT /admin/notify-channels/:id
func (h *NotifyChannelHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var req notifyChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := validateNotifyChannelRequest(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	existing, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	existing.Name = req.Name
	existing.Type = req.Type
	existing.Enabled = enabled
	existing.Events = req.Events
	existing.Config = mergeNotifyChannelConfig(existing.Config, req.Config)
	updated, err := h.repo.Update(c.Request.Context(), existing)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toNotifyChannelResponse(updated))
}

// Delete DELETE /admin/notify-channels/:id
func (h *NotifyChannelHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// Test POST /admin/notify-channels/:id/test 测试发送
func (h *NotifyChannelHandler) Test(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	ch, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	sender, ok := h.senders[ch.Type]
	if !ok {
		response.BadRequest(c, "未知渠道类型: "+ch.Type)
		return
	}
	ev := service.NotifyEvent{
		Source: ch.Scope, Severity: "info",
		Title: "测试通知", Summary: "这是一条来自上游管理的测试通知",
		Items: []string{"如果你看到这条消息,说明渠道配置正确"},
	}
	sendErr := sender.Send(c.Request.Context(), ch, ev)
	_ = h.repo.MarkResult(c.Request.Context(), ch.ID, sendErr)
	if sendErr != nil {
		response.BadRequest(c, "发送失败: "+sendErr.Error())
		return
	}
	response.Success(c, gin.H{"sent": true})
}

func validateNotifyChannelRequest(req *notifyChannelRequest) error {
	if req.Scope != domain.NotifyScopeUpstream {
		return errBadScope
	}
	switch req.Type {
	case domain.NotifyChannelTypeEmail, domain.NotifyChannelTypeWebhook:
	default:
		return errBadChannelType
	}
	return nil
}

var (
	errBadScope       = errors.New("scope 当前仅支持 upstream")
	errBadChannelType = errors.New("type 仅支持 email | webhook")
)
