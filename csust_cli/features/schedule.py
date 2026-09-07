"""Schedule query feature."""

from __future__ import annotations

import argparse
import re

from ..core import CsustError, DAY_NAMES, Client, Element, ParseError, _option_value, _safe_terminal_text, _table_rows, ensure_session, parse_html
from .academic import _query_response, _table


def parse_ints(value: str) -> list[int]:
    return [int(item) for item in re.findall(r"\d+", value)]


def expand_weeks(expression: str, parity: str) -> list[int]:
    weeks: list[int] = []
    for part in expression.replace("，", ",").split(","):
        part = part.strip()
        if not part:
            continue
        bounds = parse_ints(part)
        if len(bounds) == 1:
            weeks.append(bounds[0])
        elif len(bounds) >= 2:
            weeks.extend(range(bounds[0], bounds[1] + 1))
    if parity == "单周":
        weeks = [week for week in weeks if week % 2]
    elif parity == "双周":
        weeks = [week for week in weeks if week % 2 == 0]
    return sorted(set(weeks))


def parse_week_spec(value: str, fallback_sections: str = "") -> tuple[list[int], list[int]]:
    match = re.search(r"(.+?)[(（](单周|双周|周)[)）](?:\[([^]]*)\])?", value.replace(" ", ""))
    if not match:
        return [], parse_ints(fallback_sections)
    weeks = expand_weeks(match.group(1), match.group(2))
    sections = parse_ints(match.group(3) or fallback_sections)
    return weeks, sections


def parse_schedule_cell(cell: Element, fallback_sections: str) -> list[dict[str, object]]:
    contents = [
        node
        for node in cell.find_all("div")
        if (node.has_class("kbcontent") or node.has_class("kbcontent1")) and node.text(include_scripts=False)
    ]
    items: list[dict[str, object]] = []
    for content in contents:
        for fragment in re.split(r"-{5,}", content.to_html()):
            fragment_doc = parse_html(fragment)
            if not fragment_doc.text(include_scripts=False):
                continue
            lines = [line.strip() for line in fragment_doc.raw_text(include_scripts=False).splitlines() if line.strip() and line.strip() != "\xa0"]
            if not lines:
                continue
            teacher_node = next((node for node in fragment_doc.find_all("font") if node.attr("title") == "老师"), None)
            week_node = next((node for node in fragment_doc.find_all("font") if node.attr("title") == "周次(节次)"), None)
            room_node = next((node for node in fragment_doc.find_all("font") if node.attr("title") == "教室"), None)
            weeks, sections = parse_week_spec(week_node.text(include_scripts=False) if week_node else "", fallback_sections)
            if not weeks:
                continue
            items.append(
                {
                    "course": lines[0],
                    "teacher": teacher_node.text(include_scripts=False) if teacher_node else "",
                    "room": room_node.text(include_scripts=False) if room_node else "",
                    "weeks": weeks,
                    "sections": sections,
                }
            )
    return items


def parse_schedule(source: str, term: str | None = None) -> list[dict[str, object]]:
    document = parse_html(source)
    table = _table(document, "kbtable")
    if table is None:
        if "未查询到数据" in document.text(include_scripts=False):
            return []
        raise ParseError("未找到课表")
    rows = _table_rows(table)
    day_names = list(DAY_NAMES)
    if rows:
        header_names = [cell.text(include_scripts=False) for cell in rows[0].find_all("th")]
        found = [name for name in header_names if name in DAY_NAMES]
        if len(found) == 7:
            day_names = found
    result: list[dict[str, object]] = []
    seen: set[tuple[object, ...]] = set()
    for row in rows[1:]:
        cells = row.direct("td")
        if not cells:
            continue
        header_cells = row.direct("th")
        section_text = header_cells[0].text(include_scripts=False) if header_cells else ""
        if len(cells) == len(day_names) + 1 and not cells[0].find_all("div"):
            cells = cells[1:]
        for day_index, cell in enumerate(cells[: len(day_names)]):
            for item in parse_schedule_cell(cell, section_text):
                record = {"term": term, "weekday": day_names[day_index], **item}
                key = (
                    record["course"],
                    record["teacher"],
                    record["room"],
                    record["weekday"],
                    tuple(record["weeks"]),
                    tuple(record["sections"]),
                )
                if key not in seen:
                    seen.add(key)
                    result.append(record)
    return result


def selected_option(document: Element, element_id: str) -> str | None:
    select = document.first("select", element_id=element_id)
    if select is None:
        select = next((item for item in document.find_all("select") if item.attr("name") == element_id), None)
    if select is None or select.is_disabled():
        return None
    options = [item for item in select.find_all("option") if not item.is_disabled()]
    option = next((item for item in options if "selected" in item.attrs), None)
    if option is None and "multiple" not in select.attrs and options:
        option = options[0]
    if option is None:
        return None
    return _option_value(option).strip() or None


def register(subparsers: argparse._SubParsersAction) -> None:
    parser = subparsers.add_parser("schedule", aliases=["timetable"], help="查询课表")
    parser.add_argument("--term", help="学期，例如 2025-2026-1")
    parser.add_argument("--week", type=int, help="只显示指定周")
    parser.add_argument("--scheme-id", default="", help="课表时间模式 ID；通常留空自动使用系统默认")
    parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    parser.set_defaults(feature_runner=run, feature_renderer=render)


def run(args: argparse.Namespace, client: Client) -> dict[str, object]:
    if args.week is not None and args.week < 1:
        raise CsustError("--week 必须是正整数", code="invalid_argument")
    ensure_session(client)
    response = _query_response(
        client,
        "POST",
        "/jsxsd/xskb/xskb_list.do",
        [
            ("jx0404id", ""),
            ("cj0701id", ""),
            ("zc", str(args.week) if args.week else ""),
            ("demo", ""),
            ("xnxq01id", args.term or ""),
            ("sfFD", "1"),
            ("kbjcmsid", args.scheme_id),
        ],
    )
    document = parse_html(response.body)
    resolved_term = args.term or selected_option(document, "xnxq01id")
    items = parse_schedule(response.body, resolved_term)
    if args.week:
        items = [item for item in items if args.week in item["weeks"]]
    return {"term": resolved_term, "week": args.week, "items": items}


def render(data: dict[str, object]) -> None:
    for item in data.get("items", []):
        weeks = ",".join(str(value) for value in item["weeks"])
        sections = "-".join(str(value) for value in item["sections"])
        details = " ".join(value for value in (_safe_terminal_text(item.get("teacher")), _safe_terminal_text(item.get("room"))) if value)
        print(f"{_safe_terminal_text(item.get('weekday'))} 第{sections}节 第{weeks}周 {_safe_terminal_text(item.get('course'))} {details}".rstrip())
