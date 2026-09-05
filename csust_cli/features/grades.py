"""Grade query feature."""

from __future__ import annotations

import argparse
import re

from ..core import DEFAULT_BASE_URL, Client, ParseError, _safe_terminal_text, _safe_url, _safe_urljoin, _save_cookie_refresh, _table_rows, ensure_session, internal_url, parse_html, require_logged_in


GRADE_FIELDS = (
    "semester",
    "course_id",
    "course",
    "group",
    "score",
    "study_mode",
    "grade_id",
    "credit",
    "hours",
    "grade_point",
    "retake_semester",
    "assessment_method",
    "exam_nature",
    "course_attribute",
    "course_nature",
    "course_category",
)
GRADE_FIELDS_CURRENT = (
    "semester",
    "course_id",
    "course",
    "group",
    "score",
    "score_mark",
    "credit",
    "hours",
    "grade_point",
    "general_elective",
    "original_score",
    "description",
    "note",
    "retake_semester",
    "assessment_method",
    "exam_type",
    "course_attribute",
    "course_nature",
    "course_category",
)


def parse_grades(source: str, base_url: str = DEFAULT_BASE_URL) -> list[dict[str, object]]:
    document = parse_html(source)
    table = document.first("table", element_id="dataList")
    if table is None:
        if "未查询到数据" in document.text(include_scripts=False):
            return []
        raise ParseError("未找到成绩表")
    result: list[dict[str, object]] = []
    for row in _table_rows(table):
        cells = row.direct("td")
        if len(cells) < 6:
            continue
        values = [cell.text(include_scripts=False) for cell in cells]
        if values[0].strip() in {"序号", "序"}:
            continue
        if not any(values):
            continue
        fields: dict[str, object] = {"cells": values}
        if len(values) >= 20:
            for index, name in enumerate(GRADE_FIELDS_CURRENT, start=1):
                if index < len(values):
                    fields[name] = values[index]
        elif len(values) >= 17:
            for index, name in enumerate(GRADE_FIELDS, start=1):
                if index < len(values):
                    fields[name] = values[index]
        else:
            fields.update({"course": values[0], "score": values[-1]})
        anchor = cells[5].first("a") if len(cells) > 5 else None
        if anchor is not None:
            href = anchor.attr("href")
            if href.lower().startswith("javascript:"):
                quoted = re.search(r"['\"]([^'\"]+)['\"]", href)
                href = quoted.group(1) if quoted else ""
            if href:
                detail_url = _safe_urljoin(base_url, href)
                if detail_url:
                    fields["grade_detail_url"] = detail_url
        result.append(fields)
    return result


def parse_grade_detail(source: str, page_url: str) -> dict[str, object]:
    document = parse_html(source)
    table = document.first("table", element_id="dataList")
    if table is None:
        raise ParseError("未找到成绩详情表")
    rows = _table_rows(table)
    if len(rows) < 2:
        raise ParseError("成绩详情表行数不足")
    header_cells = rows[0].direct("th") or rows[0].direct("td")
    value_cells = rows[1].direct("td")
    headers = [cell.text(include_scripts=False) for cell in header_cells]
    values = [cell.text(include_scripts=False) for cell in value_cells]
    fields = {header: values[index] for index, header in enumerate(headers) if header and index < len(values)}
    return {"url": _safe_url(page_url), "fields": fields, "headers": headers, "cells": values, "text": rows[1].text(include_scripts=False)}


def register(subparsers: argparse._SubParsersAction) -> None:
    parser = subparsers.add_parser("grades", aliases=["scores"], help="查询成绩")
    parser.add_argument("--term", help="学期，例如 2025-2026-1；省略则查询全部")
    parser.add_argument("--course-nature", default="", help="课程性质 ID")
    parser.add_argument("--course-name", default="", help="课程名称关键词")
    parser.add_argument("--display", choices=("all", "best"), default="all", help="成绩显示：全部或最好成绩")
    parser.add_argument("--study-mode-id", default="2", help="修读方式 ID，默认主修")
    parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    parser.set_defaults(feature_runner=run, feature_renderer=render)
    children = parser.add_subparsers(dest="grades_command")
    detail = children.add_parser("detail", help="查询成绩组成详情")
    detail.add_argument("--path", required=True, help="成绩查询返回的详情路径")
    detail.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    detail.set_defaults(feature_runner=run_detail, feature_renderer=render)


def run(args: argparse.Namespace, client: Client) -> dict[str, object]:
    ensure_session(client)
    response = client.post(
        "/jsxsd/kscj/cjcx_list",
        {
            "kksj": args.term or "",
            "kcxz": args.course_nature,
            "kcmc": args.course_name,
            "xsfs": "max" if args.display == "best" else "all",
            "fxkc": args.study_mode_id,
        },
        headers={"Referer": client.url("/jsxsd/kscj/cjcx_query")},
    )
    require_logged_in(response)
    _save_cookie_refresh(client, response)
    return {"term": args.term, "items": parse_grades(response.body, response.url)}


def run_detail(args: argparse.Namespace, client: Client) -> dict[str, object]:
    target = internal_url(client, args.path)
    ensure_session(client)
    response = client.get(target)
    require_logged_in(response)
    _save_cookie_refresh(client, response)
    return parse_grade_detail(response.body, response.url)


def render(data: dict[str, object]) -> None:
    if "fields" in data and "items" not in data:
        for name, value in data.get("fields", {}).items():
            print(f"{_safe_terminal_text(name)}: {_safe_terminal_text(value)}")
        return
    for item in data.get("items", []):
        print("\t".join(_safe_terminal_text(item.get(key)) for key in ("semester", "course", "score", "credit", "grade_point")))
