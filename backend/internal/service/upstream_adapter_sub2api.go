package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Sub2APIAdapter 上游 sub2api 适配器(spec §5.3)。
// 响应形状 {code:0,message,data} 与本仓库一致。
type Sub2APIAdapter struct{}

func NewSub2APIAdapter() *Sub2APIAdapter { return &Sub2APIAdapter{} }

func (a *Sub2APIAdapter) authHeaders(p *UpstreamProvider) map[string]string {
	h := map[string]string{}
	if tok, _ := p.Credentials["access_token"].(string); tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	return h
}

// Login 账密换 JWT;响应含 2FA 挑战标识 → errUpstream2FA(spec §5.3)。
// R1.8: 成功响应字段名为 access_token(非 token);2FA 场景检测 requires_2fa 布尔字段。
func (a *Sub2APIAdapter) Login(ctx context.Context, p *UpstreamProvider, _ string) (map[string]any, error) {
	client, err := newUpstreamHTTPClient(p)
	if err != nil {
		return nil, err
	}
	username, _ := p.Credentials["username"].(string)
	password, _ := p.Credentials["password"].(string)
	if username == "" || password == "" {
		return nil, errUpstreamAuth
	}
	var env sub2apiEnvelope
	if _, err := client.doJSON(ctx, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": username, "password": password}, nil, &env); err != nil {
		return nil, err
	}
	if env.Code != 0 {
		return nil, errUpstreamAuth
	}
	// R1.8: 先检测 requires_2fa 布尔;成功路径取 access_token(非 token)
	var data struct {
		AccessToken string `json:"access_token"`
		Requires2FA bool   `json:"requires_2fa"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, fmt.Errorf("解析登录响应失败")
	}
	if data.Requires2FA {
		return nil, errUpstream2FA
	}
	if data.AccessToken == "" {
		return nil, errUpstreamAuth
	}
	creds := map[string]any{"access_token": data.AccessToken}
	if exp := jwtExpiry(data.AccessToken); exp != nil {
		creds["token_expires_at"] = exp.UTC().Format(time.RFC3339)
	}
	return creds, nil
}

func (a *Sub2APIAdapter) FetchSnapshot(ctx context.Context, p *UpstreamProvider) (*domain.UpstreamSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamSnapshotTimeout)
	defer cancel()
	client, err := newUpstreamHTTPClient(p)
	if err != nil {
		return nil, err
	}
	snap := &domain.UpstreamSnapshot{FetchedAt: time.Now().UTC(), Currency: "USD"}

	// 1) 余额/用户(认证失败上抛供续期)
	var profEnv sub2apiEnvelope
	if _, err := client.doJSON(ctx, http.MethodGet, "/api/v1/user/profile", nil, a.authHeaders(p), &profEnv); err != nil {
		return nil, err
	}
	var prof struct {
		ID       int64   `json:"id"`
		Username string  `json:"username"`
		Balance  float64 `json:"balance"`
	}
	if profEnv.Code == 0 && json.Unmarshal(profEnv.Data, &prof) == nil {
		balance := prof.Balance
		snap.Balance = &balance
		snap.UserInfo = &domain.UpstreamUserInfo{ID: prof.ID, Username: prof.Username}
	} else {
		snap.Partial = true
	}

	// 2) 分组+倍率(失败仅 partial)
	groupRatios := map[string]*float64{}
	groupDesc := map[string]string{}
	var groupNames []string
	var grpEnv sub2apiEnvelope
	if _, err := client.doJSON(ctx, http.MethodGet, "/api/v1/groups/available", nil, a.authHeaders(p), &grpEnv); err == nil && grpEnv.Code == 0 {
		var groups []struct {
			Name           string   `json:"name"`
			RateMultiplier *float64 `json:"rate_multiplier"`
			Description    string   `json:"description"`
		}
		if json.Unmarshal(grpEnv.Data, &groups) == nil {
			for _, g := range groups {
				groupNames = append(groupNames, g.Name)
				groupRatios[g.Name] = g.RateMultiplier
				groupDesc[g.Name] = g.Description
			}
		}
	} else {
		snap.Partial = true
	}

	// 3) 分组可用模型:model-plaza 倒排(未开启 → partial,模型置空,spec §5.3)
	groupModels := map[string][]string{}
	var plazaEnv sub2apiEnvelope
	if _, err := client.doJSON(ctx, http.MethodGet, "/api/v1/model-plaza", nil, a.authHeaders(p), &plazaEnv); err == nil && plazaEnv.Code == 0 {
		var plaza struct {
			Enabled bool `json:"enabled"`
			Models  []struct {
				Name   string `json:"name"`
				Groups []struct {
					Name string `json:"name"`
				} `json:"groups"`
			} `json:"models"`
		}
		if json.Unmarshal(plazaEnv.Data, &plaza) == nil && plaza.Enabled {
			for _, m := range plaza.Models {
				for _, g := range m.Groups {
					groupModels[g.Name] = append(groupModels[g.Name], m.Name)
				}
			}
		} else {
			snap.Partial = true
		}
	} else {
		snap.Partial = true
	}

	sort.Strings(groupNames)
	for _, name := range groupNames {
		models := groupModels[name]
		sort.Strings(models)
		snap.Groups = append(snap.Groups, domain.UpstreamGroupSnapshot{
			Name: name, Ratio: groupRatios[name],
			Description: groupDesc[name], Models: models,
		})
	}
	return snap, nil
}

func (a *Sub2APIAdapter) ListTokens(ctx context.Context, p *UpstreamProvider) ([]UpstreamToken, error) {
	client, err := newUpstreamHTTPClient(p)
	if err != nil {
		return nil, err
	}
	var env sub2apiEnvelope
	if _, err := client.doJSON(ctx, http.MethodGet, "/api/v1/keys", nil, a.authHeaders(p), &env); err != nil {
		return nil, err
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("上游返回失败")
	}
	var wrapper struct {
		Items []map[string]any `json:"items"`
	}
	var list []map[string]any
	if json.Unmarshal(env.Data, &wrapper) == nil && wrapper.Items != nil {
		list = wrapper.Items
	} else if err := json.Unmarshal(env.Data, &list); err != nil {
		return nil, fmt.Errorf("解析 key 列表失败")
	}
	out := make([]UpstreamToken, 0, len(list))
	for _, item := range list {
		tok := UpstreamToken{Raw: item}
		tok.ID = item["id"]
		tok.Name, _ = item["name"].(string)
		out = append(out, tok)
	}
	return out, nil
}

func (a *Sub2APIAdapter) CreateToken(ctx context.Context, p *UpstreamProvider, req CreateUpstreamTokenRequest) (*UpstreamToken, error) {
	client, err := newUpstreamHTTPClient(p)
	if err != nil {
		return nil, err
	}
	var env sub2apiEnvelope
	if _, err := client.doJSON(ctx, http.MethodPost, "/api/v1/keys",
		map[string]any{"name": req.Name}, a.authHeaders(p), &env); err != nil {
		return nil, err
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("上游创建 key 失败: %s", env.Message)
	}
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, fmt.Errorf("解析创建响应失败")
	}
	tok := UpstreamToken{Raw: data}
	tok.ID = data["id"]
	tok.Name, _ = data["name"].(string)
	tok.Key, _ = data["key"].(string)
	return &tok, nil
}

// jwtExpiry 解析 JWT exp(不验签,仅取过期时间);失败返回 nil。
// import: encoding/base64, strings(已在本文件顶部)。
// 测试中的 "jwt-abc" 非三段式 → 返回 nil → token_expires_at 不写入,符合预期。
func jwtExpiry(token string) *time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return nil
	}
	t := time.Unix(claims.Exp, 0)
	return &t
}

// sub2apiEnvelope 是 sub2api 标准响应包(与本仓库 response.Success 一致)。
// 注意:newapi 适配器使用 newapiEnvelope(success bool),此处单独定义避免混淆。
type sub2apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}
