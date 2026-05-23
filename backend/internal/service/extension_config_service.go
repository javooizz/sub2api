package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// AgentIDImageGen 是 image-gen 智能体的 agent slug。
// 与 onebool-flow 的 src/agents/image-gen/manifest.ts id 保持一致。
const AgentIDImageGen = "image-gen"

// ====== Domain Types ======

// ExtensionConfigRecord 是 ext config 表行的 service 层表示。
type ExtensionConfigRecord struct {
	ID        int64
	AgentID   string
	Payload   domain.ExtensionConfigPayload
	UpdatedBy *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// VisibleExtensionConfig 是 GetForUser 返回的顶层结构，含跨 agent 共享字段 + 各 agent 子配置。
type VisibleExtensionConfig struct {
	OneboolOrigin string                 `json:"onebool_origin,omitempty"`
	ImageGen      *VisibleImageGenConfig `json:"image_gen"`
}

// VisibleImageGenConfig 是普通用户视角的 image-gen 配置，
// 已按 user_allowed_groups 过滤、并附带端点 URL（不暴露 raw payload）。
type VisibleImageGenConfig struct {
	Endpoints           []VisibleEndpoint `json:"endpoints"`
	DefaultEndpointName string            `json:"default_endpoint_name,omitempty"`
	Groups              []VisibleGroup    `json:"groups"`
}

type VisibleEndpoint struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

type VisibleGroup struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Models      []string `json:"models"`
}

// EnsureKeyResult 是 EnsureKey 的返回值。
type EnsureKeyResult struct {
	APIKey       string
	KeyID        int64
	BaseURL      string
	GroupID      int64
	GroupName    string
	Created      bool
	EndpointName string // 实际使用的端点 name（空表示用系统默认 baseUrl）
}

// ====== Errors ======

var (
	ErrExtensionConfigAgentRequired   = infraerrors.BadRequest("EXTENSION_CONFIG_AGENT_REQUIRED", "agent_id is required")
	ErrExtensionConfigInvalidEndpoint = infraerrors.BadRequest("EXTENSION_CONFIG_INVALID_ENDPOINT", "endpoint_name not in enabled_endpoint_names or settings.custom_endpoints")
	ErrExtensionConfigInvalidGroup    = infraerrors.BadRequest("EXTENSION_CONFIG_INVALID_GROUP", "group_id not active or not exists")
	ErrExtensionConfigInvalidPayload  = infraerrors.BadRequest("EXTENSION_CONFIG_INVALID_PAYLOAD", "payload validation failed")
	ErrExtensionConfigNotFound        = infraerrors.NotFound("EXTENSION_CONFIG_NOT_FOUND", "extension config not found")
	ErrExtensionConfigGroupNotVisible = infraerrors.Forbidden("EXTENSION_CONFIG_GROUP_NOT_VISIBLE", "group not in workbench whitelist or user not authorized")
)

// ====== Repository Interface ======

// ExtensionConfigRepository 持久化层接口（实现在 internal/repository）。
type ExtensionConfigRepository interface {
	GetByAgentID(ctx context.Context, agentID string) (*ExtensionConfigRecord, error)
	Upsert(ctx context.Context, agentID string, payload domain.ExtensionConfigPayload, updatedBy *int64) (*ExtensionConfigRecord, error)
	Delete(ctx context.Context, agentID string) error
	ListAll(ctx context.Context) ([]*ExtensionConfigRecord, error)
}

// UserAllowedGroupLister 抽象出查询用户授权 group 的接口。
// 复用现有 userRepository.loadAllowedGroups 的能力（实现适配在 repository 层）。
type UserAllowedGroupLister interface {
	ListAllowedGroupIDs(ctx context.Context, userID int64) ([]int64, error)
}

// ====== Service ======

// ExtensionConfigService 负责扩展配置的 CRUD 与 user-scoped key 自动配发。
type ExtensionConfigService struct {
	repo              ExtensionConfigRepository
	settingService    *SettingService
	groupRepo         GroupRepository
	apiKeyRepo        APIKeyRepository
	apiKeyService     *APIKeyService
	userAllowedLister UserAllowedGroupLister
	onUpdate          func() // CSP frame-src 缓存失效回调
}

