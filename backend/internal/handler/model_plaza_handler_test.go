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

type fakePlazaRuntime struct{ enabled bool }

func (f *fakePlazaRuntime) GetModelPlazaRuntime(ctx context.Context) service.ModelPlazaRuntime {
	return service.ModelPlazaRuntime{Enabled: f.enabled}
}

type fakePlazaData struct {
	data *service.ModelPlazaData
	err  error
}

func (f *fakePlazaData) GetPlazaForUser(ctx context.Context, userID int64) (*service.ModelPlazaData, error) {
	return f.data, f.err
}

func plazaTestContext(t *testing.T, authed bool) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/model-plaza", nil)
	if authed {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1})
	}
	return c, w
}

func TestModelPlaza_Unauthenticated401(t *testing.T) {
	h := &ModelPlazaHandler{} // 401 路径不触达依赖
	c, w := plazaTestContext(t, false)

	h.Get(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestModelPlaza_DisabledReturnsEnabledFalse(t *testing.T) {
	h := &ModelPlazaHandler{
		plaza:   &fakePlazaData{},
		setting: &fakePlazaRuntime{enabled: false},
	}
	c, w := plazaTestContext(t, true)

	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data modelPlazaResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.False(t, resp.Data.Enabled)
	require.Empty(t, resp.Data.Models)
}

func TestModelPlaza_EnabledReturnsModels(t *testing.T) {
	in, out := 9e-7, 4.5e-6
	h := &ModelPlazaHandler{
		plaza: &fakePlazaData{data: &service.ModelPlazaData{
			Announcement: "## hi",
			Models: []service.PlazaModel{{
				Name: "claude-sonnet-4-6", Platform: "anthropic",
				Description: "desc", BillingMode: service.BillingModeToken,
				Pricing: &service.ChannelModelPricing{
					BillingMode: service.BillingModeToken,
					InputPrice:  &in, OutputPrice: &out,
				},
				Groups: []service.PlazaGroupRef{{
					AvailableGroupRef: service.AvailableGroupRef{
						ID: 10, Name: "cc_max", Platform: "anthropic",
						SubscriptionType: "standard", RateMultiplier: 1.8,
					},
					Accessible: true,
				}},
			}},
		}},
		setting: &fakePlazaRuntime{enabled: true},
	}
	c, w := plazaTestContext(t, true)

	h.Get(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data modelPlazaResponse `json:"data"`
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

func TestModelPlaza_DTOWhitelist(t *testing.T) {
	// 序列化 plazaGroupDTO 验证白名单：不得出现内部字段（如 status / created_at）。
	raw, err := json.Marshal(plazaGroupDTO{ID: 1, Name: "g", Platform: "anthropic", SubscriptionType: "standard", RateMultiplier: 1.5, IsExclusive: false, Accessible: true})
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
