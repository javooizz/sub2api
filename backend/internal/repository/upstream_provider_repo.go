package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/upstreamchangeevent"
	"github.com/Wei-Shaw/sub2api/ent/upstreamprovider"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type upstreamProviderRepository struct {
	client *dbent.Client
}

// NewUpstreamProviderRepository 构造上游站点 Repository。
func NewUpstreamProviderRepository(client *dbent.Client) service.UpstreamProviderRepository {
	return &upstreamProviderRepository{client: client}
}

// toServiceUpstreamProvider 将 ent 对象映射为业务对象(含所有 Nillable 字段)。
func toServiceUpstreamProvider(e *dbent.UpstreamProvider) *service.UpstreamProvider {
	if e == nil {
		return nil
	}
	return &service.UpstreamProvider{
		ID:                     e.ID,
		Name:                   e.Name,
		Type:                   e.Type,
		SiteURL:                e.SiteURL,
		APIBaseURL:             e.APIBaseURL,
		Status:                 e.Status,
		Credentials:            e.Credentials,
		BalanceThreshold:       e.BalanceThreshold,
		NotifyOnPriceChange:    e.NotifyOnPriceChange,
		RefreshIntervalMinutes: e.RefreshIntervalMinutes,
		LatestSnapshot:         e.LatestSnapshot,
		LastRefreshedAt:        e.LastRefreshedAt,
		LastError:              e.LastError,
		ConsecutiveFailures:    e.ConsecutiveFailures,
		BalanceAlerted:         e.BalanceAlerted,
		RefreshStartedAt:       e.RefreshStartedAt,
		Remark:                 e.Remark,
		CreatedAt:              e.CreatedAt,
		UpdatedAt:              e.UpdatedAt,
	}
}

// Create 创建上游站点记录。Nillable 字段(BalanceThreshold)仅在非 nil 时 Set。
func (r *upstreamProviderRepository) Create(ctx context.Context, p *service.UpstreamProvider) (*service.UpstreamProvider, error) {
	c := r.client.UpstreamProvider.Create().
		SetName(p.Name).
		SetType(p.Type).
		SetSiteURL(p.SiteURL).
		SetAPIBaseURL(p.APIBaseURL).
		SetStatus(p.Status).
		SetCredentials(p.Credentials).
		SetNotifyOnPriceChange(p.NotifyOnPriceChange).
		SetRefreshIntervalMinutes(p.RefreshIntervalMinutes).
		SetRemark(p.Remark)
	if p.BalanceThreshold != nil {
		c = c.SetBalanceThreshold(*p.BalanceThreshold)
	}
	created, err := c.Save(ctx)
	if err != nil {
		return nil, err
	}
	return toServiceUpstreamProvider(created), nil
}

// GetByID 按 ID 查询;不存在返回 ErrUpstreamProviderNotFound。
func (r *upstreamProviderRepository) GetByID(ctx context.Context, id int64) (*service.UpstreamProvider, error) {
	e, err := r.client.UpstreamProvider.Get(ctx, id)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrUpstreamProviderNotFound
		}
		return nil, err
	}
	return toServiceUpstreamProvider(e), nil
}

// List 返回全部上游站点(按 ID 升序)。
func (r *upstreamProviderRepository) List(ctx context.Context) ([]*service.UpstreamProvider, error) {
	rows, err := r.client.UpstreamProvider.Query().
		Order(dbent.Asc(upstreamprovider.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*service.UpstreamProvider, 0, len(rows))
	for _, e := range rows {
		out = append(out, toServiceUpstreamProvider(e))
	}
	return out, nil
}

// Update 更新上游站点字段。BalanceThreshold 为 nil 时用 Clear* 清空。
func (r *upstreamProviderRepository) Update(ctx context.Context, p *service.UpstreamProvider) (*service.UpstreamProvider, error) {
	u := r.client.UpstreamProvider.UpdateOneID(p.ID).
		SetName(p.Name).
		SetType(p.Type).
		SetSiteURL(p.SiteURL).
		SetAPIBaseURL(p.APIBaseURL).
		SetStatus(p.Status).
		SetCredentials(p.Credentials).
		SetNotifyOnPriceChange(p.NotifyOnPriceChange).
		SetRefreshIntervalMinutes(p.RefreshIntervalMinutes).
		SetRemark(p.Remark)
	if p.BalanceThreshold != nil {
		u = u.SetBalanceThreshold(*p.BalanceThreshold)
	} else {
		u = u.ClearBalanceThreshold()
	}
	updated, err := u.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrUpstreamProviderNotFound
		}
		return nil, err
	}
	return toServiceUpstreamProvider(updated), nil
}

