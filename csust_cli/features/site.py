"""Generic access to public and authenticated CSUST subdomains."""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import deque
from pathlib import Path
from urllib.parse import unquote, urlparse, urlunsplit

from ..core import (
    AUTHSERVER_BASE_URL,
    Client,
    CsustError,
    HttpError,
    NetworkError,
    Response,
    _append_query,
    _decode_body,
    _same_origin_or_upgrade,
    _safe_url,
    _safe_urljoin,
    _save_cookie_refresh,
    _url_origin,
    login_sso_service,
)
from .web import (
    READ_ONLY_METHODS,
    SUPPORTED_METHODS,
    _download_result,
    _feedback,
    _is_side_effect_get,
    _page_payload,
    _pairs,
    _parse_upload_files,
    _request,
    _retry_read,
    _run_action_common,
    _run_form,
    _supported_method,
    inspect_page,
)


_BODY_UNSET = object()


CSUST_ROOT_DOMAIN = "csust.edu.cn"
SITE_COOKIE_DIR = Path.home() / ".config" / "csust-cli" / "sites"

# This is a useful starting map, not a hard limit. `site discover` is the
# live source of truth and the generic commands accept newly added subdomains.
SITE_CATALOG = (
    ("official", "www.csust.edu.cn", "学校主页", "https://www.csust.edu.cn/"),
    ("ehall", "ehall.csust.edu.cn", "统一门户", "https://ehall.csust.edu.cn/"),
    ("auth", "authserver.csust.edu.cn", "统一身份认证", "https://authserver.csust.edu.cn/authserver/login"),
    ("app", "app.csust.edu.cn", "掌上长理", "http://app.csust.edu.cn:8087/magus/appapi/downloadpage"),
    ("academic", "xk.csust.edu.cn", "教务系统", "http://xk.csust.edu.cn/"),
    ("vpn", "vpn.csust.edu.cn", "VPN 门户", "https://vpn.csust.edu.cn/enclient/start.html"),
    ("sunshine", "fuwu.csust.edu.cn", "教育阳光服务网", "https://fuwu.csust.edu.cn/"),
    ("map", "gis.csust.edu.cn", "校园地图", "https://gis.csust.edu.cn/"),
    ("mail", "mail.csust.edu.cn", "校园邮箱", "https://mail.csust.edu.cn/"),
    ("library", "lib.csust.edu.cn", "图书馆", "https://lib.csust.edu.cn/"),
    ("library-remote", "tsgvpn2.csust.edu.cn", "图书馆远程访问", "https://tsgvpn2.csust.edu.cn/"),
    ("recruitment", "rczpw.csust.edu.cn", "人才招聘", "https://rczpw.csust.edu.cn/zp.html"),
    ("jxjy", "jxjy.csust.edu.cn", "继续教育", "https://jxjy.csust.edu.cn/"),
    ("zyjx", "zyjx.csust.edu.cn", "专业技术人员继续教育", "https://zyjx.csust.edu.cn/"),
    ("equipment", "cslgdygx.csust.edu.cn", "校友服务", "https://cslgdygx.csust.edu.cn/"),
    ("highway", "highwayexperiment.csust.edu.cn", "公路工程实验中心", "https://highwayexperiment.csust.edu.cn/"),
    ("training", "gcxljxgl.csust.edu.cn", "工程训练教学管理系统", "http://gcxljxgl.csust.edu.cn/"),
    ("research", "ky.csust.edu.cn", "科研管理系统", "http://ky.csust.edu.cn/"),
    ("mooc", "mooc.csust.edu.cn", "慕课教学平台", "http://mooc.csust.edu.cn/portal"),
    ("legacy-portal", "my.csust.edu.cn", "统一身份认证旧入口", "http://my.csust.edu.cn/"),
    ("theol", "pt.csust.edu.cn", "网络教学综合平台", "http://pt.csust.edu.cn/meol/homepage/common/"),
    ("journal", "cslgqk.csust.edu.cn", "期刊社", "https://cslgqk.csust.edu.cn/"),
    ("journal-social", "cslgxbsk.csust.edu.cn", "学报社科版", "https://cslgxbsk.csust.edu.cn/"),
    ("journal-science", "cslgxbzk.csust.edu.cn", "学报自然科学版", "https://cslgxbzk.csust.edu.cn/"),
    ("journal-experiment", "syjx.csust.edu.cn", "实验教学与仪器", "https://syjx.csust.edu.cn/"),
    ("quality", "zbxt.csust.edu.cn", "教学评价系统", "https://zbxt.csust.edu.cn/login"),
    ("admissions", "zslq.csust.edu.cn", "招生录取查询", "https://zslq.csust.edu.cn/"),
    ("journal-highway", "zwgl.csust.edu.cn", "中外公路", "https://zwgl.csust.edu.cn/"),
    ("journal-highway-legacy", "zwgl1980.csust.edu.cn", "中外公路旧入口", "https://zwgl1980.csust.edu.cn/"),
    ("finance-query", "cwcx.csust.edu.cn", "财务查询", "https://cwcx.csust.edu.cn/"),
    ("fcmg", "fcmg.csust.edu.cn", "fcmg 服务", "https://fcmg.csust.edu.cn/"),
    ("union", "gonghui.csust.edu.cn", "智慧工会", "https://gonghui.csust.edu.cn/front/page.do?dispatch=proindex"),
    ("transport-lab", "jtsysyy.csust.edu.cn", "交通学院实验室预约管理平台", "http://jtsysyy.csust.edu.cn/Login/Index"),
    ("transport-info", "jtxxgl.csust.edu.cn", "交通运输工程学院综合信息服务平台", "https://jtxxgl.csust.edu.cn/"),
    ("transport-mobile", "jtyxxh.csust.edu.cn", "交通运输信息化平台", "https://jtyxxh.csust.edu.cn/"),
    ("academic-affairs", "jwc.csust.edu.cn", "教务处", "http://jwc.csust.edu.cn/"),
    ("sqyrjd", "sqyrjd.csust.edu.cn", "sqyrjd 服务", "https://sqyrjd.csust.edu.cn/"),
    ("srv", "srv.csust.edu.cn", "srv 服务", "http://srv.csust.edu.cn/"),
    ("continuing-education", "xwwy.csust.edu.cn", "继续教育信息服务平台", "https://xwwy.csust.edu.cn/"),
    ("graduate-management", "yjsgl.csust.edu.cn", "研究生管理系统", "https://yjsgl.csust.edu.cn/"),
    ("admissions-system", "zs.csust.edu.cn", "招生系统", "https://zs.csust.edu.cn/"),
    ("icsai2003", "icsai2003.csust.edu.cn", "icsai2003 服务", "http://icsai2003.csust.edu.cn/"),
    ("training-platform", "peixun.csust.edu.cn", "干部培训平台", "http://peixun.csust.edu.cn/"),
    ("trx", "trx.csust.edu.cn", "trx 服务", "http://trx.csust.edu.cn/"),
    ("v", "v.csust.edu.cn", "v 服务", "http://v.csust.edu.cn/"),
    ("alumni", "xy.csust.edu.cn", "校友服务", "https://xy.csust.edu.cn/"),
    ("live", "live.csust.edu.cn", "live 服务", "http://live.csust.edu.cn/"),
)

