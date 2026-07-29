//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// modelCatalogSetterRepoStub 仅实现 SetModelCatalogEnabled 所需的 Set，其余方法不应被触达。
type modelCatalogSetterRepoStub struct {
	sets map[string]string
}

func (s *modelCatalogSetterRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *modelCatalogSetterRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *modelCatalogSetterRepoStub) Set(ctx context.Context, key, value string) error {
	if s.sets == nil {
		s.sets = make(map[string]string)
	}
	s.sets[key] = value
	return nil
}

func (s *modelCatalogSetterRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *modelCatalogSetterRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *modelCatalogSetterRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *modelCatalogSetterRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

// TestSettingService_SetModelCatalogEnabled_FiresOnUpdateCallback 钉住生产 bug 的修复：
//
// 模型广场开关经专项端点（PUT /admin/settings/model-plaza → SetModelCatalogEnabled）
// 写库后，必须像通用 UpdateSettings 路径一样触发 onUpdate 回调。该回调在 router.go
// 中绑定了 frontendServer.InvalidateCache()——SSR 注入的 window.__APP_CONFIG__ 含
// model_plaza_enabled 字段并被缓存进 index.html。若不触发失效，缓存的注入快照会
// 冻结在开关切换前的旧值，导致"开关已开却永不展示"。
func TestSettingService_SetModelCatalogEnabled_FiresOnUpdateCallback(t *testing.T) {
	repo := &modelCatalogSetterRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	called := 0
	svc.SetOnUpdateCallback(func() { called++ })

	err := svc.SetModelCatalogEnabled(context.Background(), true)
	require.NoError(t, err)

	require.Equal(t, "true", repo.sets[SettingKeyModelCatalogEnabled], "开关值必须持久化")
	require.Equal(t, 1, called, "专项 setter 必须触发 onUpdate 以失效前端 HTML 缓存")
}

// TestSettingService_SetModelCatalogEnabled_NoCallbackIsSafe 确保未注册回调时不 panic。
func TestSettingService_SetModelCatalogEnabled_NoCallbackIsSafe(t *testing.T) {
	repo := &modelCatalogSetterRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	require.NoError(t, svc.SetModelCatalogEnabled(context.Background(), false))
	require.Equal(t, "false", repo.sets[SettingKeyModelCatalogEnabled])
}
