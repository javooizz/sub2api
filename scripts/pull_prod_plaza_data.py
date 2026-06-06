#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
生产数据拉取脚本: 为本地调试「模型广场」灌入真实数据
=====================================================

用途
----
模型广场的数据源是 渠道(model_pricing) + 分组(rate_multiplier 等),
本脚本把生产环境的这两类数据通过 admin API 同步到本地:

  1. 拉取生产全部分组 → 按【分组名】幂等 upsert 到本地
     (同步 description/platform/rate_multiplier/is_exclusive/subscription_type/status,
      含 allow_image_generation / image_rate_* / image_price_×3 图片计费字段,
      供广场"分组图片价"真实计费链展示);
  2. 渠道定价取自 docs/channel-pricing-public-groups.json
     (由蓝本脚本 sync_public_group_pricing.py 预演生成, 可用 --refresh-snapshot 现场重抓);
  3. 把渠道 payload 中的生产 group_ids 按分组名重映射为本地 id,
     再按【渠道名】幂等 upsert 到本地。

安全
----
对生产只做 GET(只读); 写入目标必须是 localhost/127.0.0.1, 否则直接拒绝。

用法
----
  # 1) 预演 (默认): 只打印规划, 不写本地
  python3 scripts/pull_prod_plaza_data.py

  # 2) 真正写入本地
  python3 scripts/pull_prod_plaza_data.py --apply

  # 3) 先用蓝本脚本现场重抓生产最新定价快照, 再写入
  python3 scripts/pull_prod_plaza_data.py --refresh-snapshot --apply

