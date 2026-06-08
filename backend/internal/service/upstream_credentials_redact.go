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

// UpstreamIdentityCredentialKeys 非敏感、但属于「账号身份/续期凭证」的键。
// 合并时与敏感键一样:incoming 未提供则保留 existing。原因:自动续期(Login/CF 过盾)
// 只回传 {access_token,user_id} / {cf_*} 等部分产物、从不含 username,若按"非敏感键由
// incoming 决定"的旧语义合并,会把 username 抹掉,导致此后再也无法自动登录续期、永久
// credential_error。注意:这些键不脱敏,RedactUpstreamCredentials 仍照常返回前端。
var UpstreamIdentityCredentialKeys = []string{"username", "user_id"}

// upstreamPreserveWhenOmittedKeys 合并时 incoming 未提供即保留 existing 的键集合
// = 敏感键 ∪ 身份键。
var upstreamPreserveWhenOmittedKeys = append(
	append([]string{}, UpstreamSensitiveCredentialKeys...),
	UpstreamIdentityCredentialKeys...,
)

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

// MergeUpstreamCredentials PUT/续期合并:敏感键与身份键(username/user_id)在 incoming
// 未提供时保留 existing、显式提供则覆盖;其余非敏感键完全由 incoming 决定。返回新 map。
func MergeUpstreamCredentials(existing, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(incoming)+len(upstreamPreserveWhenOmittedKeys))
	for k, v := range incoming {
		out[k] = v
	}
	for _, key := range upstreamPreserveWhenOmittedKeys {
		if _, hasIncoming := incoming[key]; hasIncoming {
			continue
		}
		if existingVal, ok := existing[key]; ok {
			out[key] = existingVal
		}
	}
	return out
}
