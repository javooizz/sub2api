#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
公开分组渠道定价同步脚本 (sub2api / onebool 运维工具)
====================================================

用途
----
为所有「公开分组」(is_exclusive=false) 自动配置渠道定价:
  1. 扫描每个公开分组下【可调度账号】实际可用的模型 (非平台默认全集);
  2. 从 LiteLLM 默认目录抓取每个模型的标准基准价;
  3. 以「渠道映射后的模型」为计费基准 (billing_model_source=channel_mapped);
  4. 幂等地创建 / 更新渠道 (按渠道名匹配, 已存在则 PUT 更新, 否则 POST 新建)。

渠道存「标准基准价」, 各分组的 rate_multiplier 负责差异化定价。

鉴权
----
admin API key 走 HTTP 头 `x-api-key: admin-<64hex>` (不是 Authorization: Bearer)。

用法
----
  # 1) 只读预演: 扫描 + 生成 JSON 到 docs/, 不写生产 (默认行为, 强烈建议先跑)
  ONEBOOL_ADMIN_KEY=admin-xxxx python3 scripts/sync_public_group_pricing.py

  # 2) 确认无误后真正写入
  ONEBOOL_ADMIN_KEY=admin-xxxx python3 scripts/sync_public_group_pricing.py --apply

  # 3) 只处理单个渠道 (用于"先建1个验证再继续")
  ONEBOOL_ADMIN_KEY=admin-xxxx python3 scripts/sync_public_group_pricing.py --apply --only cc_max

