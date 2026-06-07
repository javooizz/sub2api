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
	}
	for _, c := range cases {
		got := UpstreamURLMatches(c.provider, c.account)
		if got != c.want {
			t.Fatalf("%s: provider=%s account=%s got=%v want=%v", c.desc, c.provider, c.account, got, c.want)
		}
	}
}
