package domain

// ExtensionConfigPayload 是 extension_configs.payload jsonb 字段的强类型表达。
// 顶层按 agent_id 路由到不同子对象；全部 omitempty 保证向后兼容。
// 关联文档：onebool-flow/docs/integration-protocol.md §9。
type ExtensionConfigPayload struct {
	// Version 协议版本（v1 起为 1），便于未来 break change 兼容判别。
	Version int `json:"version,omitempty"`

	// OneboolOrigin onebool-flow 的部署 origin（如 https://image.sub2api.com 或 http://localhost:5173）。
	// sub2api 用它：
	//   - 构造 iframe src（取代环境变量 VITE_ONEBOOL_ORIGIN）
	//   - postMessage target origin
	// 空字符串 → sub2api 前端 fallback 到 VITE_ONEBOOL_ORIGIN env / 'http://localhost:5173'。
	OneboolOrigin string `json:"onebool_origin,omitempty"`

	// ImageGen 当 agent_id == "image-gen" 时生效。
	ImageGen *ImageGenExtensionConfig `json:"image_gen,omitempty"`

	// ModelPlaza 当 agent_id == "model-plaza" 时生效。
	ModelPlaza *ModelPlazaExtensionConfig `json:"model_plaza,omitempty"`

	// 未来扩展其他智能体时在此追加同级字段，例如：
	// PPTGen *PPTGenExtensionConfig `json:"ppt_gen,omitempty"`
}

// ImageGenExtensionConfig 描述 image-gen 智能体允许使用的端点池、分组白名单
// 和每个分组下暴露的模型列表。
type ImageGenExtensionConfig struct {
	// EnabledEndpointNames 可用端点 name 列表。
	// 每个 name 必须存在于 system_setting.custom_endpoints[].name。
	EnabledEndpointNames []string `json:"enabled_endpoint_names,omitempty"`

	// DefaultEndpointName 默认端点 name（必须在 EnabledEndpointNames 内）。
	// 空字符串表示使用 system_setting.api_base_url。
	DefaultEndpointName string `json:"default_endpoint_name,omitempty"`

	// EnabledGroupIDs 启用的 group id 白名单。
	// 用户实际可见 = 此集合 ∩ user_allowed_groups。
	EnabledGroupIDs []int64 `json:"enabled_group_ids,omitempty"`

	// GroupModels 每个分组下暴露给用户的模型字符串列表。
	// key 是字符串化 group_id（json 不支持 int key），value 元素长度 1-100，数组长度 ≤ 50。
	GroupModels map[string][]string `json:"group_models,omitempty"`
}

// ModelPlazaExtensionConfig 模型广场展示配置。
// 只影响广场展示，不影响绑定/计费权限（绑定权限由 GetAvailableGroups 把关）。
type ModelPlazaExtensionConfig struct {
	// ExcludedChannelIDs 不在广场展示的渠道 ID 黑名单。
	// 仅校验为正整数，不校验存在性——渠道删除后黑名单残留无害（聚合时自然失配跳过）。
	ExcludedChannelIDs []int64 `json:"excluded_channel_ids,omitempty"`

	// ExcludedGroupIDs 不在广场展示的分组 ID 黑名单。同上仅校验正整数。
	ExcludedGroupIDs []int64 `json:"excluded_group_ids,omitempty"`

	// ModelDescriptions 模型名 → 展示描述。key 长度 1-100 字节，value ≤500 字符（rune）。
	ModelDescriptions map[string]string `json:"model_descriptions,omitempty"`

	// Announcement 广场顶部公告（Markdown，≤2000 字符 rune）。
	Announcement string `json:"announcement,omitempty"`
}
