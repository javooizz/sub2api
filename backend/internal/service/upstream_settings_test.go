package service

import "testing"

func TestMaskProxyURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"with password", "http://user:pass@host:8080", "http://user:***@host:8080"},
		{"socks5 with password", "socks5://u:secret@1.2.3.4:1080", "socks5://u:***@1.2.3.4:1080"},
		{"no userinfo", "socks5://1.2.3.4:1080", "socks5://1.2.3.4:1080"},
		{"username only", "http://user@host:8080", "http://user@host:8080"},
		{"unparseable returned as-is", "host:8080", "host:8080"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MaskUpstreamProxyURL(c.in); got != c.want {
				t.Errorf("MaskUpstreamProxyURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMergeProxyURL(t *testing.T) {
	const existing = "http://user:pass@host:8080"
	cases := []struct {
		name     string
		existing string
		incoming string
		want     string
	}{
		{"masked unchanged keeps existing", existing, "http://user:***@host:8080", existing},
		{"any *** keeps existing", existing, "***", existing},
		{"empty clears", existing, "", ""},
		{"new value replaces", existing, "socks5://new:pw@h2:1080", "socks5://new:pw@h2:1080"},
		{"set from empty", "", "http://u:p@h:1", "http://u:p@h:1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MergeUpstreamProxyURL(c.existing, c.incoming); got != c.want {
				t.Errorf("MergeUpstreamProxyURL(%q, %q) = %q, want %q", c.existing, c.incoming, got, c.want)
			}
		})
	}
}
