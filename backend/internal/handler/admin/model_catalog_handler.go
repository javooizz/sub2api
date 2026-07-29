package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelCatalogHandler 模型目录管理端查询。
type ModelCatalogHandler struct {
	modelCatalogService *service.ModelCatalogService
}

// NewModelCatalogHandler 创建模型目录管理端 handler。
func NewModelCatalogHandler(modelCatalogService *service.ModelCatalogService) *ModelCatalogHandler {
	return &ModelCatalogHandler{modelCatalogService: modelCatalogService}
}

// modelIdentityDTO 模型身份条目（platform + name 唯一）。
type modelIdentityDTO struct {
	Platform string `json:"platform"`
	Name     string `json:"name"`
}

// ListModels 返回全部 active 渠道聚合出的模型清单（去重、不带黑名单/用户过滤），
// 供"扩展配置 → 模型目录 → 模型描述"编辑器使用（避免前端翻页 admin channels 漏聚合）。
// GET /api/v1/admin/model-catalog/models
func (h *ModelCatalogHandler) ListModels(c *gin.Context) {
	identities, err := h.modelCatalogService.ListAllModelIdentities(c.Request.Context())
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
