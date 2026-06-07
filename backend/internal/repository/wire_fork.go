package repository

import "github.com/google/wire"

// ForkProviderSet 汇集本 fork 相对上游新增的 Repository / Provider。
//
// 目的：把 fork 独有的依赖注入条目与上游共享的 ProviderSet（wire.go）物理隔离。
// 长期合并上游时，上游对 ProviderSet 的增删不会与 fork 的新增行相邻，
// 从而避免反复出现“列表追加”型合并冲突。
//
// 约定：fork 新增的 Repository Provider 一律加到这里，不要直接写进 wire.go 的 ProviderSet。
var ForkProviderSet = wire.NewSet(
	NewExtensionConfigRepository,     // 浏览器扩展配置仓储
	NewUserAllowedGroupLister,        // 用户可见分组列表
	NewNotifyChannelRepository,       // 通知渠道仓储
	NewUpstreamProviderRepository,    // 上游站点仓储
	NewUpstreamChangeEventRepository, // 上游变更事件仓储
	NewUpstreamAccountLister,         // 关联帐号列表（type=upstream 账号）
)
