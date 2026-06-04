//go:build unit

package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 只验证请求绑定失败路径（无需 service 依赖）：`Enabled *bool` 收到字符串时
// json 解码即报错，在触达 service 之前返回 400。成功路径由 Task 9 端到端验证。
func TestUpdateModelPlazaSettings_InvalidJSON400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SettingHandler{} // 绑定失败在触达 service 之前返回
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/model-plaza",
		strings.NewReader(`{"enabled": "not-a-bool"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateModelPlazaSettings(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// 完全缺 enabled 字段（{}）→ binding:"required" 校验失败 → 400。
// 与 InvalidJSON400 区分：这条命中的是 required 校验而非 json 解码错误，
// 锁住"缺字段不得被静默当 false 写库"的设计点。
func TestUpdateModelPlazaSettings_MissingEnabled400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &SettingHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/model-plaza",
		strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateModelPlazaSettings(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}
