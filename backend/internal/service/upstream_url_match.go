package service

import (
	"errors"
	"net/url"
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
	hostPort := host
	if port != "" {
		hostPort = host + ":" + port
	}
	path := normalizePathSegments(u.Path)
	return scheme + "://" + hostPort + path, nil
}

// normalizePathSegments 压缩重复斜杠、去尾斜杠;根路径返回空串。
func normalizePathSegments(p string) string {
	segs := pathSegments(p)
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

// UpstreamURLMatches 关联帐号匹配(spec §9 精确规则):
// scheme 相等 + host 小写精确相等 + 默认端口归一后相等 + 路径段边界前缀。
func UpstreamURLMatches(providerURL, accountURL string) bool {
	pn, err1 := NormalizeUpstreamURL(providerURL)
	an, err2 := NormalizeUpstreamURL(accountURL)
	if err1 != nil || err2 != nil {
		return false
	}
	pu, _ := url.Parse(pn)
	au, _ := url.Parse(an)
	if pu.Scheme != au.Scheme || pu.Host != au.Host {
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
