"""Shared HTTP, session, HTML and login primitives for the CSUST CLI."""

from __future__ import annotations

import html
import json
import gzip
import os
import re
import secrets
import shlex
import sys
import tempfile
from dataclasses import dataclass
from html.parser import HTMLParser
from http.client import HTTPException
from http.cookiejar import DefaultCookiePolicy, MozillaCookieJar
from pathlib import Path
from typing import Callable
from urllib.error import HTTPError, URLError
from urllib.parse import unquote, urlencode, urljoin, urlparse
from urllib.request import HTTPRedirectHandler, HTTPCookieProcessor, Request, build_opener

from . import __version__


DEFAULT_BASE_URL = "http://xk.csust.edu.cn"
USER_AGENT = f"Mozilla/5.0 (Macintosh; Intel Mac OS X) csust-cli/{__version__}"
LOGIN_PROBE_PATH = "/jsxsd/xskb/xskb_list.do"
CAPTCHA_RETRIES = 3
DAY_NAMES = ("星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日")
HTML_VOID_TAGS = frozenset({"area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"})
_JSON_BODY_UNSET = object()


class CsustError(RuntimeError):
    """An expected CLI failure with a stable machine-readable code."""

    code = "error"

    def __init__(self, message: str, *, code: str | None = None, details: dict[str, object] | None = None):
        super().__init__(message)
        self.code = code or self.code
        self.details = details or {}


class LoginRequired(CsustError):
    code = "login_required"


class CredentialsRequired(CsustError):
    code = "credentials_required"


class CaptchaError(CsustError):
    code = "captcha_failed"


class CaptchaRequired(CaptchaError):
    """Compatibility name for callers that imported the old exception."""


class OcrUnavailable(CsustError):
    code = "ocr_unavailable"


class AuthenticationFailed(CsustError):
    code = "authentication_failed"


class ParseError(CsustError):
    code = "parse_error"


class NetworkError(CsustError):
    code = "network_error"


def _cookie_domain_matches_host(cookie_domain: object, host: str) -> bool:
    domain = str(cookie_domain or "").lstrip(".").rstrip(".").lower()
    host = host.rstrip(".").lower()
    effective_host = f"{host}.local" if "." not in host else host
    return bool(domain and host and (effective_host == domain or effective_host.endswith("." + domain)))


def _has_url_control(value: str) -> bool:
    return any(ord(character) < 0x20 or 0x7F <= ord(character) <= 0x9F for character in value)


def _encode_multipart(parts: list[tuple[str, object]]) -> tuple[bytes, str]:
    boundary = "----csust-cli-" + secrets.token_hex(16)
    encoded: list[bytes] = []
    for name, value in parts:
        if not isinstance(name, str) or not name or _has_url_control(name) or '"' in name:
            raise CsustError("multipart 字段名格式无效", code="invalid_argument")
        encoded.extend((f"--{boundary}\r\n".encode("ascii"),))
        if isinstance(value, tuple) and len(value) == 3:
            filename, content, content_type = value
            if not isinstance(filename, str) or not filename or _has_url_control(filename):
                raise CsustError("multipart 文件名格式无效", code="invalid_argument")
            if not isinstance(content, bytes) or not isinstance(content_type, str) or not content_type or _has_url_control(content_type):
                raise CsustError("multipart 文件参数无效", code="invalid_argument")
            safe_filename = filename.replace("\\", "\\\\").replace('"', '\\"')
            encoded.append(
                f'Content-Disposition: form-data; name="{name}"; filename="{safe_filename}"\r\n'
                f"Content-Type: {content_type}\r\n\r\n".encode("utf-8")
            )
            encoded.extend((content, b"\r\n"))
        else:
            if not isinstance(value, str):
                raise CsustError("multipart 字段值必须是文本或文件元组", code="invalid_argument")
            encoded.append(f'Content-Disposition: form-data; name="{name}"\r\n\r\n'.encode("utf-8"))
            encoded.extend((value.encode("utf-8"), b"\r\n"))
    encoded.append(f"--{boundary}--\r\n".encode("ascii"))
    return b"".join(encoded), boundary


def _url_origin(value: str) -> tuple[str, str, int | None]:
    parsed = urlparse(value)
    port = parsed.port
    if port is None:
        port = {"http": 80, "https": 443}.get(parsed.scheme.lower())
    return parsed.scheme.lower(), (parsed.hostname or "").lower(), port


def _parse_http_url(value: object) -> tuple[object, tuple[str, str, int | None]] | None:
    if not isinstance(value, str) or not value or _has_url_control(value):
        return None
    try:
        parsed = urlparse(value)
        if parsed.scheme.lower() not in {"http", "https"} or not parsed.netloc or not parsed.hostname:
            return None
        if parsed.username is not None or parsed.password is not None:
            return None
        parsed.port
        return parsed, _url_origin(value)
    except (TypeError, UnicodeError, ValueError):
        return None


class _SafeCookiePolicy(DefaultCookiePolicy):
    def __init__(self, base_url: str):
        super().__init__()
        base = urlparse(base_url)
        self._base_origin = _url_origin(base_url)
        self._base_host = base.hostname or ""

    def return_ok(self, cookie, request):
        request_origin = _url_origin(request.get_full_url())
        secure_upgrade = (
            self._base_origin[0] == "http"
            and request_origin[0] == "https"
            and self._base_origin[1] == request_origin[1]
            and self._base_origin[2] == 80
            and request_origin[2] == 443
        )
        if request_origin != self._base_origin and not secure_upgrade and _cookie_domain_matches_host(
            cookie.domain, self._base_host
        ):
            return False
        return super().return_ok(cookie, request)


