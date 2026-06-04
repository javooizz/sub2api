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
