package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ExtensionConfigHandler 管理员侧 CRUD：读 / 覆盖式更新 / 删除某个 agent 的扩展配置。
// 关联：onebool-flow 工作台页面（/admin/extension-configs/workbench）的后端入口。
type ExtensionConfigHandler struct {
	extensionConfigService *service.ExtensionConfigService
}

// NewExtensionConfigHandler 构造（wire 注入）。
func NewExtensionConfigHandler(svc *service.ExtensionConfigService) *ExtensionConfigHandler {
	return &ExtensionConfigHandler{extensionConfigService: svc}
}

// ====== DTOs ======

type extensionConfigResponse struct {
	AgentID   string                        `json:"agent_id"`
	Payload   domain.ExtensionConfigPayload `json:"payload"`
	UpdatedBy *int64                        `json:"updated_by"`
	CreatedAt string                        `json:"created_at"`
	UpdatedAt string                        `json:"updated_at"`
}

type upsertExtensionConfigRequest struct {
	Payload domain.ExtensionConfigPayload `json:"payload" binding:"required"`
}

func toExtensionConfigResponse(rec *service.ExtensionConfigRecord) extensionConfigResponse {
	if rec == nil {
		return extensionConfigResponse{}
	}
	return extensionConfigResponse{
		AgentID:   rec.AgentID,
		Payload:   rec.Payload,
		UpdatedBy: rec.UpdatedBy,
		CreatedAt: rec.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: rec.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ====== Endpoints ======

// Get GET /api/v1/admin/extension-configs/:agent_id
// 不存在时返回空 payload + 200（前端 fallback 用空数据），便于首次配置流程统一。
func (h *ExtensionConfigHandler) Get(c *gin.Context) {
	agentID := c.Param("agent_id")
	rec, err := h.extensionConfigService.GetAdmin(c.Request.Context(), agentID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if rec == nil {
		response.Success(c, extensionConfigResponse{
			AgentID: agentID,
			Payload: domain.ExtensionConfigPayload{},
		})
		return
	}
	response.Success(c, toExtensionConfigResponse(rec))
}

// Upsert PUT /api/v1/admin/extension-configs/:agent_id
// 完整覆盖式更新。校验由 service 内 validatePayload 完成。
func (h *ExtensionConfigHandler) Upsert(c *gin.Context) {
	agentID := c.Param("agent_id")

	var req upsertExtensionConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	var updatedBy *int64
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok && subject.UserID > 0 {
		uid := subject.UserID
		updatedBy = &uid
	}

	rec, err := h.extensionConfigService.Upsert(c.Request.Context(), agentID, req.Payload, updatedBy)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toExtensionConfigResponse(rec))
}

// Delete DELETE /api/v1/admin/extension-configs/:agent_id
func (h *ExtensionConfigHandler) Delete(c *gin.Context) {
	agentID := c.Param("agent_id")
	if err := h.extensionConfigService.Delete(c.Request.Context(), agentID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