// NewExtensionConfigService 构造（wire 注入）。
func NewExtensionConfigService(
	repo ExtensionConfigRepository,
	settingService *SettingService,
	groupRepo GroupRepository,
	apiKeyRepo APIKeyRepository,
	apiKeyService *APIKeyService,
	userAllowedLister UserAllowedGroupLister,
) *ExtensionConfigService {
	return &ExtensionConfigService{
		repo:              repo,
		settingService:    settingService,
		groupRepo:         groupRepo,
		apiKeyRepo:        apiKeyRepo,
		apiKeyService:     apiKeyService,
		userAllowedLister: userAllowedLister,
	}
}

// ====== Admin: Read / Upsert / Delete ======

// GetAdmin 读取某 agent 的完整 payload（管理员视角）。
// 不存在时返回 nil（handler 应当返回空 payload fallback，而非 404）。
func (s *ExtensionConfigService) GetAdmin(ctx context.Context, agentID string) (*ExtensionConfigRecord, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, ErrExtensionConfigAgentRequired
	}
	rec, err := s.repo.GetByAgentID(ctx, agentID)
	if err != nil {
		if errors.Is(err, ErrExtensionConfigNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rec, nil
}

// Upsert 覆盖式更新（按 agent_id）。
// payload 会经过完整校验：endpoints 来源、group_ids 有效性、group_models key 一致性等。
func (s *ExtensionConfigService) Upsert(
	ctx context.Context,
	agentID string,
	payload domain.ExtensionConfigPayload,
	updatedBy *int64,
) (*ExtensionConfigRecord, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, ErrExtensionConfigAgentRequired
	}
	if err := s.validatePayload(ctx, agentID, &payload); err != nil {
		return nil, err
	}
	rec, err := s.repo.Upsert(ctx, agentID, payload, updatedBy)
	if err != nil {
		return nil, err
	}
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return rec, nil
}

// Delete 清空配置（删除整行）。
func (s *ExtensionConfigService) Delete(ctx context.Context, agentID string) error {
	if strings.TrimSpace(agentID) == "" {
		return ErrExtensionConfigAgentRequired
	}
	if err := s.repo.Delete(ctx, agentID); err != nil {
		return err
	}
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return nil
}

// SetOnUpdateCallback 注册当 extension_config 发生 upsert/delete 时触发的回调，
// 用于失效依赖 extension_config 数据的缓存（目前是 router 层 CSP frame-src 缓存）。
// 与 SettingService.SetOnUpdateCallback 同模式；启动期单次设置，运行期只读。
func (s *ExtensionConfigService) SetOnUpdateCallback(cb func()) {
	s.onUpdate = cb
}

// ListAllOneboolOrigins 返回所有 extension_config 记录的 onebool_origin 去重列表。
// 用于动态注入 CSP frame-src。无效 origin（非 http(s) 或含 path/query）会被跳过。
func (s *ExtensionConfigService) ListAllOneboolOrigins(ctx context.Context) ([]string, error) {
	records, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, rec := range records {
		raw := rec.Payload.OneboolOrigin
		if raw == "" {
			continue
		}
		// extractOriginFromURL 只取 scheme+host,会清洗存量含 path/query 的旧数据;
		// validateOneboolOrigin 在写入路径上做严格校验,二者是"宽进严出"的有意配合。
		origin := extractOriginFromURL(raw)
		if origin == "" {
			continue
		}
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		out = append(out, origin)
	}
	return out, nil
}

// ====== User: GetForUser / EnsureKey ======

