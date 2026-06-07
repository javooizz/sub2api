package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"

	"github.com/google/wire"
)

// ForkProviderSet 汇集本 fork 相对上游新增的 Handler Provider。
// 与 repository/wire_fork.go 同模式：与上游 ProviderSet 物理隔离，
// 避免合并上游时的"列表追加"型冲突。
var ForkProviderSet = wire.NewSet(
	NewModelPlazaHandler,         // 用户侧模型广场
	admin.NewModelPlazaHandler,   // 管理侧模型清单
	admin.NewNotifyChannelHandler, // 管理侧通知渠道 CRUD + 测试发送
)
