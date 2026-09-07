"""Command-line parser and static feature registry."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import sys
import time

from .features import academic, grades, quality, schedule, site, teaching, textbooks, vpn, web
from .core import Client, CsustError, _safe_terminal_text, login, result_status


class CliArgumentParser(argparse.ArgumentParser):
    def error(self, message: str) -> None:
        raise CsustError(message, code="invalid_argument")


def _render_login(data: dict[str, object]) -> None:
    print(f"登录成功：{_safe_terminal_text(data.get('username'))}")
    print(f"会话已保存：{_safe_terminal_text(data.get('cookie_file'))}")


def _run_login(args: argparse.Namespace, client: Client) -> dict[str, object]:
    return login(args, client)


def _run_logout(_args: argparse.Namespace, client: Client) -> dict[str, object]:
    result: dict[str, object] | None = None
    primary: BaseException | None = None
    cleanup_error: BaseException | None = None
    try:
        response = client.get(f"/jsxsd/xk/LoginToXk?method=exit&tktime={int(time.time() * 1000)}")
        result = web._logout_result(response)
    except BaseException as exc:
        primary = exc
    finally:
        try:
            client.clear_cookies()
        except BaseException as exc:
            cleanup_error = exc
        try:
            client.save()
        except BaseException as exc:
            cleanup_error = cleanup_error or exc
    if primary is not None:
        raise primary
    if cleanup_error is not None:
        raise cleanup_error
    assert result is not None
    return {**result, "cookie_file": str(client.cookie_file)}


def build_parser() -> argparse.ArgumentParser:
    parser = CliArgumentParser(prog="csust", description="长沙理工大学教务系统 CLI")
    parser.add_argument("--json", action="store_true", help="输出 JSON")
    subparsers = parser.add_subparsers(dest="command", required=True)

    login_parser = subparsers.add_parser("login", help="登录并保存会话")
    login_parser.add_argument("--username", help="账号；密码从 CSUST_PASSWORD 读取")
    login_parser.add_argument("--auth", choices=("auto", "sso", "local"), default="auto", help="认证方式；auto 在标准教务地址优先使用统一认证")
    login_parser.add_argument("--captcha", help="显式指定验证码，仅用于测试或应急")
    login_parser.add_argument("--captcha-image", help="验证码图片保存路径")
    login_parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    login_parser.set_defaults(command_runner=_run_login, command_renderer=_render_login)

    logout_parser = subparsers.add_parser("logout", help="退出教务系统并清除本机会话")
    logout_parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    logout_parser.set_defaults(command_runner=_run_logout, command_renderer=lambda data: print("已退出登录"))

    schedule.register(subparsers)
    grades.register(subparsers)
    textbooks.register(subparsers)
    academic.register(subparsers)
    web.register(subparsers)
    vpn.register(subparsers)
    teaching.register(subparsers)
    quality.register(subparsers)
    site.register(subparsers)
    return parser


def _error_payload(exc: CsustError) -> dict[str, object]:
    payload: dict[str, object] = {"error": str(exc), "code": exc.code}
    if exc.details:
        payload["details"] = exc.details
    return payload


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    argv_list = sys.argv[1:] if argv is None else argv
    json_mode = "--json" in argv_list
    try:
        args = parser.parse_args(argv_list)
        json_mode = bool(getattr(args, "json", False))
        output = getattr(args, "output", None)
        if output is not None:
            if not output.strip():
                raise CsustError("下载路径不能为空", code="invalid_argument")
            try:
                target = Path(output).expanduser()
                if target.is_symlink() or (target.exists() and not target.is_file()):
                    raise ValueError("输出路径必须是普通文件")
            except (OSError, RuntimeError, ValueError) as exc:
                raise CsustError("下载路径无效", code="invalid_argument") from exc
        teaching_command = args.command in {"teaching", "theol"}
        static_catalog = (args.command == "web" and args.web_command == "catalog") or (
            args.command == "vpn" and args.vpn_command in {"routes", "controls", "catalog", "page"}
        ) or (teaching_command and args.teaching_command == "catalog") or (
            args.command in {"quality", "quality-assurance", "assurance"} and args.quality_command == "catalog"
        ) or (args.command in {"site", "domain", "portal"} and args.site_command == "catalog")
        if args.command == "vpn":
            from .features.vpn import VpnClient

            client = None if static_catalog else VpnClient(
                native=bool(getattr(args, "native", False)),
                load_cookies=args.vpn_command != "login",
                load_session=args.vpn_command != "login",
            )
        elif teaching_command:
            from .features.teaching import TeachingClient

            client = None if static_catalog else TeachingClient()
        elif args.command in {"quality", "quality-assurance", "assurance"}:
            from .features.quality import QualityClient

            client = None if static_catalog else QualityClient()
        elif args.command in {"site", "domain", "portal"}:
            client = None
        else:
            client = None if static_catalog else Client(load_cookies=args.command != "login")
        runner = getattr(args, "command_runner", None) or getattr(args, "feature_runner")
        data = runner(args, client)
        if isinstance(data, dict):
            mutating = data.get("submitted") is True
            if data.get("ok") is False and not data.get("pending") and not data.get("next"):
                raise CsustError("远端请求失败", code="mutation_rejected" if mutating else "business_rejected")
            if mutating and data.get("confirmed") is not True:
                result_status(None, mutating=True, details={key: data[key] for key in ("request", "downloaded", "output", "bytes") if key in data})
            for key in ("page", "response"):
                if key in data:
                    result_status(data[key], mutating=False)
        if json_mode:
            print(json.dumps(data, ensure_ascii=False))
        else:
            renderer = getattr(args, "command_renderer", None) or getattr(args, "feature_renderer")
            renderer(data)
        return 0
    except CsustError as exc:
        if json_mode:
            print(json.dumps(_error_payload(exc), ensure_ascii=False))
        else:
            print(f"错误: {_safe_terminal_text(exc)}", file=sys.stderr)
        return 2
    except (RuntimeError, ValueError, TypeError) as exc:
        payload = {"error": "参数或地址格式无效", "code": "invalid_argument"}
        if json_mode:
            print(json.dumps(payload, ensure_ascii=False))
        else:
            print(f"错误: {payload['error']}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
