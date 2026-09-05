from __future__ import annotations

import contextlib
import base64
import json
import os
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from types import SimpleNamespace
from unittest import mock
from urllib.parse import parse_qsl, quote, urlparse

from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes

from csust_cli.features import vpn


class VpnHandler(BaseHTTPRequestHandler):
    requests: list[tuple[str, str, dict[str, str], object]] = []
    cas_enabled = False

    def log_message(self, *_args: object) -> None:
        return

    @classmethod
    def reset(cls) -> None:
        cls.requests = []
        cls.cas_enabled = False

    def _json(self, value: object, *, cookie: str | None = None) -> None:
        body = json.dumps(value).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        if cookie:
            self.send_header("Set-Cookie", cookie)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _capture(self) -> object:
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length)
        try:
            value: object = json.loads(raw.decode("utf-8")) if raw else None
        except ValueError:
            value = raw.decode("utf-8", errors="replace")
        type(self).requests.append((self.command, urlparse(self.path).path, dict(self.headers), value))
        return value

    def do_GET(self) -> None:  # noqa: N802 - stdlib handler API
        self._capture()
        path = urlparse(self.path).path
        if path == "/cas/start":
            service = f"http://127.0.0.1:{self.server.server_port}/cas/callback"
            self.send_response(302)
            self.send_header("Location", f"/cas/login?service={quote(service)}")
            self.end_headers()
        elif path == "/cas/login":
            body = (
                '<form id="pwdFromId" method="post" action="/cas/login">'
                '<input type="text" id="username" name="username">'
                '<input type="password" id="password" name="passwordText">'
                '<input type="hidden" id="saltPassword" name="password">'
                '<input type="hidden" id="_eventId" name="_eventId" value="submit">'
                '<input type="hidden" id="cllt" name="cllt" value="userNameLogin">'
                '<input type="hidden" id="dllt" name="dllt" value="generalLogin">'
                '<input type="hidden" id="pwdEncryptSalt" value="0123456789abcdef">'
                '<input type="hidden" id="execution" name="execution" value="e1s1">'
                '</form>'
            ).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        elif path == "/cas/callback":
            self._json({"code": "200", "data": {"name": "CAS User"}}, cookie="access_token=cas-cookie; Path=/")
        elif path.endswith("/generateKey"):
            self._json({"code": "200", "data": {"enToken": "enclient-12345678-abcdefghijklmnop"}}, cookie="access_token=seed; Path=/")
        elif path.endswith("/users/info"):
            self._json({"code": "200", "data": {"name": "Test User"}})
        else:
            self._json({"code": "200", "data": {"path": path}})

    def do_POST(self) -> None:  # noqa: N802 - stdlib handler API
        value = self._capture()
        path = urlparse(self.path).path
        if path.endswith("/auth/login"):
            expected = vpn._encrypt_password("secret", "abcdefghijklmnop")
            if value != {"account": "student", "passwd": expected}:
                self._json({"code": "2000", "messages": "bad"})
            else:
                self._json({"code": "200", "data": {"token": "session-token", "refreshToken": "refresh-token", "userId": 7}})
        elif path == "/cas/login":
            form = dict(parse_qsl(str(value), keep_blank_values=True))
            encrypted = form.get("password", "").encode("ascii", errors="ignore")
            try:
                ciphertext = base64.b64decode(encrypted, validate=True)
                decryptor = Cipher(algorithms.AES(b"0123456789abcdef"), modes.CBC(bytes(16))).decryptor()
                decrypted = decryptor.update(ciphertext) + decryptor.finalize()
                decrypted = decrypted[:-decrypted[-1]]
                accepted = form.get("username") == "student" and decrypted.endswith(b"secret")
            except Exception:
                accepted = False
            if not accepted:
                self._json({"code": "2000", "message": "bad"})
            else:
                self.send_response(302)
                self.send_header("Location", "/cas/callback")
                self.end_headers()
        elif path.endswith("/custom/page/login/cfg/select"):
            data = {"privacyProtocolSwitch": True}
            if type(self).cas_enabled:
                data.update({"defaultAuthType": "SSO_CAS", "casLoginUrl": "/cas/start"})
            self._json({"code": "200", "data": data})
        else:
            self._json({"code": "200", "data": {"ok": True}})