SITE_SERVICES = {service: {"service": service, "host": host, "name": name, "url": url} for service, host, name, url in SITE_CATALOG}


def _official_host(host: str) -> bool:
    value = host.rstrip(".").lower()
    return value == CSUST_ROOT_DOMAIN or value.endswith("." + CSUST_ROOT_DOMAIN)


def _service_info(value: str, scheme: str | None = None) -> dict[str, str]:
    key = str(value or "").strip().casefold()
    requested_scheme = str(scheme or "").strip().lower()
    if requested_scheme and requested_scheme not in {"http", "https"}:
        raise CsustError("服务传输方案只能是 http 或 https", code="invalid_argument")
    if key in SITE_SERVICES:
        info = SITE_SERVICES[key].copy()
        if requested_scheme:
            parsed = urlparse(info["url"])
            info["url"] = parsed._replace(scheme=requested_scheme).geturl()
        return info
    if not key or "/" in key or "?" in key or "#" in key:
        raise CsustError("服务必须是目录名或官方主机名，不是 URL", code="invalid_argument")
    try:
        parsed = urlparse("//" + key)
        parsed.port
    except (TypeError, ValueError) as exc:
        raise CsustError("服务主机名格式无效", code="invalid_argument") from exc
    host = (parsed.hostname or "").rstrip(".").lower()
    if not _official_host(host) or parsed.username is not None or parsed.password is not None:
        raise CsustError("服务必须是 csust.edu.cn 及其子域名", code="invalid_argument")
    netloc = host + (f":{parsed.port}" if parsed.port else "")
    return {"service": host, "host": host, "name": host, "url": f"{requested_scheme or 'https'}://{netloc}/"}


