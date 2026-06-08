package service

import (
	"context"
	"regexp"
	"strings"
)

// 设置键(spec §9 专项设置端点)
const (
	SettingKeyUpstreamBrowserCDPURL       = "upstream_browser_cdp_url"
	SettingKeyUpstreamAllowPrivateWebhook = "upstream_notify_allow_private_webhook"
	// SettingKeyUpstreamProxyURL 全局采集代理(同时用于 HTTP 采集与 CloakBrowser 过盾)。
	SettingKeyUpstreamProxyURL = "upstream_proxy_url"
)

// UpstreamManagementRuntime 上游管理运行时设置。
type UpstreamManagementRuntime struct {
	BrowserCDPURL       string `json:"browser_cdp_url"`
	AllowPrivateWebhook bool   `json:"allow_private_webhook"`
	// ProxyURL 全局采集代理。内部读取返回原始值;经 admin API 出入时由 handler 做
	// maskProxyURL(响应脱敏) / mergeProxyURL(保留旧值)处理。
	ProxyURL string `json:"proxy_url"`
}

// GetUpstreamManagementRuntime 读取上游管理设置(缺省:CDP 为空=浏览器禁用,私网 webhook 拒绝)。
func (s *SettingService) GetUpstreamManagementRuntime(ctx context.Context) UpstreamManagementRuntime {
	rt := UpstreamManagementRuntime{}
	if v, err := s.settingRepo.GetValue(ctx, SettingKeyUpstreamBrowserCDPURL); err == nil {
		rt.BrowserCDPURL = v
	}
	if v, err := s.settingRepo.GetValue(ctx, SettingKeyUpstreamAllowPrivateWebhook); err == nil {
		rt.AllowPrivateWebhook = v == "true"
	}
	if v, err := s.settingRepo.GetValue(ctx, SettingKeyUpstreamProxyURL); err == nil {
		rt.ProxyURL = v
	}
	return rt
}

// SetUpstreamManagementSettings 写入上游管理设置。
func (s *SettingService) SetUpstreamManagementSettings(ctx context.Context, rt UpstreamManagementRuntime) error {
	if err := s.settingRepo.Set(ctx, SettingKeyUpstreamBrowserCDPURL, rt.BrowserCDPURL); err != nil {
		return err
	}
	val := "false"
	if rt.AllowPrivateWebhook {
		val = "true"
	}
	if err := s.settingRepo.Set(ctx, SettingKeyUpstreamAllowPrivateWebhook, val); err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, SettingKeyUpstreamProxyURL, rt.ProxyURL)
}

// proxyURLPwdRe 匹配 "scheme://user:password@" 中的密码段(group1 = scheme://user)。
var proxyURLPwdRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.\-]*://[^:/@]+):[^@/]*@`)

// MaskUpstreamProxyURL 把代理 URL 的密码段替换为 ***(保留 scheme/user/host 便于识别),供 admin API 响应脱敏。
// 无 :// 或无 userinfo 密码时原样返回。
func MaskUpstreamProxyURL(raw string) string {
	if raw == "" {
		return ""
	}
	return proxyURLPwdRe.ReplaceAllString(raw, "$1:***@")
}

// MergeUpstreamProxyURL PUT 合并:incoming 含 *** 占位 = 未修改,保留 existing;空串 = 显式清除;
// 其余 = 采用新值。规避单字段掩码部分编辑破坏密码的坑(改代理须填完整 URL)。
func MergeUpstreamProxyURL(existing, incoming string) string {
	if strings.Contains(incoming, "***") {
		return existing
	}
	return incoming
}
