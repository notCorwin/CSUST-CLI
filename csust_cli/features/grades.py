"""Grade query feature."""

from __future__ import annotations

import argparse
import re

from ..core import DEFAULT_BASE_URL, Client, ParseError, _safe_terminal_text, _safe_url, _safe_urljoin, _table_rows, ensure_session, internal_url, parse_html
from .academic import _query_response


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


def _grade_header(value: str) -> str:
    value = re.sub(r"\s+", "", value)
    if re.search(r"学期", value):
        return "semester"
    if re.search(r"课程(?:代码|编号)|课号", value):
        return "course_id"
    if value in {"课程", "课程名称", "科目", "科目名称"} or re.search(r"课程名称|科目名称", value):
        return "course"
    if re.search(r"(?:总评)?成绩|分数|得分", value):
        return "score"
    if re.search(r"修读方式|学习方式", value):
        return "study_mode"
    if re.search(r"学分", value):
        return "credit"
    if re.search(r"学时", value):
        return "hours"
    if re.search(r"绩点|学分绩点", value):
        return "grade_point"
    if re.search(r"课程性质", value):
        return "course_nature"
    if re.search(r"课程属性", value):
        return "course_attribute"
    if re.search(r"课程类别", value):
        return "course_category"
    if re.search(r"考核方式|考试方式", value):
        return "assessment_method"
    if re.search(r"重修学期|补考学期", value):
        return "retake_semester"
    return ""


def parse_grades(source: str, base_url: str = DEFAULT_BASE_URL) -> list[dict[str, object]]:
    document = parse_html(source)
    table = document.first("table", element_id="dataList")
    if table is None:
        if "未查询到数据" in document.text(include_scripts=False):
            return []
        raise ParseError("未找到成绩表")
    rows = _table_rows(table)
    header_row = rows[0] if rows else None
    header_cells = header_row.direct("th") if header_row is not None else []
    if not header_cells and header_row is not None:
        header_cells = header_row.direct("td")
    header_values = [cell.text(include_scripts=False).strip() for cell in header_cells]
    has_header = bool(header_row and (header_row.direct("th") or header_values and (header_values[0] in {"序号", "序"} or sum(bool(_grade_header(value)) for value in header_values) >= 2)))
    result: list[dict[str, object]] = []
    for row in rows:
        if row is header_row and has_header:
            continue
        cells = row.direct("td")
        if len(cells) < 6:
            continue
        values = [cell.text(include_scripts=False) for cell in cells]
        if values[0].strip() in {"序号", "序"}:
            continue
        if not any(values):
            continue
        fields: dict[str, object] = {"cells": values}
        if has_header:
            for index, header in enumerate(header_values):
                field = _grade_header(header)
                if field and index < len(values):
                    fields[field] = values[index]
        elif len(values) >= 20:
            for index, name in enumerate(GRADE_FIELDS_CURRENT, start=1):
                if index < len(values):
                    fields[name] = values[index]
        elif len(values) >= 17:
            for index, name in enumerate(GRADE_FIELDS, start=1):
                if index < len(values):
                    fields[name] = values[index]
        else:
            fields.update({"course": values[0], "score": values[-1]})
        anchor = next((cell.first("a") for cell in cells if cell.first("a") is not None), None)
        if anchor is not None:
            href = anchor.attr("href")
            if href.lower().startswith("javascript:"):
                quoted = re.search(r"['\"]([^'\"]+)['\"]", href)
                href = quoted.group(1) if quoted else ""
            if href:
                detail_url = _safe_url(_safe_urljoin(base_url, href), base_url)
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
    response = _query_response(
        client,
        "POST",
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
    return {"term": args.term, "items": parse_grades(response.body, response.url)}


def run_detail(args: argparse.Namespace, client: Client) -> dict[str, object]:
    target = internal_url(client, args.path)
    ensure_session(client)
    response = _query_response(client, "GET", target)
    return parse_grade_detail(response.body, response.url)


def render(data: dict[str, object]) -> None:
    if "fields" in data and "items" not in data:
        for name, value in data.get("fields", {}).items():
            print(f"{_safe_terminal_text(name)}: {_safe_terminal_text(value)}")
        return
    for item in data.get("items", []):
        print("\t".join(_safe_terminal_text(item.get(key)) for key in ("semester", "course", "score", "credit", "grade_point")))