class _SafeRedirectHandler(HTTPRedirectHandler):
    def http_error_308(self, req, fp, code, msg, headers):
        return self.http_error_302(req, fp, code, msg, headers)

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        if not isinstance(newurl, str) or not newurl.strip():
            raise NetworkError("已拒绝无效的重定向地址")
        if _has_url_control(newurl):
            raise NetworkError("已拒绝包含控制字符的重定向地址")
        newurl = newurl.strip()
        try:
            old_parts = _parse_http_url(req.full_url)
            redirect_url = urljoin(req.full_url, newurl).replace(" ", "%20")
            target_parts = _parse_http_url(redirect_url)
        except (TypeError, ValueError) as exc:
            raise NetworkError("已拒绝无效的重定向地址") from exc
        if old_parts is None or target_parts is None:
            raise NetworkError("已拒绝非 HTTP(S) 重定向")
        old, old_origin = old_parts
        target, target_origin = target_parts
        if old.scheme.lower() == "https" and target.scheme.lower() == "http":
            raise NetworkError("已拒绝 HTTPS 到 HTTP 的重定向降级")
        cross_origin = old_origin != target_origin
        method = req.get_method().upper()
        if cross_origin and (method not in {"GET", "HEAD", "OPTIONS"} or req.data is not None):
            raise NetworkError("已拒绝把请求数据重定向到其他站点")
        if code in (307, 308):
            redirected = Request(
                redirect_url,
                data=req.data,
                headers=dict(req.headers),
                origin_req_host=req.origin_req_host,
                unverifiable=True,
                method=method,
            )
        elif method in {"HEAD", "OPTIONS"} and code in (301, 302, 303):
            content_headers = {"content-length", "content-type"}
            newheaders = {key: value for key, value in req.headers.items() if key.lower() not in content_headers}
            redirected = Request(
                redirect_url,
                headers=newheaders,
                origin_req_host=req.origin_req_host,
                unverifiable=True,
                method=method,
            )
        else:
            redirected = super().redirect_request(req, fp, code, msg, headers, redirect_url)
        if cross_origin and redirected is not None:
            safe_headers = {"accept", "user-agent"}
            for header_map in (redirected.headers, redirected.unredirected_hdrs):
                for key in list(header_map):
                    if key.lower() not in safe_headers:
                        del header_map[key]
        return redirected


class HttpError(NetworkError):
    code = "http_error"

    def __init__(self, status: int, reason: str):
        self.status = status
        super().__init__(f"HTTP {status} {reason}", details={"status": status})


class MutationUnverified(CsustError):
    code = "mutation_unverified"


def business_state(payload: object, *, success_codes: tuple[str, ...] = ("0", "200")) -> bool | None:
    """Interpret feedback only; None means the response proves neither outcome."""
    states: list[bool | None] = []
    if isinstance(payload, dict):
        if "text" in payload and ("forms" in payload or set(payload) <= {"text", "messages"}):
            messages = payload.get("messages") or []
            states.extend(business_state(message) for message in messages)
            if not any(payload.get(key) for key in ("forms", "links", "tables", "actions")):
                states.append(business_state(payload.get("text", "")))
        else:
            if isinstance(payload.get("success"), bool):
                states.append(payload["success"])
            code = payload.get("code")
            if isinstance(code, (str, int)) and not isinstance(code, bool) and str(code):
                states.append(str(code) in success_codes)
            for key in ("message", "messages", "msg", "error", "response"):
                if key in payload:
                    values = payload[key] if isinstance(payload[key], list) else [payload[key]]
                    states.extend(business_state(value, success_codes=success_codes) for value in values)
    elif isinstance(payload, str):
        text = payload.strip()
        if _has_failure_signal(text) or re.search(r"无效|拒绝|异常|\b(?:failed|failure|error|denied|invalid|unauthorized|forbidden)\b", text, re.I):
            return False
        # ponytail: short acknowledgements only; add endpoint-specific evidence for richer responses.
        if re.fullmatch(r"(?:邮件发送|操作|提交|保存|更新|删除|发布|评价|报名|选课|缴费|撤销|订购|退订|选订|处理|发送|修改|设置|上传|排序)?(?:成功|完成)[！!。.]?", text) or re.fullmatch(
            r"已(?:保存|提交|更新|删除|发布|评价|报名|选课|缴费|撤销)[！!。.]?", text
        ):
            return True
    if False in states:
        return False
    return True if True in states else None


def result_status(payload: object, *, mutating: bool, details: dict[str, object] | None = None,
                  success_codes: tuple[str, ...] = ("0", "200")) -> dict[str, object]:
    state = business_state(payload, success_codes=success_codes)
    context = {**(details or {}), "submitted": mutating, "confirmed": False, "evidence": "rejected" if state is False else "unknown"}
    if state is False:
        raise CsustError("远端明确报告操作失败", code="mutation_rejected" if mutating else "business_rejected", details=context)
    if mutating and state is not True:
        raise MutationUnverified("请求已提交但未验证，请查询状态后再决定是否重试", details=context)
    return {"ok": True, "submitted": mutating, "confirmed": True}


