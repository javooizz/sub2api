package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// sub2apiTestServer 模拟上游 sub2api 站点(spec §5.3;响应形状 {code:0,message,data},
// 参照本仓库 internal/pkg/response 与 user_handler/api_key_handler DTO)。
// R1.8 修订:Login 成功响应使用 access_token(非 token);2FA 使用 temp_token(非 two_fa_token)。
func sub2apiTestServer(t *testing.T, with2FA bool) *httptest.Server {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, data any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
	}
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if with2FA {
			// R1.8: 2FA 场景字段名 temp_token(非 two_fa_token)
			ok(w, map[string]any{"requires_2fa": true, "temp_token": "tmp"})
			return
		}
		// R1.8: 成功响应字段 access_token(非 token)
		ok(w, map[string]any{
			"access_token":  "jwt-abc",
			"refresh_token": "r",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"user":          map[string]any{"id": 9, "username": "bob"},
		})
	})
	mux.HandleFunc("/api/v1/user/profile", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer jwt-abc" {
			w.WriteHeader(401)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 401, "message": "unauthorized"})
			return
		}
		ok(w, map[string]any{"id": 9, "username": "bob", "balance": 12.5})
	})
	mux.HandleFunc("/api/v1/groups/available", func(w http.ResponseWriter, r *http.Request) {
		ok(w, []map[string]any{
			{"id": 1, "name": "default", "rate_multiplier": 1.0, "description": "默认"},
			{"id": 2, "name": "vip", "rate_multiplier": 0.8},
		})
	})
	mux.HandleFunc("/api/v1/model-plaza", func(w http.ResponseWriter, r *http.Request) {
		ok(w, map[string]any{
			"enabled": true,
			"models": []map[string]any{
				{"name": "claude-x", "platform": "anthropic", "groups": []map[string]any{{"name": "default"}, {"name": "vip"}}},
				{"name": "gemini-y", "platform": "gemini", "groups": []map[string]any{{"name": "vip"}}},
			},
		})
	})
	mux.HandleFunc("/api/v1/keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			ok(w, map[string]any{"id": 3, "name": "from-mgr", "key": "sk-created"})
			return
		}
		ok(w, map[string]any{"items": []map[string]any{{"id": 1, "name": "k1", "key_preview": "sk-***"}}})
	})
	return httptest.NewServer(mux)
}

func sub2apiProvider(baseURL string) *UpstreamProvider {
	return &UpstreamProvider{
		ID: 2, Type: "sub2api", SiteURL: baseURL,
		Credentials: map[string]any{"access_token": "jwt-abc"},
	}
}

func TestSub2APIAdapter_FetchSnapshot(t *testing.T) {
	srv := sub2apiTestServer(t, false)
	defer srv.Close()

	a := NewSub2APIAdapter()
	snap, err := a.FetchSnapshot(context.Background(), sub2apiProvider(srv.URL))
	if err != nil {
		t.Fatalf("采集失败: %v", err)
	}
	if snap.Balance == nil || *snap.Balance != 12.5 {
		t.Fatalf("余额不符: %v", snap.Balance)
	}
	groups := map[string][]string{}
	for _, g := range snap.Groups {
		groups[g.Name] = g.Models
	}
	if len(groups["vip"]) != 2 || len(groups["default"]) != 1 {
		t.Fatalf("分组模型不符(model-plaza 倒排): %v", groups)
	}
	if snap.Partial {
		t.Fatal("全部成功不应 partial")
	}
}

func TestSub2APIAdapter_PlazaDisabledIsPartial(t *testing.T) {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, data any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
	}
	mux.HandleFunc("/api/v1/user/profile", func(w http.ResponseWriter, r *http.Request) {
		ok(w, map[string]any{"id": 9, "username": "bob", "balance": 1.0})
	})
	mux.HandleFunc("/api/v1/groups/available", func(w http.ResponseWriter, r *http.Request) {
		ok(w, []map[string]any{{"name": "default", "rate_multiplier": 1.0}})
	})
	mux.HandleFunc("/api/v1/model-plaza", func(w http.ResponseWriter, r *http.Request) {
		ok(w, map[string]any{"enabled": false, "models": []any{}})
	})
	mux.HandleFunc("/api/v1/keys", func(w http.ResponseWriter, r *http.Request) {
		ok(w, map[string]any{"items": []any{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := NewSub2APIAdapter()
	snap, err := a.FetchSnapshot(context.Background(), sub2apiProvider(srv.URL))
	if err != nil {
		t.Fatalf("采集失败: %v", err)
	}
	if !snap.Partial {
		t.Fatal("plaza 未开启应 partial(模型数据不可用,spec §5.3)")
	}
	if len(snap.Groups) != 1 || len(snap.Groups[0].Models) != 0 {
		t.Fatalf("分组应保留但模型为空: %v", snap.Groups)
	}
}

func TestSub2APIAdapter_LoginSuccess(t *testing.T) {
	srv := sub2apiTestServer(t, false)
	defer srv.Close()

	p := sub2apiProvider(srv.URL)
	p.Credentials = map[string]any{"username": "bob@x.com", "password": "pw"}
	a := NewSub2APIAdapter()
	creds, err := a.Login(context.Background(), p, "")
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	// R1.8: 返回 map 中 key 为 access_token,值为 jwt-abc
	if creds["access_token"] != "jwt-abc" {
		t.Fatalf("应返回 JWT: %v", creds)
	}
}

func TestSub2APIAdapter_Login2FAIsError(t *testing.T) {
	srv := sub2apiTestServer(t, true)
	defer srv.Close()

	p := sub2apiProvider(srv.URL)
	p.Credentials = map[string]any{"username": "bob@x.com", "password": "pw"}
	a := NewSub2APIAdapter()
	_, err := a.Login(context.Background(), p, "")
	if !errors.Is(err, errUpstream2FA) {
		t.Fatalf("2FA 应返回 errUpstream2FA,实际: %v", err)
	}
}

func TestSub2APIAdapter_AuthError(t *testing.T) {
	srv := sub2apiTestServer(t, false)
	defer srv.Close()

	p := sub2apiProvider(srv.URL)
	p.Credentials = map[string]any{"access_token": "wrong"}
	a := NewSub2APIAdapter()
	_, err := a.FetchSnapshot(context.Background(), p)
	if !errors.Is(err, errUpstreamAuth) {
		t.Fatalf("401 应分类 errUpstreamAuth,实际: %v", err)
	}
}

func TestSub2APIAdapter_CreateToken(t *testing.T) {
	srv := sub2apiTestServer(t, false)
	defer srv.Close()

	a := NewSub2APIAdapter()
	tok, err := a.CreateToken(context.Background(), sub2apiProvider(srv.URL), CreateUpstreamTokenRequest{Name: "from-mgr"})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if tok.Key != "sk-created" {
		t.Fatalf("key 不符: %+v", tok)
	}
}