def _normalize_url(value: str, *, base_url: str | None = None) -> str:
    if not isinstance(value, str) or not value.strip():
        raise CsustError("site URL 不能为空", code="invalid_path")
    candidate = value.strip()
    if not candidate.lower().startswith(("http://", "https://")):
        if base_url is None:
            candidate = "https://" + candidate
        else:
            candidate = base_url.rstrip("/") + "/" + candidate.lstrip("/")
    try:
        parsed = urlparse(candidate)
        parsed.port
        host = (parsed.hostname or "").rstrip(".").lower()
        decoded_path = unquote(unquote(parsed.path))
    except (TypeError, UnicodeError, ValueError) as exc:
        raise CsustError("site URL 格式无效", code="invalid_path") from exc
    if (
        parsed.scheme.lower() not in {"http", "https"}
        or not parsed.netloc
        or not host
        or not _official_host(host)
        or parsed.username is not None
        or parsed.password is not None
        or "\\" in decoded_path
        or any(part in {".", ".."} for part in decoded_path.split("/"))
    ):
        raise CsustError("site 只允许访问 csust.edu.cn 及其子域名", code="invalid_path")
    target = parsed._replace(fragment="").geturl()
    if base_url is not None:
        if not _same_origin_or_upgrade(_url_origin(base_url), _url_origin(target)):
            raise CsustError("site 操作必须保持当前子域名", code="invalid_path")
    return target


def _cookie_path(url: str) -> Path:
    parsed = urlparse(url)
    host = (parsed.hostname or "site").lower().replace(".", "_")
    if parsed.port:
        host += f"_{parsed.port}"
    return SITE_COOKIE_DIR / f"{host}.cookies.txt"


class SiteClient(Client):
    """A same-origin client whose base can be any official CSUST host."""

    allow_anonymous_pages = True

    def __init__(self, url: str, cookie_file: Path | None = None, *, load_cookies: bool = True, allow_external: bool = False) -> None:
        normalized = _normalize_url(url)
        parsed = urlparse(normalized)
        base = urlunsplit((parsed.scheme, parsed.netloc, "", "", ""))
        super().__init__(base, cookie_file or _cookie_path(base), load_cookies=load_cookies)
        self.set_redirect_origins({_url_origin(self.base_url), _url_origin(AUTHSERVER_BASE_URL)})
        self.allow_external = allow_external

    def web_url(self, value: str) -> str:
        if not isinstance(value, str) or value.lstrip().lower().startswith(("javascript:", "mailto:", "data:")):
            raise CsustError("site 动作目标不是可请求的 HTTP(S) 地址", code="invalid_path")
        target = self.url(value)
        try:
            return _normalize_url(target, base_url=self.base_url)
        except CsustError:
            if not self.allow_external:
                raise
            try:
                parsed = urlparse(target)
                parsed.port
                decoded_path = unquote(unquote(parsed.path))
            except (TypeError, UnicodeError, ValueError) as exc:
                raise CsustError("外部动作目标格式无效", code="invalid_path") from exc
            if (
                parsed.scheme.lower() not in {"http", "https"}
                or not parsed.netloc
                or not parsed.hostname
                or parsed.username is not None
                or parsed.password is not None
                or "\\" in decoded_path
                or any(part in {".", ".."} for part in decoded_path.split("/"))
            ):
                raise CsustError("外部动作目标格式无效", code="invalid_path")
            if urlparse(self.base_url).scheme.lower() == "https" and parsed.scheme.lower() == "http" and (parsed.hostname or "").lower() == (urlparse(self.base_url).hostname or "").lower():
                raise CsustError("不允许 HTTPS 页面降级到 HTTP 动作", code="invalid_path")
            target = parsed._replace(fragment="").geturl()
            origins = set(self._redirect_origins or ())
            origins.add(_url_origin(target))
            self.set_redirect_origins(origins)
            return target

    def ensure_web_session(self) -> None:
        # Generic services do not share a reliable probe path. Saved cookies
        # are used as-is; `site login` is the explicit recovery operation.
        return


