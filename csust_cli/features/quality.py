"""CLI access to the teaching quality/academic portal behind VPN SSO."""

from __future__ import annotations

import argparse
import json
import time
from pathlib import Path
from urllib.parse import urlparse

from ..core import (
    AuthenticationFailed,
    CAPTCHA_RETRIES,
    CaptchaError,
    CsustError,
    Response,
    _credentials,
    _decode_body,
    _login_failure,
    _save_cookie_refresh,
    _write_private_file,
    generate_encoded,
    is_login_page,
    solve_captcha,
)
from . import academic, teaching, vpn, web
from .teaching import TeachingClient
from .vpn import _has_session_cookie, _json_argument, _parse_files, _parse_pairs


QUALITY_SERVICE_NAME = "教学一体化"
QUALITY_SYSTEM_NAME = "教学质量保障系统"

QUALITY_ROUTE_CATALOG = tuple(
    {
        "name": command,
        "section": group,
        "label": label,
        "path": path,
        "description": label,
    }
    for command, group, label, path in web.ROUTE_CATALOG
)
QUALITY_ROUTE_BY_NAME = {str(item["name"]): item for item in QUALITY_ROUTE_CATALOG}


class QualityClient(TeachingClient):
    """Reuse the VPN web gateway while retaining its cookies during portal login."""

    def __init__(
        self,
        base_url: str | None = None,
        cookie_file: Path | None = None,
        session_file: Path | None = None,
        *,
        prefix: str | None = None,
        load_cookies: bool = True,
    ) -> None:
        super().__init__(
            base_url,
            cookie_file,
            session_file,
            prefix=prefix,
            service_name=QUALITY_SERVICE_NAME,
            load_cookies=load_cookies,
        )
        self.quality_logged_in = False

    def _ensure_vpn_session(self, args: argparse.Namespace | None = None) -> None:
        if self.web_prefix or self.session.get("token") or _has_session_cookie(self):
            return
        login_args = argparse.Namespace(
            username=getattr(args, "username", None) if args else None,
            password_stdin=False,
            auth="cas",
            captcha_info=getattr(args, "vpn_captcha_info", None) if args else None,
        )
        result = vpn.login(login_args, self)
        if not isinstance(result, dict) or not result.get("ok"):
            raise AuthenticationFailed("VPN 统一认证未建立有效会话")

    def web_url(self, path: str) -> str:
        self._ensure_vpn_session()
        if isinstance(path, str):
            value = path.strip()
            if value.lower().startswith(("http://", "https://")) or value.startswith(("/http/", "/https/")):
                target = self.url(value)
                parsed = urlparse(target)
                base = urlparse(self.base_url)
                marker = parsed.path.find("/jsxsd/")
                service = self.ensure_service()
                prefix = str(service.get("urlPlus") or self.web_prefix).rstrip("/")
                if parsed.netloc.lower() == base.netloc.lower() and marker >= 0 and prefix:
                    if not parsed.path.startswith(prefix + "/") and parsed.path != prefix:
                        path = parsed._replace(path=prefix + parsed.path[marker:]).geturl()
        return super().web_url(path)

    def request_web(self, path: str, **kwargs: object) -> Response:
        self._ensure_vpn_session()
        return super().request_web(path, **kwargs)

    def login(self, args: argparse.Namespace | None = None) -> dict[str, object]:
        self._ensure_vpn_session(args)
        account, password = _credentials(getattr(args, "username", None) if args else None)
        captcha_override = getattr(args, "captcha", None) if args else None
        captcha_image = getattr(args, "captcha_image", None) if args else None
        landing = self.request_web("/")
        last_error: CaptchaError | None = None
        for attempt in range(1, CAPTCHA_RETRIES + 1):
            captcha_response = self.request_web("/verifycode.servlet", output=True)
            image = captcha_response.body
            if isinstance(image, str):
                image = image.encode("utf-8")
            if not isinstance(image, bytes) or not image:
                raise CaptchaError("教学质量保障系统返回了空验证码", code="captcha_failed")
            if captcha_image:
                _write_private_file(Path(captcha_image).expanduser(), image, code="captcha_write_failed", label="验证码图片")
            try:
                captcha = str(captcha_override or solve_captcha(image)).strip()
            except CaptchaError as exc:
                last_error = exc
                if captcha_override or attempt == CAPTCHA_RETRIES:
                    raise
                continue
            seed = self.request_web("/Logon.do?method=logon&flag=sess", method="POST", data=[])
            encoded = generate_encoded(account, password, _decode_body(seed.body, seed.headers).strip())
            result = self.request_web(
                "/Logon.do?method=logon",
                method="POST",
                data=[("userAccount", ""), ("userPassword", ""), ("RANDOMCODE", captcha), ("encoded", encoded)],
                referer=landing.url,
            )
            failure = _login_failure(_decode_body(result.body, result.headers))
            if failure is not None:
                if isinstance(failure, CaptchaError) and not captcha_override and attempt < CAPTCHA_RETRIES:
                    last_error = failure
                    continue
                raise failure
            probe = self.request_web("/jsxsd/framework/xsMain.jsp")
            if not is_login_page(probe):
                self.quality_logged_in = True
                self.save()
                return {
                    "ok": True,
                    "username": account,
                    "auth": "vpn-cas+quality-portal",
                    "service": QUALITY_SERVICE_NAME,
                    "system": QUALITY_SYSTEM_NAME,
                }
            if attempt == CAPTCHA_RETRIES:
                raise AuthenticationFailed("教学质量保障系统未建立有效会话")
        raise last_error or AuthenticationFailed("教学质量保障系统登录失败")

    def ensure_quality_session(self) -> None:
        if self.quality_logged_in:
            return
        self._ensure_vpn_session()
        probe = self.request_web("/jsxsd/framework/xsMain.jsp")
        if is_login_page(probe):
            self.login()
        else:
            self.quality_logged_in = True

    def logout(self) -> Response:
        self.ensure_quality_session()
        response = self.request_web(f"/jsxsd/xk/LoginToXk?method=exit&tktime={int(time.time() * 1000)}")
        self.quality_logged_in = False
        _save_cookie_refresh(self, response)
        return response


