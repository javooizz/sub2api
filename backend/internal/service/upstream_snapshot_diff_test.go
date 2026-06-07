package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

func f64(v float64) *float64 { return &v }

func snap(balance *float64, groups []domain.UpstreamGroupSnapshot, pricing map[string]json.RawMessage) *domain.UpstreamSnapshot {
	return &domain.UpstreamSnapshot{Balance: balance, Groups: groups, ModelPricing: pricing}
}

func findEvent(drafts []UpstreamChangeEventDraft, typ string) *UpstreamChangeEventDraft {
	for i := range drafts {
		if drafts[i].Type == typ {
			return &drafts[i]
		}
	}
	return nil
}

func TestDiff_FirstSnapshotOnlyBalanceCheck(t *testing.T) {
	// 首次快照(old=nil):只做 balance_low,不产生变更类事件(spec §7.2 补充语义)
	in := DiffInput{
		Old: nil,
		New: snap(f64(3), []domain.UpstreamGroupSnapshot{{Name: "vip", Ratio: f64(1)}}, nil),
		BalanceThreshold: f64(5), BalanceAlerted: false, NotifyOnPriceChange: true,
	}
	out := DiffUpstreamSnapshots(in)
	if ev := findEvent(out.Events, domain.UpstreamEventBalanceLow); ev == nil || !ev.Notify {
		t.Fatal("首次快照余额低应产生 balance_low 且通知")
	}
	if !out.BalanceAlerted {
		t.Fatal("balance_alerted 应置位")
	}
	for _, ev := range out.Events {
		if ev.Type != domain.UpstreamEventBalanceLow {
			t.Fatalf("首次快照不应产生变更类事件,出现: %s", ev.Type)
		}
	}
}

func TestDiff_BalanceCrossingAndReset(t *testing.T) {
	// 已告警状态下持续低于阈值 → 不重复告警
	in := DiffInput{
		Old: snap(f64(3), nil, nil), New: snap(f64(2), nil, nil),
		BalanceThreshold: f64(5), BalanceAlerted: true, NotifyOnPriceChange: true,
	}
	out := DiffUpstreamSnapshots(in)
	if ev := findEvent(out.Events, domain.UpstreamEventBalanceLow); ev != nil {
		t.Fatal("已告警未回升不应重复 balance_low")
	}
	if !out.BalanceAlerted {
		t.Fatal("仍低于阈值应保持告警标记")
	}

	// 回升 → balance_recovered(仅记录)+ 标记重置
	in = DiffInput{
		Old: snap(f64(2), nil, nil), New: snap(f64(10), nil, nil),
		BalanceThreshold: f64(5), BalanceAlerted: true, NotifyOnPriceChange: true,
	}
	out = DiffUpstreamSnapshots(in)
	ev := findEvent(out.Events, domain.UpstreamEventBalanceRecovered)
	if ev == nil {
		t.Fatal("回升应产生 balance_recovered")
	}
	if ev.Notify {
		t.Fatal("balance_recovered 仅记录不通知")
	}
	if out.BalanceAlerted {
		t.Fatal("回升应重置告警标记")
	}

	// 重置后再次跌破 → 再次告警
	in = DiffInput{
		Old: snap(f64(10), nil, nil), New: snap(f64(4), nil, nil),
		BalanceThreshold: f64(5), BalanceAlerted: false, NotifyOnPriceChange: true,
	}
	out = DiffUpstreamSnapshots(in)
	if ev := findEvent(out.Events, domain.UpstreamEventBalanceLow); ev == nil || !ev.Notify {
		t.Fatal("重置后跌破应再次 balance_low")
	}
}

func TestDiff_NoThresholdNoBalanceEvent(t *testing.T) {
	in := DiffInput{
		Old: snap(f64(10), nil, nil), New: snap(f64(0.1), nil, nil),
		BalanceThreshold: nil, BalanceAlerted: false, NotifyOnPriceChange: true,
	}
	out := DiffUpstreamSnapshots(in)
	if len(out.Events) != 0 {
		t.Fatalf("未设阈值不应有余额事件: %v", out.Events)
	}
}

func TestDiff_GroupRatioChanged(t *testing.T) {
	in := DiffInput{
		Old: snap(nil, []domain.UpstreamGroupSnapshot{{Name: "vip", Ratio: f64(1.0)}}, nil),
		New: snap(nil, []domain.UpstreamGroupSnapshot{{Name: "vip", Ratio: f64(1.5)}}, nil),
		NotifyOnPriceChange: true,
	}
	out := DiffUpstreamSnapshots(in)
	ev := findEvent(out.Events, domain.UpstreamEventPriceChanged)
	if ev == nil || !ev.Notify {
		t.Fatalf("倍率变化应产生可通知的 price_changed: %v", out.Events)
	}
	if ev.Summary != "分组 vip 倍率 1 → 1.5" {
		t.Fatalf("摘要不符: %q", ev.Summary)
	}
}

