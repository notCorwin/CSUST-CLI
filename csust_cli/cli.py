"""Command-line parser and static feature registry."""

from __future__ import annotations

import argparse
import json
import sys
import time

from .features import academic, grades, quality, schedule, teaching, textbooks, vpn, web
from .core import Client, CsustError, _safe_terminal_text, login


class CliArgumentParser(argparse.ArgumentParser):
    def error(self, message: str) -> None:
        raise CsustError(message, code="invalid_argument")


def _render_login(data: dict[str, object]) -> None:
    print(f"登录成功：{_safe_terminal_text(data.get('username'))}")
    print(f"会话已保存：{_safe_terminal_text(data.get('cookie_file'))}")


def _run_login(args: argparse.Namespace, client: Client) -> dict[str, object]:
    return login(args, client)


def _run_logout(_args: argparse.Namespace, client: Client) -> dict[str, object]:
    try:
        client.get(f"/jsxsd/xk/LoginToXk?method=exit&tktime={int(time.time() * 1000)}")
    finally:
        client.clear_cookies()
        client.save()
    return {"ok": True, "logged_out": True, "cookie_file": str(client.cookie_file)}


def build_parser() -> argparse.ArgumentParser:
    parser = CliArgumentParser(prog="csust", description="长沙理工大学教务系统 CLI")
    parser.add_argument("--json", action="store_true", help="输出 JSON")
    subparsers = parser.add_subparsers(dest="command", required=True)

    login_parser = subparsers.add_parser("login", help="登录并保存会话")
    login_parser.add_argument("--username", help="账号；密码从 CSUST_PASSWORD 读取")
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
        static_catalog = (args.command == "web" and args.web_command == "catalog") or (
            args.command == "vpn" and args.vpn_command in {"routes", "controls", "catalog", "page"}
        ) or (args.command == "teaching" and args.teaching_command == "catalog") or (
            args.command in {"quality", "quality-assurance", "assurance"} and args.quality_command == "catalog"
        )
        if args.command == "vpn":
            from .features.vpn import VpnClient

            client = None if static_catalog else VpnClient(native=bool(getattr(args, "native", False)))
        elif args.command == "teaching":
            from .features.teaching import TeachingClient

            client = None if static_catalog else TeachingClient()
        elif args.command in {"quality", "quality-assurance", "assurance"}:
            from .features.quality import QualityClient

            client = None if static_catalog else QualityClient()
        else:
            client = None if static_catalog else Client(load_cookies=args.command != "login")
        runner = getattr(args, "command_runner", None) or getattr(args, "feature_runner")
        data = runner(args, client)
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


if __name__ == "__main__":
    raise SystemExit(main())
