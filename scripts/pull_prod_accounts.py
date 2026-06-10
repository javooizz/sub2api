#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
生产 apikey 账号 → 本地「真实可转发」复制脚本
==============================================

用途
----
把生产环境**全部 type=apikey 账号**忠实复制到本地，用于「帐号管理 / 上游管理」
全量验证。复制包含真实凭据（可真实转发），并还原分组绑定与 schedulable 开关。

为什么需要三段式
----------------
官方导出/导入端点 `GET/POST /accounts/data` 是「管理员备份」能力，**明文携带真实
凭据**（故意不走脱敏路径，见 backend/internal/handler/admin/account_data.go:50），
但导出结构 DataAccount **不含 group_ids、schedulable、status**。因此本脚本：

  ① 导入真实凭据   prod GET /accounts/data?type=apikey → 排除本地已有同名 →
                   local POST /accounts/data（skip_default_group_bind=true）
  ② 补绑分组       按账号名取生产 group_ids → 经【分组名】重映射为本地分组 id →
                   local PUT /accounts/:id { group_ids }
  ③ 还原 schedulable  生产 schedulable=false 的账号 → local POST /accounts/:id/schedulable

幂等：① 只导入本地不存在的名字；②③ 每次都对齐到生产状态（可安全重跑）。

安全
----
对生产只做 GET（只读）；写入目标必须是 localhost/127.0.0.1，否则直接拒绝。

用法
----
  # 预演（默认，只打印计划，不写本地）
  ONEBOOL_PROD_KEY=admin-<生产key> ONEBOOL_LOCAL_KEY=admin-<本地key> \
    python3 scripts/pull_prod_accounts.py

  # 确认无误后写入本地
  ONEBOOL_PROD_KEY=... ONEBOOL_LOCAL_KEY=... \
    python3 scripts/pull_prod_accounts.py --apply

环境变量 / 参数
  ONEBOOL_PROD_KEY    生产 admin API key（必填，或 --prod-key）
  ONEBOOL_LOCAL_KEY   本地 admin API key（必填，或 --local-key）
  ONEBOOL_PROD_BASE   生产 admin 基址（默认 https://onebool.com/api/v1/admin）
  ONEBOOL_LOCAL_BASE  本地 admin 基址（默认 http://localhost:3000/api/v1/admin）

