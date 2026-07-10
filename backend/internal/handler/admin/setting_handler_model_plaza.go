package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// GetModelPlazaSettings 获取模型广场功能开关
// GET /api/v1/admin/settings/model-plaza
func (h *SettingHandler) GetModelPlazaSettings(c *gin.Context) {
	rt := h.settingService.GetModelPlazaRuntime(c.Request.Context())
	response.Success(c, dto.ModelPlazaSettings{Enabled: rt.Enabled})
}

// UpdateModelPlazaSettingsRequest 更新模型广场开关请求
type UpdateModelPlazaSettingsRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// UpdateModelPlazaSettings 更新模型广场功能开关
// PUT /api/v1/admin/settings/model-plaza
func (h *SettingHandler) UpdateModelPlazaSettings(c *gin.Context) {
	var req UpdateModelPlazaSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.settingService.SetModelPlazaEnabled(c.Request.Context(), *req.Enabled); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	rt := h.settingService.GetModelPlazaRuntime(c.Request.Context())
	response.Success(c, dto.ModelPlazaSettings{Enabled: rt.Enabled})
}
