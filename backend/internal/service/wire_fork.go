package service

import "github.com/google/wire"

// ForkProviderSet 汇集本 fork 相对上游新增的 Service Provider。
//
// 与 repository/wire_fork.go 同模式：把 fork 独有的依赖注入条目与上游共享的
// ProviderSet（wire.go）物理隔离，避免合并上游时的"列表追加"型冲突。
//
// 约定：fork 新增的 Service Provider 一律加到这里，不要直接写进 wire.go 的 ProviderSet。
var ForkProviderSet = wire.NewSet(
	NewModelPlazaService, // 模型广场聚合
	ProvideNotifySenders, // 通知渠道 sender 聚合(email/webhook)
	NewNotifyDispatcher,  // 通知分发器(Task 15 消费)
)
