package handler

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ExtensionConfigHandler 普通用户侧入口：
//   - GET  /api/v1/extension-configs/:agent_id          → 拉过滤后的可见配置
//   - POST /api/v1/extension-configs/:agent_id/ensure-key → 给定 group_id 查/建 key
//
// 关联：onebool-flow iframe 通过 OneboolFlowEmbed.vue 的 postMessage 代理调用。
type ExtensionConfigHandler struct {
	extensionConfigService *service.ExtensionConfigService
}

// NewExtensionConfigHandler 构造（wire 注入）。
func NewExtensionConfigHandler(svc *service.ExtensionConfigService) *ExtensionConfigHandler {
	return &ExtensionConfigHandler{extensionConfigService: svc}
}

// ====== DTOs ======

type userExtensionConfigResponse struct {
	AgentID       string                         `json:"agent_id"`
	OneboolOrigin string                         `json:"onebool_origin"`
	ImageGen      *service.VisibleImageGenConfig `json:"image_gen"`
}

type ensureKeyRequest struct {
	GroupID      int64   `json:"group_id" binding:"required"`
	EndpointName *string `json:"endpoint_name"`
}

type ensureKeyResponse struct {
	APIKey       string  `json:"api_key"`
	KeyID        int64   `json:"key_id"`
	BaseURL      string  `json:"base_url"`
	GroupID      int64   `json:"group_id"`
	GroupName    string  `json:"group_name"`
	Created      bool    `json:"created"`
	EndpointName *string `json:"endpoint_name"`
}

// ====== Endpoints ======

// Get GET /api/v1/extension-configs/:agent_id
// 返回过滤后的可见配置（按 user_allowed_groups ∩ workbench whitelist）。
// 配置不存在 → image_gen=null，前端隐藏选择器走老 profile 流程。
func (h *ExtensionConfigHandler) Get(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	agentID := c.Param("agent_id")
	cfg, err := h.extensionConfigService.GetForUser(c.Request.Context(), subject.UserID, agentID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	resp := userExtensionConfigResponse{AgentID: agentID}
	if cfg != nil {
		resp.OneboolOrigin = cfg.OneboolOrigin
		resp.ImageGen = cfg.ImageGen
	}
	response.Success(c, resp)
}

// EnsureKey POST /api/v1/extension-configs/:agent_id/ensure-key
// 查现有 active key（user+group），无则自动 Create。包裹幂等装饰器，TTL 见 idempotency 默认。
func (h *ExtensionConfigHandler) EnsureKey(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	agentID := c.Param("agent_id")

	var req ensureKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 幂等 scope = user.extension_configs.ensure_key:{agentID}:{groupID}:{endpointName}
	scope := "user.extension_configs.ensure_key:" + agentID + ":" + strconv.FormatInt(req.GroupID, 10)
	if req.EndpointName != nil {
		scope += ":" + *req.EndpointName
	}

	executeUserIdempotentJSON(
		c,
		scope,
		req,
		service.DefaultWriteIdempotencyTTL(),
		func(ctx context.Context) (any, error) {
			result, err := h.extensionConfigService.EnsureKey(ctx, subject.UserID, agentID, req.GroupID, req.EndpointName)
			if err != nil {
				return nil, err
			}
			var epPtr *string
			if result.EndpointName != "" {
				ep := result.EndpointName
				epPtr = &ep
			}
			return ensureKeyResponse{
				APIKey:       result.APIKey,
				KeyID:        result.KeyID,
				BaseURL:      result.BaseURL,
				GroupID:      result.GroupID,
				GroupName:    result.GroupName,
				Created:      result.Created,
				EndpointName: epPtr,
			}, nil
		},
	)
}