func TestDiff_PriceChangeNotifyOff(t *testing.T) {
	in := DiffInput{
		Old: snap(nil, []domain.UpstreamGroupSnapshot{{Name: "vip", Ratio: f64(1.0)}}, nil),
		New: snap(nil, []domain.UpstreamGroupSnapshot{{Name: "vip", Ratio: f64(2.0)}}, nil),
		NotifyOnPriceChange: false,
	}
	out := DiffUpstreamSnapshots(in)
	ev := findEvent(out.Events, domain.UpstreamEventPriceChanged)
	if ev == nil {
		t.Fatal("事件仍应记录")
	}
	if ev.Notify {
		t.Fatal("notify_on_price_change=false 不应通知")
	}
}

func TestDiff_GroupAndModelAddRemove(t *testing.T) {
	in := DiffInput{
		Old: snap(nil, []domain.UpstreamGroupSnapshot{
			{Name: "a", Models: []string{"m1", "m2"}},
			{Name: "gone", Models: []string{"m1"}},
		}, nil),
		New: snap(nil, []domain.UpstreamGroupSnapshot{
			{Name: "a", Models: []string{"m1", "m3"}},
			{Name: "fresh", Models: []string{"m1"}},
		}, nil),
		NotifyOnPriceChange: true,
	}
	out := DiffUpstreamSnapshots(in)
	if ev := findEvent(out.Events, domain.UpstreamEventGroupAdded); ev == nil || ev.Summary != "新增分组 fresh" {
		t.Fatalf("group_added 不符: %v", ev)
	}
	if ev := findEvent(out.Events, domain.UpstreamEventGroupRemoved); ev == nil || ev.Summary != "移除分组 gone" {
		t.Fatalf("group_removed 不符: %v", ev)
	}
	if ev := findEvent(out.Events, domain.UpstreamEventModelAdded); ev == nil || ev.Summary != "分组 a 新增模型: m3" {
		t.Fatalf("model_added 不符: %v", ev)
	}
	if ev := findEvent(out.Events, domain.UpstreamEventModelRemoved); ev == nil || ev.Summary != "分组 a 移除模型: m2" {
		t.Fatalf("model_removed 不符: %v", ev)
	}
}

func TestDiff_ModelPricingDeepCompare(t *testing.T) {
	oldP := map[string]json.RawMessage{
		"gpt-x": json.RawMessage(`{"input":1,"output":2}`),
		"keep":  json.RawMessage(`{"input":1}`),
	}
	newP := map[string]json.RawMessage{
		"gpt-x": json.RawMessage(`{"input":1,"output":3}`),
		"keep":  json.RawMessage(`{"input":1}`),
	}
	in := DiffInput{Old: snap(nil, nil, oldP), New: snap(nil, nil, newP), NotifyOnPriceChange: true}
	out := DiffUpstreamSnapshots(in)
	ev := findEvent(out.Events, domain.UpstreamEventPriceChanged)
	if ev == nil || ev.Summary != "模型价格变更: gpt-x" {
		t.Fatalf("模型价格深度比较不符: %v", out.Events)
	}
}

func TestDiff_PartialNewSkipsAbsentSections(t *testing.T) {
	// 新快照 partial 且 groups 缺失 → 不应误报"全部分组被移除"
	in := DiffInput{
		Old: snap(f64(10), []domain.UpstreamGroupSnapshot{{Name: "a"}}, nil),
		New: &domain.UpstreamSnapshot{Balance: f64(10), Groups: nil, Partial: true},
		NotifyOnPriceChange: true,
	}
	out := DiffUpstreamSnapshots(in)
	if len(out.Events) != 0 {
		t.Fatalf("partial 快照缺失部分应跳过 diff: %v", out.Events)
	}
}

func TestDiff_NilNewSnapshotNoEvents(t *testing.T) {
	out := DiffUpstreamSnapshots(DiffInput{Old: snap(f64(10), nil, nil), New: nil, BalanceThreshold: f64(5), NotifyOnPriceChange: true})
	if len(out.Events) != 0 {
		t.Fatalf("New 为 nil 应无事件,实际: %v", out.Events)
	}
}