环境变量 / 参数
  ONEBOOL_ADMIN_KEY   admin API key (必填, 或用 --api-key)
  ONEBOOL_BASE_URL    admin API 基址 (默认 https://onebool.com/api/v1/admin, 或用 --base-url)

依赖: 仅需系统自带的 python3 + curl。
"""

import argparse
import json
import os
import subprocess
import sys
import time

# ===========================================================================
# 可配置项 (运维按需调整)
# ===========================================================================

# 合并分组: 多个公开分组共用一个渠道。按【分组名】匹配, 不依赖易变的 group id。
#   channel_name   : 合并后渠道名
#   group_names    : 参与合并的分组名 (取模型并集)
#   description_from: 渠道 description 取哪个分组的备注
MERGES = [
    {
        "channel_name": "codex_plus",
        "group_names": ["codex_plus", "codex-plus（量大干净清爽无异味）"],
        "description_from": "codex_plus",
    },
]

# 排除模型: 某渠道不计入这些模型 (即使账号扫描到也剔除)。键为渠道名。
EXCLUDE_MODELS = {
    "codex_plus": ["gpt-5.3-codex-spark"],
    "gpt-image": ["gpt-5.3-codex-spark"],
}

# 固定策略
BILLING_MODEL_SOURCE = "channel_mapped"   # 以渠道映射后的模型计费
DEFAULT_BILLING_MODE = "token"            # LiteLLM 目录查不到 mode 时的兜底

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DEFAULT_BASE_URL = "https://onebool.com/api/v1/admin"
DOC_PATH = os.path.join(REPO_ROOT, "docs", "channel-pricing-public-groups.json")
# LiteLLM 目录本地缓存 (backend 拉取的官方 model_prices 数据)。
# admin /channels/model-pricing 端点只回 flat 价格、不带 mode/按张价,
# 计费模式 (image_generation → image) 与 output_cost_per_image 从这里补齐。
LITELLM_CATALOG_PATH = os.path.join(REPO_ROOT, "backend", "data", "model_pricing.json")

# ===========================================================================
# HTTP (用 curl, 规避 WAF 对脚本 UA 的拦截)
# ===========================================================================

class Api:
    def __init__(self, base_url, api_key):
        self.base = base_url.rstrip("/")
        self.key = api_key

    def _curl(self, args):
        # 串行大量请求时偶发空响应 (WAF/网络抖动), 重试 3 次, 间隔递增
        last = ""
        for attempt in range(3):
            if attempt:
                time.sleep(2 * attempt)
            out = subprocess.run(
                ["curl", "-s", "-m", "30", "-H", f"x-api-key: {self.key}"] + args,
                capture_output=True, text=True,
            ).stdout
            try:
                return json.loads(out)
            except json.JSONDecodeError:
                last = out
        raise RuntimeError(f"非 JSON 响应 (重试 3 次后, {args[-1]}): {last[:300]}")

    def get(self, path):
        return self._curl([f"{self.base}{path}"])

    def post(self, path, payload):
        return self._curl(["-X", "POST", "-H", "Content-Type: application/json",
                            "--data", json.dumps(payload, ensure_ascii=False),
                            f"{self.base}{path}"])

    def put(self, path, payload):
        return self._curl(["-X", "PUT", "-H", "Content-Type: application/json",
                            "--data", json.dumps(payload, ensure_ascii=False),
                            f"{self.base}{path}"])


# ===========================================================================
# 数据采集
# ===========================================================================

def list_public_groups(api):
    """返回公开分组 (is_exclusive=false 且 status=active)。"""
    r = api.get("/groups/all")
    if r.get("code") != 0:
        raise RuntimeError(f"获取分组失败: {r}")
    groups = []
    for g in r["data"]:
        if not g.get("is_exclusive") and g.get("status") == "active":
            groups.append({
                "id": g["id"], "name": g["name"],
                "description": g.get("description", ""),
                "platform": g.get("platform", ""),
                "rate_multiplier": g.get("rate_multiplier"),
            })
    return groups


def aggregate_group_models(api, group_id):
    """聚合某分组下【可调度账号】实际可用的模型 (并集, 排序)。"""
    r = api.get(f"/accounts?group={group_id}&page=1&page_size=500&lite=true")
    data = r.get("data", {})
    items = data["items"] if isinstance(data, dict) else data
    models = set()
    for a in items:
        if not a.get("schedulable"):
            continue
        mr = api.get(f"/accounts/{a['id']}/models")
        for m in (mr.get("data") or []):
            models.add(m["id"] if isinstance(m, dict) else m)
    return sorted(models)


# 默认定价缓存, 避免重复请求
_pricing_cache = {}

def model_default_pricing(api, model):
    if model in _pricing_cache:
        return _pricing_cache[model]
    # base 已含 /api/v1/admin, 故此处路径为 /channels/model-pricing
    r = api.get(f"/channels/model-pricing?model={model}")
    p = r.get("data", {}) if r.get("code") == 0 else {}
    _pricing_cache[model] = p
    return p


# LiteLLM 目录懒加载缓存 (None=未加载, {}=加载失败降级)
_litellm_catalog = None

def litellm_entry(model):
    """从本地 LiteLLM 目录缓存查模型条目 (mode / output_cost_per_image 等)。
    键可能是裸名或带 provider 前缀, 逐一尝试; 查不到返回 {}。"""
    global _litellm_catalog
    if _litellm_catalog is None:
        try:
            with open(LITELLM_CATALOG_PATH, encoding="utf-8") as f:
                d = json.load(f)
            _litellm_catalog = d.get("data", d)
        except (OSError, json.JSONDecodeError) as e:
            print(f"  ⚠️  LiteLLM 目录加载失败 ({e}), billing_mode 全部回退 token", file=sys.stderr)
            _litellm_catalog = {}
    for key in (model, f"openai/{model}", f"anthropic/{model}", f"gemini/{model}"):
        entry = _litellm_catalog.get(key)
        if isinstance(entry, dict):
            return entry
    return {}


def billing_mode_for(model):
    """计费模式判定 (与后端渠道校验语义对齐: image 模式=按张计费, 必须有每张价):
    - LiteLLM mode=image_generation 且有 output_cost_per_image (真按张, gemini image 系)
      → image + per_request_price
    - mode=image_generation 但无按张价 (gpt-image 系, 按图像 token 计费)
      → token (保留 image_output_price, 由前端 token 模式渲染图像输出行)
    - 其余 → token"""
    le = litellm_entry(model)
    if le.get("mode") == "image_generation" and le.get("output_cost_per_image"):
        return "image"
    return DEFAULT_BILLING_MODE


def pricing_entry(api, platform, model):
    p = model_default_pricing(api, model)
    if not p.get("found"):
        print(f"  ⚠️  模型 {model} 无默认定价, 价格留空, 请人工补", file=sys.stderr)
    mode = billing_mode_for(model)
    # 按张计费价 (gemini image 系模型有 output_cost_per_image; admin 端点不返回, 从目录补)。
    # 仅 image 模式才带——chat 模型也可能有 output_cost_per_image (聊天附带出图),
    # 塞进 token 条目会污染展示/计费语义。
    per_image = (litellm_entry(model).get("output_cost_per_image") or None) if mode == "image" else None

    def nz(v):
        """LiteLLM 语义: 0 = 未配置 (与后端 nonZeroPtr 同款), 不物化为真实 0 价。"""
        return v if v else None

    return {
        "platform": platform,
        "models": [model],
        "billing_mode": mode,
        "input_price": nz(p.get("input_price")),
        "output_price": nz(p.get("output_price")),
        "cache_write_price": nz(p.get("cache_write_price")),
        "cache_read_price": nz(p.get("cache_read_price")),
        "image_output_price": nz(p.get("image_output_price")),
        "per_request_price": per_image,
        "intervals": [],
    }


# ===========================================================================
# 渠道规划
# ===========================================================================

def plan_channels(api, public_groups):
    """根据公开分组 + MERGES 配置, 规划出待同步的渠道列表。"""
    by_name = {g["name"]: g for g in public_groups}
    merged_names = set()
    channels = []

    # 合并渠道
    for m in MERGES:
        member_groups = [by_name[n] for n in m["group_names"] if n in by_name]
        if not member_groups:
            continue
        merged_names.update(g["name"] for g in member_groups)
        primary = next((g for g in member_groups if g["name"] == m["description_from"]),
                       member_groups[0])
        channels.append({
            "name": m["channel_name"],
            "description": primary["description"],
            "platform": member_groups[0]["platform"],
            "group_ids": [g["id"] for g in member_groups],
            "_group_labels": [f"{g['name']}(id={g['id']},x{g['rate_multiplier']})" for g in member_groups],
        })

    # 未合并的分组各自成渠道
    for g in public_groups:
        if g["name"] in merged_names:
            continue
        channels.append({
            "name": g["name"],
            "description": g["description"],
            "platform": g["platform"],
            "group_ids": [g["id"]],
            "_group_labels": [f"{g['name']}(id={g['id']},x{g['rate_multiplier']})"],
        })

    # 填充模型 + 定价
    for ch in channels:
        models = set()
        for gid in ch["group_ids"]:
            models.update(aggregate_group_models(api, gid))
        excl = set(EXCLUDE_MODELS.get(ch["name"], []))
        models = sorted(m for m in models if m not in excl)
        ch["model_pricing"] = [pricing_entry(api, ch["platform"], m) for m in models]
    return channels


def build_payload(ch):
    return {
        "name": ch["name"],
        "description": ch["description"],
        "group_ids": ch["group_ids"],
        "billing_model_source": BILLING_MODEL_SOURCE,
        "restrict_models": False,
        "model_mapping": {},
        "model_pricing": ch["model_pricing"],
    }


# ===========================================================================
# 写入 (幂等: 按渠道名匹配, 存在则 PUT, 否则 POST)
# ===========================================================================

def existing_channels(api):
    r = api.get("/channels?page=1&page_size=200")
    data = r.get("data", {})
    items = data.get("items", []) if isinstance(data, dict) else (data or [])
    return {c["name"]: c for c in items}


def apply_channel(api, ch, existing):
    payload = build_payload(ch)
    if ch["name"] in existing:
        cid = existing[ch["name"]]["id"]
        # 更新需带上当前状态等字段
        cur = existing[ch["name"]]
        payload.update({
            "status": cur.get("status", "active"),
            "apply_pricing_to_account_stats": cur.get("apply_pricing_to_account_stats", False),
            "account_stats_pricing_rules": cur.get("account_stats_pricing_rules", []),
        })
        r = api.put(f"/channels/{cid}", payload)
        action = "更新"
    else:
        r = api.post("/channels", payload)
        action = "新建"
    ok = r.get("code") == 0
    d = r.get("data", {})
    print(f"  {'✅' if ok else '❌'} {action} {ch['name']:<18} "
          f"id={d.get('id')} group_ids={d.get('group_ids')} 模型数={len(d.get('model_pricing', []))}"
          + ("" if ok else f"  错误: {r}"))
    return ok


# ===========================================================================
# 主流程
# ===========================================================================

def write_doc(channels):
    doc = {
        "_meta": {
            "purpose": "公开分组(is_exclusive=false)渠道定价 — 由 scripts/sync_public_group_pricing.py 生成",
            "billing_model_source": BILLING_MODEL_SOURCE,
            "billing_mode": "按 LiteLLM mode 逐模型判定 (image_generation → image, 其余 token)",
            "price_unit": "USD per token (admin API 存储单位); per_request_price 为每张图 USD",
            "price_source": "GET /admin/channels/model-pricing (LiteLLM 默认目录价) + 本地目录补 mode/按张价",
            "models_source": "各分组【可调度账号】/accounts/:id/models 实际聚合, 非平台默认全集",
            "merges": MERGES,
            "exclude_models": EXCLUDE_MODELS,
        },
        "channels": [
            {"_groups": ch["_group_labels"], **build_payload(ch)} for ch in channels
        ],
    }
    os.makedirs(os.path.dirname(DOC_PATH), exist_ok=True)
    with open(DOC_PATH, "w", encoding="utf-8") as f:
        json.dump(doc, f, ensure_ascii=False, indent=2)
    print(f"\n📄 已写入 {DOC_PATH}")


def main():
    ap = argparse.ArgumentParser(description="公开分组渠道定价同步")
    ap.add_argument("--apply", action="store_true", help="真正写入生产 (默认只预演)")
    ap.add_argument("--only", metavar="NAME", help="只处理指定渠道名")
    ap.add_argument("--api-key", default=os.environ.get("ONEBOOL_ADMIN_KEY"),
                    help="admin API key (或环境变量 ONEBOOL_ADMIN_KEY)")
    ap.add_argument("--base-url", default=os.environ.get("ONEBOOL_BASE_URL", DEFAULT_BASE_URL),
                    help=f"admin API 基址 (默认 {DEFAULT_BASE_URL})")
    args = ap.parse_args()

    if not args.api_key:
        ap.error("缺少 admin API key, 请设置 ONEBOOL_ADMIN_KEY 或传 --api-key")

    api = Api(args.base_url, args.api_key)

    print(f"🔍 基址: {api.base}")
    public = list_public_groups(api)
    print(f"🔍 公开分组 {len(public)} 个: " + ", ".join(f"{g['name']}(id={g['id']})" for g in public))

    channels = plan_channels(api, public)
    if args.only:
        channels = [c for c in channels if c["name"] == args.only]
        if not channels:
            ap.error(f"未找到渠道名 {args.only}")

    print(f"\n📋 规划 {len(channels)} 个渠道:")
    for ch in channels:
        print(f"  {ch['name']:<18} group_ids={ch['group_ids']} 模型数={len(ch['model_pricing'])} "
              f"模型={[p['models'][0] for p in ch['model_pricing']]}")

    write_doc(channels)

    if not args.apply:
        print("\n💡 预演模式 (未写生产)。确认 docs JSON 无误后加 --apply 真正写入。")
        return

    print("\n🚀 写入生产 (幂等 upsert):")
    existing = existing_channels(api)
    ok = sum(apply_channel(api, ch, existing) for ch in channels)
    print(f"\n完成: {ok}/{len(channels)} 成功")


if __name__ == "__main__":
    main()
