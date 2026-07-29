package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// GetModelCatalogSettings 获取模型目录功能开关
// GET /api/v1/admin/settings/model-catalog
func (h *SettingHandler) GetModelCatalogSettings(c *gin.Context) {
	rt := h.settingService.GetModelCatalogRuntime(c.Request.Context())
	response.Success(c, dto.ModelCatalogSettings{Enabled: rt.Enabled})
}

// UpdateModelCatalogSettingsRequest 更新模型目录开关请求
type UpdateModelCatalogSettingsRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// UpdateModelCatalogSettings 更新模型目录功能开关
// PUT /api/v1/admin/settings/model-catalog
func (h *SettingHandler) UpdateModelCatalogSettings(c *gin.Context) {
	var req UpdateModelCatalogSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.settingService.SetModelCatalogEnabled(c.Request.Context(), *req.Enabled); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	rt := h.settingService.GetModelCatalogRuntime(c.Request.Context())
	response.Success(c, dto.ModelCatalogSettings{Enabled: rt.Enabled})
}
