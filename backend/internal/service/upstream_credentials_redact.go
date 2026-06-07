package service

// UpstreamSensitiveCredentialKeys 上游站点凭证中绝不允许返回到前端的子键。
// 独立于 account 的 SensitiveCredentialKeys(不改上游文件)。
var UpstreamSensitiveCredentialKeys = []string{
	"password", "access_token", "cf_clearance", "cf_user_agent",
}

var upstreamSensitiveKeySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(UpstreamSensitiveCredentialKeys))
	for _, k := range UpstreamSensitiveCredentialKeys {
		m[k] = struct{}{}
	}
	return m
}()

// UpstreamCredentialStatus 脱敏响应附带的状态信息(spec §9)。
type UpstreamCredentialStatus struct {
	HasPassword     bool   `json:"has_password"`
	HasAccessToken  bool   `json:"has_access_token"`
	AccessTokenTail string `json:"access_token_tail,omitempty"`
}

// RedactUpstreamCredentials 返回剥离敏感键的拷贝 + 状态。不修改入参。
func RedactUpstreamCredentials(in map[string]any) (map[string]any, UpstreamCredentialStatus) {
	out := make(map[string]any, len(in))
	status := UpstreamCredentialStatus{}
	for k, v := range in {
		if _, sensitive := upstreamSensitiveKeySet[k]; sensitive {
			continue
		}
		out[k] = v
	}
	if s, ok := in["password"].(string); ok && s != "" {
		status.HasPassword = true
	}
	if s, ok := in["access_token"].(string); ok && s != "" {
		status.HasAccessToken = true
		if len(s) > 8 {
			status.AccessTokenTail = s[len(s)-4:]
		}
	}
	return out, status
}

// MergeUpstreamCredentials PUT 合并:敏感键 incoming 未提供则保留 existing,
// 显式提供则覆盖;非敏感键完全由 incoming 决定。返回新 map。
func MergeUpstreamCredentials(existing, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(incoming)+len(UpstreamSensitiveCredentialKeys))
	for k, v := range incoming {
		out[k] = v
	}
	for _, key := range UpstreamSensitiveCredentialKeys {
		if _, hasIncoming := incoming[key]; hasIncoming {
			continue
		}
		if existingVal, ok := existing[key]; ok {
			out[key] = existingVal
		}
	}
	return out
}
