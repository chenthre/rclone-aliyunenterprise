#!/usr/bin/env python3
# 企业空间(drive 2) 对照实验：create dir -> upload file -> list 子目录
import json, os, hashlib, requests
API = "https://bj37789.api.aliyunfile.com"
KEY = [l.split("=", 1)[1] for l in open("/home/rl2lab/programming/notes-sync/.env").read().splitlines()
       if l.startswith("API_KEY=")][0].strip()
H = {"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"}

def call(path, body):
    r = requests.post(API + path, headers=H, json=body, timeout=25)
    return r.status_code, r.json()

DRIVE = "2"
print("== 0. drive 2 根目录现有内容 ==")
st, d = call("/v2/file/list", {"drive_id": DRIVE, "parent_file_id": "root", "limit": 100})
print("   status:", st, " items:", [(i["name"], i["type"]) for i in d.get("items", [])][:20])

print("\n== 1. 创建测试目录 probe_ent ==")
st, d = call("/v2/file/create", {"drive_id": DRIVE, "parent_file_id": "root",
                                 "name": "probe_ent", "type": "folder", "check_name_mode": "refuse"})
print("   status:", st, " ->", {k: d.get(k) for k in ("file_id", "file_name", "parent_file_id")})
DIR = d.get("file_id")

print("\n== 2. 上传文件 ent_note.txt (REST 手工) ==")
content = b"enterprise drive subfolder list probe\n"
sha1 = hashlib.sha1(content).hexdigest().upper()
st, d = call("/v2/file/create", {"drive_id": DRIVE, "parent_file_id": DIR,
                                 "name": "ent_note.txt", "type": "file", "size": len(content),
                                 "content_hash": sha1, "content_hash_name": "sha1",
                                 "check_name_mode": "refuse"})
print("   create:", st, " file_id=", d.get("file_id"))
FILE = d.get("file_id"); UP = d.get("upload_id")
rr = requests.put(d["part_info_list"][0]["upload_url"], data=content, timeout=30)
print("   put part:", rr.status_code)
st, d = call("/v2/file/complete", {"drive_id": DRIVE, "file_id": FILE, "upload_id": UP,
                                   "name": "ent_note.txt", "parent_file_id": DIR,
                                   "part_info_list": [{"part_number": 1}]})
print("   complete:", st)

print("\n== 3. 立即 list 子目录 probe_ent (企业空间!!!) ==")
st, d = call("/v2/file/list", {"drive_id": DRIVE, "parent_file_id": DIR, "limit": 100})
items = d.get("items", [])
print("   status:", st, " items:", [(i["name"], i["type"], i.get("size")) for i in items])

print("\n== 4. get-file 验证 ==")
st, d = call("/v2/file/get", {"drive_id": DRIVE, "file_id": FILE})
print("   status:", st, " name=", d.get("name"), " parent=", d.get("parent_file_id"), " size=", d.get("size"))

print("\n== 5. search 对照 ==")
st, d = call("/v2/file/search", {"drive_id": DRIVE, 'query': 'name match "ent_note.txt"', "limit": 10})
print("   search:", [(i["name"], i["parent_file_id"]) for i in d.get("items", [])])

print("\n== 6. root list 更新？ ==")
st, d = call("/v2/file/list", {"drive_id": DRIVE, "parent_file_id": "root", "limit": 100})
print("   items:", [(i["name"], i["type"]) for i in d.get("items", [])][:20])

print("\nNEW_ENT_DIR =", DIR)
print("NEW_ENT_FILE =", FILE)