def _service_path(args: argparse.Namespace) -> tuple[dict[str, str], str]:
    info = _service_info(args.service, getattr(args, "scheme", None))
    path = str(getattr(args, "path", "") or "")
    if not path:
        parsed = urlparse(info["url"])
        path = parsed.path or "/"
        if parsed.query:
            path += "?" + parsed.query
    if path.lower().startswith(("http://", "https://")):
        raise CsustError("--path 只能是服务内路径，不能填写 URL", code="invalid_argument")
    if not path.startswith("/"):
        path = "/" + path
    return info, path


def _client(args: argparse.Namespace, service: str, *, load_cookies: bool = True) -> SiteClient:
    info = _service_info(service, getattr(args, "scheme", None))
    cookie_file = getattr(args, "cookie_file", None)
    try:
        path = Path(cookie_file).expanduser() if cookie_file else None
    except (OSError, RuntimeError, ValueError) as exc:
        raise CsustError("site 会话文件路径无效", code="cookie_read_failed") from exc
    return SiteClient(info["url"], path, load_cookies=load_cookies, allow_external=bool(getattr(args, "allow_external", False)))


def _target(client: SiteClient, value: str, params: list[tuple[str, str]] = ()) -> str:
    target = client.web_url(value)
    return _append_query(target, params)


def _json_argument(value: str | None) -> object:
    if value is None:
        raise CsustError("缺少 JSON 请求体", code="invalid_argument")
    source = value
    if value == "-":
        source = sys.stdin.read()
    elif value.startswith("@"):
        try:
            source = Path(value[1:]).expanduser().read_text(encoding="utf-8")
        except (OSError, UnicodeError) as exc:
            raise CsustError(f"无法读取 JSON 请求体：{exc}", code="invalid_argument") from exc
    try:
        return json.loads(source)
    except (TypeError, ValueError) as exc:
        raise CsustError("--data-json 必须是有效 JSON，或使用 @FILE/−", code="invalid_argument") from exc


def _headers(values: list[str]) -> dict[str, str]:
    pairs = _pairs(values, "--header")
    return {name: value for name, value in pairs}


def run_catalog(_args: argparse.Namespace, _client: Client | None = None) -> dict[str, object]:
    return {
        "source": "https://www.csust.edu.cn/",
        "catalog": [{"service": service, "host": host, "name": name, "url": url} for service, host, name, url in SITE_CATALOG],
        "domain": CSUST_ROOT_DOMAIN,
        "note": "清单是已观察到的入口；site discover 才是实时发现，site 命令接受新子域名。",
    }


def run_get(args: argparse.Namespace, client: SiteClient | None = None) -> dict[str, object]:
    _info, path = _service_path(args)
    client = client or _client(args, args.service)
    target = _target(client, path, _pairs(args.param, "--param"))
    response, saved = _request(
        client,
        "GET",
        target,
        output=args.output,
        require_session=args.require_login,
    )
    if saved:
        return {**saved, "site": client.base_url}
    return {
        "ok": True,
        "submitted": False,
        "confirmed": True,
        "site": client.base_url,
        "response": _page_payload(response),
    }


