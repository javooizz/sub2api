package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelPlazaHandler 模型广场管理端查询。
type ModelPlazaHandler struct {
	modelPlazaService *service.ModelPlazaService
}

// NewModelPlazaHandler 创建模型广场管理端 handler。
func NewModelPlazaHandler(modelPlazaService *service.ModelPlazaService) *ModelPlazaHandler {
	return &ModelPlazaHandler{modelPlazaService: modelPlazaService}
}

// modelIdentityDTO 模型身份条目（platform + name 唯一）。
type modelIdentityDTO struct {
	Platform string `json:"platform"`
	Name     string `json:"name"`
}

// ListModels 返回全部 active 渠道聚合出的模型清单（去重、不带黑名单/用户过滤），
// 供"扩展配置 → 模型广场 → 模型描述"编辑器使用（避免前端翻页 admin channels 漏聚合）。
// GET /api/v1/admin/model-plaza/models
func (h *ModelPlazaHandler) ListModels(c *gin.Context) {
	identities, err := h.modelPlazaService.ListAllModelIdentities(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]modelIdentityDTO, 0, len(identities))
	for _, m := range identities {
		out = append(out, modelIdentityDTO{Platform: m.Platform, Name: m.Name})
	}
	response.Success(c, out)
}