// GetForUser 返回普通用户视角的可见配置（已按 user_allowed_groups 过滤）。
// 配置不存在时返回 nil；配置存在但 image_gen 为空时仍返回非 nil（用于回传 parent_origin）。
func (s *ExtensionConfigService) GetForUser(
	ctx context.Context,
	userID int64,
	agentID string,
) (*VisibleExtensionConfig, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, ErrExtensionConfigAgentRequired
	}
	rec, err := s.repo.GetByAgentID(ctx, agentID)
	if err != nil {
		if errors.Is(err, ErrExtensionConfigNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	out := &VisibleExtensionConfig{OneboolOrigin: rec.Payload.OneboolOrigin}
	if rec.Payload.ImageGen == nil {
		return out, nil
	}
	cfg := rec.Payload.ImageGen

	// 1. user 授权 group 列表
	allowedGroupIDs, err := s.userAllowedLister.ListAllowedGroupIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load user allowed groups: %w", err)
	}
	allowedSet := make(map[int64]struct{}, len(allowedGroupIDs))
	for _, id := range allowedGroupIDs {
		allowedSet[id] = struct{}{}
	}

	// 2. 过滤可见 group
	visibleGroupIDs := make([]int64, 0, len(cfg.EnabledGroupIDs))
	for _, gid := range cfg.EnabledGroupIDs {
		if _, ok := allowedSet[gid]; ok {
			visibleGroupIDs = append(visibleGroupIDs, gid)
		}
	}

	// 3. 拼装 groups + models
	groups := make([]VisibleGroup, 0, len(visibleGroupIDs))
	for _, gid := range visibleGroupIDs {
		g, err := s.groupRepo.GetByID(ctx, gid)
		if err != nil || g == nil {
			continue
		}
		models := append([]string{}, cfg.GroupModels[strconv.FormatInt(gid, 10)]...)
		groups = append(groups, VisibleGroup{
			ID:          g.ID,
			Name:        g.Name,
			Description: g.Description,
			Models:      models,
		})
	}

	// 4. 拼装 endpoints
	endpointPool, err := s.loadCustomEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("load custom endpoints: %w", err)
	}
	endpoints := make([]VisibleEndpoint, 0, len(cfg.EnabledEndpointNames))
	for _, name := range cfg.EnabledEndpointNames {
		if url, ok := endpointPool[name]; ok {
			endpoints = append(endpoints, VisibleEndpoint{Name: name, Endpoint: url})
		}
	}

	out.ImageGen = &VisibleImageGenConfig{
		Endpoints:           endpoints,
		DefaultEndpointName: cfg.DefaultEndpointName,
		Groups:              groups,
	}
	return out, nil
}

// EnsureKey 根据当前用户 + group_id (+ 可选 endpoint_name) 查找或创建一把 active key。
// 校验：必须在 workbench whitelist + user_allowed_groups 双交集内。
func (s *ExtensionConfigService) EnsureKey(
	ctx context.Context,
	userID int64,
	agentID string,
	groupID int64,
	endpointName *string,
) (*EnsureKeyResult, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, ErrExtensionConfigAgentRequired
	}
	// 1. 可见性双重校验
	rec, err := s.repo.GetByAgentID(ctx, agentID)
	if err != nil {
		if errors.Is(err, ErrExtensionConfigNotFound) {
			return nil, ErrExtensionConfigGroupNotVisible
		}
		return nil, err
	}
	if rec == nil || rec.Payload.ImageGen == nil {
		return nil, ErrExtensionConfigGroupNotVisible
	}
	cfg := rec.Payload.ImageGen
	if !int64InSlice(groupID, cfg.EnabledGroupIDs) {
		return nil, ErrExtensionConfigGroupNotVisible
	}
	allowed, err := s.userAllowedLister.ListAllowedGroupIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load user allowed groups: %w", err)
	}
	if !int64InSlice(groupID, allowed) {
		return nil, ErrExtensionConfigGroupNotVisible
	}

	// 2. 解析 baseURL（endpoint_name 优先，否则用 default）
	resolvedEP := ""
	if endpointName != nil && *endpointName != "" {
		resolvedEP = *endpointName
	} else if cfg.DefaultEndpointName != "" {
		resolvedEP = cfg.DefaultEndpointName
	}
	// resolvedEP 不为空时必须存在于 enabled_endpoint_names
	if resolvedEP != "" && !stringInSlice(resolvedEP, cfg.EnabledEndpointNames) {
		return nil, ErrExtensionConfigInvalidEndpoint
	}
	baseURL, err := s.resolveBaseURL(ctx, resolvedEP)
	if err != nil {
		return nil, err
	}

	// 3. 查 group 元数据（拿 name）
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil || group == nil {
		return nil, ErrExtensionConfigInvalidGroup
	}

	// 4. 查现有 active key（按 user + group）
	existing, err := s.findActiveKeyByUserAndGroup(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &EnsureKeyResult{
			APIKey:       existing.Key,
			KeyID:        existing.ID,
			BaseURL:      baseURL,
			GroupID:      groupID,
			GroupName:    group.Name,
			Created:      false,
			EndpointName: resolvedEP,
		}, nil
	}

	// 5. 自动创建一把
	autoName := fmt.Sprintf("auto:%s:%s", agentID, group.Name)
	gid := groupID
	created, err := s.apiKeyService.Create(ctx, userID, CreateAPIKeyRequest{
		Name:    autoName,
		GroupID: &gid,
	})
	if err != nil {
		return nil, fmt.Errorf("auto create api key: %w", err)
	}
	return &EnsureKeyResult{
		APIKey:       created.Key,
		KeyID:        created.ID,
		BaseURL:      baseURL,
		GroupID:      groupID,
		GroupName:    group.Name,
		Created:      true,
		EndpointName: resolvedEP,
	}, nil
}