// Delete 事务内级联删除变更事件再删除 provider 本身(spec §4.2)。
// 所有 DB 操作均使用事务 tx.*。
func (r *upstreamProviderRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// 先级联删除该 provider 的所有变更事件
	if _, err := tx.UpstreamChangeEvent.Delete().
		Where(upstreamchangeevent.ProviderIDEQ(id)).
		Exec(ctx); err != nil {
		return err
	}
	// 再删除 provider 本身
	if err := tx.UpstreamProvider.DeleteOneID(id).Exec(ctx); err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrUpstreamProviderNotFound
		}
		return err
	}
	return tx.Commit()
}

// ListDue 返回到期应刷新的 provider ID 列表。
// 查询 status != disabled 的全部记录,在 Go 侧按 interval 过滤(避免 DB 层时间计算差异)。
func (r *upstreamProviderRepository) ListDue(ctx context.Context, now time.Time) ([]int64, error) {
	rows, err := r.client.UpstreamProvider.Query().
		Where(upstreamprovider.StatusNEQ(domain.UpstreamStatusDisabled)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	var due []int64
	for _, e := range rows {
		interval := time.Duration(e.RefreshIntervalMinutes) * time.Minute
		if e.LastRefreshedAt == nil || e.LastRefreshedAt.Add(interval).Before(now) {
			due = append(due, e.ID)
		}
	}
	return due, nil
}

// ClaimRefresh 条件 UPDATE 抢占刷新锁(spec §7.1)。
// WHERE id=? AND (refresh_started_at IS NULL OR refresh_started_at < staleBefore)
// SET refresh_started_at=now。返回受影响行数:n==0 表示锁已被他人持有或被误判为 stale。
func (r *upstreamProviderRepository) ClaimRefresh(ctx context.Context, id int64, now, staleBefore time.Time) (service.ClaimedRefresh, bool, error) {
	n, err := r.client.UpstreamProvider.Update().
		Where(
			upstreamprovider.IDEQ(id),
			upstreamprovider.Or(
				upstreamprovider.RefreshStartedAtIsNil(),
				upstreamprovider.RefreshStartedAtLT(staleBefore),
			),
		).
		SetRefreshStartedAt(now).
		Save(ctx)
	if err != nil {
		return service.ClaimedRefresh{}, false, err
	}
	if n == 0 {
		// 锁已被他人持有或条件不满足:返回 ok=false
		return service.ClaimedRefresh{}, false, nil
	}
	return service.ClaimedRefresh{ProviderID: id, ClaimedAt: now}, true, nil
}

// CommitRefresh 单事务原子提交刷新结果(spec §7.1)。
// 步骤:
//  1. 条件 UPDATE provider WHERE id=? AND refresh_started_at=ClaimedAt
//     验证锁仍由本实例持有(乐观锁);n==0 → Rollback 返回 committed=false
//  2. 逐条插入变更事件(tx.UpstreamChangeEvent.Create)
//  3. Commit
//
// 所有 DB 操作均使用事务 tx.*,不使用外层 r.client.*。
func (r *upstreamProviderRepository) CommitRefresh(ctx context.Context, in service.CommitRefreshInput) (bool, []service.UpstreamChangeEvent, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1) 条件更新 provider:乐观锁验证 + 状态写入
	u := tx.UpstreamProvider.Update().
		Where(
			upstreamprovider.IDEQ(in.Claim.ProviderID),
			upstreamprovider.RefreshStartedAtEQ(in.Claim.ClaimedAt),
		).
		SetStatus(in.Status).
		SetLastError(in.LastError).
		SetConsecutiveFailures(in.ConsecFailures).
		SetBalanceAlerted(in.BalanceAlerted).
		SetLastRefreshedAt(in.RefreshedAt).
		ClearRefreshStartedAt()
	if in.Snapshot != nil {
		u = u.SetLatestSnapshot(in.Snapshot)
	}
	n, err := u.Save(ctx)
	if err != nil {
		return false, nil, err
	}
	if n == 0 {
		// 锁已被他人抢走(stale 误判):整体丢弃,不提交任何变更;defer 负责 Rollback
		return false, nil, nil
	}

	// 2) 逐条插入变更事件(均用 tx.UpstreamChangeEvent,不用 r.client)
	inserted := make([]service.UpstreamChangeEvent, 0, len(in.Events))
	for _, d := range in.Events {
		e, err := tx.UpstreamChangeEvent.Create().
			SetProviderID(in.Claim.ProviderID).
			SetType(d.Type).
			SetSummary(d.Summary).
			SetDetail(d.Detail).
			SetNotified(false).
			Save(ctx)
		if err != nil {
			return false, nil, err
		}
		inserted = append(inserted, service.UpstreamChangeEvent{
			ID:         e.ID,
			ProviderID: e.ProviderID,
			Type:       e.Type,
			Summary:    e.Summary,
			Detail:     e.Detail,
			Notified:   e.Notified,
			CreatedAt:  e.CreatedAt,
		})
	}

	// 3) 提交事务
	if err := tx.Commit(); err != nil {
		return false, nil, err
	}
	return true, inserted, nil
}

