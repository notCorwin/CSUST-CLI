from __future__ import annotations

import contextlib
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from csust_cli.core import Response
from csust_cli.features import quality, web
from csust_cli.features.quality import QUALITY_ROUTE_CATALOG, QualityClient


class QualityHandler(BaseHTTPRequestHandler):
    last_path = ""

    def log_message(self, *_args: object) -> None:
        return

    def _send(self, body: str | bytes, *, content_type: str = "text/html", cookie: str = "") -> None:
        payload = body.encode("utf-8") if isinstance(body, str) else body
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        if cookie:
            self.send_header("Set-Cookie", cookie)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_GET(self) -> None:  # noqa: N802 - stdlib handler API
        type(self).last_path = self.path
        if self.path.endswith("verifycode.servlet"):
            self._send(b"captcha", content_type="image/jpeg")
            return
        if "AUTH=1" in self.headers.get("Cookie", ""):
            self._send("<html><title>教学质量保障系统</title><body>main</body></html>")
        else:
            self._send('<html><form id="loginForm"><input name="userAccount"><input name="userPassword"></form></html>')

    def do_POST(self) -> None:  # noqa: N802 - stdlib handler API
        type(self).last_path = self.path
        if self.path.endswith("flag=sess"):
            self._send("scode#" + ("0" * 20), content_type="text/plain")
            return
        self._send("ok", cookie="AUTH=1; Path=/")


@contextlib.contextmanager
def quality_server():
    server = ThreadingHTTPServer(("127.0.0.1", 0), QualityHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        thread.join(timeout=2)
        server.server_close()


class QualityTests(unittest.TestCase):
    def test_catalog_reuses_the_student_facing_portal_routes(self) -> None:
        self.assertEqual(len(QUALITY_ROUTE_CATALOG), 69)
        self.assertEqual(len({item["name"] for item in QUALITY_ROUTE_CATALOG}), 69)

    def test_quality_rebases_old_gateway_urls_to_the_current_prefix(self) -> None:
        with quality_server() as base, tempfile.TemporaryDirectory() as directory:
            client = QualityClient(
                base_url=base,
                cookie_file=Path(directory) / "cookies.txt",
                session_file=Path(directory) / "session.json",
                load_cookies=False,
            )
            client.session["token"] = "vpn-token"
            client._ensure_vpn_session = mock.Mock()  # type: ignore[method-assign]
            client.ensure_service = mock.Mock(return_value={"urlPlus": "/http/current"})  # type: ignore[method-assign]
            self.assertEqual(
                client.web_url(f"{base}/http/old/jsxsd/xspj/form?batch=1"),
                f"{base}/http/current/jsxsd/xspj/form?batch=1",
            )

    def test_quality_graduation_design_uses_the_quality_portal_session(self) -> None:
        client = mock.Mock()
        client.ensure_quality_session = mock.Mock()
        client.request_web = mock.Mock(
            return_value=Response(
                "https://vpn.example/http/current/jsxsd/framework/xsMain.jsp",
                200,
                {},
                """<script>function towptjbs(a,b,c){window.location='https://oauth.fanyu.com/sso/cas/10536/1004';}</script>
                <a onclick=\"towptjbs('a','b','c')\">毕业设计</a>""",
            )
        )
        self.assertEqual(web._graduation_design_url(client), "https://oauth.fanyu.com/sso/cas/10536/1004")
        client.ensure_quality_session.assert_called_once_with()
        client.request_web.assert_called_once_with("/jsxsd/framework/xsMain.jsp")

    def test_quality_form_reuses_web_form_submission_through_the_gateway(self) -> None:
        client = mock.Mock()
        client.web_url.side_effect = lambda path: path if path.startswith("https://") else "https://vpn.example/http/current" + path
        client.request_web.side_effect = [
            Response(
                "https://vpn.example/http/current/jsxsd/page",
                200,
                {},
                "<form action='/jsxsd/save' method='POST'><input type='hidden' name='token' value='t'><button>保存</button></form>",
            ),
            Response("https://vpn.example/http/current/jsxsd/save", 200, {}, "<div>保存成功</div>"),
        ]
        result = quality.run_form(
            SimpleNamespace(
                path="/jsxsd/page",
                form=1,
                button=None,
                param=[],
                data=[],
                yes=True,
                output=None,
            ),
            client,
        )
        self.assertTrue(result["confirmed"])
        self.assertEqual(client.request_web.call_args_list[1].kwargs["method"], "POST")
        self.assertEqual(client.request_web.call_args_list[1].kwargs["data"], [("token", "t")])

    def test_quality_public_paths_are_rebased_to_the_gateway(self) -> None:
        with quality_server() as base, tempfile.TemporaryDirectory() as directory:
            client = QualityClient(
                base_url=base,
                prefix="/http/current",
                cookie_file=Path(directory) / "cookies.txt",
                session_file=Path(directory) / "session.json",
                load_cookies=False,
            )
            client.session["token"] = "vpn-token"
            self.assertEqual(web._public_target(client, "/findmm.jsp"), f"{base}/http/current/findmm.jsp")

    def test_quality_login_preserves_gateway_and_establishes_portal_session(self) -> None:
        with quality_server() as base, tempfile.TemporaryDirectory() as directory:
            client = QualityClient(
                base_url=base,
                prefix="/http/gateway",
                cookie_file=Path(directory) / "cookies.txt",
                session_file=Path(directory) / "session.json",
                load_cookies=False,
            )
            client.session["token"] = "vpn-token"
            with mock.patch("csust_cli.features.quality._credentials", return_value=("account", "password")), mock.patch(
                "csust_cli.features.quality.solve_captcha", return_value="1234"
            ):
                result = client.login()
            self.assertTrue(result["ok"])
            self.assertEqual(result["service"], "教学一体化")
            self.assertEqual(QualityHandler.last_path, "/http/gateway/jsxsd/framework/xsMain.jsp")

    def test_quality_login_bootstraps_vpn_cas_when_no_session_exists(self) -> None:
        with quality_server() as base, tempfile.TemporaryDirectory() as directory:
            client = QualityClient(
                base_url=base,
                cookie_file=Path(directory) / "cookies.txt",
                session_file=Path(directory) / "session.json",
                load_cookies=False,
            )

            def bootstrap(_args: object, current: QualityClient) -> dict[str, object]:
                current.web_prefix = "/http/gateway"
                return {"ok": True}

            with mock.patch("csust_cli.features.quality.vpn.login", side_effect=bootstrap) as vpn_login, mock.patch(
                "csust_cli.features.quality._credentials", return_value=("account", "password")
            ), mock.patch("csust_cli.features.quality.solve_captcha", return_value="1234"):
                result = client.login()
            self.assertTrue(result["ok"])
            self.assertEqual(vpn_login.call_args.args[0].auth, "cas")

    def test_quality_session_probe_calls_portal_login_when_gateway_returns_login(self) -> None:
        with quality_server() as base, tempfile.TemporaryDirectory() as directory:
            client = QualityClient(
                base_url=base,
                prefix="/http/gateway",
                cookie_file=Path(directory) / "cookies.txt",
                session_file=Path(directory) / "session.json",
                load_cookies=False,
            )
            client.session["token"] = "vpn-token"
            client.login = mock.Mock(return_value={"ok": True})  # type: ignore[method-assign]
            client.ensure_quality_session()
            client.login.assert_called_once_with()


if __name__ == "__main__":
    unittest.main()
