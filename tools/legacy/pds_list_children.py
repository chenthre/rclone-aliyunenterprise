#!/usr/bin/env python3
"""可靠枚举 PDS 目录直属子项。

已知 bj37789 的团队空间 drive 101 在子目录调用 /v2/file/list 时会错误地
返回空数组。本模块默认先调用 list；当子目录结果为空时，自动用
/v2/file/search 的 parent_file_id 精确查询回退，并用 /v2/file/get 过滤搜索
索引中尚未收敛的旧位置。

命令行示例：
    python3 pds_list_children.py --drive-id 101 --parent-file-id <file_id> --pretty

API_KEY 从环境变量读取；若未设置，则读取脚本同目录的 .env。密钥不会输出。
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable

import requests


DEFAULT_API = "https://bj37789.api.aliyunfile.com"
MAX_PAGE_SIZE = 100


class PDSAPIError(RuntimeError):
    """PDS 返回非 2xx 响应。"""

    def __init__(self, path: str, status_code: int, payload: Any):
        self.path = path
        self.status_code = status_code
        self.payload = payload
        if isinstance(payload, dict):
            detail = payload.get("message") or payload.get("code") or str(payload)
        else:
            detail = str(payload)
        super().__init__(f"{path} HTTP {status_code}: {detail}")


@dataclass(frozen=True)
class ListResult:
    items: list[dict[str, Any]]
    source: str
    fallback_used: bool

    def as_dict(self) -> dict[str, Any]:
        return {
            "items": self.items,
            "next_marker": "",
            "_workaround": {
                "source": self.source,
                "fallback_used": self.fallback_used,
                "count": len(self.items),
            },
        }


class PDSClient:
    def __init__(
        self,
        api_key: str,
        api: str = DEFAULT_API,
        timeout: float = 25,
        session: Any = None,
    ) -> None:
        if not api_key:
            raise ValueError("API_KEY 为空")
        self.api = api.rstrip("/")
        self.timeout = timeout
        self.session = session or requests.Session()
        self.headers = {
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            "User-Agent": "notes-sync/pds-list-workaround",
        }

    def _post(self, path: str, body: dict[str, Any]) -> dict[str, Any]:
        response = self.session.post(
            self.api + path,
            headers=self.headers,
            json=body,
            timeout=self.timeout,
        )
        try:
            payload = response.json()
        except Exception:
            payload = {"raw": response.text[:300]}
        if not 200 <= response.status_code < 300:
            raise PDSAPIError(path, response.status_code, payload)
        if not isinstance(payload, dict):
            raise PDSAPIError(path, response.status_code, payload)
        return payload

    def _paginate(
        self,
        path: str,
        body: dict[str, Any],
        page_size: int,
    ) -> list[dict[str, Any]]:
        page_size = max(1, min(page_size, MAX_PAGE_SIZE))
        marker = ""
        seen_markers: set[str] = set()
        items_by_id: dict[str, dict[str, Any]] = {}

        while True:
            request_body = {**body, "limit": page_size}
            if marker:
                request_body["marker"] = marker
            payload = self._post(path, request_body)
            for item in payload.get("items") or []:
                file_id = item.get("file_id")
                if file_id:
                    items_by_id[file_id] = item

            next_marker = payload.get("next_marker") or ""
            if not next_marker:
                break
            if next_marker in seen_markers:
                raise RuntimeError(f"{path} 返回了重复 next_marker，停止分页")
            seen_markers.add(next_marker)
            marker = next_marker

        return list(items_by_id.values())

    def _list_api(
        self, drive_id: str, parent_file_id: str, page_size: int
    ) -> list[dict[str, Any]]:
        return self._paginate(
            "/v2/file/list",
            {"drive_id": drive_id, "parent_file_id": parent_file_id},
            page_size,
        )

    def _search_api(
        self, drive_id: str, parent_file_id: str, page_size: int
    ) -> list[dict[str, Any]]:
        # file_id 由服务端生成，但仍按查询字符串规则转义，避免工具被误用于任意值。
        escaped_parent = parent_file_id.replace("\\", "\\\\").replace('"', '\\"')
        return self._paginate(
            "/v2/file/search",
            {
                "drive_id": drive_id,
                "query": f'parent_file_id = "{escaped_parent}"',
                # 必须为 false：true 表示递归范围，会把父目录本身和后代混入结果。
                "recursive": False,
                "return_total_count": True,
            },
            page_size,
        )

    def _verify_search_items(
        self,
        drive_id: str,
        parent_file_id: str,
        candidates: Iterable[dict[str, Any]],
    ) -> list[dict[str, Any]]:
        verified: list[dict[str, Any]] = []
        for candidate in candidates:
            file_id = candidate.get("file_id")
            if not file_id:
                continue
            try:
                current = self._post(
                    "/v2/file/get", {"drive_id": drive_id, "file_id": file_id}
                )
            except PDSAPIError as exc:
                # 搜索索引可能短暂保留已删除对象；其他错误仍应暴露给调用方。
                if exc.status_code == 404:
                    continue
                raise
            if (
                current.get("parent_file_id") == parent_file_id
                and current.get("status", "available") == "available"
            ):
                # get 只用于权威复核；保留 search 返回的枚举元数据形态。
                verified.append(candidate)
        return verified

    def list_children(
        self,
        drive_id: str,
        parent_file_id: str,
        *,
        strategy: str = "auto",
        page_size: int = MAX_PAGE_SIZE,
        verify_search: bool = True,
        search_retries: int = 0,
        retry_delay: float = 2,
    ) -> ListResult:
        """列出直属子项。

        strategy:
          - auto: 先 list；子目录异常返回空时改用 search（默认）
          - list: 只调用原始 list，便于诊断
          - search: 强制用 parent_file_id 搜索回退

        search_retries 可缓解 search 的最终一致性，但无法提供强一致性保证。
        """
        if strategy not in {"auto", "list", "search"}:
            raise ValueError("strategy 必须是 auto、list 或 search")
        if search_retries < 0:
            raise ValueError("search_retries 不能小于 0")

        if strategy in {"auto", "list"}:
            listed = self._list_api(drive_id, parent_file_id, page_size)
            if strategy == "list" or parent_file_id == "root" or listed:
                return ListResult(listed, "file/list", False)

        searched: list[dict[str, Any]] = []
        for attempt in range(search_retries + 1):
            searched = self._search_api(drive_id, parent_file_id, page_size)
            if searched or attempt == search_retries:
                break
            time.sleep(retry_delay)

        if verify_search:
            searched = self._verify_search_items(
                drive_id, parent_file_id, searched
            )
            source = "file/search + file/get"
        else:
            # 即使关闭 get 复核，也做一次客户端父目录过滤。
            searched = [
                item
                for item in searched
                if item.get("parent_file_id") == parent_file_id
            ]
            source = "file/search"
        return ListResult(searched, source, strategy == "auto")


def load_api_key(env_file: Path | None = None) -> str:
    key = os.environ.get("API_KEY", "").strip()
    if key:
        return key

    path = env_file or Path(__file__).resolve().parent.parent / ".env"
    if not path.exists():
        raise RuntimeError(f"未设置 API_KEY，且找不到 {path}")
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.startswith("API_KEY="):
            key = line.split("=", 1)[1].strip()
            if key:
                return key
    raise RuntimeError(f"{path} 中缺少有效的 API_KEY")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--drive-id", required=True)
    parser.add_argument("--parent-file-id", default="root")
    parser.add_argument("--api", default=DEFAULT_API)
    parser.add_argument("--env-file", type=Path)
    parser.add_argument(
        "--strategy", choices=("auto", "list", "search"), default="auto"
    )
    parser.add_argument("--page-size", type=int, default=MAX_PAGE_SIZE)
    parser.add_argument("--search-retries", type=int, default=0)
    parser.add_argument("--retry-delay", type=float, default=2)
    parser.add_argument(
        "--no-verify-search",
        action="store_true",
        help="不逐项调用 file/get；更快，但可能返回搜索索引中的旧位置",
    )
    parser.add_argument("--pretty", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        client = PDSClient(load_api_key(args.env_file), api=args.api)
        result = client.list_children(
            args.drive_id,
            args.parent_file_id,
            strategy=args.strategy,
            page_size=args.page_size,
            verify_search=not args.no_verify_search,
            search_retries=args.search_retries,
            retry_delay=args.retry_delay,
        )
    except (OSError, RuntimeError, ValueError, requests.RequestException) as exc:
        print(f"错误: {exc}", file=sys.stderr)
        return 1

    print(
        json.dumps(
            result.as_dict(),
            ensure_ascii=False,
            indent=2 if args.pretty else None,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
