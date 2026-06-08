package service

import "testing"

func TestNormalizeProxyAlias(t *testing.T) {
	cases := map[string]string{
		"":                    "",
		"  socks://u:p@h:1  ": "socks5://u:p@h:1", // trim + 别名
		"socks://h:1080":      "socks5://h:1080",
		"SOCKS://h:1080":      "socks5://h:1080", // 大小写不敏感
		"socks5://h:1080":     "socks5://h:1080", // 不动(交给 proxyurl 升级)
		"socks5h://h:1080":    "socks5h://h:1080",
		"http://u:p@h:8080":   "http://u:p@h:8080",
	}
	for in, want := range cases {
		if got := normalizeProxyAlias(in); got != want {
			t.Errorf("normalizeProxyAlias(%q)=%q want %q", in, got, want)
		}
	}
}

func TestBrowserProxyScheme(t *testing.T) {
	cases := map[string]string{
		"":                  "",
		"socks://u:p@h:1":   "socks5://u:p@h:1",
		"socks5h://u:p@h:1": "socks5://u:p@h:1", // Chromium 不识别 socks5h → 降为 socks5
		"socks5://h:1080":   "socks5://h:1080",
		"http://h:8080":     "http://h:8080",
	}
	for in, want := range cases {
		if got := browserProxyScheme(in); got != want {
			t.Errorf("browserProxyScheme(%q)=%q want %q", in, got, want)
		}
	}
}