环境变量 / 参数
  ONEBOOL_PROD_KEY    生产 admin API key (必填, 或 --prod-key)
  ONEBOOL_LOCAL_KEY   本地 admin API key (必填, 或 --local-key)
  ONEBOOL_PROD_BASE   生产 admin 基址 (默认 https://onebool.com/api/v1/admin)
  ONEBOOL_LOCAL_BASE  本地 admin 基址 (默认 http://localhost:3000/api/v1/admin)

依赖: 仅需系统自带的 python3 + curl。
"""

import argparse
import json
import os
import re
import subprocess
import sys

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SNAPSHOT_PATH = os.path.join(REPO_ROOT, "docs", "channel-pricing-public-groups.json")
BLUEPRINT_SCRIPT = os.path.join(REPO_ROOT, "scripts", "sync_public_group_pricing.py")

DEFAULT_PROD_BASE = "https://onebool.com/api/v1/admin"
DEFAULT_LOCAL_BASE = "http://localhost:3000/api/v1/admin"

# 分组同步白名单字段: 只同步模型广场展示所需的核心属性,
# 不碰 model_routing / fallback_group_id 等跨实体引用 (本地无对应数据)。
# 图片计费字段 (allow/rate/price×3) 供广场"分组图片价"真实计费链展示 (规格 2026-06-07 §6.1)。
GROUP_SYNC_FIELDS = ("description", "platform", "rate_multiplier",
                     "is_exclusive", "subscription_type",
                     "allow_image_generation", "image_rate_independent",
                     "image_rate_multiplier",
                     "image_price_1k", "image_price_2k", "image_price_4k")

# 指针价格字段: 本地 PUT/POST 时 JSON null 会被 Go 当"未提供"忽略,
# 更新场景需转 -1 触发清除 (normalizePrice 负数→nil)。
# 注意 image_rate_multiplier 不在此列——后端校验负数会 400, 生产为 null 时跳过该键。
CLEARABLE_PRICE_FIELDS = ("image_price_1k", "image_price_2k", "image_price_4k")


# ===========================================================================
# HTTP (用 curl, 与蓝本脚本一致)
# ===========================================================================

class Api:
    def __init__(self, base_url, api_key):
        self.base = base_url.rstrip("/")
        self.key = api_key

    def _curl(self, args):
        out = subprocess.run(
            ["curl", "-s", "-H", f"x-api-key: {self.key}"] + args,
            capture_output=True, text=True,
        ).stdout
        try:
            return json.loads(out)
        except json.JSONDecodeError:
            raise RuntimeError(f"非 JSON 响应 ({args[-1]}): {out[:300]}")

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

def fetch_groups(api):
    r = api.get("/groups/all")
    if r.get("code") != 0:
        raise RuntimeError(f"获取分组失败: {r}")
    return r["data"]


def fetch_channels(api):
    items, page = [], 1
    while True:
        r = api.get(f"/channels?page={page}&page_size=200")
        if r.get("code") != 0:
            raise RuntimeError(f"获取渠道失败: {r}")
        d = r["data"]
        batch = d["items"] if isinstance(d, dict) else d
        items.extend(batch or [])
        if not isinstance(d, dict) or page >= d.get("pages", 1):
            return items
        page += 1


def load_snapshot(refresh, prod_base, prod_key):
    """读取蓝本脚本生成的渠道定价快照; --refresh-snapshot 时先现场重抓(对生产只读)。"""
    if refresh:
        print(f"🔄 重抓生产定价快照 (蓝本脚本预演模式, 只读) ...")
        rc = subprocess.run(
            [sys.executable, BLUEPRINT_SCRIPT, "--base-url", prod_base, "--api-key", prod_key],
            cwd=REPO_ROOT,
        ).returncode
        if rc != 0:
            raise RuntimeError(f"蓝本脚本预演失败 (exit={rc})")
    if not os.path.exists(SNAPSHOT_PATH):
        raise RuntimeError(f"快照不存在: {SNAPSHOT_PATH}, 请加 --refresh-snapshot 生成")
    with open(SNAPSHOT_PATH, encoding="utf-8") as f:
        return json.load(f)


def parse_group_names(labels):
    """从快照 _groups 标签解析分组名: 'cc_max(id=2,x1.6)' → 'cc_max'。
    分组名本身可能含全角括号, 故按最后一个 '(id=' 切分。"""
    names = []
    for label in labels:
        m = re.match(r"^(.*)\(id=\d+,x[\d.]+\)$", label)
        if not m:
            raise RuntimeError(f"无法解析 _groups 标签: {label}")
        names.append(m.group(1))
    return names


# ===========================================================================
# 写入本地 (幂等: 按名称匹配, 存在则 PUT, 否则 POST)
# ===========================================================================

def upsert_group(local, prod_group, existing_by_name, apply):
    name = prod_group["name"]
    payload = {k: prod_group.get(k) for k in GROUP_SYNC_FIELDS}
    cur = existing_by_name.get(name)
    if cur:
        # 已有同名分组: 仅当核心字段有差异时 PUT
        diff = {k: v for k, v in payload.items() if cur.get(k) != v}
        if not diff:
            print(f"  ⏭️  分组 {name:<36} 无变化 (本地 id={cur['id']})")
            return cur["id"]
        # 清除语义: 价格字段生产为 null 且本地非 null → 传 -1 触发后端清除
        for k in CLEARABLE_PRICE_FIELDS:
            if k in diff and diff[k] is None:
                diff[k] = -1
        # image_rate_multiplier 后端拒绝负数; 生产为 null(理论上不出现, 默认 1)时跳过保持本地
        if "image_rate_multiplier" in diff and diff["image_rate_multiplier"] is None:
            del diff["image_rate_multiplier"]
        diff["status"] = prod_group.get("status", "active")
        if not apply:
            print(f"  📝 分组 {name:<36} 将更新: {json.dumps(diff, ensure_ascii=False)}")
            return cur["id"]
        r = local.put(f"/groups/{cur['id']}", diff)
        ok = r.get("code") == 0
        print(f"  {'✅' if ok else '❌'} 更新分组 {name:<36} id={cur['id']}"
              + ("" if ok else f"  错误: {r}"))
        return cur["id"] if ok else None
    # 新建
    payload["name"] = name
    if not apply:
        print(f"  📝 分组 {name:<36} 将新建: platform={payload['platform']} "
              f"x{payload['rate_multiplier']} excl={payload['is_exclusive']}")
        return None
    r = local.post("/groups", payload)
    ok = r.get("code") == 0
    gid = (r.get("data") or {}).get("id")
    print(f"  {'✅' if ok else '❌'} 新建分组 {name:<36} id={gid}"
          + ("" if ok else f"  错误: {r}"))
    return gid if ok else None


# ===========================================================================
# 占位账号 (模型广场账号真相源修订后, 本地需要账号才有模型可展示)
# ===========================================================================

def fetch_group_models(prod, group_id):
    """聚合某分组下可调度账号实际可用的模型 (与蓝本脚本同口径, 对生产只读)。"""
    r = prod.get(f"/accounts?group={group_id}&page=1&page_size=500&lite=true")
    data = r.get("data", {})
    items = data["items"] if isinstance(data, dict) else (data or [])
    models = set()
    for a in items:
        if not a.get("schedulable"):
            continue
        mr = prod.get(f"/accounts/{a['id']}/models")
        for m in (mr.get("data") or []):
            models.add(m["id"] if isinstance(m, dict) else m)
    return sorted(models)


def list_local_accounts(local):
    r = local.get("/accounts?page=1&page_size=500")
    data = r.get("data", {})
    items = data["items"] if isinstance(data, dict) else (data or [])
    return {a["name"]: a for a in items}


def upsert_seed_account(local, group_name, local_gid, platform, models, existing_by_name, apply):
    """为分组创建/更新 1 个占位账号: 假凭据 + model_mapping={模型:模型}。
    mapping 键即广场推导结果, 展示与生产等价; 无真实凭据, 不可真实转发。"""
    name = f"plaza-seed-{group_name}"
    payload = {
        "name": name,
        "platform": platform,
        "type": "apikey",
        "credentials": {
            "api_key": "placeholder-not-usable",
            "model_mapping": {m: m for m in models},
        },
        "group_ids": [local_gid],
        "priority": 99,
        "concurrency": 1,
    }
    cur = existing_by_name.get(name)
    action = "更新" if cur else "新建"
    if not apply:
        # 预演模式不拉本地账号列表, 无法区分新建/更新, 文案统一为"upsert"
        print(f"  📝 占位账号 {name:<44} 将 upsert: 模型数={len(models)}")
        return True
    r = local.put(f"/accounts/{cur['id']}", payload) if cur else local.post("/accounts", payload)
    ok = r.get("code") == 0
    print(f"  {'✅' if ok else '❌'} {action}占位账号 {name:<44} 模型数={len(models)}"
          + ("" if ok else f"  错误: {r}"))
    return ok


# ===========================================================================
# 渠道同步
# ===========================================================================

def upsert_channel(local, ch, local_group_ids, existing_by_name, apply):
    name = ch["name"]
    payload = {
        "name": name,
        "description": ch.get("description", ""),
        "group_ids": local_group_ids,
        "billing_model_source": ch.get("billing_model_source", "channel_mapped"),
        "restrict_models": ch.get("restrict_models", False),
        "model_mapping": ch.get("model_mapping") or {},
        "model_pricing": ch["model_pricing"],
    }
    models = [m for p in ch["model_pricing"] for m in p["models"]]
    cur = existing_by_name.get(name)
    action = "更新" if cur else "新建"
    if not apply:
        print(f"  📝 渠道 {name:<20} 将{action}: group_ids={local_group_ids} "
              f"模型数={len(models)}")
        return True
    if cur:
        payload["status"] = cur.get("status", "active")
        r = local.put(f"/channels/{cur['id']}", payload)
    else:
        r = local.post("/channels", payload)
    ok = r.get("code") == 0
    d = r.get("data") or {}
    print(f"  {'✅' if ok else '❌'} {action}渠道 {name:<20} id={d.get('id')} "
          f"group_ids={d.get('group_ids')} 定价条目={len(d.get('model_pricing') or [])}"
          + ("" if ok else f"  错误: {r}"))
    return ok


# ===========================================================================
# 主流程
# ===========================================================================

def main():
    ap = argparse.ArgumentParser(description="拉取生产分组+渠道定价到本地 (模型广场调试)")
    ap.add_argument("--apply", action="store_true", help="真正写入本地 (默认只预演)")
    ap.add_argument("--refresh-snapshot", action="store_true",
                    help="先用蓝本脚本现场重抓生产定价快照 (对生产只读, 较慢)")
    ap.add_argument("--prod-key", default=os.environ.get("ONEBOOL_PROD_KEY"))
    ap.add_argument("--local-key", default=os.environ.get("ONEBOOL_LOCAL_KEY"))
    ap.add_argument("--prod-base", default=os.environ.get("ONEBOOL_PROD_BASE", DEFAULT_PROD_BASE))
    ap.add_argument("--local-base", default=os.environ.get("ONEBOOL_LOCAL_BASE", DEFAULT_LOCAL_BASE))
    args = ap.parse_args()

    if not args.prod_key:
        ap.error("缺少生产 admin key (ONEBOOL_PROD_KEY 或 --prod-key)")
    if not args.local_key:
        ap.error("缺少本地 admin key (ONEBOOL_LOCAL_KEY 或 --local-key)")
    # 安全闸: 写入目标只允许本机
    if not re.match(r"^https?://(localhost|127\.0\.0\.1)([:/]|$)", args.local_base):
        ap.error(f"写入目标必须是 localhost: {args.local_base}")

    prod = Api(args.prod_base, args.prod_key)
    local = Api(args.local_base, args.local_key)
    print(f"🔍 生产(只读): {prod.base}\n🔍 本地(写入): {local.base}")

    # 1) 分组同步
    prod_groups = [g for g in fetch_groups(prod) if g.get("status") == "active"]
    local_groups = fetch_groups(local)
    local_by_name = {g["name"]: g for g in local_groups}
    print(f"\n📋 分组同步 (生产 {len(prod_groups)} 个 → 本地, 按名称幂等):")
    for g in prod_groups:
        upsert_group(local, g, local_by_name, args.apply)

    # 写入后重取本地分组, 建立 名称→本地id 映射
    if args.apply:
        local_by_name = {g["name"]: g for g in fetch_groups(local)}
    name_to_local_id = {n: g["id"] for n, g in local_by_name.items()}

    # 2) 渠道同步 (快照 → group_ids 按名称重映射 → 本地 upsert)
    snapshot = load_snapshot(args.refresh_snapshot, args.prod_base, args.prod_key)
    channels = snapshot["channels"]
    existing_ch = {c["name"]: c for c in fetch_channels(local)}
    print(f"\n📋 渠道同步 (快照 {len(channels)} 个 → 本地, 按名称幂等):")
    ok = 0
    for ch in channels:
        group_names = parse_group_names(ch["_groups"])
        missing = [n for n in group_names if n not in name_to_local_id]
        if missing:
            if args.apply:
                print(f"  ❌ 渠道 {ch['name']:<20} 本地缺少分组: {missing}, 跳过")
                continue
            print(f"  📝 渠道 {ch['name']:<20} 依赖分组 {group_names} (apply 时映射)")
            ok += 1
            continue
        local_ids = [name_to_local_id[n] for n in group_names]
        ok += bool(upsert_channel(local, ch, local_ids, existing_ch, args.apply))

    print(f"渠道 {ok}/{len(channels)} 成功"
          + ("" if args.apply else "  💡 预演模式"))

    # 3) 占位账号 (账号真相源: 本地无账号则广场空白)
    print(f"\n📋 占位账号同步 (生产 {len(prod_groups)} 个分组 → 本地, 按账号名幂等):")
    existing_acc = list_local_accounts(local) if args.apply else {}
    ok_acc = 0
    for g in prod_groups:
        models = fetch_group_models(prod, g["id"])
        if not models:
            print(f"  ⏭️  {g['name']:<36} 生产无可用模型, 跳过")
            continue
        lgid = name_to_local_id.get(g["name"])
        if lgid is None:
            print(f"  ❌ {g['name']:<36} 本地缺少分组, 跳过")
            continue
        ok_acc += bool(upsert_seed_account(local, g["name"], lgid, g["platform"], models,
                                           existing_acc, args.apply))
    print(f"占位账号: {ok_acc} 个处理完成"
          + ("" if args.apply else "  💡 预演模式, 加 --apply 真正写入本地"))


if __name__ == "__main__":
    main()