class Element:
    def __init__(self, tag: str = "#document", attrs: dict[str, str] | None = None, parent: "Element | None" = None):
        self.tag = tag
        self.attrs = attrs or {}
        self.parent = parent
        self.children: list[Element | str] = []

    def add(self, child: "Element | str") -> None:
        self.children.append(child)

    def attr(self, name: str, default: str = "") -> str:
        return self.attrs.get(name.lower(), default)

    def has_class(self, name: str) -> bool:
        return name in self.attr("class").split()

    def is_disabled(self) -> bool:
        if "disabled" in self.attrs:
            return True
        ancestor = self.parent
        while ancestor is not None:
            if ancestor.tag == "optgroup" and "disabled" in ancestor.attrs:
                return True
            if ancestor.tag == "fieldset" and "disabled" in ancestor.attrs:
                first_legend = next(
                    (child for child in ancestor.children if isinstance(child, Element) and child.tag == "legend"),
                    None,
                )
                current = self.parent
                while current is not None and current is not ancestor:
                    if current is first_legend:
                        break
                    current = current.parent
                else:
                    return True
            ancestor = ancestor.parent
        return False

    def find_all(
        self,
        tag: str | None = None,
        *,
        element_id: str | None = None,
        class_name: str | None = None,
    ) -> list["Element"]:
        found: list[Element] = []
        wanted_tag = tag.lower() if tag else None
        wanted_id = element_id.strip() if element_id else None
        stack = [child for child in reversed(self.children) if isinstance(child, Element)]
        while stack:
            child = stack.pop()
            if (
                (wanted_tag is None or child.tag == wanted_tag)
                and (wanted_id is None or child.attr("id").strip() == wanted_id)
                and (class_name is None or child.has_class(class_name))
            ):
                found.append(child)
            stack.extend(descendant for descendant in reversed(child.children) if isinstance(descendant, Element))
        return found

    def first(self, tag: str | None = None, **kwargs: str) -> "Element | None":
        return next(iter(self.find_all(tag, **kwargs)), None)

    def direct(self, tag: str) -> list["Element"]:
        return [child for child in self.children if isinstance(child, Element) and child.tag == tag]

    def raw_text(self, *, include_scripts: bool = True) -> str:
        if not include_scripts and self.tag in {"script", "style"}:
            return ""
        chunks: list[str] = []
        stack: list[tuple[Element | str, bool, bool]] = [(self, False, False)]
        while stack:
            node, closing, add_newline = stack.pop()
            if isinstance(node, str):
                chunks.append(node)
                continue
            if closing:
                if add_newline:
                    chunks.append("\n")
                continue
            if not include_scripts and node.tag in {"script", "style"}:
                continue
            stack.append((node, True, node is not self and node.tag in {"br", "p", "div", "tr"}))
            stack.extend((child, False, False) for child in reversed(node.children))
        return "".join(chunks)

    def text(self, *, include_scripts: bool = True) -> str:
        return " ".join(self.raw_text(include_scripts=include_scripts).replace("\xa0", " ").split())

    def to_html(self) -> str:
        chunks: list[str] = []
        stack: list[tuple[Element | str, bool]] = [(self, False)]
        while stack:
            node, closing = stack.pop()
            if isinstance(node, str):
                chunks.append(html.escape(node))
                continue
            if closing:
                chunks.append(f"</{node.tag}>")
                continue
            if node.tag == "#document":
                stack.extend((child, False) for child in reversed(node.children))
                continue
            attrs = "".join(f' {key}="{html.escape(value, quote=True)}"' for key, value in node.attrs.items())
            chunks.append(f"<{node.tag}{attrs}>")
            if node.tag not in HTML_VOID_TAGS:
                stack.append((node, True))
                stack.extend((child, False) for child in reversed(node.children))
        return "".join(chunks)


def _table_rows(table: Element) -> list[Element]:
    rows: list[Element] = []
    for row in table.find_all("tr"):
        ancestor = row.parent
        while ancestor is not None and ancestor.tag != "table":
            ancestor = ancestor.parent
        if ancestor is table:
            rows.append(row)
    return rows


class DocumentParser(HTMLParser):
    VOID_TAGS = HTML_VOID_TAGS
    AUTO_CLOSE = {"option": {"option"}, "tr": {"tr"}, "td": {"td", "th"}, "th": {"td", "th"}}

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.root = Element()
        self.stack = [self.root]
        self._stack_tags: dict[str, int] = {}

    def error(self, _message: str) -> None:
        return

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        tag = tag.lower()
        if tag == "form" and self._stack_tags.get("form"):
            return
        closable = self.AUTO_CLOSE.get(tag)
        boundary = "select" if tag == "option" else "table"
        if closable:
            for index in range(len(self.stack) - 1, 0, -1):
                if self.stack[index].tag in closable:
                    self._trim_stack(index)
                    break
                if self.stack[index].tag == boundary:
                    break
        node_attrs: dict[str, str] = {}
        for key, value in attrs:
            node_attrs.setdefault(key.lower(), value or "")
        node = Element(tag, node_attrs, self.stack[-1])
        self.stack[-1].add(node)
        if tag not in self.VOID_TAGS:
            self.stack.append(node)
            self._stack_tags[tag] = self._stack_tags.get(tag, 0) + 1

    def handle_startendtag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        self.handle_starttag(tag, attrs)
        if tag.lower() not in self.VOID_TAGS and len(self.stack) > 1:
            self._trim_stack(len(self.stack) - 1)

    def handle_endtag(self, tag: str) -> None:
        wanted = tag.lower()
        if not self._stack_tags.get(wanted):
            return
        for index in range(len(self.stack) - 1, 0, -1):
            if self.stack[index].tag == wanted:
                self._trim_stack(index)
                return

    def _trim_stack(self, index: int) -> None:
        for node in self.stack[index:]:
            count = self._stack_tags[node.tag] - 1
            if count:
                self._stack_tags[node.tag] = count
            else:
                del self._stack_tags[node.tag]
        del self.stack[index:]

    def handle_data(self, data: str) -> None:
        self.stack[-1].add(data)


