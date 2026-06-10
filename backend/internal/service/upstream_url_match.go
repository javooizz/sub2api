package service

import (
	"errors"
	"net"
	"net/url"
	"path"
	"strings"
)

// NormalizeUpstreamURL 归一化:scheme/host 小写、默认端口剥离、路径压缩多斜杠去尾斜杠。
// 仅允许 http/https(spec §4.1)。
func NormalizeUpstreamURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("仅支持 http/https 地址")
	}
	if u.Host == "" {
		return "", errors.New("地址缺少 host")
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}

	// 修复 1: IPv6 host 归一化 — 用 net.JoinHostPort 为带端口的 IPv6 加方括号;
	// 无端口的裸 IPv6 地址(含冒号)同样需要方括号。
	var hostPort string
	switch {
	case port != "":
		hostPort = net.JoinHostPort(host, port) // 自动为 IPv6 加 []
	case strings.Contains(host, ":"):
		hostPort = "[" + host + "]" // 无端口的 IPv6
	default:
		hostPort = host
	}

	p := normalizePathSegments(u.Path)
	return scheme + "://" + hostPort + p, nil
}

// normalizePathSegments 规范化路径:先用 path.Clean 解析 ../.  段,
// 再压缩重复斜杠、去尾斜杠;根路径返回空串。
// 修复 2: 引入 path.Clean 防止 ../  段穿越(如 /tenant_a/../tenant_b 误匹配)。
func normalizePathSegments(p string) string {
	// path.Clean 需要以 "/" 开头才能正确解析 ".." 至根
	cleaned := path.Clean("/" + p)
	segs := pathSegments(cleaned)
	if len(segs) == 0 {
		return ""
	}
	return "/" + strings.Join(segs, "/")
}

func pathSegments(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// stripWWWHost 剥离 host 的精确 "www." 前缀,使 www.example.com 与 example.com 视为同站。
// 仅剥精确前缀,不影响 wwwx.example.com / evil-www.example.com 等(防误匹配)。
func stripWWWHost(host string) string {
	return strings.TrimPrefix(host, "www.")
}

// UpstreamURLMatches 关联帐号匹配(spec §9 精确规则):
// scheme 相等 + host 小写精确相等 + 默认端口归一后相等 + 路径段边界前缀。
func UpstreamURLMatches(providerURL, accountURL string) bool {
	pn, err1 := NormalizeUpstreamURL(providerURL)
	an, err2 := NormalizeUpstreamURL(accountURL)
	if err1 != nil || err2 != nil {
		return false
	}

	// 修复 3: nil 守卫 — 归一化后二次 Parse 失败时安全返回 false 而非 panic。
	pu, err1b := url.Parse(pn)
	au, err2b := url.Parse(an)
	if err1b != nil || err2b != nil || pu == nil || au == nil {
		return false
	}

	if pu.Scheme != au.Scheme || stripWWWHost(pu.Host) != stripWWWHost(au.Host) {
		return false
	}
	pSegs := pathSegments(pu.Path)
	aSegs := pathSegments(au.Path)
	if len(aSegs) < len(pSegs) {
		return false
	}
	for i, seg := range pSegs {
		if aSegs[i] != seg {
			return false
		}
	}
	return true
}