def _entry(name: str | None, path: str | None) -> tuple[str, str]:
    if name:
        item = QUALITY_ROUTE_BY_NAME.get(name.strip())
        if item is None:
            raise CsustError(f"未知教学质量保障页面：{name}；先运行 csust quality catalog", code="unknown_route")
        return str(item["path"]), "GET"
    if not path:
        raise CsustError("--name 与 --path 至少指定一个", code="invalid_argument")
    return path, "GET"


def _payload(response: Response, *, raw: bool = False) -> object:
    return teaching._response_payload(response, raw=raw)


def run_catalog(_args: argparse.Namespace, _client: QualityClient | None = None) -> dict[str, object]:
    return {
        "system": QUALITY_SYSTEM_NAME,
        "service": QUALITY_SERVICE_NAME,
        "menus": [{"command": command, "name": name, "code": code} for command, name, code in web.MAIN_MENU_CATALOG],
        "groups": [{"name": name, "menu": menu, "code": code} for name, menu, code in web.SECOND_LEVEL_CATALOG],
        "routes": list(QUALITY_ROUTE_CATALOG),
        "route_count": len(QUALITY_ROUTE_CATALOG),
        "evaluation": {
            "commands": ["batches", "courses", "form", "save", "submit"],
            "page": "/jsxsd/xspj/xspj_find.do",
        },
        "conditional": [{"name": "毕业设计", "command": "graduation-design", "kind": "external-sso"}],
        "public": [
            {"name": name, "label": label, "path": path}
            for name, label, path in web.PUBLIC_CATALOG
        ],
        "request": "csust quality request --path /jsxsd/... --method POST --data NAME=VALUE --yes --json",
    }


def run_login(args: argparse.Namespace, client: QualityClient) -> dict[str, object]:
    return client.login(args)


def run_status(_args: argparse.Namespace, client: QualityClient) -> dict[str, object]:
    client.ensure_quality_session()
    return {"ok": True, "logged_in": True, "service": QUALITY_SERVICE_NAME, "system": QUALITY_SYSTEM_NAME}


def run_logout(_args: argparse.Namespace, client: QualityClient) -> dict[str, object]:
    response = client.logout()
    return {"ok": response.status < 400, "logged_out": response.status < 400, "status": response.status, "service": QUALITY_SERVICE_NAME}


def run_routes(_args: argparse.Namespace, client: QualityClient) -> dict[str, object]:
    client.ensure_quality_session()
    response = client.request_web("/jsxsd/framework/xsMain.jsp")
    return {"ok": response.status < 400, "service": QUALITY_SERVICE_NAME, "system": QUALITY_SYSTEM_NAME, "page": _payload(response)}


def run_form(args: argparse.Namespace, client: QualityClient) -> dict[str, object]:
    return web._run_form(args, client, args.path)


