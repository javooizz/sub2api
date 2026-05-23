package repository

import (
	"context"
	"errors"
	"sort"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/extensionconfig"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/userallowedgroup"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// extensionConfigRepository 是 ExtensionConfigRepository 的 ent 实现。
type extensionConfigRepository struct {
	client *dbent.Client
}

// NewExtensionConfigRepository 构造（wire 注入）。
func NewExtensionConfigRepository(client *dbent.Client) service.ExtensionConfigRepository {
	return &extensionConfigRepository{client: client}
}

func (r *extensionConfigRepository) GetByAgentID(ctx context.Context, agentID string) (*service.ExtensionConfigRecord, error) {
	client := clientFromContext(ctx, r.client)
	row, err := client.ExtensionConfig.Query().
		Where(extensionconfig.AgentIDEQ(agentID)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrExtensionConfigNotFound
		}
		return nil, err
	}
	return extensionConfigEntToRecord(row), nil
}

func (r *extensionConfigRepository) Upsert(
	ctx context.Context,
	agentID string,
	payload domain.ExtensionConfigPayload,
	updatedBy *int64,
) (*service.ExtensionConfigRecord, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()

	existing, err := client.ExtensionConfig.Query().
		Where(extensionconfig.AgentIDEQ(agentID)).
		Only(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
	}

	if existing == nil {
		builder := client.ExtensionConfig.Create().
			SetAgentID(agentID).
			SetPayload(payload).
			SetCreatedAt(now).
			SetUpdatedAt(now)
		if updatedBy != nil {
			builder.SetUpdatedBy(*updatedBy)
		}
		created, cerr := builder.Save(ctx)
		if cerr != nil {
			return nil, cerr
		}
		return extensionConfigEntToRecord(created), nil
	}

	upd := client.ExtensionConfig.UpdateOne(existing).
		SetPayload(payload).
		SetUpdatedAt(now)
	if updatedBy != nil {
		upd.SetUpdatedBy(*updatedBy)
	} else {
		upd.ClearUpdatedBy()
	}
	updated, uerr := upd.Save(ctx)
	if uerr != nil {
		return nil, uerr
	}
	return extensionConfigEntToRecord(updated), nil
}

func (r *extensionConfigRepository) Delete(ctx context.Context, agentID string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.ExtensionConfig.Delete().
		Where(extensionconfig.AgentIDEQ(agentID)).
		Exec(ctx)
	if err != nil && !dbent.IsNotFound(err) {
		return err
	}
	return nil
}

// ListAll 返回所有 extension_config 记录,供 CSP frame-src 注入使用。
func (r *extensionConfigRepository) ListAll(ctx context.Context) ([]*service.ExtensionConfigRecord, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.ExtensionConfig.Query().All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.ExtensionConfigRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, extensionConfigEntToRecord(row))
	}
	return out, nil
}

func extensionConfigEntToRecord(e *dbent.ExtensionConfig) *service.ExtensionConfigRecord {
	if e == nil {
		return nil
	}
	rec := &service.ExtensionConfigRecord{
		ID:        e.ID,
		AgentID:   e.AgentID,
		Payload:   e.Payload,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
	// ent 可空字段 UpdatedBy 通过 *int64 暴露（schema.Nillable）。
	if e.UpdatedBy != nil {
		rec.UpdatedBy = e.UpdatedBy
	}
	return rec
}

// ====== UserAllowedGroupLister adapter ======

// userAllowedGroupLister 复用 user_allowed_groups 联接表查授权列表，
// 满足 service.UserAllowedGroupLister 接口。
type userAllowedGroupLister struct {
	client *dbent.Client
}

// NewUserAllowedGroupLister 构造（wire 注入）。
func NewUserAllowedGroupLister(client *dbent.Client) service.UserAllowedGroupLister {
	return &userAllowedGroupLister{client: client}
}

// ListAllowedGroupIDs 返回用户可见的 group 列表，包含两部分：
//  1. 专属分组：user_allowed_group 表里显式绑定的
//  2. 公开分组：所有 is_exclusive=false & status=active & subscription_type=standard 的分组
//
// 这与 domain.User.CanBindGroup 的语义保持一致（公开分组对所有用户放行）。
// 调用方（ExtensionConfigService.GetForUser / EnsureKey）依赖此函数过滤用户视角的可见分组。
func (l *userAllowedGroupLister) ListAllowedGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	if userID <= 0 {
		return nil, errors.New("userID must be > 0")
	}
	client := clientFromContext(ctx, l.client)

	exclusiveRows, err := client.UserAllowedGroup.Query().
		Where(userallowedgroup.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	publicRows, err := client.Group.Query().
		Where(
			group.IsExclusiveEQ(false),
			group.StatusEQ(domain.StatusActive),
			group.SubscriptionTypeEQ(domain.SubscriptionTypeStandard),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	seen := make(map[int64]struct{}, len(exclusiveRows)+len(publicRows))
	ids := make([]int64, 0, len(exclusiveRows)+len(publicRows))
	for i := range exclusiveRows {
		gid := exclusiveRows[i].GroupID
		if _, ok := seen[gid]; ok {
			continue
		}
		seen[gid] = struct{}{}
		ids = append(ids, gid)
	}
	for i := range publicRows {
		gid := publicRows[i].ID
		if _, ok := seen[gid]; ok {
			continue
		}
		seen[gid] = struct{}{}
		ids = append(ids, gid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}