def parse_html(source: str) -> Element:
    if not isinstance(source, str):
        raise TypeError("HTML source must be str")
    parser = DocumentParser()
    try:
        parser.feed(source)
        parser.close()
    except (NotImplementedError, TypeError, UnboundLocalError):
        # Python 3.9's markupbase raises on malformed marked sections after
        # keeping the portion it already parsed.
        pass
    return parser.root


@dataclass
class Response:
    url: str
    status: int
    headers: object
    body: str | bytes


def _header_value(headers: object, name: str) -> str:
    wanted = name.lower()
    getter = getattr(headers, "get", None)
    if callable(getter):
        try:
            value = getter(name)
        except (LookupError, TypeError, ValueError):
            value = None
        if value:
            return str(value)
    items = getattr(headers, "items", None)
    if callable(items):
        try:
            return next(
                (str(value) for key, value in items() if str(key).lower() == wanted and value),
                "",
            )
        except (LookupError, TypeError, ValueError):
            return ""
    return ""


def _has_set_cookie(response: Response) -> bool:
    return bool(_header_value(response.headers, "Set-Cookie"))


def _decode_body(body: str | bytes, headers: object) -> str:
    if isinstance(body, str):
        return body
    charset = None
    try:
        getter = getattr(headers, "get_content_charset", None)
        charset = getter() if callable(getter) else None
        if not charset:
            content_type = _header_value(headers, "Content-Type")
            match = re.search(r"(?:^|;)\s*charset\s*=\s*[\"']?([^;\"'\s]+)", content_type, re.I)
            charset = match.group(1) if match else None
    except (LookupError, TypeError, ValueError):
        charset = None
    if not charset:
        if body.startswith(b"\xef\xbb\xbf"):
            charset = "utf-8-sig"
        elif body.startswith((b"\xff\xfe", b"\xfe\xff")):
            charset = "utf-16"
        else:
            match = re.search(
                rb"<meta\b[^>]*charset\s*=\s*[\"']?\s*([A-Za-z0-9._:-]+)",
                body[:8192],
                re.I,
            )
            if match:
                charset = match.group(1).decode("ascii")
    try:
        return body.decode(charset or "utf-8", errors="replace")
    except (LookupError, UnicodeError):
        return body.decode("utf-8", errors="replace")


