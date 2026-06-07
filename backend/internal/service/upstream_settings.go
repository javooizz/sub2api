package service

import "context"

// 设置键(spec §9 专项设置端点)
const (
	SettingKeyUpstreamBrowserCDPURL       = "upstream_browser_cdp_url"
	SettingKeyUpstreamAllowPrivateWebhook = "upstream_notify_allow_private_webhook"
)

// UpstreamManagementRuntime 上游管理运行时设置。
type UpstreamManagementRuntime struct {
	BrowserCDPURL       string `json:"browser_cdp_url"`
	AllowPrivateWebhook bool   `json:"allow_private_webhook"`
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
	return s.settingRepo.Set(ctx, SettingKeyUpstreamAllowPrivateWebhook, val)
}