@contextlib.contextmanager
def vpn_server():
    VpnHandler.reset()
    server = ThreadingHTTPServer(("127.0.0.1", 0), VpnHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        thread.join(timeout=2)
        server.server_close()


class VpnTests(unittest.TestCase):
    def test_catalog_and_mappings_are_unique_and_resolvable(self) -> None:
        self.assertEqual(len(vpn.VPN_API_CATALOG), 249)
        self.assertEqual(len({item.path for item in vpn.VPN_API_CATALOG}), 249)
        self.assertEqual(len(vpn.VPN_ROUTE_CATALOG), 115)
        self.assertEqual(len(vpn.VPN_CONTROL_CATALOG), 77)
        self.assertEqual(len({item["name"] for item in vpn.VPN_ROUTE_CATALOG}), 115)
        self.assertEqual(len({item["path"] for item in vpn.VPN_ROUTE_CATALOG}), 115)
        required_routes = {
            "/home/long-range-Control",
            "/login/about",
            "/login/authPhoneOrEmail",
            "/login/authrigister",
            "/login/device-unbind-auth-detail",
            "/login/deviceUnBind",
            "/login/firstLoginUpdatePwd",
            "/login/forciblyAuth",
            "/login/second-auth-old",
            "/login/second-auth-old-other",
            "/login/sign-in?type=toLogin",
            "/login/systemConf",
            "/prePage/logExport",
            "/prePage/logReport",
            "/home/appMarket",
            "/home/approve-center?tabName=Initiated",
            "/home/approve-center?tabName=Pending",
            "/home/flie-share?name=myReceive",
            "/home/saveCenter",
            "/home/secure-center/safety-detail?isRepair=all&scanType=",
            "/home/secure-center/safety-detail?scanType=",
            "/home/service",
            "/home/sys-config?type=companies",
            "/home/sys-config?type=system",
            "/home/sys-config?type=update",
            "/home/workbench",
            "/home/workbench/file_apply",
            "/login/codeRigister",
            "/login/newdevice",
            "/login/rigisterFail",
            "/prePage/companycode",
            "/prePage/macPrivacy",
            "/prePage/oldLogin",
            "/prePage/serveConf",
        }
        self.assertTrue(required_routes <= {item["path"] for item in vpn.VPN_ROUTE_CATALOG})
        for item in (*vpn.VPN_ROUTE_CATALOG, *vpn.VPN_CONTROL_CATALOG):
            for name in item["apis"]:
                self.assertIn(name.lower(), vpn.VPN_API_BY_NAME, (item["name"], name))
        route_names = {item["name"] for item in vpn.VPN_ROUTE_CATALOG}
        for item in vpn.VPN_CONTROL_CATALOG:
            self.assertTrue(set(item["routes"]) <= route_names, item["name"])
        self.assertEqual(vpn.run_page(SimpleNamespace(route="/login/"))["route"]["name"], "login")

    def test_json_and_multipart_modes_are_mutually_exclusive(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            client = vpn.VpnClient(
                base_url="http://127.0.0.1:1",
                cookie_file=Path(directory) / "cookies.txt",
                session_file=Path(directory) / "session.json",
                load_cookies=False,
            )
            args = SimpleNamespace(
                name="auth-login",
                path=None,
                method=None,
                data_json="{}",
                form=["field=value"],
                file=[],
                param=[],
                path_arg=[],
                output=None,
                yes=True,
                native=False,
            )
            with self.assertRaises(vpn.CsustError) as caught:
                vpn.run_api(args, client)
            self.assertEqual(caught.exception.code, "invalid_argument")

    def test_json_null_is_sent_as_json(self) -> None:
        with vpn_server() as base, tempfile.TemporaryDirectory() as directory:
            client = vpn.VpnClient(base_url=base, cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            client.request("/null", method="POST", json_body=None)
            headers = VpnHandler.requests[-1][2]
            self.assertEqual(headers.get("Content-Type"), "application/json")
            self.assertEqual(headers.get("Content-Length"), "4")

    def test_multipart_file_upload_is_sent(self) -> None:
        with vpn_server() as base, tempfile.TemporaryDirectory() as directory:
            client = vpn.VpnClient(base_url=base, cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            client.request(
                "/upload",
                method="POST",
                multipart=[("file", ("avatar.png", b"PNG", "image/png"))],
            )
            headers = VpnHandler.requests[-1][2]
            self.assertTrue(headers.get("Content-Type", "").startswith("multipart/form-data; boundary="))
            self.assertIn('filename="avatar.png"', str(VpnHandler.requests[-1][3]))
            self.assertIn("PNG", str(VpnHandler.requests[-1][3]))

    def test_login_reproduces_portal_json_and_aes_protocol(self) -> None:
        with vpn_server() as base, tempfile.TemporaryDirectory() as directory, contextlib.ExitStack() as stack:
            stack.enter_context(mock.patch.dict(os.environ, {"CSUST_PASSWORD": "secret"}, clear=False))
            client = vpn.VpnClient(base_url=base, cookie_file=Path(directory) / "cookies.txt", session_file=Path(directory) / "session.json", load_cookies=False)
            result = vpn.login(SimpleNamespace(username="student", password_stdin=False, captcha_info=None), client)
            self.assertTrue(result["ok"])
            self.assertEqual(client.session["token"], "session-token")
            self.assertEqual(json.loads((Path(directory) / "session.json").read_text())["userId"], 7)
            login_request = next(item for item in VpnHandler.requests if item[1].endswith("/auth/login"))
            self.assertTrue(login_request[2].get("Authorization", "").startswith("Bearer enclient-"))

    def test_login_uses_cas_when_portal_hides_local_login(self) -> None:
        with vpn_server() as base, tempfile.TemporaryDirectory() as directory, contextlib.ExitStack() as stack:
            VpnHandler.cas_enabled = True
            stack.enter_context(mock.patch.dict(os.environ, {"CSUST_PASSWORD": "secret"}, clear=False))
            client = vpn.VpnClient(
                base_url=base,
                cookie_file=Path(directory) / "cookies.txt",
                session_file=Path(directory) / "session.json",
                load_cookies=False,
            )
            result = vpn.login(SimpleNamespace(username="student", password_stdin=False, captcha_info=None, auth="auto"), client)
            self.assertTrue(result["ok"])
            self.assertEqual(result["auth"], "cas")
            self.assertEqual(client.session["auth"], "cas")
            self.assertTrue(any(path == "/cas/callback" for _method, path, _headers, _value in VpnHandler.requests))
            info_request = next(item for item in VpnHandler.requests if item[1].endswith("/users/info"))
            self.assertIn("access_token=cas-cookie", info_request[2].get("Cookie", ""))

    def test_dynamic_path_and_mutation_confirmation(self) -> None:
        with vpn_server() as base, tempfile.TemporaryDirectory() as directory:
            client = vpn.VpnClient(base_url=base, cookie_file=Path(directory) / "cookies.txt", session_file=Path(directory) / "session.json", load_cookies=False)
            spec = vpn._spec_by_name("share-link-delete-id")
            with self.assertRaises(vpn.CsustError):
                client.call_spec(spec, data=None, params=(), path_args=["42"])
            result = client.call_spec(spec, data=None, params=(), path_args=["42"], yes=True)
            self.assertTrue(result["ok"])
            self.assertTrue(VpnHandler.requests[-1][1].endswith("/share/link/delete/42"))


if __name__ == "__main__":
    unittest.main()