def run_request(args: argparse.Namespace, client: SiteClient | None = None) -> dict[str, object]:
    method = _supported_method(args.method)
    body = _json_argument(args.data_json) if args.data_json is not None else _BODY_UNSET
    data = _pairs(args.data, "--data")
    files = _parse_upload_files(args.file)
    if body is not _BODY_UNSET and (data or files):
        raise CsustError("--data-json 不能与 --data/--file 同时使用", code="invalid_argument")
    if method in READ_ONLY_METHODS and (body is not _BODY_UNSET or files or data):
        raise CsustError("GET/HEAD/OPTIONS 请使用 --param", code="invalid_argument")
    _info, path = _service_path(args)
    client = client or _client(args, args.service)
    target = _target(client, path, _pairs(args.param, "--param"))
    mutating = method not in READ_ONLY_METHODS or _is_side_effect_get(target)
    if mutating and not args.yes:
        raise CsustError("site 请求可能修改远端数据，请加 --yes", code="confirmation_required")
    request = {
        "method": method,
        "url": _safe_url(target),
        "fields": [name for name, _ in data],
        "json": body is not _BODY_UNSET,
    }
    request_options: dict[str, object] = {
        "headers": _headers(args.header),
    }
    if body is not _BODY_UNSET:
        request_options["json_body"] = body
    response, saved = _request(
        client,
        method,
        target,
        data or None,
        output=args.output,
        require_session=args.require_login,
        multipart=files or None,
        mutating=mutating,
        **request_options,
    )
    if mutating:
        if saved:
            return {**_download_result(response, {**saved, "request": request}, True), "site": client.base_url}
        return {**_mutation_response(response, request), "site": client.base_url}
    if saved:
        return {**saved, "request": request, "site": client.base_url}
    return {
        "ok": True,
        "submitted": False,
        "confirmed": True,
        "request": request,
        "site": client.base_url,
        "response": _page_payload(response),
    }


def _mutation_response(response: Response, request: dict[str, object]) -> dict[str, object]:
    from ..core import result_status

    payload = _page_payload(response)
    return {**result_status(_feedback(response), mutating=True, details={"request": request}), "request": request, "response": payload}


def run_form(args: argparse.Namespace, client: SiteClient | None = None) -> dict[str, object]:
    _info, path = _service_path(args)
    client = client or _client(args, args.service)
    return _run_form(args, client, path)


def run_action(args: argparse.Namespace, client: SiteClient | None = None) -> dict[str, object]:
    _info, path = _service_path(args)
    client = client or _client(args, args.service)
    return _run_action_common(args, client, path)


def _script_endpoints(source: str, page_url: str) -> list[str]:
    endpoints: set[str] = set()
    # ponytail: regex extraction covers route-like literals; add a JS parser only if a real page needs it.
    for value in re.findall(r"[\"']([^\"'\s<>]+)[\"']", source):
        if not value.startswith(("/", "http://", "https://")):
            continue
        target = _safe_urljoin(page_url, value)
        try:
            parsed = urlparse(target)
        except ValueError:
            continue
        if not _official_host(parsed.hostname or ""):
            continue
        if value.startswith(("/api/", "/ajax/", "/rest/", "/service/", "/graphql", "/oauth/", "/auth/", "/v1/", "/v2/")) or re.search(
            r"\.(?:do|action|json|jsp|php)(?:[?#]|$)", parsed.path, re.I
        ):
            endpoints.add(_safe_url(target))
    return sorted(endpoint for endpoint in endpoints if endpoint)