// ====== Helpers ======

// validateOneboolOrigin 校验 OneboolOrigin 字段必须为 http(s) origin（无 path/query/fragment）。
func (s *ExtensionConfigService) validateOneboolOrigin(p *domain.ExtensionConfigPayload) error {
	raw := p.OneboolOrigin
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%w: onebool_origin must be a valid URL", ErrExtensionConfigInvalidPayload)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: onebool_origin scheme must be http or https", ErrExtensionConfigInvalidPayload)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("%w: onebool_origin must not contain path", ErrExtensionConfigInvalidPayload)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: onebool_origin must not contain query or fragment", ErrExtensionConfigInvalidPayload)
	}
	return nil
}

func (s *ExtensionConfigService) validatePayload(
	ctx context.Context,
	agentID string,
	p *domain.ExtensionConfigPayload,
) error {
	if err := s.validateOneboolOrigin(p); err != nil {
		return err
	}
	// 仅 image-gen 详细校验。其他 agent 后续扩展时同步加。
	if agentID == AgentIDImageGen && p.ImageGen != nil {
		cfg := p.ImageGen

		// endpoints 校验
		if len(cfg.EnabledEndpointNames) > 0 {
			pool, err := s.loadCustomEndpoints(ctx)
			if err != nil {
				return err
			}
			for _, name := range cfg.EnabledEndpointNames {
				if _, ok := pool[name]; !ok {
					return fmt.Errorf("%w: endpoint %q not found in settings.custom_endpoints", ErrExtensionConfigInvalidPayload, name)
				}
			}
		}
		if cfg.DefaultEndpointName != "" && !stringInSlice(cfg.DefaultEndpointName, cfg.EnabledEndpointNames) {
			return fmt.Errorf("%w: default_endpoint_name not in enabled_endpoint_names", ErrExtensionConfigInvalidPayload)
		}

		// groups 校验
		for _, gid := range cfg.EnabledGroupIDs {
			g, err := s.groupRepo.GetByID(ctx, gid)
			if err != nil || g == nil {
				return fmt.Errorf("%w: group %d not found", ErrExtensionConfigInvalidPayload, gid)
			}
			if g.Status != StatusActive {
				return fmt.Errorf("%w: group %d not active", ErrExtensionConfigInvalidPayload, gid)
			}
		}

		// group_models 校验
		for key, models := range cfg.GroupModels {
			gid, err := strconv.ParseInt(key, 10, 64)
			if err != nil {
				return fmt.Errorf("%w: group_models key %q is not a valid int64", ErrExtensionConfigInvalidPayload, key)
			}
			if !int64InSlice(gid, cfg.EnabledGroupIDs) {
				return fmt.Errorf("%w: group_models references group %d not in enabled_group_ids", ErrExtensionConfigInvalidPayload, gid)
			}
			if len(models) > 50 {
				return fmt.Errorf("%w: group_models[%d] exceeds 50 entries", ErrExtensionConfigInvalidPayload, gid)
			}
			for _, m := range models {
				if l := len(m); l == 0 || l > 100 {
					return fmt.Errorf("%w: group_models[%d] contains invalid model name length=%d", ErrExtensionConfigInvalidPayload, gid, l)
				}
			}
		}
	}
	return nil
}

