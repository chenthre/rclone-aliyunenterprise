import unittest

from pds_list_children import PDSClient


class FakeResponse:
    def __init__(self, payload, status_code=200):
        self.payload = payload
        self.status_code = status_code
        self.text = str(payload)

    def json(self):
        return self.payload


class FakeSession:
    def __init__(self, handler):
        self.handler = handler
        self.calls = []

    def post(self, url, **kwargs):
        self.calls.append((url, kwargs["json"]))
        return FakeResponse(self.handler(url, kwargs["json"]))


class PDSClientTest(unittest.TestCase):
    def client(self, handler):
        return PDSClient("test-key", session=FakeSession(handler))

    def test_auto_keeps_working_list_response(self):
        def handler(url, body):
            self.assertTrue(url.endswith("/v2/file/list"))
            return {"items": [{"file_id": "f1", "name": "a.md"}]}

        result = self.client(handler).list_children("101", "folder")
        self.assertEqual(["f1"], [item["file_id"] for item in result.items])
        self.assertEqual("file/list", result.source)
        self.assertFalse(result.fallback_used)

    def test_auto_falls_back_and_get_filters_stale_search_item(self):
        def handler(url, body):
            if url.endswith("/v2/file/list"):
                return {"items": [], "next_marker": ""}
            if url.endswith("/v2/file/search"):
                self.assertFalse(body["recursive"])
                self.assertEqual('parent_file_id = "folder"', body["query"])
                return {
                    "items": [
                        {"file_id": "current", "parent_file_id": "folder"},
                        {"file_id": "stale", "parent_file_id": "folder"},
                    ],
                    "next_marker": "",
                }
            if body["file_id"] == "current":
                return {
                    "file_id": "current",
                    "parent_file_id": "folder",
                    "status": "available",
                }
            return {
                "file_id": "stale",
                "parent_file_id": "somewhere-else",
                "status": "available",
            }

        result = self.client(handler).list_children("101", "folder")
        self.assertEqual(["current"], [item["file_id"] for item in result.items])
        self.assertEqual("file/search + file/get", result.source)
        self.assertTrue(result.fallback_used)

    def test_search_paginates_and_deduplicates(self):
        def handler(url, body):
            if not body.get("marker"):
                return {
                    "items": [{"file_id": "f1", "parent_file_id": "folder"}],
                    "next_marker": "page-2",
                }
            return {
                "items": [
                    {"file_id": "f1", "parent_file_id": "folder"},
                    {"file_id": "f2", "parent_file_id": "folder"},
                ],
                "next_marker": "",
            }

        result = self.client(handler).list_children(
            "101", "folder", strategy="search", verify_search=False
        )
        self.assertEqual(["f1", "f2"], [item["file_id"] for item in result.items])

    def test_root_does_not_fallback_when_list_is_empty(self):
        session = FakeSession(lambda _url, _body: {"items": [], "next_marker": ""})
        client = PDSClient("test-key", session=session)
        result = client.list_children("101", "root")
        self.assertEqual([], result.items)
        self.assertEqual(1, len(session.calls))
        self.assertTrue(session.calls[0][0].endswith("/v2/file/list"))


if __name__ == "__main__":
    unittest.main()
