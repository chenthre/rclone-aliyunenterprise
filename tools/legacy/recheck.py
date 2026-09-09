#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
独立复核脚本：检测「Sync 团队空间(drive 101) 子目录 file/list 恒为空」问题。
用法：python3 recheck.py
前置：同目录 .env 含 API_KEY=uk-...（只读，绝不打印明文）
行为：每次创建全新随机目录+文件 -> 检测子目录 list -> 对照 root list -> 自动归档到 trash。
输出：以 [PASS]/[FAIL] 标记「环境准备」断言；以「复现检测」单独给出结论。
"""
import datetime, hashlib, json, os, time, requests

API = "https://bj37789.api.aliyunfile.com"
DRIVE = "101"
TS = datetime.datetime.now().strftime("%Y%m%d_%H%M%S")
DIR_NAME = f"recheck_dir_{TS}"
FILE_NAME = f"recheck_file_{TS}.txt"

env_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".env")
assert os.path.exists(env_path), f"缺少 {env_path}"
KEY = [l.split("=", 1)[1].strip() for l in open(env_path).read().splitlines()
       if l.startswith("API_KEY=")][0]
assert KEY.startswith("uk-"), "API_KEY 格式异常"
H = {"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"}

def call(path, body):
    r = requests.post(API + path, headers=H, json=body, timeout=25)
    try:
        return r.status_code, r.json()
    except Exception:
        return r.status_code, {"_raw": r.text[:200]}

print(f"== 对象：{DIR_NAME} / {FILE_NAME}（drive {DRIVE}）==")
print("\n[阶段A: 环境准备断言]")
st, d = call("/v2/file/create", {"drive_id": DRIVE, "parent_file_id": "root",
                                 "name": DIR_NAME, "type": "folder", "check_name_mode": "refuse"})
ok_dir = st in (200, 201) and bool(d.get("file_id"))
print(f"  [{'PASS' if ok_dir else 'FAIL'}] 创建目录 HTTP {st}")
DIR = d.get("file_id")

content = f"recheck content {TS}\n".encode()
sha1 = hashlib.sha1(content).hexdigest().upper()
st, d = call("/v2/file/create", {"drive_id": DRIVE, "parent_file_id": DIR,
                                 "name": FILE_NAME, "type": "file", "size": len(content),
                                 "content_hash": sha1, "content_hash_name": "sha1",
                                 "check_name_mode": "refuse"})
ok_file = st in (200, 201) and bool(d.get("file_id"))
print(f"  [{'PASS' if ok_file else 'FAIL'}] 创建文件记录 HTTP {st}")
FILE = d.get("file_id"); UP = d.get("upload_id")
rr = requests.put(d["part_info_list"][0]["upload_url"], data=content, timeout=30)
st, d2 = call("/v2/file/complete", {"drive_id": DRIVE, "file_id": FILE, "upload_id": UP,
                                    "name": FILE_NAME, "parent_file_id": DIR,
                                    "part_info_list": [{"part_number": 1}]})
ok_cmp = rr.status_code == 200 and st == 200
print(f"  [{'PASS' if ok_cmp else 'FAIL'}] 上传分片+complete（put={rr.status_code} complete={st}）")

st, d = call("/v2/file/get", {"drive_id": DRIVE, "file_id": FILE})
ok_get = st == 200 and d.get("size") == len(content) and d.get("parent_file_id") == DIR
print(f"  [{'PASS' if ok_get else 'FAIL'}] get-file 权威确认: 文件存在 size={d.get('size')} "
      f"parent={d.get('parent_file_id')[:16]}... status={d.get('status')}")
print(f"  [{'PASS' if ok_dir and ok_file and ok_cmp and ok_get else 'FAIL'}] 环境准备全部就绪 "
      f"(dir={DIR[:16]}... file={FILE[:16]}...)")

print("\n[阶段B: 复现检测]")
st, d = call("/v2/file/list", {"drive_id": DRIVE, "parent_file_id": DIR, "limit": 100})
n_sub = len(d.get("items", []))
print(f"  > 子目录 list({DIR_NAME}) 返回条目数 = {n_sub}")
repro = n_sub == 0
print(f"  [{'复现 ✓：子目录 list 返回 0 条（异常）' if repro else '未复现：子目录 list 返回 {0} 条（服务端可能已修复）'.format(n_sub)}]")

print("\n[阶段C: 对照验证 —— 证明 list 接口本身工作，问题仅限『子目录作为 parent』]")
st, _ = call("/v2/file/move", {"drive_id": DRIVE, "file_id": FILE,
                               "to_parent_file_id": "root", "check_name_mode": "ignore"})
st, d = call("/v2/file/list", {"drive_id": DRIVE, "parent_file_id": "root", "limit": 100})
root_names = [i["name"] for i in d.get("items", [])]
c1 = FILE_NAME in root_names
print(f"  [{'PASS' if c1 else 'FAIL'}] move 到 root 后，root list 可见该文件（items={len(root_names)}）")

st, _ = call("/v2/file/move", {"drive_id": DRIVE, "file_id": FILE,
                               "to_parent_file_id": DIR, "check_name_mode": "ignore"})
st, d = call("/v2/file/list", {"drive_id": DRIVE, "parent_file_id": DIR, "limit": 100})
c2 = len(d.get("items", [])) == 0
print(f"  [{'复现 ✓' if c2 else 'FAIL'}] 同一文件 move 回子目录后，list 子目录又变 0 条")
st, d = call("/v2/file/get", {"drive_id": DRIVE, "file_id": FILE})
c3 = st == 200 and d.get("parent_file_id") == DIR
print(f"  [{'PASS' if c3 else 'FAIL'}] get-file（权威，无索引延迟）确认文件仍在子目录内 "
      f"(parent={d.get('parent_file_id')[:16]}...)")
# search 有秒级索引延迟，仅作补充信息（带重试）
found = False
for _ in range(3):
    st, d = call("/v2/file/search", {"drive_id": DRIVE,
                                     'query': f'name match "{FILE_NAME}"', "limit": 5})
    if any(i.get("file_id") == FILE for i in d.get("items", [])):
        found = True; break
    time.sleep(2)
print(f"  [info] search 最终可见该文件: {'是' if found else '否（索引延迟或异常）'}")

# 可落地回退：parent_file_id 精确查询 + recursive=false 等价于列直属子项。
fallback_found = False
for _ in range(3):
    st, d = call("/v2/file/search", {
        "drive_id": DRIVE,
        "query": f'parent_file_id = "{DIR}"',
        "recursive": False,
        "return_total_count": True,
        "limit": 100,
    })
    if any(i.get("file_id") == FILE and i.get("parent_file_id") == DIR
           for i in d.get("items", [])):
        fallback_found = True; break
    time.sleep(2)
print(f"  [{'PASS' if fallback_found else 'FAIL'}] 解决方案验证: search(parent_file_id, "
      f"recursive=false) 可枚举直属子项")

print("\n[阶段D: 收尾归档到 _trash_chenthre_sync（不做删除）]")
st, d = call("/v2/file/list", {"drive_id": DRIVE, "parent_file_id": "root", "limit": 100})
trash = next((i["file_id"] for i in d.get("items", [])
              if i.get("type") == "folder" and i.get("name") == "_trash_chenthre_sync"), None)
if not trash:
    st, d = call("/v2/file/create", {"drive_id": DRIVE, "parent_file_id": "root",
                                     "name": "_trash_chenthre_sync", "type": "folder",
                                     "check_name_mode": "refuse"})
    trash = d.get("file_id")
call("/v2/file/move", {"drive_id": DRIVE, "file_id": DIR,
                       "to_parent_file_id": trash, "check_name_mode": "ignore"})
print(f"  已归档 {DIR_NAME} -> trash({trash[:16]}...)")

print("\n===== 复核结论 =====")
if repro and c1 and c2 and c3 and fallback_found:
    print("复现成功：团队空间子目录 file/list 恒为空；root list 与 search/get/move 正常。")
    print("服务端异常仍存在；客户端可用 search(parent_file_id, recursive=false) 回退解决枚举。")
elif repro:
    print("核心现象已出现，但对照链或回退验证不完整，请人工核验阶段C输出。")
else:
    print("未复现（子目录 list 返回了条目）：服务端可能已修复，或环境配置与本文不符。")
