package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// catalogDataProvider / modelCatalogRuntimeReader 把 handler 依赖收窄为本文件所需，
// 便于单测注入 fake；wire 仍注入具体 service。
type catalogDataProvider interface {
	GetCatalogForUser(ctx context.Context, userID int64) (*service.ModelCatalogData, error)
}

type modelCatalogRuntimeReader interface {
	GetModelCatalogRuntime(ctx context.Context) service.ModelCatalogRuntime
}

// ModelCatalogHandler 处理用户侧"模型广场"查询。
//
// 响应做字段白名单：分组只暴露 id/name/platform/subscription_type/
// rate_multiplier/is_exclusive/accessible；图像生成模型的分组额外带 image_pricing 子对象；
// 定价复用可用渠道的 userSupportedModelPricing DTO（见 available_channel_handler.go）。
type ModelCatalogHandler struct {
	catalog catalogDataProvider
	setting modelCatalogRuntimeReader
}

// NewModelCatalogHandler 创建用户侧模型广场 handler。
func NewModelCatalogHandler(
	modelCatalogService *service.ModelCatalogService,
	settingService *service.SettingService,
) *ModelCatalogHandler {
	return &ModelCatalogHandler{catalog: modelCatalogService, setting: settingService}
}

// catalogGroupDTO 用户可见的分组白名单字段。
// Accessible=false 表示"公开订阅型但未订阅"——前端显示"需订阅"标签。
type catalogGroupDTO struct {
	ID               int64                      `json:"id"`
	Name             string                     `json:"name"`
	Platform         string                     `json:"platform"`
	SubscriptionType string                     `json:"subscription_type"`
	RateMultiplier   float64                    `json:"rate_multiplier"`
	IsExclusive      bool                       `json:"is_exclusive"`
	Accessible       bool                       `json:"accessible"`
	ImagePricing     *catalogGroupImagePricingDTO `json:"image_pricing,omitempty"`
}

// catalogGroupImagePricingDTO 分组出图计费展示信息(仅图像生成模型的分组带,规格 2026-06-07 §4.4)。
type catalogGroupImagePricingDTO struct {
	Allowed            bool     `json:"allowed"`
	Price1K            *float64 `json:"price_1k"`
	Price2K            *float64 `json:"price_2k"`
	Price4K            *float64 `json:"price_4k"`
	MultiplierOverride *float64 `json:"multiplier_override"`
}

// toCatalogImagePricingDTO 转换 service 层出图展示信息;nil 透传。
func toCatalogImagePricingDTO(p *service.CatalogGroupImagePricing) *catalogGroupImagePricingDTO {
	if p == nil {
		return nil
	}
	return &catalogGroupImagePricingDTO{
		Allowed:            p.Allowed,
		Price1K:            p.Price1K,
		Price2K:            p.Price2K,
		Price4K:            p.Price4K,
		MultiplierOverride: p.MultiplierOverride,
	}
}

// catalogModelDTO 用户可见的模型条目。唯一身份 = (platform, name)。
type catalogModelDTO struct {
	Name        string                     `json:"name"`
	Platform    string                     `json:"platform"`
	Description string                     `json:"description"`
	BillingMode string                     `json:"billing_mode"`
	Pricing     *userSupportedModelPricing `json:"pricing"`
	Groups      []catalogGroupDTO            `json:"groups"`
}

// modelCatalogResponse 是 GET /model-catalog 的响应体。
type modelCatalogResponse struct {
	Enabled      bool            `json:"enabled"`
	Announcement string          `json:"announcement"`
	Models       []catalogModelDTO `json:"models"`
}

// Get 返回当前用户可见的模型广场视图。
// GET /api/v1/model-catalog
func (h *ModelCatalogHandler) Get(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	// Feature 未启用时返回 enabled=false（不暴露模型信息）。检查放在认证之后，
	// 与可用渠道 handler 保持一致：未登录先 401，登录后再按开关决定。
	if !h.setting.GetModelCatalogRuntime(c.Request.Context()).Enabled {
		response.Success(c, modelCatalogResponse{Enabled: false, Models: []catalogModelDTO{}})
		return
	}

	data, err := h.catalog.GetCatalogForUser(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	models := make([]catalogModelDTO, 0, len(data.Models))
	for i := range data.Models {
		m := &data.Models[i]
		groups := make([]catalogGroupDTO, 0, len(m.Groups))
		for _, g := range m.Groups {
			groups = append(groups, catalogGroupDTO{
				ID:               g.ID,
				Name:             g.Name,
				Platform:         g.Platform,
				SubscriptionType: g.SubscriptionType,
				RateMultiplier:   g.RateMultiplier,
				IsExclusive:      g.IsExclusive,
				Accessible:       g.Accessible,
				ImagePricing:     toCatalogImagePricingDTO(g.ImagePricing),
			})
		}
		models = append(models, catalogModelDTO{
			Name:        m.Name,
			Platform:    m.Platform,
			Description: m.Description,
			BillingMode: string(m.BillingMode),
			Pricing:     toUserPricing(m.Pricing),
			Groups:      groups,
		})
	}

	response.Success(c, modelCatalogResponse{
		Enabled:      true,
		Announcement: data.Announcement,
		Models:       models,
	})
}