// UpdateCredentials 仅更新凭证字段(登录续期后回写,不触碰刷新锁)。
func (r *upstreamProviderRepository) UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error {
	return r.client.UpstreamProvider.UpdateOneID(id).
		SetCredentials(credentials).
		Exec(ctx)
}

// MarkEventsNotified 批量将事件标记为已通知。
func (r *upstreamProviderRepository) MarkEventsNotified(ctx context.Context, eventIDs []int64) error {
	if len(eventIDs) == 0 {
		return nil
	}
	_, err := r.client.UpstreamChangeEvent.Update().
		Where(upstreamchangeevent.IDIn(eventIDs...)).
		SetNotified(true).
		Save(ctx)
	return err
}

// ─────────────────────────── UpstreamAccountLister ───────────────────────────

// upstreamAccountListerImpl 列出 type=upstream 的账号(关联帐号匹配用,spec §9)。
type upstreamAccountListerImpl struct {
	client *dbent.Client
}

// NewUpstreamAccountLister 构造 service.UpstreamAccountLister 实现(R2.3 导出接口)。
func NewUpstreamAccountLister(client *dbent.Client) service.UpstreamAccountLister {
	return &upstreamAccountListerImpl{client: client}
}

// ListUpstreamTypeAccounts 返回所有 type=upstream 的账号，credentials["base_url"] 映射为 BaseURL。
func (r *upstreamAccountListerImpl) ListUpstreamTypeAccounts(ctx context.Context) ([]service.UpstreamLinkedAccount, error) {
	rows, err := r.client.Account.Query().
		Where(account.TypeEQ(domain.AccountTypeUpstream)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.UpstreamLinkedAccount, 0, len(rows))
	for _, a := range rows {
		baseURL, _ := a.Credentials["base_url"].(string)
		out = append(out, service.UpstreamLinkedAccount{
			ID:       a.ID,
			Name:     a.Name,
			Platform: a.Platform,
			Status:   a.Status,
			BaseURL:  baseURL,
		})
	}
	return out, nil
}
