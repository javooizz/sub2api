package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestToUpstreamProviderResponse_RedactsCredentials(t *testing.T) {
	p := &service.UpstreamProvider{
		ID: 1, Name: "demo", Type: "newapi", SiteURL: "https://a.com",
		Credentials: map[string]any{
			"username": "alice", "password": "secret",
			"access_token": "tok-123456789", "cf_clearance": "cfc",
		},
	}
	resp := toUpstreamProviderResponse(p, true)
	if _, exists := resp.Credentials["password"]; exists {
		t.Fatal("password 不应出现在响应")
	}
	if _, exists := resp.Credentials["access_token"]; exists {
		t.Fatal("access_token 不应出现在响应")
	}
	if resp.Credentials["username"] != "alice" {
		t.Fatal("非敏感键应保留")
	}
	if !resp.CredentialStatus.HasPassword || !resp.CredentialStatus.HasAccessToken {
		t.Fatalf("状态布尔不符: %+v", resp.CredentialStatus)
	}
	if resp.CredentialStatus.AccessTokenTail != "6789" {
		t.Fatalf("尾 4 位不符: %s", resp.CredentialStatus.AccessTokenTail)
	}
}

func TestValidDiagnosticsFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"20260608120000.png", true},
		{"../secret.png", false},
		{"..%2Fsecret.png", false},
		{"a/b.png", false},
		{"shot.exe", false},
		{"", false},
	}
	for _, c := range cases {
		if got := validDiagnosticsFile(c.name); got != c.want {
			t.Fatalf("%q: got %v want %v", c.name, got, c.want)
		}
	}
}