def run_scripts(args: argparse.Namespace, client: SiteClient | None = None) -> dict[str, object]:
    if args.max_scripts < 1 or args.max_scripts > 100:
        raise CsustError("--max-scripts 必须在 1 到 100 之间", code="invalid_argument")
    _info, path = _service_path(args)
    client = client or _client(args, args.service)
    root = _target(client, path, _pairs(args.param, "--param"))
    page_response = _retry_read("GET", root, lambda: client.get(root))
    _save_cookie_refresh(client, page_response)
    page = inspect_page(_decode_body(page_response.body, page_response.headers), page_response.url)
    script_sources = page.get("scripts", {}).get("src", []) if isinstance(page.get("scripts"), dict) else []
    scripts: list[dict[str, object]] = []
    endpoints: set[str] = set()
    for source in list(script_sources)[: args.max_scripts]:
        source_url = str(source or "")
        try:
            target = _normalize_url(_safe_urljoin(page_response.url, source_url), base_url=client.base_url)
        except CsustError:
            scripts.append({"url": _safe_url(source_url, page_response.url), "skipped": True, "reason": "external"})
            continue
        try:
            response = _retry_read("GET", target, lambda: client.get(target))
            text = _decode_body(response.body, response.headers)
            found = _script_endpoints(text, response.url)
            endpoints.update(found)
            scripts.append({"url": _safe_url(response.url), "bytes": len(text.encode("utf-8")), "endpoints": found})
        except (CsustError, HttpError, NetworkError) as exc:
            scripts.append({"url": _safe_url(target), "error": str(exc), "code": getattr(exc, "code", "error")})
    return {
        "ok": True,
        "page": {"url": page.get("url"), "title": page.get("title"), "kind": page.get("kind")},
        "scripts": scripts,
        "script_count": len(scripts),
        "endpoints": sorted(endpoints),
    }


