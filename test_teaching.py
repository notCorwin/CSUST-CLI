from __future__ import annotations

import contextlib
import json
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from csust_cli.core import Response
from csust_cli.features import teaching


class TeachingHandler(BaseHTTPRequestHandler):
    last: tuple[str, str, dict[str, list[str]]] | None = None

    def log_message(self, *_args: object) -> None:
        return

    def _body(self, body: str, content_type: str = "text/html; charset=utf-8") -> None:
        encoded = body.encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def do_GET(self) -> None:  # noqa: N802 - stdlib handler API
        parsed = urlparse(self.path)
        type(self).last = (self.command, parsed.path, parse_qs(parsed.query))
        if parsed.path.endswith("/json"):
            self._body(json.dumps({"code": "200", "data": {"ok": True}}), "application/json")
            return
        self._body(
            '<html><head><title>课程</title></head><body>'
            '<form method="post" action="/meol/submit"><input name="courseId" value="1">'
            '<button type="submit">提交</button></form>'
            '<a href="/meol/detail?id=1">详情</a></body></html>'
        )

    def do_POST(self) -> None:  # noqa: N802 - stdlib handler API
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length).decode("utf-8")
        parsed = urlparse(self.path)
        type(self).last = (self.command, parsed.path, parse_qs(body))
        self._body(json.dumps({"code": "200", "data": {"ok": True}}), "application/json")


@contextlib.contextmanager
def teaching_server():
    server = ThreadingHTTPServer(("127.0.0.1", 0), TeachingHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        thread.join(timeout=2)
        server.server_close()


class TeachingTests(unittest.TestCase):
    def test_catalog_is_unique_and_covers_mixed_platform(self) -> None:
        self.assertEqual(len(teaching.TEACHING_ROUTE_CATALOG), len(teaching.TEACHING_ROUTE_BY_NAME))
        self.assertEqual(len(teaching._TEACHING_API_ROWS), len(teaching.TEACHING_API_BY_NAME))
        self.assertEqual(len(teaching.TEACHING_ACTION_CATALOG), len(teaching.TEACHING_ACTION_BY_NAME))
        self.assertIn("course-resource-search", teaching.TEACHING_API_BY_NAME)
        self.assertIn("homework-stu-submit-do", teaching.TEACHING_API_BY_NAME)
        self.assertIn("course.forum.post", teaching.TEACHING_ACTION_BY_NAME)
        self.assertEqual(teaching.TEACHING_API_BY_NAME["platform-login"]["path"], "/meol/loginCheck.do")
        self.assertEqual(teaching.TEACHING_API_BY_NAME["podcast-favorite"]["path"], "/meol/common/vblog/favorite/add_favorite.jsp")
        self.assertEqual(teaching.TEACHING_API_BY_NAME["mobile-login"]["path"], "/meol/mobileLogin.do")
        self.assertEqual(teaching.TEACHING_ACTION_BY_NAME["personal.mobile.ignore"]["path"], "/meol/bindMobileIgnore.do")
        for name in (
            "password-email-account",
            "password-question-account",
            "password-captcha",
            "social-upload-temp",
            "social-delete-temp",
            "resource-sso",
        ):
            self.assertIn(name, teaching.TEACHING_API_BY_NAME)
        self.assertEqual(teaching.TEACHING_ROUTE_BY_NAME["course-departments"]["path"], "/meol/allDepartment.do")
        self.assertIn("/moocresource/resource/resourceInfoList.do", {item["path"] for item in teaching._TEACHING_API_ROWS})

    def test_page_inspection_exposes_teaching_script_endpoints(self) -> None:
        page = teaching.web.inspect_page(
            "<html><script>var login = '/meol/mobileLogin.do'; var res = '/moocresource/view/reslist.jsp';</script></html>",
            "https://example.test/http/gateway/meol/personal.do",
        )
        self.assertIn("https://example.test/meol/mobileLogin.do", page["endpoints"])
        self.assertIn("https://example.test/moocresource/view/reslist.jsp", page["endpoints"])

    def test_web_client_discovers_safe_prefixed_url_and_parses_html(self) -> None:
        with teaching_server() as base, tempfile.TemporaryDirectory() as directory:
            client = teaching.TeachingClient(
                base_url=base,
                prefix="/http/gateway",
                cookie_file=Path(directory) / "cookies.txt",
                session_file=Path(directory) / "session.json",
                load_cookies=False,
            )
            response = client.request_web("/meol/page")
            page = teaching._response_payload(response)
            self.assertEqual(response.status, 200)
            self.assertEqual(page["title"], "课程")
            self.assertEqual(urlparse(page["forms"][0]["action"]).path, "/meol/submit")
            self.assertEqual(page["links"][0]["text"], "详情")

    def test_service_discovery_opens_the_gateway_sso_handoff(self) -> None:
        with teaching_server() as base, tempfile.TemporaryDirectory() as directory:
            client = teaching.TeachingClient(
                base_url=base,
                cookie_file=Path(directory) / "cookies.txt",
                session_file=Path(directory) / "session.json",
                load_cookies=False,
            )
            service = {
                "id": "service-id",
                "name": teaching.TEACHING_SERVICE_NAME,
                "type": "web",
                "url": "/enclient/api/users/service/open/service-id&enclient-token",
                "urlPlus": "/http/gateway/meol/homepage/common/sso_login.jsp",
            }
            client.request_api = lambda *_args, **_kwargs: (
                Response("", 200, {}, ""),
                {"data": {"children": [{"serviceList": [service]}]}},
            )
            result = client.ensure_service()
            self.assertEqual(result["urlPlus"], "/http/gateway")
            self.assertEqual(TeachingHandler.last[0:2], ("GET", "/enclient/api/users/service/open/service-id&enclient-token"))

    def test_json_post_uses_form_or_json_without_leaking_gateway(self) -> None:
        with teaching_server() as base, tempfile.TemporaryDirectory() as directory:
            client = teaching.TeachingClient(
                base_url=base,
                prefix="/http/gateway",
                cookie_file=Path(directory) / "cookies.txt",
                load_cookies=False,
            )
            response = client.request_web("/meol/json", method="POST", json_body={"courseId": 1})
            self.assertEqual(response.status, 200)
            self.assertEqual(TeachingHandler.last[0:2], ("POST", "/http/gateway/meol/json"))


if __name__ == "__main__":
    unittest.main()
