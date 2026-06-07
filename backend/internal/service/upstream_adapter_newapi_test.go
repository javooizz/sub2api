package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newapiTestServer 模拟 new-api 站点(spec §5.2 端点)。
func newapiTestServer(t *testing.T, quotaPerUnit float64) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "data": map[string]any{"quota_per_unit": quotaPerUnit},
		})
	})
	mux.HandleFunc("/api/user/self", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-123" || r.Header.Get("New-Api-User") != "42" {
			w.WriteHeader(401)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "unauthorized"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]any{"id": 42, "username": "alice", "quota": 2500000.0, "group": "default"},
		})
	})
	mux.HandleFunc("/api/user/models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "data": []string{"gpt-x", "claude-y"},
		})
	})
	mux.HandleFunc("/api/pricing", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": []map[string]any{
				{"model_name": "gpt-x", "model_ratio": 1.0, "completion_ratio": 2.0, "enable_groups": []string{"default", "vip"}},
				{"model_name": "claude-y", "model_ratio": 3.0, "completion_ratio": 5.0, "enable_groups": []string{"vip"}},
			},
			"group_ratio": map[string]any{"default": 1.0, "vip": 1.5},
		})
	})
	mux.HandleFunc("/api/token/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 7, "name": "from-sub2api", "key": "nk-secret", "group": "vip"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{"items": []map[string]any{
				{"id": 7, "name": "t1", "key": "nk-1", "group": "default"},
			}},
		})
	})
	return httptest.NewServer(mux)
}

func newapiProvider(baseURL string) *UpstreamProvider {
	return &UpstreamProvider{
		ID: 1, Type: "newapi", SiteURL: baseURL,
		Credentials: map[string]any{"access_token": "tok-123", "user_id": float64(42)},
	}
}

func TestNewAPIAdapter_FetchSnapshot(t *testing.T) {
	srv := newapiTestServer(t, 500000)
	defer srv.Close()

	a := NewNewAPIAdapter()
	snap, err := a.FetchSnapshot(context.Background(), newapiProvider(srv.URL))
	if err != nil {
		t.Fatalf("采集失败: %v", err)
	}
	if snap.Balance == nil || *snap.Balance != 5.0 { // 2500000 / 500000
		t.Fatalf("余额换算不符: %v", snap.Balance)
	}
	if snap.UserInfo == nil || snap.UserInfo.Username != "alice" || snap.UserInfo.ID != 42 {
		t.Fatalf("用户信息不符: %+v", snap.UserInfo)
	}
	// 分组:default 含 gpt-x(交集 user/models),vip 含 gpt-x+claude-y
	groups := map[string][]string{}
	ratios := map[string]float64{}
	for _, g := range snap.Groups {
		groups[g.Name] = g.Models
		if g.Ratio != nil {
			ratios[g.Name] = *g.Ratio
		}
	}
	if ratios["vip"] != 1.5 || ratios["default"] != 1.0 {
		t.Fatalf("分组倍率不符: %v", ratios)
	}
	if len(groups["vip"]) != 2 || len(groups["default"]) != 1 {
		t.Fatalf("分组模型不符: %v", groups)
	}
	if len(snap.ModelPricing) != 2 {
		t.Fatalf("模型价格表不符: %d", len(snap.ModelPricing))
	}
	if snap.Partial {
		t.Fatal("全部接口成功不应 partial")
	}
}

func TestNewAPIAdapter_QuotaPerUnitFallback(t *testing.T) {
	// /api/status 不可用 → 回退 500000(spec §5.2)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	mux.HandleFunc("/api/user/self", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 42, "username": "a", "quota": 1000000.0}})
	})
	mux.HandleFunc("/api/user/models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []string{}})
	})
	mux.HandleFunc("/api/pricing", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := NewNewAPIAdapter()
	snap, err := a.FetchSnapshot(context.Background(), newapiProvider(srv.URL))
	if err != nil {
		t.Fatalf("采集失败: %v", err)
	}
	if snap.Balance == nil || *snap.Balance != 2.0 {
		t.Fatalf("回退换算不符: %v", snap.Balance)
	}
	if !snap.Partial {
		t.Fatal("status 探测失败应标记 partial")
	}
}

func TestNewAPIAdapter_AuthErrorClassified(t *testing.T) {
	srv := newapiTestServer(t, 500000)
	defer srv.Close()

	p := newapiProvider(srv.URL)
	p.Credentials["access_token"] = "wrong"
	a := NewNewAPIAdapter()
	_, err := a.FetchSnapshot(context.Background(), p)
	if !errors.Is(err, errUpstreamAuth) {
		t.Fatalf("401 应分类为 errUpstreamAuth,实际: %v", err)
	}
}

func TestNewAPIAdapter_CreateToken(t *testing.T) {
	srv := newapiTestServer(t, 500000)
	defer srv.Close()

	a := NewNewAPIAdapter()
	tok, err := a.CreateToken(context.Background(), newapiProvider(srv.URL), CreateUpstreamTokenRequest{Name: "from-sub2api", Group: "vip"})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if tok.Key != "nk-secret" || tok.Group != "vip" {
		t.Fatalf("token 不符: %+v", tok)
	}
}

func TestNewAPIAdapter_ListTokens(t *testing.T) {
	srv := newapiTestServer(t, 500000)
	defer srv.Close()

	a := NewNewAPIAdapter()
	tokens, err := a.ListTokens(context.Background(), newapiProvider(srv.URL))
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Name != "t1" {
		t.Fatalf("token 列表不符: %+v", tokens)
	}
}
