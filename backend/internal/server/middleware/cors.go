package middleware

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

var corsWarningOnce sync.Once

// CORS 跨域中间件
func CORS(cfg config.CORSConfig) gin.HandlerFunc {
	allowedOrigins := normalizeOrigins(cfg.AllowedOrigins)
	allowAll := false
	for _, origin := range allowedOrigins {
		if origin == "*" {
			allowAll = true
			break
		}
	}
	wildcardWithSpecific := allowAll && len(allowedOrigins) > 1
	if wildcardWithSpecific {
		allowedOrigins = []string{"*"}
	}
	allowCredentials := cfg.AllowCredentials

	corsWarningOnce.Do(func() {
		if len(allowedOrigins) == 0 {
			log.Println("Warning: CORS allowed_origins not configured; cross-origin requests will be rejected.")
		}
		if wildcardWithSpecific {
			log.Println("Warning: CORS allowed_origins includes '*'; wildcard will take precedence over explicit origins.")
		}
		if allowAll && allowCredentials {
			log.Println("Warning: CORS allowed_origins set to '*', disabling allow_credentials.")
		}
	})
	if allowAll && allowCredentials {
		allowCredentials = false
	}

	allowedSet := make(map[string]struct{}, len(allowedOrigins))
	var wildcardSuffixes []string
	for _, origin := range allowedOrigins {
		if origin == "" || origin == "*" {
			continue
		}
		if suffix, ok := parseWildcardSuffix(origin); ok {
			wildcardSuffixes = append(wildcardSuffixes, suffix)
			continue
		}
		allowedSet[origin] = struct{}{}
	}
	allowHeaders := []string{
		"Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization",
		"accept", "origin", "Cache-Control", "X-Requested-With", "X-API-Key",
	}
	// OpenAI Node SDK 会发送 x-stainless-* 请求头，需在 CORS 中显式放行。
	openAIProperties := []string{
		"lang", "package-version", "os", "arch", "retry-count", "runtime",
		"runtime-version", "async", "helper-method", "poll-helper", "custom-poll-interval", "timeout",
	}
	for _, prop := range openAIProperties {
		allowHeaders = append(allowHeaders, "x-stainless-"+prop)
	}
	allowHeadersValue := strings.Join(allowHeaders, ", ")

	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		originAllowed := allowAll
		if origin != "" && !allowAll {
			if _, ok := allowedSet[origin]; ok {
				originAllowed = true
			} else if originMatchesWildcard(origin, wildcardSuffixes) {
				originAllowed = true
			}
		}

		if originAllowed {
			if allowAll {
				c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Add("Vary", "Origin")
			}
			if allowCredentials {
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			c.Writer.Header().Set("Access-Control-Allow-Headers", allowHeadersValue)
			c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
			c.Writer.Header().Set("Access-Control-Expose-Headers", "ETag")
			c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		}
		// 处理预检请求
		if c.Request.Method == http.MethodOptions {
			if originAllowed {
				c.AbortWithStatus(http.StatusNoContent)
			} else {
				c.AbortWithStatus(http.StatusForbidden)
			}
			return
		}

		c.Next()
	}
}

func normalizeOrigins(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

// parseWildcardSuffix 把配置项中的 "*.<suffix>" 解析为 suffix,失败返回 ("", false)。
// 拒绝顶级通配(suffix 必须含至少一个 ".")与多重通配(suffix 自身不能含 "*")。
func parseWildcardSuffix(origin string) (string, bool) {
	const prefix = "*."
	if !strings.HasPrefix(origin, prefix) {
		return "", false
	}
	suffix := strings.TrimPrefix(origin, prefix)
	if suffix == "" || strings.Contains(suffix, "*") || !strings.Contains(suffix, ".") {
		return "", false
	}
	return suffix, true
}

// originMatchesWildcard 检查 origin 是否匹配任何一个 *.suffix 模式。
// 要求:scheme 为 https;host 不等于 suffix(裸根域不算命中);host 以 ".suffix" 结尾。
func originMatchesWildcard(origin string, suffixes []string) bool {
	if len(suffixes) == 0 {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	host := u.Hostname() // 去端口
	for _, suffix := range suffixes {
		if host == suffix {
			continue
		}
		if strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
