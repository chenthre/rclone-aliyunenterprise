#!/usr/bin/env python3
# 针对子目录 list 为空的系统性排查（排除参数/端点形式问题）
import json, os, requests
API = "https://bj37789.api.aliyunfile.com"
DRIVE = "101"
DIR = "6a9f898dc8e3dbfceeb545bfa02d2981fb99b0ff"   # probe_rest
FILE = "6a9f898de10ca2230ca44f9d8f6cf3127b6debca"  # hello_rest.txt

KEY = [l.split("=", 1)[1] for l in open("/home/rl2lab/programming/notes-sync/.env").read().splitlines()
       if l.startswith("API_KEY=")][0].strip()
H = {"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"}
H2 = dict(H); H2["x-pds-domain-id"] = "bj37789"

def show(tag, r):
    body = r.text[:220].replace("\n", " ")
    print(f"[{tag}] HTTP {r.status_code}: {body}")

print("=== A. 目录/文件对象本身（get-file）===")
for fid, nm in [(DIR, "dir"), (FILE, "file")]:
    r = requests.post(f"{API}/v2/file/get", headers=H,
                      json={"drive_id": DRIVE, "file_id": fid}, timeout=20)
    d = r.json()
    print(f"  {nm}: name={d.get('name')} parent={d.get('parent_file_id')} drive={d.get('drive_id')} size={d.get('size')} status={d.get('status')}")

print("\n=== B. POST body 标准（基准）===")
r = requests.post(f"{API}/v2/file/list", headers=H,
                  json={"drive_id": DRIVE, "parent_file_id": DIR, "limit": 100}, timeout=20)
show("B", r)

print("\n=== C. URL query 携带参数 ===")
r = requests.post(f"{API}/v2/file/list?drive_id={DRIVE}&parent_file_id={DIR}&limit=100",
                  headers=H, json={}, timeout=20)
show("C", r)

print("\n=== D. GET 方法 ===")
r = requests.get(f"{API}/v2/file/list?drive_id={DRIVE}&parent_file_id={DIR}&limit=100",
                 headers=H, timeout=20)
show("D", r)

print("\n=== E. 加 x-pds-domain-id header ===")
r = requests.post(f"{API}/v2/file/list", headers=H2,
                  json={"drive_id": DRIVE, "parent_file_id": DIR, "limit": 100}, timeout=20)
show("E", r)

print("\n=== F. body 加 drive_type/fields 等所有可选参数 ===")
r = requests.post(f"{API}/v2/file/list", headers=H,
                  json={"drive_id": DRIVE, "parent_file_id": DIR, "limit": 100,
                        "category": "", "status": "available", "type": "all",
                        "order_by": "name", "order_direction": "ASC",
                        "fields": "url,user_tags,dir_size", "marker": ""}, timeout=20)
show("F", r)

print("\n=== G. v2/batch 包一层 ===")
r = requests.post(f"{API}/v2/batch", headers=H,
                  json={"requests": [{"method": "POST", "url": "/v2/file/list",
                                      "body": {"drive_id": DRIVE, "parent_file_id": DIR, "limit": 100},
                                      "headers": {}, "id": "1"}, ],
                        "resource": "file"}, timeout=20)
show("G", r)

print("\n=== H. api-vpc 端点 ===")
try:
    r = requests.post("https://bj37789.api-vpc.aliyunfile.com/v2/file/list", headers=H,
                      json={"drive_id": DRIVE, "parent_file_id": DIR, "limit": 100}, timeout=20)
    show("H", r)
except Exception as e:
    print("[H] err:", e)

print("\n=== I. search 重试（hello_rest.txt 索引就绪？）===")
r = requests.post(f"{API}/v2/file/search", headers=H,
                  json={"drive_id": DRIVE, 'query': 'name match "hello_rest.txt"', "limit": 10}, timeout=20)
show("I", r)
try:
    items = r.json().get("items", [])
    for i in items:
        print("   ", i["file_id"], i["name"], "parent=", i.get("parent_file_id"))
except Exception:
    pass

print("\n=== J. delta 再试 ===")
r = requests.post(f"{API}/v2/file/list_delta", headers=H,
                  json={"drive_id": DRIVE, "cursor": ""}, timeout=20)
show("J", r)