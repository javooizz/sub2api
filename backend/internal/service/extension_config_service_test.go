package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock ExtensionConfigRepository ---

type mockExtensionConfigRepo struct {
	records         []*ExtensionConfigRecord
	upsertErr       error
	deleteErr       error
	listErr         error
	deletedAgentID  string
	upsertedAgentID string
	upsertedPayload domain.ExtensionConfigPayload
}

func (m *mockExtensionConfigRepo) GetByAgentID(ctx context.Context, agentID string) (*ExtensionConfigRecord, error) {
	for _, r := range m.records {
		if r.AgentID == agentID {
			return r, nil
		}
	}
	return nil, ErrExtensionConfigNotFound
}

func (m *mockExtensionConfigRepo) Upsert(
	ctx context.Context,
	agentID string,
	payload domain.ExtensionConfigPayload,
	updatedBy *int64,
) (*ExtensionConfigRecord, error) {
	if m.upsertErr != nil {
		return nil, m.upsertErr
	}
	m.upsertedAgentID = agentID
	m.upsertedPayload = payload
	rec := &ExtensionConfigRecord{AgentID: agentID, Payload: payload}
	m.records = append(m.records, rec)
	return rec, nil
}

func (m *mockExtensionConfigRepo) Delete(ctx context.Context, agentID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedAgentID = agentID
	return nil
}

func (m *mockExtensionConfigRepo) ListAll(ctx context.Context) ([]*ExtensionConfigRecord, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.records, nil
}

// --- 测试 ListAllOneboolOrigins ---

func TestListAllOneboolOrigins_DedupAndSkipsInvalid(t *testing.T) {
	repo := &mockExtensionConfigRepo{
		records: []*ExtensionConfigRecord{
			{AgentID: "a1", Payload: domain.ExtensionConfigPayload{OneboolOrigin: "https://flow.onebool.com"}},
			{AgentID: "a2", Payload: domain.ExtensionConfigPayload{OneboolOrigin: "https://flow.onebool.com"}}, // dup
			{AgentID: "a3", Payload: domain.ExtensionConfigPayload{OneboolOrigin: ""}},                         // empty skip
			{AgentID: "a4", Payload: domain.ExtensionConfigPayload{OneboolOrigin: "not-a-url"}},                // invalid skip
			{AgentID: "a5", Payload: domain.ExtensionConfigPayload{OneboolOrigin: "https://image.sub2api.com"}},
		},
	}
	svc := &ExtensionConfigService{repo: repo}

	got, err := svc.ListAllOneboolOrigins(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"https://flow.onebool.com", "https://image.sub2api.com"}, got)
}

func TestListAllOneboolOrigins_RepoError(t *testing.T) {
	repo := &mockExtensionConfigRepo{listErr: errors.New("boom")}
	svc := &ExtensionConfigService{repo: repo}
	_, err := svc.ListAllOneboolOrigins(context.Background())
	assert.Error(t, err)
}

// --- 测试 onUpdate 回调 ---

