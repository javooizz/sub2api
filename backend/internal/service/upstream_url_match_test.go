package service

import "testing"

func TestNormalizeUpstreamURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"HTTPS://Example.COM/", "https://example.com"},
		{"https://example.com:443/api/", "https://example.com/api"},   // 默认端口归一
		{"http://example.com:80", "http://example.com"},
		{"https://example.com:8443/relay", "https://example.com:8443/relay"}, // 非默认端口保留
		{"https://example.com/api//v1/", "https://example.com/api/v1"},
	}
	for _, c := range cases {
		got, err := NormalizeUpstreamURL(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("%s → %s,期望 %s", c.in, got, c.want)
		}
	}
	if _, err := NormalizeUpstreamURL("ftp://x.com"); err == nil {
		t.Fatal("非 http/https 应报错")
	}
	if _, err := NormalizeUpstreamURL("not a url"); err == nil {
		t.Fatal("非法 URL 应报错")
	}
}

func TestUpstreamURLMatches(t *testing.T) {
	cases := []struct {
		provider, account string
		want              bool
		desc              string
	}{
		{"https://example.com", "https://example.com", true, "完全相同"},
		{"https://example.com", "https://example.com/v1", true, "账号带路径"},
		{"https://example.com", "https://EXAMPLE.com:443/v1/", true, "大小写/默认端口/尾斜杠归一"},
		{"https://example.com", "https://example.com.evil", false, "host 后缀攻击"},
		{"https://example.com", "http://example.com", false, "scheme 不同"},
		{"https://example.com", "https://example.com:8443", false, "端口不同"},
		{"https://example.com/api", "https://example.com/api/v1", true, "路径段前缀"},
		{"https://example.com/api", "https://example.com/api2", false, "段边界:/api 不匹配 /api2"},
		{"https://example.com/api", "https://example.com", false, "账号路径短于 provider 前缀"},
		{"https://codexapis.com", "https://www.codexapis.com", true, "www 子域归一为同站"},
		{"https://www.codexapis.com", "https://codexapis.com", true, "www 反向归一为同站"},
		{"https://codexapis.com/v1", "https://www.codexapis.com/v1/key1", true, "www 归一 + 路径段前缀"},
		{"https://example.com", "https://wwwx.example.com", false, "wwwx 非精确 www. 前缀,不归一"},
		{"https://example.com", "https://evil-www.example.com", false, "evil-www 非 www. 前缀,不归一"},
	}
	for _, c := range cases {
		got := UpstreamURLMatches(c.provider, c.account)
		if got != c.want {
			t.Fatalf("%s: provider=%s account=%s got=%v want=%v", c.desc, c.provider, c.account, got, c.want)
		}
	}
}

// --- 安全回归测试(修复 1/2/3 对应) ---

func TestNormalizeUpstreamURL_IPv6(t *testing.T) {
	// 修复 1: IPv6 带端口应正确归一化(方括号由 net.JoinHostPort 处理)
	got, err := NormalizeUpstreamURL("https://[::1]:8080/api/")
	if err != nil {
		t.Fatalf("IPv6 应可归一化: %v", err)
	}
	if got != "https://[::1]:8080/api" {
		t.Fatalf("IPv6 归一化不符: %s", got)
	}
	// 默认端口剥离 + IPv6 无端口时仍保留方括号
	got2, err2 := NormalizeUpstreamURL("https://[::1]:443/")
	if err2 != nil {
		t.Fatalf("IPv6 默认端口应可归一化: %v", err2)
	}
	if got2 != "https://[::1]" {
		t.Fatalf("IPv6 默认端口剥离不符: %s", got2)
	}
}

func TestUpstreamURLMatches_IPv6NoPanic(t *testing.T) {
	// 修复 1/3: 修复前二次 url.Parse 拿到裸 IPv6(无方括号)会产生畸形 URL,
	// Host 字段不可信或 panic;修复后应正常返回布尔值。
	if !UpstreamURLMatches("https://[::1]:8080", "https://[::1]:8080/v1") {
		t.Fatal("相同 IPv6 host 应匹配")
	}
	if UpstreamURLMatches("https://[::1]:8080", "https://[::2]:8080") {
		t.Fatal("不同 IPv6 host 不应匹配")
	}
}

func TestUpstreamURLMatches_DotDotSegmentNoBypass(t *testing.T) {
	// 修复 2: /tenant_a/../tenant_b 经 path.Clean 后实际路径是 /tenant_b,
	// 不应匹配 provider /tenant_a(安全穿越防护)。
	if UpstreamURLMatches(
		"https://api.example.com/tenant_a",
		"https://api.example.com/tenant_a/../tenant_b/secret",
	) {
		t.Fatal(".. 段穿越:实际路径 /tenant_b 不应匹配 provider /tenant_a")
	}
	// 正常前缀仍应匹配
	if !UpstreamURLMatches(
		"https://api.example.com/tenant_a",
		"https://api.example.com/tenant_a/key1",
	) {
		t.Fatal("正常前缀应匹配")
	}
}

func TestUpstreamURLMatches_UserinfoNoBypass(t *testing.T) {
	// userinfo 注入: "https://example.com@evil.com/v1" 中实际 host 是 evil.com,
	// 不应与 provider "https://example.com" 匹配。
	if UpstreamURLMatches("https://example.com", "https://example.com@evil.com/v1") {
		t.Fatal("userinfo 注入不应匹配(实际 host 是 evil.com)")
	}
}