class Client:
    def __init__(self, base_url: str | None = None, cookie_file: Path | None = None, *, load_cookies: bool = True):
        configured_base = base_url if base_url is not None else env_value("CSUST_BASE_URL", DEFAULT_BASE_URL)
        self.base_url = configured_base.rstrip("/") if isinstance(configured_base, str) else ""
        if _parse_http_url(self.base_url) is None:
            raise CsustError("CSUST_BASE_URL 必须是不含账号信息的 HTTP(S) 地址", code="invalid_base_url")
        try:
            self.cookie_file = Path(
                cookie_file or env_value("CSUST_COOKIE_FILE", str(Path.home() / ".config" / "csust-cli" / "cookies.txt"))
            ).expanduser()
        except (OSError, RuntimeError, ValueError) as exc:
            code = "cookie_read_failed" if load_cookies else "cookie_write_failed"
            raise CsustError("会话文件路径无效", code=code) from exc
        if load_cookies and "\x00" in str(self.cookie_file):
            raise CsustError(f"无法读取会话文件 {self.cookie_file}: 路径包含无效字符", code="cookie_read_failed")
        self.cookies = MozillaCookieJar(str(self.cookie_file), policy=_SafeCookiePolicy(self.base_url))
        try:
            cookie_is_symlink = load_cookies and self.cookie_file.is_symlink()
            cookie_exists = load_cookies and self.cookie_file.exists()
        except (OSError, ValueError) as exc:
            raise CsustError(f"无法读取会话文件 {self.cookie_file}: {exc}", code="cookie_read_failed") from exc
        if cookie_is_symlink:
            raise CsustError(f"无法读取会话文件 {self.cookie_file}: 会话文件不能是符号链接", code="cookie_read_failed")
        if cookie_exists:
            try:
                if self.cookie_file.is_symlink() or not self.cookie_file.is_file():
                    raise OSError("会话文件必须是普通文件且不能是符号链接")
                self.cookies.load(ignore_discard=True, ignore_expires=True)
            except (OSError, ValueError) as exc:
                raise CsustError(f"无法读取会话文件 {self.cookie_file}: {exc}", code="cookie_read_failed") from exc
            try:
                os.chmod(self.cookie_file, 0o600)
            except OSError:
                pass
        self.opener = build_opener(_SafeRedirectHandler(), HTTPCookieProcessor(self.cookies))
        self._insecure_warning_shown = False
        self._saved_cookie_state = self._cookie_state()

    def url(self, path: str) -> str:
        if not isinstance(path, str) or _has_url_control(path):
            raise CsustError("请求地址格式无效", code="invalid_path")
        try:
            return path if path.lower().startswith(("http://", "https://")) else urljoin(self.base_url + "/", path.lstrip("/"))
        except (TypeError, ValueError) as exc:
            raise CsustError("请求地址格式无效", code="invalid_path") from exc

    def warn_if_insecure(self, target: str | None = None) -> None:
        target = target or self.base_url
        if urlparse(target).scheme.lower() == "http" and not self._insecure_warning_shown:
            print("警告：当前教务系统使用 HTTP，账号密码可能未受传输层加密保护；可用 CSUST_BASE_URL 指向 HTTPS。", file=sys.stderr)
            self._insecure_warning_shown = True

    def request(
        self,
        path: str,
        *,
        method: str = "GET",
        data: dict[str, str] | list[tuple[str, str]] | None = None,
        json_body: object = _JSON_BODY_UNSET,
        multipart: list[tuple[str, object]] | None = None,
        headers: dict[str, str] | None = None,
        binary: bool = False,
        with_metadata: bool = False,
    ) -> Response | bytes:
        target = self.url(path)
        if _parse_http_url(target) is None:
            raise CsustError("请求地址格式无效", code="invalid_path")
        if not isinstance(method, str) or not method.strip():
            raise CsustError("请求方法格式无效", code="invalid_argument")
        method = method.strip().upper()
        self.warn_if_insecure(target)
        encoded = None
        request_headers = {
            "User-Agent": USER_AGENT,
            "Accept": "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8",
        }
        try:
            request_headers.update(headers or {})
            body_modes = sum(value is not None for value in (data, multipart)) + (json_body is not _JSON_BODY_UNSET)
            if body_modes > 1:
                raise CsustError("不能同时使用表单、multipart 和 JSON 参数", code="invalid_argument")
            if multipart is not None:
                encoded, boundary = _encode_multipart(multipart)
                request_headers["Content-Type"] = f"multipart/form-data; boundary={boundary}"
            elif data is not None:
                encoded = urlencode(data, doseq=True).encode("utf-8")
                request_headers.setdefault("Content-Type", "application/x-www-form-urlencoded")
            elif json_body is not _JSON_BODY_UNSET:
                encoded = json.dumps(json_body, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
                request_headers.setdefault("Content-Type", "application/json")
        except (TypeError, UnicodeError, ValueError) as exc:
            raise CsustError("请求参数格式无效", code="invalid_argument") from exc
        try:
            request = Request(target, data=encoded, headers=request_headers, method=method)
        except (TypeError, ValueError) as exc:
            raise CsustError("请求地址格式无效", code="invalid_path") from exc
        try:
            with self.opener.open(request, timeout=30) as response:
                content = response.read()
                if _header_value(response.headers, "Content-Encoding").lower().split(";", 1)[0].strip() == "gzip":
                    try:
                        content = gzip.decompress(content)
                    except OSError:
                        pass
                if binary and not with_metadata:
                    return content
                if binary:
                    return Response(response.geturl(), response.status, response.headers, content)
                return Response(response.geturl(), response.status, response.headers, _decode_body(content, response.headers))
        except HTTPError as exc:
            try:
                exc.read()
            except Exception:
                pass
            finally:
                try:
                    exc.close()
                except Exception:
                    pass
            raise HttpError(exc.code, str(exc.reason)) from exc
        except (URLError, TimeoutError, OSError, HTTPException) as exc:
            reason = getattr(exc, "reason", None) or str(exc)
            raise NetworkError(f"无法连接教务系统: {reason}") from exc
        except ValueError as exc:
            raise CsustError("请求地址格式无效", code="invalid_path") from exc

    def get(self, path: str, **kwargs: object) -> Response:
        response = self.request(path, method="GET", **kwargs)
        assert isinstance(response, Response)
        return response

    def post(self, path: str, data: dict[str, str] | list[tuple[str, str]], **kwargs: object) -> Response:
        response = self.request(path, method="POST", data=data, **kwargs)
        assert isinstance(response, Response)
        return response

    def clear_cookies(self) -> None:
        self.cookies.clear()

    def _cookie_state(self) -> frozenset[tuple[object, ...]]:
        return frozenset(
            (cookie.domain, cookie.path, cookie.name, cookie.value, cookie.expires, cookie.secure, cookie.discard)
            for cookie in self.cookies
        )

    def save(self) -> None:
        temporary: str | None = None
        try:
            self.cookie_file.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            if self.cookie_file.is_symlink() or (self.cookie_file.exists() and not self.cookie_file.is_file()):
                raise OSError("会话文件必须是普通文件且不能是符号链接")
            descriptor, temporary = tempfile.mkstemp(prefix=f".{self.cookie_file.name}.", dir=self.cookie_file.parent)
            os.close(descriptor)
            persistent = MozillaCookieJar(temporary)
            host = (urlparse(self.base_url).hostname or "").rstrip(".").lower()
            for cookie in self.cookies:
                if _cookie_domain_matches_host(cookie.domain, host):
                    persistent.set_cookie(cookie)
            persistent.save(temporary, ignore_discard=True, ignore_expires=True)
            os.chmod(temporary, 0o600)
            os.replace(temporary, self.cookie_file)
            self._saved_cookie_state = self._cookie_state()
        except (OSError, ValueError) as exc:
            raise CsustError(f"无法保存会话文件 {self.cookie_file}: {exc}", code="cookie_write_failed") from exc
        finally:
            if temporary:
                try:
                    os.unlink(temporary)
                except OSError:
                    pass


def _save_cookie_refresh(client: Client, response: Response) -> None:
    state_getter = getattr(client, "_cookie_state", None)
    saved_state = getattr(client, "_saved_cookie_state", None)
    changed = callable(state_getter) and saved_state is not None and state_getter() != saved_state
    if _has_set_cookie(response) or changed:
        client.save()


def same_origin_url(client: Client, path: str) -> str:
    """Resolve an HTTP(S) URL on the configured academic-system origin."""
    target = client.url(path)
    target_parts = _parse_http_url(target)
    base_parts = _parse_http_url(client.base_url)
    if target_parts is None or base_parts is None or target_parts[1] != base_parts[1]:
        raise CsustError("只允许访问当前教务系统地址", code="invalid_path")
    return target


def _safe_urljoin(base_url: str, value: str) -> str:
    if not isinstance(base_url, str) or not isinstance(value, str) or _has_url_control(base_url) or _has_url_control(value):
        return ""
    try:
        return urljoin(base_url, value)
    except (TypeError, ValueError):
        return ""


def _append_query(url: str, params: list[tuple[str, str]]) -> str:
    if not params:
        return url
    try:
        parsed = urlparse(url)
    except ValueError as exc:
        raise CsustError("请求地址格式无效", code="invalid_path") from exc
    query = parsed.query + ("&" if parsed.query else "") + urlencode(params, doseq=True)
    return parsed._replace(query=query).geturl()


_REDACTED_URL = "%3Credacted%3E"
_URL_SENSITIVE = re.compile(r"pass|password|token|secret|sign|randomcode|ticket|encoded|cookie|session|jsessionid|csrf|nonce", re.I)
_SENSITIVE_FIELD = re.compile(
    r"password|passwd|cookie|session|captcha|randomcode|encoded|token|secret|ticket|sign|yztoken|csrf|nonce",
    re.I,
)
_FAILURE_SIGNAL = re.compile(r"(?:未(?:能)?|不|无法|没有)(?:成功|完成)|失败|错误|不能|不允许|无权限|未登录|不存在")
_CONTROL_CHARS = re.compile(r"[\x00-\x1f\x7f-\x9f]")
_ANSI_ESCAPE = re.compile(r"\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\)|[@-Z\\-_])")


def _safe_terminal_text(value: object) -> str:
    text = "" if value is None else str(value)
    return _CONTROL_CHARS.sub(" ", _ANSI_ESCAPE.sub("", text))


def _has_failure_signal(value: object) -> bool:
    return bool(_FAILURE_SIGNAL.search(str(value or "")))


def _safe_event(value: str) -> str:
    value = _safe_terminal_text(value)
    value = re.sub(r"(\btowptjbs\s*\().*?(\))", r"\1<redacted>\2", value or "", flags=re.I)
    value = re.sub(r"([?#&][^=&#\s]+)=([^&#\s'\";)]+)", r"\1=<redacted>", value)
    value = re.sub(
        r"([A-Za-z_][A-Za-z0-9_-]*\s*=\s*)([^\s&;,'\";)]+)",
        lambda match: f"{match.group(1)}<redacted>"
        if _SENSITIVE_FIELD.search(match.group(1))
        else match.group(0),
        value,
        flags=re.I,
    )
    return re.sub(r"[A-Za-z0-9_.-]{18,}", "<redacted>", value)


def _is_inert_event(value: str) -> bool:
    return bool(
        re.fullmatch(
            r"\s*(?:javascript:\s*)?(?:return\s+(?:false\s*)?|return\s*;|void\s*(?:\(\s*0\s*\)|0))\s*;?\s*",
            value or "",
            re.I,
        )
    )


def _safe_url(value: str, page_url: str = "") -> str:
    """Hide credentials and SSO signatures while keeping a usable path."""
    if not isinstance(value, str) or not value:
        return ""
    if value.lstrip().lower().startswith("javascript:"):
        return _safe_event(value)
    target = _safe_urljoin(page_url, value) if page_url else value
    try:
        parsed = urlparse(target)
        parsed.port
    except (TypeError, ValueError):
        return ""
    if parsed.username is not None or parsed.password is not None:
        parsed = parsed._replace(netloc=parsed.netloc.rsplit("@", 1)[-1])
    path = re.sub(r";(?:j?sessionid)=[^;/?#]*", f";jsessionid={_REDACTED_URL}", parsed.path, flags=re.I)
    params = re.sub(r"(^|;)(?:j?sessionid)=[^;/?#]*", rf"\1jsessionid={_REDACTED_URL}", parsed.params, flags=re.I)
    params = re.sub(
        r"(^|;)([^=;]+)=([^;#]*)",
        lambda match: f"{match.group(1)}{match.group(2)}={_REDACTED_URL}"
        if _URL_SENSITIVE.search(unquote(unquote(match.group(2))))
        or re.search(r"[A-Za-z0-9_.-]{18,}", unquote(unquote(match.group(3))))
        else match.group(0),
        params,
        flags=re.I,
    )
    parsed = parsed._replace(path=path, params=params)
    fragment = (
        _REDACTED_URL
        if _URL_SENSITIVE.search(parsed.fragment) or re.search(r"[A-Za-z0-9_.-]{18,}", parsed.fragment)
        else parsed.fragment
    )
    if not parsed.query:
        return parsed._replace(fragment=fragment).geturl()
    query = []
    for name, item in re.findall(r"([^=&]+)=?([^&]*)", parsed.query):
        sensitive = _URL_SENSITIVE.search(unquote(unquote(name))) or re.search(
            r"[A-Za-z0-9_.-]{18,}", unquote(unquote(item))
        )
        query.append(f"{name}={_REDACTED_URL}" if sensitive else f"{name}={item}")
    return parsed._replace(query="&".join(query), fragment=fragment).geturl()


def _option_value(option: Element) -> str:
    return option.attr("value") if "value" in option.attrs else option.text(include_scripts=False)


def _input_value(node: Element) -> str:
    if "value" in node.attrs:
        return node.attr("value")
    if node.attr("type", "text").lower() in {"checkbox", "radio"}:
        return "on"
    return node.text(include_scripts=False)


def _image_fields(node: Element) -> list[tuple[str, str]]:
    prefix = node.attr("name")
    return [(f"{prefix}.x" if prefix else "x", "0"), (f"{prefix}.y" if prefix else "y", "0")]


def internal_url(client: Client, path: str) -> str:
    """Resolve only same-site academic paths for generic page commands."""
    target = same_origin_url(client, path)
    try:
        request_path = unquote(unquote(urlparse(target).path))
    except ValueError as exc:
        raise CsustError("页面地址格式无效", code="invalid_path") from exc
    if (
        not request_path.startswith("/jsxsd/")
        or "\\" in request_path
        or any(part in {".", ".."} for part in request_path.split("/"))
    ):
        raise CsustError("只允许访问当前教务系统的 /jsxsd/ 页面", code="invalid_path")
    return target


def _write_private_file(path: Path, content: bytes, *, code: str, label: str) -> None:
    temporary: str | None = None
    try:
        path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        if path.is_symlink() or (path.exists() and not path.is_file()):
            raise OSError(f"{path} 不是普通文件或是符号链接")
        descriptor, temporary = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(content)
        os.chmod(temporary, 0o600)
        os.replace(temporary, path)
    except (OSError, ValueError) as exc:
        raise CsustError(f"无法保存{label} {path}: {exc}", code=code) from exc
    finally:
        if temporary:
            try:
                os.unlink(temporary)
            except OSError:
                pass


def is_login_page(response: Response) -> bool:
    body = _decode_body(response.body, response.headers)
    document = parse_html(body)
    if document.first("form", element_id="loginForm") is not None:
        return True
    for form in document.find_all("form"):
        inputs = form.find_all("input")
        login_form = form.attr("id") == "pwdFromId" or bool(re.search(r"login|logon|auth", form.attr("action"), re.I))
        if login_form and any(node.attr("type").lower() == "password" for node in inputs) and any(
            node.attr("name").lower() in {"username", "useraccount", "account", "loginid", "j_username"} for node in inputs
        ):
            return True
    visible_text = document.text(include_scripts=False)
    return "请输入账号" in visible_text and "用户登录" in visible_text


def require_logged_in(response: Response) -> None:
    if is_login_page(response):
        raise LoginRequired("会话已失效，请自动登录或先设置 CSUST_USERNAME、CSUST_PASSWORD")


def generate_encoded(account: str, password: str, data: str) -> str:
    """Match the current login page's interleaving algorithm."""
    try:
        scode, suffix = data.split("#", 1)
    except ValueError as exc:
        raise CsustError("教务系统返回的登录参数格式异常", code="login_protocol_error") from exc
    code = f"{account}%%%{password}"
    encoded: list[str] = []
    cursor = 0
    for index, character in enumerate(code):
        if index >= 20:
            encoded.append(code[index:])
            break
        if index >= len(suffix):
            raise CsustError("教务系统返回的登录参数长度异常", code="login_protocol_error")
        try:
            count = int(suffix[index])
        except ValueError as exc:
            raise CsustError("教务系统返回的登录参数校验失败", code="login_protocol_error") from exc
        if count > len(scode) - cursor:
            raise CsustError("教务系统返回的登录参数校验失败", code="login_protocol_error")
        encoded.append(character)
        encoded.append(scode[cursor : cursor + count])
        cursor += count
    return "".join(encoded)


def fetch_captcha(client: Client, path: str | None = None) -> tuple[Path, bytes]:
    if path:
        try:
            output = Path(path).expanduser()
        except (OSError, RuntimeError, ValueError) as exc:
            raise CsustError("验证码图片路径无效", code="captcha_write_failed") from exc
    else:
        output = client.cookie_file.parent / "captcha.png"
    content = client.request("/verifycode.servlet", binary=True)
    assert isinstance(content, bytes)
    if not content:
        raise CaptchaError("教务系统返回了空验证码", details={"captcha_image": str(output)})
    _write_private_file(output, content, code="captcha_write_failed", label="验证码图片")
    return output, content


def save_captcha(client: Client, path: str | None = None) -> Path:
    """Compatibility helper that fetches and stores a captcha image."""
    output, _ = fetch_captcha(client, path)
    return output


_ocr_engine: object | None = None


def _dotenv_values() -> dict[str, str]:
    """Read a local dotenv file without executing it or expanding secrets."""
    try:
        path = Path(os.environ.get("CSUST_ENV_FILE", ".env")).expanduser()
        if not path.is_file():
            return {}
    except (OSError, RuntimeError, ValueError):
        return {}
    values: dict[str, str] = {}
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except (OSError, UnicodeError):
        return {}
    for line in lines:
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].lstrip()
        if "=" not in line:
            continue
        name, raw = line.split("=", 1)
        name = name.strip()
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", name):
            continue
        try:
            parts = shlex.split(raw, comments=False, posix=True)
        except ValueError:
            continue
        values[name] = parts[0] if parts else ""
    return values


