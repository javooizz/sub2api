//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// modelPlazaSetterRepoStub 仅实现 SetModelPlazaEnabled 所需的 Set，其余方法不应被触达。
type modelPlazaSetterRepoStub struct {
	sets map[string]string
}

func (s *modelPlazaSetterRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *modelPlazaSetterRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *modelPlazaSetterRepoStub) Set(ctx context.Context, key, value string) error {
	if s.sets == nil {
		s.sets = make(map[string]string)
	}
	s.sets[key] = value
	return nil
}

func (s *modelPlazaSetterRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *modelPlazaSetterRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *modelPlazaSetterRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *modelPlazaSetterRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

// TestSettingService_SetModelPlazaEnabled_FiresOnUpdateCallback 钉住生产 bug 的修复：
//
// 模型广场开关经专项端点（PUT /admin/settings/model-plaza → SetModelPlazaEnabled）
// 写库后，必须像通用 UpdateSettings 路径一样触发 onUpdate 回调。该回调在 router.go
// 中绑定了 frontendServer.InvalidateCache()——SSR 注入的 window.__APP_CONFIG__ 含
// model_plaza_enabled 字段并被缓存进 index.html。若不触发失效，缓存的注入快照会
// 冻结在开关切换前的旧值，导致"开关已开却永不展示"。
func TestSettingService_SetModelPlazaEnabled_FiresOnUpdateCallback(t *testing.T) {
	repo := &modelPlazaSetterRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	called := 0
	svc.SetOnUpdateCallback(func() { called++ })

	err := svc.SetModelPlazaEnabled(context.Background(), true)
	require.NoError(t, err)

	require.Equal(t, "true", repo.sets[SettingKeyModelPlazaEnabled], "开关值必须持久化")
	require.Equal(t, 1, called, "专项 setter 必须触发 onUpdate 以失效前端 HTML 缓存")
}

// TestSettingService_SetModelPlazaEnabled_NoCallbackIsSafe 确保未注册回调时不 panic。
func TestSettingService_SetModelPlazaEnabled_NoCallbackIsSafe(t *testing.T) {
	repo := &modelPlazaSetterRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	require.NoError(t, svc.SetModelPlazaEnabled(context.Background(), false))
	require.Equal(t, "false", repo.sets[SettingKeyModelPlazaEnabled])
}
