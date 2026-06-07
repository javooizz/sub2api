package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// DiffInput 快照对比输入(spec §7.2)。
type DiffInput struct {
	Old                 *domain.UpstreamSnapshot // nil = 首次快照
	New                 *domain.UpstreamSnapshot
	BalanceThreshold    *float64
	BalanceAlerted      bool
	NotifyOnPriceChange bool
}

// DiffOutput 事件草稿 + 新的余额告警标记。
type DiffOutput struct {
	Events         []UpstreamChangeEventDraft
	BalanceAlerted bool
}

// DiffUpstreamSnapshots 纯函数:对比新旧快照生成事件草稿。
// 首次快照只做余额检测;partial 快照缺失的部分跳过对应 diff(spec §7.2 补充语义)。
func DiffUpstreamSnapshots(in DiffInput) DiffOutput {
	out := DiffOutput{BalanceAlerted: in.BalanceAlerted}

	// —— 余额跨越式检测(首次快照也执行)——
	if in.BalanceThreshold != nil && in.New.Balance != nil {
		bal, th := *in.New.Balance, *in.BalanceThreshold
		switch {
		case bal < th && !in.BalanceAlerted:
			out.Events = append(out.Events, UpstreamChangeEventDraft{
				Type:    domain.UpstreamEventBalanceLow,
				Summary: fmt.Sprintf("余额 %s 低于阈值 %s", trimFloat(bal), trimFloat(th)),
				Detail:  map[string]any{"balance": bal, "threshold": th},
				Notify:  true,
			})
			out.BalanceAlerted = true
		case bal >= th && in.BalanceAlerted:
			out.Events = append(out.Events, UpstreamChangeEventDraft{
				Type:    domain.UpstreamEventBalanceRecovered,
				Summary: fmt.Sprintf("余额回升至 %s(阈值 %s)", trimFloat(bal), trimFloat(th)),
				Detail:  map[string]any{"balance": bal, "threshold": th},
				Notify:  false,
			})
			out.BalanceAlerted = false
		}
	}

	// —— 变更类事件:首次快照不做 ——
	if in.Old == nil {
		return out
	}

	// 分组与模型(双方都有数据才比较;partial 缺失跳过)
	if in.Old.Groups != nil && in.New.Groups != nil {
		out.Events = append(out.Events, diffGroups(in.Old.Groups, in.New.Groups, in.NotifyOnPriceChange)...)
	}
	// 模型价格深度比较
	if in.Old.ModelPricing != nil && in.New.ModelPricing != nil {
		out.Events = append(out.Events, diffModelPricing(in.Old.ModelPricing, in.New.ModelPricing, in.NotifyOnPriceChange)...)
	}
	return out
}

func diffGroups(oldGroups, newGroups []domain.UpstreamGroupSnapshot, notifyPrice bool) []UpstreamChangeEventDraft {
	var events []UpstreamChangeEventDraft
	oldMap := make(map[string]domain.UpstreamGroupSnapshot, len(oldGroups))
	for _, g := range oldGroups {
		oldMap[g.Name] = g
	}
	newMap := make(map[string]domain.UpstreamGroupSnapshot, len(newGroups))
	for _, g := range newGroups {
		newMap[g.Name] = g
	}

	for _, g := range diffSortedGroupKeys(newMap) {
		ng := newMap[g]
		og, existed := oldMap[g]
		if !existed {
			events = append(events, UpstreamChangeEventDraft{
				Type: domain.UpstreamEventGroupAdded, Summary: "新增分组 " + g,
				Detail: map[string]any{"group": g}, Notify: notifyPrice,
			})
			continue
		}
		// 倍率变化
		if !floatPtrEqual(og.Ratio, ng.Ratio) {
			events = append(events, UpstreamChangeEventDraft{
				Type:    domain.UpstreamEventPriceChanged,
				Summary: fmt.Sprintf("分组 %s 倍率 %s → %s", g, floatPtrString(og.Ratio), floatPtrString(ng.Ratio)),
				Detail:  map[string]any{"group": g, "old_ratio": og.Ratio, "new_ratio": ng.Ratio},
				Notify:  notifyPrice,
			})
		}
		// 模型增删(集合差)
		added, removed := stringSetDiff(og.Models, ng.Models)
		if len(added) > 0 {
			events = append(events, UpstreamChangeEventDraft{
				Type:    domain.UpstreamEventModelAdded,
				Summary: "分组 " + g + " 新增模型: " + joinSorted(added),
				Detail:  map[string]any{"group": g, "models": added}, Notify: notifyPrice,
			})
		}
		if len(removed) > 0 {
			events = append(events, UpstreamChangeEventDraft{
				Type:    domain.UpstreamEventModelRemoved,
				Summary: "分组 " + g + " 移除模型: " + joinSorted(removed),
				Detail:  map[string]any{"group": g, "models": removed}, Notify: notifyPrice,
			})
		}
	}
	for _, g := range diffSortedGroupKeys(oldMap) {
		if _, exists := newMap[g]; !exists {
			events = append(events, UpstreamChangeEventDraft{
				Type: domain.UpstreamEventGroupRemoved, Summary: "移除分组 " + g,
				Detail: map[string]any{"group": g}, Notify: notifyPrice,
			})
		}
	}
	return events
}

// diffModelPricing 按模型键深度比较原始 JSON(spec §7.2:不解析价格字段语义)。
func diffModelPricing(oldP, newP map[string]json.RawMessage, notifyPrice bool) []UpstreamChangeEventDraft {
	var changed []string
	for model, newRaw := range newP {
		oldRaw, existed := oldP[model]
		if !existed {
			continue // 模型新增由 groups diff 表达,定价表新键不单独报
		}
		if !jsonSemanticEqual(oldRaw, newRaw) {
			changed = append(changed, model)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	sort.Strings(changed)
	return []UpstreamChangeEventDraft{{
		Type:    domain.UpstreamEventPriceChanged,
		Summary: "模型价格变更: " + joinSorted(changed),
		Detail:  map[string]any{"models": changed},
		Notify:  notifyPrice,
	}}
}

// —— 工具 ——

// jsonSemanticEqual 字节不等时再做规范化比较(键序无关)。
func jsonSemanticEqual(a, b []byte) bool {
	if bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b)) {
		return true
	}
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return false
	}
	na, _ := json.Marshal(canonicalize(va))
	nb, _ := json.Marshal(canonicalize(vb))
	return bytes.Equal(na, nb)
}

// canonicalize map 经 json.Marshal 已按键排序,递归处理即可。
func canonicalize(v any) any { return v }

func floatPtrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func floatPtrString(v *float64) string {
	if v == nil {
		return "-"
	}
	return trimFloat(*v)
}

func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func stringSetDiff(oldList, newList []string) (added, removed []string) {
	oldSet := make(map[string]struct{}, len(oldList))
	for _, s := range oldList {
		oldSet[s] = struct{}{}
	}
	newSet := make(map[string]struct{}, len(newList))
	for _, s := range newList {
		newSet[s] = struct{}{}
		if _, ok := oldSet[s]; !ok {
			added = append(added, s)
		}
	}
	for _, s := range oldList {
		if _, ok := newSet[s]; !ok {
			removed = append(removed, s)
		}
	}
	return added, removed
}

func joinSorted(items []string) string {
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	out := ""
	for i, s := range sorted {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

func diffSortedGroupKeys(m map[string]domain.UpstreamGroupSnapshot) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