def env_value(name: str, default: str = "") -> str:
    """Use process environment first, then the local .env file."""
    if name in os.environ:
        return os.environ[name]
    return _dotenv_values().get(name, default)


def solve_captcha(image: bytes) -> str:
    """Recognize a captcha locally with the open-source ddddocr model."""
    global _ocr_engine
    if _ocr_engine is None:
        try:
            import ddddocr
        except ImportError as exc:
            raise OcrUnavailable("未安装本地 OCR 依赖，请安装 csust-cli 的 OCR 依赖", details={"package": "ddddocr"}) from exc
        _ocr_engine = ddddocr.DdddOcr(show_ad=False)
    try:
        value = _ocr_engine.classification(image)  # type: ignore[attr-defined]
    except Exception as exc:  # OCR backends expose different exception types.
        raise CaptchaError(f"OCR 识别失败：{exc}") from exc
    value = re.sub(r"\s+", "", str(value or ""))
    if not value:
        raise CaptchaError("OCR 未识别出验证码")
    return value


def _credentials(username: str | None = None) -> tuple[str, str]:
    account = username or env_value("CSUST_USERNAME", env_value("username")).strip()
    password = env_value("CSUST_PASSWORD", env_value("password"))
    missing: list[str] = []
    if not account:
        missing.append("CSUST_USERNAME")
    if not password:
        missing.append("CSUST_PASSWORD")
    if missing:
        raise CredentialsRequired("缺少登录凭据：" + ", ".join(missing) + "（也可在 .env 中设置 username/password）")
    return account, password