// loadCustomEndpoints 从 settings.custom_endpoints 拉出 name -> endpoint 映射。
// 解析 settings JSON：SystemSettings.CustomEndpoints 是 JSON 字符串。
func (s *ExtensionConfigService) loadCustomEndpoints(ctx context.Context) (map[string]string, error) {
	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil {
		return nil, err
	}
	endpoints, err := parseCustomEndpoints(settings.CustomEndpoints)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(endpoints))
	for _, ep := range endpoints {
		out[ep.Name] = ep.Endpoint
	}
	return out, nil
}

// customEndpointEntry 对应 settings.custom_endpoints[] 元素。
// 形状与前端 CustomEndpoint 类型保持一致：{ name, endpoint, description }。
type customEndpointEntry struct {
	Name        string `json:"name"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description,omitempty"`
}

// parseCustomEndpoints 接受 SystemSettings.CustomEndpoints（JSON 字符串）。
// 空字符串视为空数组。
func parseCustomEndpoints(raw string) ([]customEndpointEntry, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil
	}
	var list []customEndpointEntry
	if err := json.Unmarshal([]byte(s), &list); err != nil {
		return nil, fmt.Errorf("parse custom_endpoints: %w", err)
	}
	return list, nil
}

// resolveBaseURL：endpoint name 为空 → 用 settings.api_base_url；否则用 endpoint 实际 URL。
// 返回值会自动追加 /v1（与 OneboolFlowEmbed.vue 保持一致）。
func (s *ExtensionConfigService) resolveBaseURL(ctx context.Context, endpointName string) (string, error) {
	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil {
		return "", err
	}
	var rawBase string
	if endpointName == "" {
		rawBase = settings.APIBaseURL
	} else {
		eps, perr := parseCustomEndpoints(settings.CustomEndpoints)
		if perr != nil {
			return "", perr
		}
		for _, ep := range eps {
			if ep.Name == endpointName {
				rawBase = ep.Endpoint
				break
			}
		}
		if rawBase == "" {
			return "", ErrExtensionConfigInvalidEndpoint
		}
	}
	rawBase = strings.TrimRight(rawBase, "/")
	if rawBase == "" {
		return "", fmt.Errorf("%w: resolved base URL is empty", ErrExtensionConfigInvalidEndpoint)
	}
	return rawBase + "/v1", nil
}

// findActiveKeyByUserAndGroup 在 api_keys 中按 user+group 查 active key（取首条）。
// 直接调用 apiKeyRepo.ListByUserID + 内存过滤 group_id（避免新加专用 query 方法）。
func (s *ExtensionConfigService) findActiveKeyByUserAndGroup(
	ctx context.Context,
	userID int64,
	groupID int64,
) (*APIKey, error) {
	// 用现有的 ListByUserID(filters) — pageSize 大一些，足够覆盖单用户的 active key 数量。
	// NOTE: APIKeyListFilters 假定支持 Status 字段；wire 阶段如果字段名不同需调整。
	pageParams := pagination.PaginationParams{Page: 1, PageSize: 200}
	filters := APIKeyListFilters{Status: StatusActive}
	keys, _, err := s.apiKeyRepo.ListByUserID(ctx, userID, pageParams, filters)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		k := keys[i]
		if k.GroupID != nil && *k.GroupID == groupID && k.Status == StatusActive {
			return &k, nil
		}
	}
	return nil, nil
}

func int64InSlice(target int64, list []int64) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func stringInSlice(target string, list []string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
