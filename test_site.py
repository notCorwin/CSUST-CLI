import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from csust_cli.core import CsustError, Response
from csust_cli.features.site import SiteClient, _normalize_url, _script_endpoints, _service_path, run_discover, run_get, run_request


class SiteTests(unittest.TestCase):
    def test_site_urls_are_official_and_same_origin(self):
        self.assertEqual(_normalize_url("www.csust.edu.cn/news"), "https://www.csust.edu.cn/news")
        with self.assertRaises(CsustError):
            _normalize_url("https://example.com/")
        with tempfile.TemporaryDirectory() as directory:
            client = SiteClient("https://www.csust.edu.cn/", Path(directory) / "cookies.txt", load_cookies=False)
            with self.assertRaises(CsustError):
                client.web_url("https://ehall.csust.edu.cn/")
            client = SiteClient("https://mail.csust.edu.cn/", Path(directory) / "mail-cookies.txt", load_cookies=False, allow_external=True)
            self.assertEqual(client.web_url("https://entry.qiye.163.com/domain/domainEntLogin"), "https://entry.qiye.163.com/domain/domainEntLogin")

    def test_site_service_path_rejects_url_arguments(self):
        with self.assertRaises(CsustError):
            _service_path(SimpleNamespace(service="official", path="https://ehall.csust.edu.cn/"))

    def test_site_get_returns_the_same_structured_page_snapshot(self):
        with tempfile.TemporaryDirectory() as directory:
            client = SiteClient("https://www.csust.edu.cn/", Path(directory) / "cookies.txt", load_cookies=False)
            response = Response(
                "https://www.csust.edu.cn/",
                200,
                {"Content-Type": "text/html; charset=utf-8"},
                "<title>主页</title><form action='/search'><input name='q'></form>",
            )
            args = SimpleNamespace(service="official", path="/", param=[], output=None, require_login=False)
            with mock.patch.object(client, "request", return_value=response):
                result = run_get(args, client)
        self.assertEqual(result["response"]["title"], "主页")
        self.assertEqual(result["response"]["forms"][0]["method"], "GET")

    def test_site_request_passes_json_and_headers_without_form_reencoding(self):
        with tempfile.TemporaryDirectory() as directory:
            client = SiteClient("https://fuwu.csust.edu.cn/", Path(directory) / "cookies.txt", load_cookies=False)
            response = Response("https://fuwu.csust.edu.cn/api", 200, {}, "操作成功")
            args = SimpleNamespace(
                service="sunshine",
                path="/api",
                method="POST",
                param=[],
                data=[],
                data_json='{"title":"建议"}',
                file=[],
                header=["X-Client=test"],
                yes=True,
                output=None,
                require_login=False,
            )
            with mock.patch.object(client, "request", return_value=response) as request:
                result = run_request(args, client)
        self.assertTrue(result["confirmed"])
        self.assertEqual(request.call_args.kwargs["json_body"], {"title": "建议"})
        self.assertEqual(request.call_args.kwargs["headers"], {"X-Client": "test"})

    def test_discover_reports_official_cross_subdomain_links(self):
        with tempfile.TemporaryDirectory() as directory:
            client = SiteClient("https://www.csust.edu.cn/", Path(directory) / "cookies.txt", load_cookies=False)
            response = Response(
                "https://www.csust.edu.cn/",
                200,
                {"Content-Type": "text/html"},
                "<a href='https://ehall.csust.edu.cn/'>门户</a><a href='/info'>信息</a>",
            )
            args = SimpleNamespace(service="official", path="/", depth=0, max_pages=1, cookie_file=None)
            with mock.patch.object(client, "get", return_value=response):
                result = run_discover(args, client)
        self.assertEqual(result["page_count"], 1)
        self.assertIn("ehall.csust.edu.cn", result["hosts"])

    def test_script_scan_extracts_route_literals(self):
        endpoints = _script_endpoints(
            "const a='/api/users/info'; const b='/static/app.js'; const c='https://fuwu.csust.edu.cn/ajax/save.do';",
            "https://www.csust.edu.cn/",
        )
        self.assertEqual(endpoints, ["https://fuwu.csust.edu.cn/ajax/save.do", "https://www.csust.edu.cn/api/users/info"])


if __name__ == "__main__":
    unittest.main()