def _run_request(args: argparse.Namespace, client: QualityClient) -> dict[str, object]:
    path, named_method = _entry(getattr(args, "name", None), getattr(args, "path", None))
    method = (named_method if getattr(args, "name", None) else getattr(args, "method", "GET")).strip().upper()
    if method not in web.SUPPORTED_METHODS:
        raise CsustError("不支持的 HTTP 方法", code="invalid_argument")
    params = _parse_pairs(getattr(args, "param", []), "--param")
    data = _parse_pairs(getattr(args, "data", []), "--data")
    files = _parse_files(getattr(args, "file", []))
    data_json = getattr(args, "data_json", None)
    if files and data_json is not None:
        raise CsustError("--data-json 不能与 --file 同时使用", code="invalid_argument")
    if method in web.READ_ONLY_METHODS and (data or files or data_json is not None):
        raise CsustError("GET/HEAD/OPTIONS 只能使用 --param", code="invalid_argument")
    if method not in web.READ_ONLY_METHODS and not args.yes:
        raise CsustError("教学质量保障请求可能修改账号数据，请加 --yes", code="confirmation_required")
    client.ensure_quality_session()
    options: dict[str, object] = {
        "method": method,
        "params": params,
        "output": bool(args.output),
        "referer": getattr(args, "referer", ""),
    }
    if method in web.READ_ONLY_METHODS:
        pass
    elif files:
        options["multipart"] = files
    elif data_json is not None:
        options["json_body"] = _json_argument(data_json)
    else:
        options["data"] = data
    response = client.request_web(path, **options)
    request = {"name": getattr(args, "name", None), "method": method, "path": urlparse(path).path, "fields": [name for name, _ in data], "service": QUALITY_SERVICE_NAME}
    if args.output:
        body = response.body if isinstance(response.body, bytes) else str(response.body).encode("utf-8")
        output = Path(args.output).expanduser()
        _write_private_file(output, body, code="quality_output_write_failed", label="教学质量保障响应")
        return {"ok": response.status < 400, "downloaded": True, "status": response.status, "output": str(output), "bytes": len(body), "request": request}
    payload = _payload(response, raw=bool(args.raw))
    return {
        "ok": response.status < 400,
        "status": response.status,
        "submitted": method not in web.READ_ONLY_METHODS,
        "confirmed": teaching._mutation_confirmed(payload) if method not in web.READ_ONLY_METHODS else True,
        "request": request,
        "response": payload,
    }


def _add_request_args(parser: argparse.ArgumentParser, *, get_only: bool = False) -> None:
    parser.add_argument("--name", help="目录页面名称；可来自 quality catalog")
    parser.add_argument("--path", help="教学质量保障系统同源路径")
    if not get_only:
        parser.add_argument("--method", default="GET", help="GET/HEAD/OPTIONS/POST/PUT/PATCH/DELETE")
        parser.add_argument("--data", action="append", default=[], help="表单字段 NAME=VALUE，可重复")
        parser.add_argument("--data-json", help="JSON 请求体；可用 @FILE 或 -")
        parser.add_argument("--file", action="append", default=[], help="multipart 文件字段 NAME=PATH，可重复")
        parser.add_argument("--yes", action="store_true", help="确认执行可能改变远端状态的请求")
        parser.add_argument("--referer", default="", help="同源 Referer")
    else:
        parser.set_defaults(method="GET", data=[], data_json=None, file=[], yes=False, referer="")
    parser.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    parser.add_argument("--output", help="原样保存响应")
    parser.add_argument("--raw", action="store_true", help="附带原始正文")
    parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")