def _login_failure(body: str) -> CsustError | None:
    if re.search(r"验证码错误|验证码不正确|随机码错误", body):
        return CaptchaError("验证码错误")
    if re.search(r"密码错误|账号不存在|用户不存在|用户名或密码", body):
        return AuthenticationFailed("账号或密码错误")
    return None


def login(
    args: object | None = None,
    client: Client | None = None,
    *,
    ocr: Callable[[bytes], str] | None = None,
    max_attempts: int = CAPTCHA_RETRIES,
) -> dict[str, object]:
    """Log in without prompting and persist only the resulting Cookie session."""
    username = getattr(args, "username", None) if args is not None else None
    captcha_override = getattr(args, "captcha", None) if args is not None else None
    captcha_image = getattr(args, "captcha_image", None) if args is not None else None
    account, password = _credentials(username)
    client = client or Client(load_cookies=False)
    client.clear_cookies()
    client.warn_if_insecure()
    client.get("/")

    attempts = 1 if captcha_override or env_value("CSUST_CAPTCHA") else max(1, max_attempts)
    last_path: Path | None = None
    recognizer = ocr or solve_captcha
    for attempt in range(1, attempts + 1):
        path, image = fetch_captcha(client, captcha_image)
        last_path = path
        override = captcha_override or env_value("CSUST_CAPTCHA")
        try:
            captcha = override or recognizer(image)
        except CaptchaError as exc:
            details = dict(exc.details)
            details.update({"captcha_image": str(path), "attempts": attempt})
            if attempt >= attempts:
                raise CaptchaError(str(exc), details=details) from exc
            continue
        except Exception as exc:
            if attempt >= attempts:
                raise CaptchaError("OCR 识别失败", details={"captcha_image": str(path), "attempts": attempt}) from exc
            continue
        captcha = str(captcha).strip()
        if not captcha:
            if attempt < attempts and not override:
                continue
            raise CaptchaError("OCR 未识别出验证码", details={"captcha_image": str(path), "attempts": attempt})

        seed = client.post("/Logon.do?method=logon&flag=sess", {})
        encoded = generate_encoded(account, password, seed.body.strip())
        response = client.post(
            "/Logon.do?method=logon",
            {
                "userAccount": "",
                "userPassword": "",
                "RANDOMCODE": captcha,
                "encoded": encoded,
            },
            headers={"Referer": client.url("/")},
        )
        failure = _login_failure(response.body)
        if isinstance(failure, CaptchaError) and attempt < attempts and not override:
            continue
        if failure is not None:
            details = {"captcha_image": str(path), "attempts": attempt}
            raise type(failure)(str(failure), details=details) from failure
        probe = client.get(LOGIN_PROBE_PATH)
        if is_login_page(probe):
            if attempt < attempts and not override:
                continue
            raise AuthenticationFailed("登录失败，教务系统未建立有效会话", details={"captcha_image": str(path), "attempts": attempt})
        client.save()
        return {"ok": True, "username": account, "cookie_file": str(client.cookie_file), "attempts": attempt}

    raise CaptchaError("验证码识别失败", details={"captcha_image": str(last_path) if last_path else "", "attempts": attempts})


def ensure_session(client: Client, *, ocr: Callable[[bytes], str] | None = None) -> None:
    """Use a saved session, falling back to non-interactive env-based login."""
    try:
        if client.cookie_file.is_symlink():
            raise OSError("会话文件不能是符号链接")
        cookie_exists = client.cookie_file.exists()
    except (OSError, ValueError) as exc:
        raise CsustError(f"无法读取会话文件 {client.cookie_file}: {exc}", code="cookie_read_failed") from exc
    if not cookie_exists:
        login(client=client, ocr=ocr)
        return
    try:
        response = client.get(LOGIN_PROBE_PATH)
        require_logged_in(response)
        _save_cookie_refresh(client, response)
        return
    except LoginRequired:
        # A stale session is safe to replace; the login path never persists the password.
        login(client=client, ocr=ocr)
    except HttpError as exc:
        if exc.status != 401:
            raise
        login(client=client, ocr=ocr)