def _crawlable(client: SiteClient, value: str, root: str) -> str | None:
    try:
        target = _normalize_url(_safe_urljoin(root, value), base_url=client.base_url)
        parsed = urlparse(target)
    except CsustError:
        return None
    if _is_side_effect_get(target):
        return None
    if parsed.path.lower().endswith((".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".pdf", ".zip", ".woff", ".woff2")):
        return None
    return target


def run_discover(args: argparse.Namespace, client: SiteClient | None = None) -> dict[str, object]:
    if args.depth < 0 or args.depth > 3:
        raise CsustError("--depth 必须在 0 到 3 之间", code="invalid_argument")
    # ponytail: keep one explicit crawl ceiling; raise it only if a measured site needs more.
    if args.max_pages < 1 or args.max_pages > 2000:
        raise CsustError("--max-pages 必须在 1 到 2000 之间", code="invalid_argument")
    _info, path = _service_path(args)
    client = client or _client(args, args.service)
    root = _target(client, path, _pairs(args.param, "--param"))
    queue = deque([(root, 0)])
    queued = {root}
    pages: list[dict[str, object]] = []
    errors: list[dict[str, str]] = []
    hosts = {urlparse(root).hostname or ""}
    while queue and len(pages) < args.max_pages:
        target, depth = queue.popleft()
        try:
            response = _retry_read("GET", target, lambda: client.get(target))
            _save_cookie_refresh(client, response)
            page = inspect_page(_decode_body(response.body, response.headers), response.url)
        except (CsustError, HttpError, NetworkError) as exc:
            errors.append({"url": _safe_url(target), "code": getattr(exc, "code", "error"), "error": str(exc)})
            continue
        pages.append({"url": page.get("url"), "title": page.get("title"), "kind": page.get("kind"), "links": page.get("links", []), "forms": page.get("forms", []), "actions": page.get("actions", []), "endpoints": page.get("endpoints", [])})
        for link in page.get("links", []):
            if not isinstance(link, dict):
                continue
            value = str(link.get("path") or "")
            absolute = _safe_urljoin(response.url, value)
            try:
                host = (urlparse(absolute).hostname or "").lower()
            except ValueError:
                host = ""
            if _official_host(host):
                hosts.add(host)
            if depth >= args.depth:
                continue
            child = _crawlable(client, value, response.url)
            if child and child not in queued:
                queued.add(child)
                queue.append((child, depth + 1))
    return {
        "ok": True,
        "root": _safe_url(root),
        "depth": args.depth,
        "max_pages": args.max_pages,
        "pages": pages,
        "page_count": len(pages),
        "hosts": sorted(host for host in hosts if host),
        "errors": errors,
    }


def run_login(args: argparse.Namespace, _client: SiteClient | None = None) -> dict[str, object]:
    if args.auth not in {"auto", "sso"}:
        raise CsustError("site login 只支持 auto 或 sso", code="invalid_argument")
    _info, path = _service_path(args)
    client = _client(args, args.service, load_cookies=False)
    return login_sso_service(client, _target(client, path, _pairs(args.param, "--param")), args)


def run_logout(args: argparse.Namespace, client: SiteClient | None = None) -> dict[str, object]:
    _service_info(args.service)
    client = client or _client(args, args.service)
    client.clear_cookies()
    client.save()
    return {"ok": True, "site": client.base_url, "cookie_file": str(client.cookie_file), "logged_out": True}


def _session_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--cookie-file", help="覆盖该子域名的本机会话文件")


def _common_page_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--service", required=True, help="服务目录名或官方主机名")
    parser.add_argument("--path", help="服务内路径；默认使用目录入口")
    parser.add_argument("--scheme", choices=("http", "https"), help="覆盖服务传输方案；默认使用目录配置或 HTTPS")
    _session_args(parser)
    parser.add_argument("--output", help="原样保存响应文件")
    parser.add_argument("--require-login", action="store_true", help="把登录页视为会话失效")
    parser.add_argument("--allow-external", action="store_true", help="允许执行页面明确指向的外部 HTTP(S) 动作")
    parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS)


def _form_args(parser: argparse.ArgumentParser) -> None:
    _common_page_args(parser)
    parser.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    parser.add_argument("--form", required=True, type=int, help="按 1 起始序号选择表单")
    parser.add_argument("--button", type=int, help="表单内按 1 起始序号选择按钮")
    parser.add_argument("--data", action="append", default=[], help="覆盖表单字段 NAME=VALUE，可重复")
    parser.add_argument("--file", action="append", default=[], help="multipart 文件字段 NAME=PATH，可重复")
    parser.add_argument("--fingerprint", help="页面指纹")
    parser.add_argument("--yes", action="store_true", help="确认可能产生远端变更的操作")


def register(subparsers: argparse._SubParsersAction) -> None:
    site = subparsers.add_parser("site", aliases=["domain", "portal"], help="访问任意 csust.edu.cn 子域名")
    children = site.add_subparsers(dest="site_command", required=True)

    catalog = children.add_parser("catalog", help="列出已观察到的官方入口")
    catalog.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    catalog.set_defaults(feature_runner=run_catalog, feature_renderer=render)

    get = children.add_parser("get", help="GET 页面并输出结构化快照")
    _common_page_args(get)
    get.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    get.set_defaults(feature_runner=run_get, feature_renderer=render)

    request = children.add_parser("request", help="调用任意同源 HTTP 接口")
    _common_page_args(request)
    request.add_argument("--method", default="GET", choices=sorted(SUPPORTED_METHODS))
    request.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    request.add_argument("--data", action="append", default=[], help="表单字段 NAME=VALUE，可重复")
    request.add_argument("--data-json", help="JSON 请求体；可用 @FILE 或 -")
    request.add_argument("--file", action="append", default=[], help="multipart 文件字段 NAME=PATH，可重复")
    request.add_argument("--header", action="append", default=[], help="请求头 NAME=VALUE，可重复")
    request.add_argument("--yes", action="store_true", help="确认非只读请求")
    request.set_defaults(feature_runner=run_request, feature_renderer=render)

    form = children.add_parser("form", help="按结构化页面表单提交")
    _form_args(form)
    form.set_defaults(feature_runner=run_form, feature_renderer=render)

    action = children.add_parser("action", aliases=["run"], help="按页面动作 ref 或序号执行")
    _common_page_args(action)
    action.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    action.add_argument("--index", type=int, help="兼容模式：页面动作序号")
    action.add_argument("--ref", help="页面快照中的稳定动作 ref")
    action.add_argument("--data", action="append", default=[], help="覆盖/附加字段 NAME=VALUE，可重复")
    action.add_argument("--file", action="append", default=[], help="multipart 文件字段 NAME=PATH，可重复")
    action.add_argument("--fingerprint", help="使用 --index 时校验页面指纹")
    action.add_argument("--yes", action="store_true", help="确认可能产生远端变更的操作")
    action.set_defaults(feature_runner=run_action, feature_renderer=render)

    scripts = children.add_parser("scripts", help="读取同源脚本并提取常见 API/页面端点")
    scripts.add_argument("--service", required=True, help="服务目录名或官方主机名")
    scripts.add_argument("--path", help="脚本所在服务内路径；默认使用目录入口")
    scripts.add_argument("--scheme", choices=("http", "https"), help="覆盖服务传输方案；默认使用目录配置或 HTTPS")
    scripts.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    _session_args(scripts)
    scripts.add_argument("--max-scripts", type=int, default=30)
    scripts.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    scripts.set_defaults(feature_runner=run_scripts, feature_renderer=render)

    discover = children.add_parser("discover", help="实时抓取页面并发现同站链接和官方子域名")
    discover.add_argument("--service", required=True, help="服务目录名或官方主机名")
    discover.add_argument("--path", help="起始服务内路径；默认使用目录入口")
    discover.add_argument("--scheme", choices=("http", "https"), help="覆盖服务传输方案；默认使用目录配置或 HTTPS")
    _session_args(discover)
    discover.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    discover.add_argument("--depth", type=int, default=1)
    discover.add_argument("--max-pages", type=int, default=30)
    discover.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    discover.set_defaults(feature_runner=run_discover, feature_renderer=render)

    login = children.add_parser("login", help="通过统一身份认证登录指定子域名")
    login.add_argument("--service", required=True, help="服务目录名或官方主机名")
    login.add_argument("--path", help="需要登录的服务内路径；默认使用目录入口")
    login.add_argument("--scheme", choices=("http", "https"), help="覆盖服务传输方案；默认使用目录配置或 HTTPS")
    _session_args(login)
    login.add_argument("--param", action="append", default=[], help="服务入口查询参数 NAME=VALUE，可重复")
    login.add_argument("--username")
    login.add_argument("--auth", choices=("auto", "sso"), default="auto")
    login.add_argument("--captcha")
    login.add_argument("--captcha-image")
    login.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    login.set_defaults(feature_runner=run_login, feature_renderer=render)

    logout = children.add_parser("logout", help="清除指定子域名的本机会话")
    logout.add_argument("--service", required=True, help="服务目录名或官方主机名")
    _session_args(logout)
    logout.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    logout.set_defaults(feature_runner=run_logout, feature_renderer=render)


def render(data: dict[str, object]) -> None:
    if "catalog" in data:
        for item in data["catalog"]:
            if isinstance(item, dict):
                print("\t".join(str(item.get(key, "")) for key in ("service", "host", "name", "url")))
        return
    if data.get("downloaded"):
        print(f"已保存：{data.get('output')}（{data.get('bytes', 0)} bytes）")
        return
    if "hosts" in data:
        print(f"发现 {data.get('page_count', 0)} 个页面；官方子域名 {len(data.get('hosts', []))} 个")
        for host in data.get("hosts", []):
            print(host)
        return
    response = data.get("response", data)
    if isinstance(response, dict):
        print(f"{response.get('title', '')}\t{response.get('url', '')}".strip())
        for form in response.get("forms", []):
            if isinstance(form, dict):
                print(f"表单\t{form.get('index', '')}\t{form.get('method', '')}\t{form.get('action', '')}")
        for action in response.get("actions", []):
            if isinstance(action, dict):
                print(f"动作\t{action.get('ref', '')}\t{action.get('text', '')}\t{action.get('target', '')}")
        return
    print(json.dumps(response, ensure_ascii=False))
