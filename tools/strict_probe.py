#!/usr/bin/env python3
# 严格复验：纯 REST 全链路（create dir -> create file -> upload part -> complete -> list）
# 全程不使用插件/CLI 路径解析，仅用 file_id 参数。
import json, hashlib, sys, os
import requests

API = "https://bj37789.api.aliyunfile.com"
DRIVE = "101"

key = open(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".env")).read().strip()
KEY = [l.split("=", 1)[1] for l in key.splitlines() if l.startswith("API_KEY=")][0].strip()
KEY = open("/home/rl2lab/programming/notes-sync/.env").read().strip()
KEY = [l.split("=", 1)[1] for l in KEY.splitlines() if l.startswith("API_KEY=")][0].strip()

H = {"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"}

def call(path, body):
    r = requests.post(API + path, headers=H, json=body, timeout=20)
    return r.status_code, r.json()

ok = fail = 0
def check(name, cond, detail=""):
    global ok, fail
    if cond:
        ok += 1; print(f"  ✔ {name}")
    else:
        fail += 1; print(f"  ✘ {name}  {detail}")

print("== 1. 创建全新目录 probe_rest ==")
st, d = call("/v2/file/create", {
    "drive_id": DRIVE, "parent_file_id": "root",
    "name": "probe_rest", "type": "folder", "check_name_mode": "refuse"})
check("create dir", st == 200, d)
DIR = d.get("file_id"); print("   dir_id =", DIR)

# 2. 创建文件记录
content = b"REST strict probe file content line1\nline2\n"
sha1 = hashlib.sha1(content).hexdigest().upper()
print("== 2. 创建文件记录 hello_rest.txt ==")
st, d = call("/v2/file/create", {
    "drive_id": DRIVE, "parent_file_id": DIR,
    "name": "hello_rest.txt", "type": "file", "size": len(content),
    "content_hash": sha1, "content_hash_name": "sha1",
    "check_name_mode": "refuse"})
check("create file", st == 200, d)
FILE = d.get("file_id"); UPLOAD_ID = d.get("upload_id")
print("   file_id =", FILE, " upload_id =", UPLOAD_ID)

# 3. 上传分片（单分片）
print("== 3. PUT 分片上传 ==")
up_url = d["part_info_list"][0]["upload_url"]
rr = requests.put(up_url, data=content, timeout=30)
check("upload part", rr.status_code == 200, f"status={rr.status_code}")

# 4. complete
print("== 4. 完成上传 ==")
st, d = call("/v2/file/complete", {
    "drive_id": DRIVE, "file_id": FILE, "upload_id": UPLOAD_ID,
    "name": "hello_rest.txt", "parent_file_id": DIR,
    "part_info_list": [{"part_number": 1}]})
check("complete file", st == 200, d)

# 5. 立即 list 该目录（REST）
print("== 5. 立即 REST list 子目录 ==")
st, d = call("/v2/file/list", {"drive_id": DRIVE, "parent_file_id": DIR, "limit": 100})
items = d.get("items", [])
check("list dir", st == 200, d)
print("   items:", [(i["file_id"], i["name"], i.get("size")) for i in items])

# 6. list 变体：orders/fields/不带status
print("== 6. list 变体 ==")
for name, body in [
    ("order_by=name", {"drive_id": DRIVE, "parent_file_id": DIR, "order_by": "name"}),
    ("type=file", {"drive_id": DRIVE, "parent_file_id": DIR, "type": "file"}),
    ("fields=url", {"drive_id": DRIVE, "parent_file_id": DIR, "fields": "url"}),
]:
    st, d = call("/v2/file/list", body)
    n = len(d.get("items", []))
    check(f"list variant {name}", n > 0, f"status={st} items={n} {d}")

# 7. root list（应看到 probe_rest）
print("== 7. root list ==")
st, d = call("/v2/file/list", {"drive_id": DRIVE, "parent_file_id": "root", "limit": 100})
names = [(i["file_id"], i["name"]) for i in d.get("items", [])]
check("root list has probe_rest", any(nm == "probe_rest" for _, nm in names), names)

# 8. search 权威验证
print("== 8. search 验证 ==")
st, d = call("/v2/file/search", {"drive_id": DRIVE, "query": 'name match "hello_rest.txt"', "limit": 10})
items = d.get("items", [])
check("search finds file", any(i["file_id"] == FILE for i in items), items)
check("search parent = dir", bool(items) and items[0]["parent_file_id"] == DIR, items[:1])

# 9. get-file 验证
print("== 9. get-file ==")
st, d = call("/v2/file/get", {"drive_id": DRIVE, "file_id": FILE})
check("get file ok", st == 200 and d.get("size") == len(content), d)

# 10. delta
print("== 10. list_delta ==")
st, d = call("/v2/file/list_delta", {"drive_id": DRIVE, "cursor": ""})
print("   delta items:", len(d.get("items", [])), "has_more:", d.get("has_more"))

print(f"\n结果: {ok} passed, {fail} failed")
print("NEW_DIR_ID =", DIR)
print("NEW_FILE_ID =", FILE)