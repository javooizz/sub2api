package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

const newapiDefaultQuotaPerUnit = 500000.0

// NewAPIAdapter new-api 适配器(spec §5.2)。
type NewAPIAdapter struct{}

func NewNewAPIAdapter() *NewAPIAdapter { return &NewAPIAdapter{} }

// newapiEnvelope new-api 标准响应包。
type newapiEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (a *NewAPIAdapter) authHeaders(p *UpstreamProvider) map[string]string {
	h := map[string]string{}
	if tok, _ := p.Credentials["access_token"].(string); tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	switch v := p.Credentials["user_id"].(type) {
	case float64:
		h["New-Api-User"] = strconv.FormatInt(int64(v), 10)
	case int64:
		h["New-Api-User"] = strconv.FormatInt(v, 10)
	case string:
		h["New-Api-User"] = v
	}
	return h
}

// Login 账密登录换 system access token(spec §5.2)。
// turnstileToken 非空时附加 turnstile 参数;登录页要求 Turnstile 而未提供 → errUpstreamTurnstile。
func (a *NewAPIAdapter) Login(ctx context.Context, p *UpstreamProvider, turnstileToken string) (map[string]any, error) {
	client, err := newUpstreamHTTPClient(p)
	if err != nil {
		return nil, err
	}
	username, _ := p.Credentials["username"].(string)
	password, _ := p.Credentials["password"].(string)
	if username == "" || password == "" {
		return nil, errUpstreamAuth
	}
	path := "/api/user/login"
	if turnstileToken != "" {
		path += "?turnstile=" + turnstileToken
	}
	// 登录拿 session cookie:需要 cookie jar
	jar := newCookieJar()
	client.client.Jar = jar
	var env newapiEnvelope
	_, err = client.doJSON(ctx, http.MethodPost, path,
		map[string]string{"username": username, "password": password}, nil, &env)
	if err != nil {
		return nil, err
	}
	if !env.Success {
		// new-api 在需要 Turnstile 时返回 success=false + 相应 message
		if turnstileToken == "" {
			return nil, errUpstreamTurnstile
		}
		return nil, errUpstreamAuth
	}
	var user struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &user); err != nil || user.ID == 0 {
		return nil, fmt.Errorf("登录响应缺少用户 ID")
	}
	// session → system access token
	var tokEnv newapiEnvelope
	_, err = client.doJSON(ctx, http.MethodGet, "/api/user/token", nil, nil, &tokEnv)
	if err != nil || !tokEnv.Success {
		return nil, fmt.Errorf("兑换 access token 失败")
	}
	var accessToken string
	if err := json.Unmarshal(tokEnv.Data, &accessToken); err != nil {
		return nil, fmt.Errorf("解析 access token 失败")
	}
	return map[string]any{
		"access_token": accessToken,
		"user_id":      user.ID,
	}, nil
}

