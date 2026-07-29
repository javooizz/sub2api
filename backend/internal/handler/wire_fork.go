package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"

	"github.com/google/wire"
)

// ForkProviderSet 汇集本 fork 相对上游新增的 Handler Provider。
// 与 repository/wire_fork.go 同模式：与上游 ProviderSet 物理隔离，
// 避免合并上游时的"列表追加"型冲突。
var ForkProviderSet = wire.NewSet(
	NewModelCatalogHandler,            // 用户侧模型目录（原「模型广场」，上游同名页面合入后改名）
	NewExtensionConfigHandler,         // 用户侧扩展配置
	admin.NewModelCatalogHandler,        // 管理侧模型目录清单
	admin.NewNotifyChannelHandler,     // 管理侧通知渠道 CRUD + 测试发送
	admin.NewUpstreamProviderHandler,  // 管理侧上游站点全量 API（fork 新增）
)
