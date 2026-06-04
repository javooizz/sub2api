package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// plazaDataProvider / modelPlazaRuntimeReader 把 handler 依赖收窄为本文件所需，
// 便于单测注入 fake；wire 仍注入具体 service。
type plazaDataProvider interface {
	GetPlazaForUser(ctx context.Context, userID int64) (*service.ModelPlazaData, error)
}

type modelPlazaRuntimeReader interface {
	GetModelPlazaRuntime(ctx context.Context) service.ModelPlazaRuntime
}

// ModelPlazaHandler 处理用户侧"模型广场"查询。
//
// 响应做字段白名单：分组只暴露 id/name/platform/subscription_type/
// rate_multiplier/is_exclusive/accessible；定价复用可用渠道的
// userSupportedModelPricing DTO（见 available_channel_handler.go）。
type ModelPlazaHandler struct {
	plaza   plazaDataProvider
	setting modelPlazaRuntimeReader
}

// NewModelPlazaHandler 创建用户侧模型广场 handler。
func NewModelPlazaHandler(
	modelPlazaService *service.ModelPlazaService,
	settingService *service.SettingService,
) *ModelPlazaHandler {
	return &ModelPlazaHandler{plaza: modelPlazaService, setting: settingService}
}

// plazaGroupDTO 用户可见的分组白名单字段。
// Accessible=false 表示"公开订阅型但未订阅"——前端显示"需订阅"标签。
type plazaGroupDTO struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Platform         string  `json:"platform"`
	SubscriptionType string  `json:"subscription_type"`
	RateMultiplier   float64 `json:"rate_multiplier"`
	IsExclusive      bool    `json:"is_exclusive"`
	Accessible       bool    `json:"accessible"`
}

// plazaModelDTO 用户可见的模型条目。唯一身份 = (platform, name)。
type plazaModelDTO struct {
	Name        string                     `json:"name"`
	Platform    string                     `json:"platform"`
	Description string                     `json:"description"`
	BillingMode string                     `json:"billing_mode"`
	Pricing     *userSupportedModelPricing `json:"pricing"`
	Groups      []plazaGroupDTO            `json:"groups"`
}

// modelPlazaResponse 是 GET /model-plaza 的响应体。
type modelPlazaResponse struct {
	Enabled      bool            `json:"enabled"`
	Announcement string          `json:"announcement"`
	Models       []plazaModelDTO `json:"models"`
}

// Get 返回当前用户可见的模型广场视图。
// GET /api/v1/model-plaza
func (h *ModelPlazaHandler) Get(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	// Feature 未启用时返回 enabled=false（不暴露模型信息）。检查放在认证之后，
	// 与可用渠道 handler 保持一致：未登录先 401，登录后再按开关决定。
	if !h.setting.GetModelPlazaRuntime(c.Request.Context()).Enabled {
		response.Success(c, modelPlazaResponse{Enabled: false, Models: []plazaModelDTO{}})
		return
	}

	data, err := h.plaza.GetPlazaForUser(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	models := make([]plazaModelDTO, 0, len(data.Models))
	for i := range data.Models {
		m := &data.Models[i]
		groups := make([]plazaGroupDTO, 0, len(m.Groups))
		for _, g := range m.Groups {
			groups = append(groups, plazaGroupDTO{
				ID:               g.ID,
				Name:             g.Name,
				Platform:         g.Platform,
				SubscriptionType: g.SubscriptionType,
				RateMultiplier:   g.RateMultiplier,
				IsExclusive:      g.IsExclusive,
				Accessible:       g.Accessible,
			})
		}
		models = append(models, plazaModelDTO{
			Name:        m.Name,
			Platform:    m.Platform,
			Description: m.Description,
			BillingMode: string(m.BillingMode),
			Pricing:     toUserPricing(m.Pricing),
			Groups:      groups,
		})
	}

	response.Success(c, modelPlazaResponse{
		Enabled:      true,
		Announcement: data.Announcement,
		Models:       models,
	})
}