def register(subparsers: argparse._SubParsersAction) -> None:
    quality = subparsers.add_parser("quality", aliases=["quality-assurance", "assurance"], help="通过 VPN 统一认证访问教学质量保障系统")
    children = quality.add_subparsers(dest="quality_command", required=True)

    catalog = children.add_parser("catalog", help="列出教学质量保障系统网页目录")
    catalog.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    catalog.set_defaults(feature_runner=run_catalog, feature_renderer=render)

    login = children.add_parser("login", help="登录教学质量保障系统")
    login.add_argument("--username", help="账号；密码从 CSUST_PASSWORD/.env 读取")
    login.add_argument("--captcha", help="显式指定验证码，仅用于测试或应急")
    login.add_argument("--captcha-image", help="验证码图片保存路径")
    login.add_argument("--vpn-captcha-info", help="VPN 统一认证触发验证码时传入 JSON 对象")
    login.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    login.set_defaults(feature_runner=run_login, feature_renderer=render)

    status = children.add_parser("status", help="检查教学质量保障系统会话")
    status.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    status.set_defaults(feature_runner=run_status, feature_renderer=render)

    logout = children.add_parser("logout", help="退出教学质量保障系统")
    logout.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    logout.set_defaults(feature_runner=run_logout, feature_renderer=render)

    routes = children.add_parser("routes", help="读取登录后的教学质量保障系统菜单")
    routes.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    routes.set_defaults(feature_runner=run_routes, feature_renderer=render)

    graduation = children.add_parser("graduation-design", help="打开条件显示的毕业设计外部 SSO 入口")
    graduation.add_argument("--fetch", action="store_true", help="跟随 SSO 入口并返回目标页面")
    graduation.add_argument("--output", help="与 --fetch 一起保存目标响应")
    graduation.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    graduation.set_defaults(feature_runner=web._run_graduation_design, feature_renderer=web.render)

    public = children.add_parser("public", help="访问登录、找回密码和验证码公开页面")
    public_children = public.add_subparsers(dest="public_command", required=True)
    public_get = public_children.add_parser("get", help="GET 公开页面")
    public_get.add_argument("--path", required=True, choices=[item[2] for item in web.PUBLIC_CATALOG])
    public_get.add_argument("--param", action="append", default=[])
    public_get.add_argument("--output")
    public_get.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    public_get.set_defaults(feature_runner=web._run_public_get, feature_renderer=web.render)
    public_post = public_children.add_parser("post", help="POST 找回密码等公开表单")
    public_post.add_argument("--path", required=True, choices=["/Logon.do"])
    public_post.add_argument("--param", action="append", default=[])
    public_post.add_argument("--data", action="append", default=[])
    public_post.add_argument("--yes", action="store_true")
    public_post.add_argument("--output")
    public_post.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    public_post.set_defaults(feature_runner=web._run_public_post, feature_renderer=web.render)
    public_action = public_children.add_parser("action", help="按序号执行找回密码页面动作")
    public_action.add_argument("--path", required=True, choices=["/findmm.jsp", "/Logon.do"])
    public_action.add_argument("--index", required=True, type=int)
    public_action.add_argument("--param", action="append", default=[])
    public_action.add_argument("--data", action="append", default=[])
    public_action.add_argument("--yes", action="store_true")
    public_action.add_argument("--output")
    public_action.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    public_action.set_defaults(feature_runner=web._run_public_action, feature_renderer=web.render)

    form = children.add_parser("form", help="按页面表单序号提交教学质量保障页面")
    form.add_argument("--path", required=True, help="教学质量保障系统页面路径")
    form.add_argument("--form", required=True, type=int, help="页面表单序号")
    form.add_argument("--button", type=int, help="表单提交按钮序号")
    form.add_argument("--param", action="append", default=[], help="初始查询参数 NAME=VALUE，可重复")
    form.add_argument("--data", action="append", default=[], help="覆盖/附加字段 NAME=VALUE，可重复")
    form.add_argument("--yes", action="store_true", help="确认执行可能改变远端状态的提交")
    form.add_argument("--output", help="保存响应文件")
    form.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    form.set_defaults(feature_runner=run_form, feature_renderer=web.render)

    action = children.add_parser("action", aliases=["run"], help="按页面动作序号执行链接、脚本或表单动作")
    action.add_argument("--path", required=True, help="教学质量保障系统页面路径")
    action.add_argument("--index", required=True, type=int, help="quality get 输出的动作序号")
    action.add_argument("--param", action="append", default=[], help="初始查询参数 NAME=VALUE，可重复")
    action.add_argument("--data", action="append", default=[], help="覆盖/附加字段 NAME=VALUE，可重复")
    action.add_argument("--yes", action="store_true", help="确认执行可能改变远端状态的动作")
    action.add_argument("--output", help="保存响应文件")
    action.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    action.set_defaults(feature_runner=web._run_action, feature_renderer=web.render)

    request = children.add_parser("request", aliases=["api", "page"], help="调用任意教学质量保障页面或接口")
    _add_request_args(request)
    request.set_defaults(feature_runner=_run_request, feature_renderer=render)

    get = children.add_parser("get", help="GET 页面并输出结构化内容")
    _add_request_args(get, get_only=True)
    get.set_defaults(feature_runner=_run_request, feature_renderer=render)

    evaluation = children.add_parser("evaluation", aliases=["evaluate"], help="查询或提交学生评价")
    evaluation_children = evaluation.add_subparsers(dest="evaluation_command", required=True)
    batches = evaluation_children.add_parser("batches", help="列出评价批次")
    batches.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    batches.set_defaults(feature_runner=academic._run_evaluation, feature_renderer=academic.render)
    for name, help_text in (("courses", "列出批次中的评价课程"), ("form", "查询评价表"), ("save", "保存评价"), ("submit", "提交评价")):
        child = evaluation_children.add_parser(name, help=help_text)
        child.add_argument("--path", required=True, help="页面路径或 batches 输出的 path")
        if name in {"save", "submit"}:
            child.add_argument("--answer", action="append", default=[], help="答案，格式 QUESTION=OPTION，可重复")
            child.add_argument("--suggestion", default="", help="学生建议")
            child.add_argument("--yes", action="store_true", help="确认修改远端评价")
        child.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
        child.set_defaults(feature_runner=academic._run_evaluation, feature_renderer=academic.render)


def render(data: dict[str, object]) -> None:
    if "routes" in data and "route_count" in data:
        print(f"{QUALITY_SYSTEM_NAME}：页面 {data.get('route_count')}；服务 {data.get('service')}")
        return
    print(json.dumps(data, ensure_ascii=False))
