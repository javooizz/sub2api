package domain

import (
	"encoding/json"
	"time"
)

// 上游站点类型
const (
	UpstreamTypeSub2API = "sub2api"
	UpstreamTypeNewAPI  = "newapi"
)

// 上游站点状态(spec §11 状态机)
const (
	UpstreamStatusActive          = "active"
	UpstreamStatusCredentialError = "credential_error"
	UpstreamStatusUnreachable     = "unreachable"
	UpstreamStatusDisabled        = "disabled"
)

// 上游变更事件类型(spec §4.2)
const (
	UpstreamEventBalanceLow       = "balance_low"
	UpstreamEventBalanceRecovered = "balance_recovered"
	UpstreamEventPriceChanged     = "price_changed"
	UpstreamEventModelAdded       = "model_added"
	UpstreamEventModelRemoved     = "model_removed"
	UpstreamEventGroupAdded       = "group_added"
	UpstreamEventGroupRemoved     = "group_removed"
	UpstreamEventRefreshFailed    = "refresh_failed"
	UpstreamEventCredentialError  = "credential_error"
)

// 通知渠道
const (
	NotifyChannelTypeEmail   = "email"
	NotifyChannelTypeWebhook = "webhook"
	NotifyScopeUpstream      = "upstream"
)

// UpstreamSnapshot 是两种上游归一化后的统一快照结构(spec §4.1.2)。
// 任一子接口失败时对应字段缺失且 Partial=true,不整体报错。
type UpstreamSnapshot struct {
	FetchedAt    time.Time                  `json:"fetched_at"`
	Balance      *float64                   `json:"balance,omitempty"`
	Currency     string                     `json:"currency,omitempty"`
	UserInfo     *UpstreamUserInfo          `json:"user_info,omitempty"`
	Groups       []UpstreamGroupSnapshot    `json:"groups,omitempty"`
	ModelPricing map[string]json.RawMessage `json:"model_pricing,omitempty"`
	Partial      bool                       `json:"partial"`
}

// UpstreamUserInfo 上游用户基本信息。
type UpstreamUserInfo struct {
	ID       int64  `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
}

// UpstreamGroupSnapshot 上游单个分组的快照信息。
type UpstreamGroupSnapshot struct {
	Name        string   `json:"name"`
	Ratio       *float64 `json:"ratio,omitempty"`
	Description string   `json:"description,omitempty"`
	Models      []string `json:"models,omitempty"`
}
