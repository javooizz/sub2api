package service

import "strings"

// 上游采集代理的 scheme 规范化(fork 本地,不改共享 proxyurl 包,降低上游合并冲突面)。
//
// 背景:用户常把 SOCKS5 代理写成 socks://,但 Go 的 http.Transport 只认 socks5/socks5h,
// proxyurl.Parse 的白名单也不含裸 socks://。这里把 socks:// 当作 socks5:// 的别名统一处理。

// normalizeProxyAlias 把 socks:// 视作 socks5:// 别名(仅替换 scheme),供 HTTP 采集
// 在调用 proxyurl.Parse 前预处理;proxyurl.Parse 再做校验与 socks5→socks5h 升级。
// 空白与其余 scheme 原样返回。
func normalizeProxyAlias(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(s), "socks://") {
		return "socks5://" + s[len("socks://"):]
	}
	return s
}

// browserProxyScheme 规范化代理 scheme 供 CloakBrowser/Chromium 使用:
// socks:// 与 socks5h:// 统一为 socks5://(Chromium 不识别 socks5h,且对 socks5 默认远程 DNS)。
// http/https 等原样返回。
func browserProxyScheme(raw string) string {
	s := strings.TrimSpace(raw)
	switch low := strings.ToLower(s); {
	case strings.HasPrefix(low, "socks://"):
		return "socks5://" + s[len("socks://"):]
	case strings.HasPrefix(low, "socks5h://"):
		return "socks5://" + s[len("socks5h://"):]
	}
	return s
}