func (a *NewAPIAdapter) FetchSnapshot(ctx context.Context, p *UpstreamProvider) (*domain.UpstreamSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamSnapshotTimeout)
	defer cancel()
	client, err := newUpstreamHTTPClient(p)
	if err != nil {
		return nil, err
	}
	snap := &domain.UpstreamSnapshot{FetchedAt: time.Now().UTC(), Currency: "USD"}

	// 1) quota_per_unit 探测(失败回退默认,标 partial)
	quotaPerUnit := newapiDefaultQuotaPerUnit
	var statusEnv newapiEnvelope
	if _, err := client.doJSON(ctx, http.MethodGet, "/api/status", nil, nil, &statusEnv); err == nil && statusEnv.Success {
		var status struct {
			QuotaPerUnit float64 `json:"quota_per_unit"`
		}
		if json.Unmarshal(statusEnv.Data, &status) == nil && status.QuotaPerUnit > 0 {
			quotaPerUnit = status.QuotaPerUnit
		} else {
			snap.Partial = true
		}
	} else {
		snap.Partial = true
	}

	// 2) 用户/余额(认证失败必须上抛——凭证续期依赖该分类)
	var selfEnv newapiEnvelope
	if _, err := client.doJSON(ctx, http.MethodGet, "/api/user/self", nil, a.authHeaders(p), &selfEnv); err != nil {
		return nil, err
	}
	var self struct {
		ID       int64   `json:"id"`
		Username string  `json:"username"`
		Quota    float64 `json:"quota"`
	}
	if selfEnv.Success && json.Unmarshal(selfEnv.Data, &self) == nil {
		balance := self.Quota / quotaPerUnit
		snap.Balance = &balance
		snap.UserInfo = &domain.UpstreamUserInfo{ID: self.ID, Username: self.Username}
	} else {
		snap.Partial = true
	}

	// 3) 用户可用模型(失败仅 partial)
	userModels := map[string]struct{}{}
	var modelsEnv newapiEnvelope
	if _, err := client.doJSON(ctx, http.MethodGet, "/api/user/models", nil, a.authHeaders(p), &modelsEnv); err == nil && modelsEnv.Success {
		var models []string
		if json.Unmarshal(modelsEnv.Data, &models) == nil {
			for _, m := range models {
				userModels[m] = struct{}{}
			}
		}
	} else {
		snap.Partial = true
	}

	// 4) pricing:模型价格 + enable_groups + group_ratio(公开接口,失败仅 partial)
	var pricingResp struct {
		Success    bool               `json:"success"`
		Data       []json.RawMessage  `json:"data"`
		GroupRatio map[string]float64 `json:"group_ratio"`
	}
	if _, err := client.doJSON(ctx, http.MethodGet, "/api/pricing", nil, a.authHeaders(p), &pricingResp); err == nil && pricingResp.Success {
		snap.ModelPricing = map[string]json.RawMessage{}
		groupModels := map[string][]string{}
		for _, raw := range pricingResp.Data {
			var entry struct {
				ModelName    string   `json:"model_name"`
				EnableGroups []string `json:"enable_groups"`
			}
			if json.Unmarshal(raw, &entry) != nil || entry.ModelName == "" {
				continue
			}
			snap.ModelPricing[entry.ModelName] = raw
			// 交集口径:用户可用 ∩ 分组启用(用户模型列表缺失时跳过交集过滤)
			if len(userModels) > 0 {
				if _, ok := userModels[entry.ModelName]; !ok {
					continue
				}
			}
			for _, g := range entry.EnableGroups {
				groupModels[g] = append(groupModels[g], entry.ModelName)
			}
		}
		// 分组集合 = group_ratio ∪ enable_groups
		groupSet := map[string]struct{}{}
		for g := range pricingResp.GroupRatio {
			groupSet[g] = struct{}{}
		}
		for g := range groupModels {
			groupSet[g] = struct{}{}
		}
		names := make([]string, 0, len(groupSet))
		for g := range groupSet {
			names = append(names, g)
		}
		sort.Strings(names)
		for _, g := range names {
			gs := domain.UpstreamGroupSnapshot{Name: g}
			if ratio, ok := pricingResp.GroupRatio[g]; ok {
				r := ratio
				gs.Ratio = &r
			}
			models := groupModels[g]
			sort.Strings(models)
			gs.Models = models
			snap.Groups = append(snap.Groups, gs)
		}
	} else {
		snap.Partial = true
	}

	return snap, nil
}

func (a *NewAPIAdapter) ListTokens(ctx context.Context, p *UpstreamProvider) ([]UpstreamToken, error) {
	client, err := newUpstreamHTTPClient(p)
	if err != nil {
		return nil, err
	}
	var env newapiEnvelope
	if _, err := client.doJSON(ctx, http.MethodGet, "/api/token/?p=1&size=100", nil, a.authHeaders(p), &env); err != nil {
		return nil, err
	}
	if !env.Success {
		return nil, fmt.Errorf("上游返回失败")
	}
	// 兼容两种形状:{items:[...]} 或直接数组
	var wrapper struct {
		Items []map[string]any `json:"items"`
	}
	var list []map[string]any
	if json.Unmarshal(env.Data, &wrapper) == nil && wrapper.Items != nil {
		list = wrapper.Items
	} else if err := json.Unmarshal(env.Data, &list); err != nil {
		return nil, fmt.Errorf("解析 token 列表失败")
	}
	out := make([]UpstreamToken, 0, len(list))
	for _, item := range list {
		out = append(out, newapiTokenFromMap(item, false))
	}
	return out, nil
}

func (a *NewAPIAdapter) CreateToken(ctx context.Context, p *UpstreamProvider, req CreateUpstreamTokenRequest) (*UpstreamToken, error) {
	client, err := newUpstreamHTTPClient(p)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"name":            req.Name,
		"remain_quota":    0,
		"unlimited_quota": true,
		"expired_time":    -1, // 永不过期
	}
	if req.Group != "" {
		payload["group"] = req.Group
	}
	var env newapiEnvelope
	if _, err := client.doJSON(ctx, http.MethodPost, "/api/token/", payload, a.authHeaders(p), &env); err != nil {
		return nil, err
	}
	if !env.Success {
		return nil, fmt.Errorf("上游创建 token 失败: %s", env.Message)
	}
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, fmt.Errorf("解析创建响应失败")
	}
	tok := newapiTokenFromMap(data, true)
	return &tok, nil
}

func newapiTokenFromMap(m map[string]any, includeKey bool) UpstreamToken {
	tok := UpstreamToken{Raw: m}
	tok.ID = m["id"]
	tok.Name, _ = m["name"].(string)
	tok.Group, _ = m["group"].(string)
	if includeKey {
		tok.Key, _ = m["key"].(string)
	}
	if exp, ok := m["expired_time"].(float64); ok && exp > 0 {
		t := time.Unix(int64(exp), 0).UTC()
		tok.ExpiresAt = &t
	}
	return tok
}