func TestOnUpdate_TriggeredOnUpsertSuccess(t *testing.T) {
	repo := &mockExtensionConfigRepo{}
	// 注意:Upsert 路径会走 validatePayload,需要 payload 通过校验。
	// 我们只测 onUpdate 触发,所以 payload 用最简(image_gen=nil,onebool_origin=空)。
	svc := &ExtensionConfigService{repo: repo}
	var called int
	svc.SetOnUpdateCallback(func() { called++ })

	_, err := svc.Upsert(context.Background(), "a1", domain.ExtensionConfigPayload{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

func TestOnUpdate_TriggeredOnDeleteSuccess(t *testing.T) {
	repo := &mockExtensionConfigRepo{
		records: []*ExtensionConfigRecord{{AgentID: "a1"}},
	}
	svc := &ExtensionConfigService{repo: repo}
	var called int
	svc.SetOnUpdateCallback(func() { called++ })
	err := svc.Delete(context.Background(), "a1")
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

// --- 测试 validatePayload 的 onebool_origin 校验 ---

func TestValidatePayload_OneboolOriginRejectsInvalidScheme(t *testing.T) {
	svc := &ExtensionConfigService{}
	p := &domain.ExtensionConfigPayload{OneboolOrigin: "ftp://flow.onebool.com"}
	err := svc.validateOneboolOrigin(p)
	assert.ErrorIs(t, err, ErrExtensionConfigInvalidPayload)
}

func TestValidatePayload_OneboolOriginRejectsWithPath(t *testing.T) {
	svc := &ExtensionConfigService{}
	p := &domain.ExtensionConfigPayload{OneboolOrigin: "https://flow.onebool.com/some/path"}
	err := svc.validateOneboolOrigin(p)
	assert.ErrorIs(t, err, ErrExtensionConfigInvalidPayload)
}

func TestValidatePayload_OneboolOriginRejectsWithQuery(t *testing.T) {
	svc := &ExtensionConfigService{}
	p := &domain.ExtensionConfigPayload{OneboolOrigin: "https://flow.onebool.com?x=1"}
	err := svc.validateOneboolOrigin(p)
	assert.ErrorIs(t, err, ErrExtensionConfigInvalidPayload)
}

func TestValidatePayload_OneboolOriginRejectsWithFragment(t *testing.T) {
	svc := &ExtensionConfigService{}
	p := &domain.ExtensionConfigPayload{OneboolOrigin: "https://flow.onebool.com#section"}
	err := svc.validateOneboolOrigin(p)
	assert.ErrorIs(t, err, ErrExtensionConfigInvalidPayload)
}

func TestValidatePayload_OneboolOriginAcceptsValidHttps(t *testing.T) {
	svc := &ExtensionConfigService{}
	p := &domain.ExtensionConfigPayload{OneboolOrigin: "https://flow.onebool.com"}
	assert.NoError(t, svc.validateOneboolOrigin(p))
}

func TestValidatePayload_OneboolOriginAcceptsEmpty(t *testing.T) {
	svc := &ExtensionConfigService{}
	p := &domain.ExtensionConfigPayload{OneboolOrigin: ""}
	assert.NoError(t, svc.validateOneboolOrigin(p))
}

func TestValidatePayload_ModelPlaza(t *testing.T) {
	// model-plaza 分支不触达 repo/groupRepo，可用零值 service。
	s := &ExtensionConfigService{}
	ctx := context.Background()

	valid := domain.ExtensionConfigPayload{ModelPlaza: &domain.ModelPlazaExtensionConfig{
		ExcludedChannelIDs: []int64{1, 2},
		ExcludedGroupIDs:   []int64{3},
		ModelDescriptions:  map[string]string{"claude-sonnet-4-6": "旗舰对话模型"},
		Announcement:       "## 公告\n按量计费",
	}}
	require.NoError(t, s.validatePayload(ctx, AgentIDModelPlaza, &valid))

	tooLongAnnouncement := domain.ExtensionConfigPayload{ModelPlaza: &domain.ModelPlazaExtensionConfig{
		Announcement: strings.Repeat("公", 2001),
	}}
	require.Error(t, s.validatePayload(ctx, AgentIDModelPlaza, &tooLongAnnouncement))

	badChannelID := domain.ExtensionConfigPayload{ModelPlaza: &domain.ModelPlazaExtensionConfig{
		ExcludedChannelIDs: []int64{0},
	}}
	require.Error(t, s.validatePayload(ctx, AgentIDModelPlaza, &badChannelID))

	badGroupID := domain.ExtensionConfigPayload{ModelPlaza: &domain.ModelPlazaExtensionConfig{
		ExcludedGroupIDs: []int64{-1},
	}}
	require.Error(t, s.validatePayload(ctx, AgentIDModelPlaza, &badGroupID))

	emptyModelName := domain.ExtensionConfigPayload{ModelPlaza: &domain.ModelPlazaExtensionConfig{
		ModelDescriptions: map[string]string{"": "x"},
	}}
	require.Error(t, s.validatePayload(ctx, AgentIDModelPlaza, &emptyModelName))

	tooLongDesc := domain.ExtensionConfigPayload{ModelPlaza: &domain.ModelPlazaExtensionConfig{
		ModelDescriptions: map[string]string{"m": strings.Repeat("描", 501)},
	}}
	require.Error(t, s.validatePayload(ctx, AgentIDModelPlaza, &tooLongDesc))

	// 其他 agent 的 payload 带 ModelPlaza 字段不触发本分支校验
	otherAgent := domain.ExtensionConfigPayload{ModelPlaza: &domain.ModelPlazaExtensionConfig{
		Announcement: strings.Repeat("公", 2001),
	}}
	require.NoError(t, s.validatePayload(ctx, AgentIDImageGen, &otherAgent))
}