依赖：仅需系统自带的 python3 + curl。
"""

import argparse
import json
import os
import re
import subprocess
import sys

DEFAULT_PROD_BASE = "https://onebool.com/api/v1/admin"
DEFAULT_LOCAL_BASE = "http://localhost:3000/api/v1/admin"


# ===========================================================================
# HTTP（用 curl，与 pull_prod_plaza_data.py 一致）
# ===========================================================================

class Api:
    def __init__(self, base_url, api_key):
        self.base = base_url.rstrip("/")
        self.key = api_key

    def _curl(self, args):
        out = subprocess.run(
            ["curl", "-s", "-m", "60", "-H", f"x-api-key: {self.key}"] + args,
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


def fetch_apikey_accounts(api):
    """分页拉取全部 type=apikey 账号（脱敏列表，含 id/group_ids/schedulable/status）。"""
    items, page = [], 1
    while True:
        r = api.get(f"/accounts?type=apikey&page={page}&page_size=100")
        if r.get("code") != 0:
            raise RuntimeError(f"获取账号失败: {r}")
        d = r["data"]
        items.extend(d.get("items") or [])
        if page >= d.get("pages", 1):
            return items
        page += 1


def fetch_export(api):
    """导出全部 apikey 账号（含真实明文凭据，不含分组/schedulable）。"""
    r = api.get("/accounts/data?type=apikey&include_proxies=false")
    if r.get("code") != 0:
        raise RuntimeError(f"导出账号失败: {r}")
    return r["data"]


def index_by_name(accounts):
    return {a["name"]: a for a in accounts}


# ===========================================================================
# 分组重映射：生产 group_id → 生产分组名 → 本地 group_id
# ===========================================================================

def remap_group_ids(prod_gids, prod_gid_to_name, local_name_to_gid):
    """返回 (本地 group_id 列表, 无法映射的生产 group_id 列表)。"""
    local_ids, unmapped = [], []
    for gid in prod_gids or []:
        name = prod_gid_to_name.get(gid)
        local_gid = local_name_to_gid.get(name) if name else None
        if local_gid is None:
            unmapped.append(gid)
        else:
            local_ids.append(local_gid)
    return sorted(set(local_ids)), unmapped


# ===========================================================================
# 主流程
# ===========================================================================

def main():
    ap = argparse.ArgumentParser(description="复制生产 apikey 账号到本地（真实凭据 + 分组 + schedulable）")
    ap.add_argument("--apply", action="store_true", help="真正写入本地（默认只预演）")
    ap.add_argument("--prod-key", default=os.environ.get("ONEBOOL_PROD_KEY"))
    ap.add_argument("--local-key", default=os.environ.get("ONEBOOL_LOCAL_KEY"))
    ap.add_argument("--prod-base", default=os.environ.get("ONEBOOL_PROD_BASE", DEFAULT_PROD_BASE))
    ap.add_argument("--local-base", default=os.environ.get("ONEBOOL_LOCAL_BASE", DEFAULT_LOCAL_BASE))
    args = ap.parse_args()

    if not args.prod_key:
        ap.error("缺少生产 admin key（ONEBOOL_PROD_KEY 或 --prod-key）")
    if not args.local_key:
        ap.error("缺少本地 admin key（ONEBOOL_LOCAL_KEY 或 --local-key）")
    # 安全闸：写入目标只允许本机
    if not re.match(r"^https?://(localhost|127\.0\.0\.1)([:/]|$)", args.local_base):
        ap.error(f"写入目标必须是 localhost: {args.local_base}")

    prod = Api(args.prod_base, args.prod_key)
    local = Api(args.local_base, args.local_key)
    print(f"🔍 生产(只读): {prod.base}\n🔍 本地(写入): {local.base}\n")

    # --- 采集 ---
    prod_groups = fetch_groups(prod)
    local_groups = fetch_groups(local)
    prod_gid_to_name = {g["id"]: g["name"] for g in prod_groups}
    local_name_to_gid = {g["name"]: g["id"] for g in local_groups}

    prod_accounts = fetch_apikey_accounts(prod)        # 脱敏元数据（group_ids/schedulable/status）
    prod_meta = index_by_name(prod_accounts)
    export = fetch_export(prod)                         # 真实凭据
    export_accounts = export.get("accounts") or []
    local_accounts = fetch_apikey_accounts(local)
    local_by_name = index_by_name(local_accounts)

    print(f"📊 生产 apikey 账号 {len(prod_accounts)} 个（导出 {len(export_accounts)} 个含真实凭据）"
          f"｜本地现有 apikey {len(local_accounts)} 个\n")

    # --- ① 导入：排除本地已有同名 ---
    to_import = [a for a in export_accounts if a["name"] not in local_by_name]
    skipped = [a["name"] for a in export_accounts if a["name"] in local_by_name]
    print(f"📥 导入计划：新建 {len(to_import)} 个；跳过同名 {len(skipped)} 个 {skipped or ''}")
    if not args.apply:
        for a in to_import[:8]:
            print(f"  📝 将新建 {a['name']:<40} platform={a['platform']} "
                  f"creds={sorted(a.get('credentials',{}).keys())}")
        if len(to_import) > 8:
            print(f"  …… 其余 {len(to_import) - 8} 个略")
    elif to_import:
        payload = {
            "type": "sub2api-data", "version": 1,
            "exported_at": export.get("exported_at", ""),
            "proxies": [], "accounts": to_import,
        }
        r = local.post("/accounts/data", {"data": payload, "skip_default_group_bind": True})
        if r.get("code") != 0:
            raise RuntimeError(f"导入失败: {r}")
        res = r["data"]
        print(f"  ✅ 导入结果：created={res.get('account_created')} "
              f"failed={res.get('account_failed')}")
        for e in (res.get("errors") or []):
            print(f"    ❌ {e.get('name')}: {e.get('message')}")
        # 重取本地账号（含刚导入的），供 ②③ 对齐
        local_by_name = index_by_name(fetch_apikey_accounts(local))

    # --- ②③ 补绑分组 + 还原 schedulable（对所有本地已存在的生产账号对齐）---
    print(f"\n🔗 分组绑定 + schedulable 还原（对齐到生产）：")
    n_grp, n_sched, n_miss = 0, 0, 0
    for name, pmeta in prod_meta.items():
        loc = local_by_name.get(name)
        if not loc:
            n_miss += 1
            print(f"  ⏭️  {name:<40} 本地不存在（导入失败/跳过），略")
            continue
        desired_gids, unmapped = remap_group_ids(
            pmeta.get("group_ids"), prod_gid_to_name, local_name_to_gid)
        cur_gids = sorted(set(loc.get("group_ids") or []))
        actions = []

        if cur_gids != desired_gids:
            actions.append(f"分组 {cur_gids}→{desired_gids}")
            if args.apply:
                rr = local.put(f"/accounts/{loc['id']}",
                               {"name": name, "group_ids": desired_gids})
                if rr.get("code") != 0:
                    actions[-1] += f" ❌{rr.get('message')}"
                else:
                    n_grp += 1
        if unmapped:
            actions.append(f"⚠️无法映射的生产分组id={unmapped}")

        p_sched = pmeta.get("schedulable")
        if loc.get("schedulable") != p_sched:
            actions.append(f"schedulable {loc.get('schedulable')}→{p_sched}")
            if args.apply:
                rr = local.post(f"/accounts/{loc['id']}/schedulable", {"schedulable": bool(p_sched)})
                if rr.get("code") != 0:
                    actions[-1] += f" ❌{rr.get('message')}"
                else:
                    n_sched += 1

        if actions:
            mark = "✅" if args.apply else "📝"
            print(f"  {mark} {name:<40} {'；'.join(actions)}")

    # --- 报告：生产 status=error 的账号（不强制改本地状态，仅提示）---
    err_accounts = [n for n, m in prod_meta.items() if m.get("status") == "error"]
    print(f"\n📋 汇总："
          f"\n   分组对齐 {n_grp} 个，schedulable 还原 {n_sched} 个，本地缺失 {n_miss} 个"
          f"\n   生产 status=error 的账号 {len(err_accounts)} 个（本地不强制置错，由本地健康检查自行判定）：{err_accounts}")
    if not args.apply:
        print("\n💡 预演模式。确认无误后加 --apply 真正写入本地。")


if __name__ == "__main__":
    main()
