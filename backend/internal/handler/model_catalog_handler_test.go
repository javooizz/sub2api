//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeCatalogRuntime struct{ enabled bool }

func (f *fakeCatalogRuntime) GetModelCatalogRuntime(ctx context.Context) service.ModelCatalogRuntime {
	return service.ModelCatalogRuntime{Enabled: f.enabled}
}

type fakeCatalogData struct {
	data *service.ModelCatalogData
	err  error
}

func (f *fakeCatalogData) GetCatalogForUser(ctx context.Context, userID int64) (*service.ModelCatalogData, error) {
	return f.data, f.err
}

func catalogTestContext(t *testing.T, authed bool) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/model-catalog", nil)
	if authed {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
	}
	return c, w
}

func TestModelCatalog_Unauthenticated401(t *testing.T) {
	h := &ModelCatalogHandler{} // 401 路径不触达依赖
	c, w := catalogTestContext(t, false)

	h.Get(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestModelCatalog_DisabledReturnsEnabledFalse(t *testing.T) {
	h := &ModelCatalogHandler{
		catalog:   &fakeCatalogData{},
		setting: &fakeCatalogRuntime{enabled: false},
	}
	c, w := catalogTestContext(t, true)

	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data modelCatalogResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.False(t, resp.Data.Enabled)
	require.Empty(t, resp.Data.Models)
}

func TestModelCatalog_EnabledReturnsModels(t *testing.T) {
	in, out := 9e-7, 4.5e-6
	h := &ModelCatalogHandler{
		catalog: &fakeCatalogData{data: &service.ModelCatalogData{
			Announcement: "## hi",
			Models: []service.CatalogModel{{
				Name: "claude-sonnet-4-6", Platform: "anthropic",
				Description: "desc", BillingMode: service.BillingModeToken,
				Pricing: &service.ChannelModelPricing{
					BillingMode: service.BillingModeToken,
					InputPrice:  &in, OutputPrice: &out,
				},
				Groups: []service.CatalogGroupRef{{
					AvailableGroupRef: service.AvailableGroupRef{
						ID: 10, Name: "cc_max", Platform: "anthropic",
						SubscriptionType: "standard", RateMultiplier: 1.8,
					},
					Accessible: true,
				}},
			}},
		}},
		setting: &fakeCatalogRuntime{enabled: true},
	}
	c, w := catalogTestContext(t, true)

	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data modelCatalogResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Data.Enabled)
	require.Equal(t, "## hi", resp.Data.Announcement)
	require.Len(t, resp.Data.Models, 1)
	m := resp.Data.Models[0]
	require.Equal(t, "claude-sonnet-4-6", m.Name)
	require.Equal(t, "token", m.BillingMode)
	require.NotNil(t, m.Pricing)
	require.Len(t, m.Groups, 1)
	require.True(t, m.Groups[0].Accessible)
	require.Equal(t, 1.8, m.Groups[0].RateMultiplier)

	// 映射循环全字段断言：防止未来字段丢失静默通过
	require.Equal(t, "anthropic", m.Platform)
	require.Equal(t, "desc", m.Description)
	require.Equal(t, 9e-7, *m.Pricing.InputPrice)
	require.Equal(t, 4.5e-6, *m.Pricing.OutputPrice)
	g := m.Groups[0]
	require.Equal(t, int64(10), g.ID)
	require.Equal(t, "cc_max", g.Name)
	require.Equal(t, "anthropic", g.Platform)
	require.Equal(t, "standard", g.SubscriptionType)
	require.False(t, g.IsExclusive)
}

func TestModelCatalog_DTOWhitelist(t *testing.T) {
	// 序列化 catalogGroupDTO 验证白名单：不得出现内部字段（如 status / created_at）。
	raw, err := json.Marshal(catalogGroupDTO{ID: 1, Name: "g", Platform: "anthropic", SubscriptionType: "standard", RateMultiplier: 1.5, IsExclusive: false, Accessible: true})
	require.NoError(t, err)
	var asMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &asMap))
	require.ElementsMatch(t,
		[]string{"id", "name", "platform", "subscription_type", "rate_multiplier", "is_exclusive", "accessible"},
		mapKeys(asMap))
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestModelCatalog_GroupImagePricingDTO(t *testing.T) {
	one, four, mult := 0.6, 0.8, 1.0
	h := &ModelCatalogHandler{
		catalog: &fakeCatalogData{data: &service.ModelCatalogData{
			Models: []service.CatalogModel{{
				Name: "gpt-image-2", Platform: "openai",
				BillingMode: service.BillingModeImage,
				Groups: []service.CatalogGroupRef{
					{
						AvailableGroupRef: service.AvailableGroupRef{ID: 23, Name: "gpt-image", Platform: "openai", RateMultiplier: 0.1},
						Accessible:        true,
						ImagePricing: &service.CatalogGroupImagePricing{
							Allowed: true, Price1K: &one, Price2K: &one, Price4K: &four,
							MultiplierOverride: &mult,
						},
					},
					{ // 非图像注入分组(ImagePricing nil)→ DTO 省略 image_pricing 键
						AvailableGroupRef: service.AvailableGroupRef{ID: 10, Name: "cc_max", Platform: "anthropic", RateMultiplier: 1},
						Accessible:        true,
					},
				},
			}},
		}},
		setting: &fakeCatalogRuntime{enabled: true},
	}
	c, w := catalogTestContext(t, true)

	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct {
			Models []struct {
				Groups []map[string]json.RawMessage `json:"groups"`
			} `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	groups := resp.Data.Models[0].Groups

	var ip struct {
		Allowed            bool     `json:"allowed"`
		Price1K            *float64 `json:"price_1k"`
		Price2K            *float64 `json:"price_2k"`
		Price4K            *float64 `json:"price_4k"`
		MultiplierOverride *float64 `json:"multiplier_override"`
	}
	require.Contains(t, groups[0], "image_pricing")
	require.NoError(t, json.Unmarshal(groups[0]["image_pricing"], &ip))
	require.True(t, ip.Allowed)
	require.InDelta(t, 0.6, *ip.Price1K, 1e-10)
	require.InDelta(t, 0.6, *ip.Price2K, 1e-10)
	require.InDelta(t, 0.8, *ip.Price4K, 1e-10)
	require.InDelta(t, 1.0, *ip.MultiplierOverride, 1e-10)

	// ImagePricing nil → 响应不含 image_pricing 键(与现状字节级兼容)
	require.NotContains(t, groups[1], "image_pricing")
}
