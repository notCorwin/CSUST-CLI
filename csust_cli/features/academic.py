"""Static wrappers for the remaining student-facing academic pages."""

from __future__ import annotations

import argparse
import re

from ..core import (
    Client,
    CsustError,
    Element,
    MutationUnverified,
    ParseError,
    Response,
    _SENSITIVE_FIELD,
    _has_failure_signal,
    _option_value,
    _safe_event,
    _safe_terminal_text,
    _safe_url,
    _save_cookie_refresh,
    _table_rows,
    ensure_session,
    internal_url,
    parse_html,
    require_logged_in,
    same_origin_url,
    _safe_urljoin,
)


def _table(document: Element, *ids: str) -> Element | None:
    for element_id in ids:
        table = document.first("table", element_id=element_id)
        if table is not None:
            return table
    tables = document.find_all("table")
    return next((table for table in tables if table.find_all("tr")), None)


def _rows(table: Element | None) -> list[tuple[Element, list[Element]]]:
    if table is None:
        return []
    result: list[tuple[Element, list[Element]]] = []
    for row in _table_rows(table):
        cells = row.direct("td")
        if cells and any(cell.text(include_scripts=False) for cell in cells):
            result.append((row, cells))
    return result


def _options(document: Element, element_id: str) -> list[dict[str, object]]:
    select = document.first("select", element_id=element_id)
    if select is None:
        select = next((item for item in document.find_all("select") if item.attr("name") == element_id), None)
    if select is None or select.is_disabled():
        return []
    return [
        {
            "value": _option_value(option),
            "label": option.text(include_scripts=False),
            "selected": "selected" in option.attrs,
            "disabled": option.is_disabled(),
        }
        for option in select.find_all("option")
        if option.text(include_scripts=False) or option.attr("value")
    ]


def _selected(options: list[dict[str, object]]) -> str | None:
    available = [item for item in options if not item.get("disabled")]
    option = next((item for item in available if item.get("selected")), available[0] if available else None)
    value = str(option.get("value") or "").strip() if option else ""
    return value or None


def parse_profile(source: str, page_url: str) -> dict[str, object]:
    document = parse_html(source)
    table = _table(document, "xjkpTable", "xjxxTable")
    if table is None:
        visible_text = document.text(include_scripts=False)
        if "未登录" in visible_text or "用户没有登录" in visible_text:
            raise CsustError("个人信息页面要求登录", code="login_required")
        raise ParseError("未找到个人信息表")
    rows: list[list[str]] = []
    fields: list[dict[str, str]] = []
    values: dict[str, str] = {}
    for row, cells in _rows(table):
        row_values = [cell.text(include_scripts=False) for cell in cells]
        rows.append(row_values)
        for value in row_values:
            match = re.match(r"^\s*([^：:]{1,40})\s*[：:]\s*(.*?)\s*$", value)
            if match:
                name, field_value = match.groups()
                fields.append({"name": name.strip(), "value": field_value.strip()})
                values[name.strip()] = field_value.strip()
        for index in range(0, len(row_values) - 1, 2):
            label = row_values[index].strip().rstrip("：:")
            if label and row_values[index + 1].strip() and len(label) <= 40 and not re.search(r"[：:]", label):
                fields.append({"name": label, "value": row_values[index + 1].strip()})
                values[label] = row_values[index + 1].strip()
    return {"url": _safe_url(page_url), "fields": fields, "values": values, "rows": rows}


def parse_exams(source: str, page_url: str) -> dict[str, object]:
    document = parse_html(source)
    table = _table(document, "dataList")
    if table is None:
        if "未查询到数据" in document.text(include_scripts=False):
            return {"url": _safe_url(page_url), "items": []}
        raise ParseError("未找到考试安排表")
    items: list[dict[str, object]] = []
    for row, cells in _rows(table):
        values = [cell.text(include_scripts=False) for cell in cells]
        if len(values) < 10 or values[0] in {"序号", "课程代码", "考试安排"}:
            continue
        item: dict[str, object] = {
            "index": len(items) + 1,
            "sequence": values[0],
            "campus": values[1] if len(values) > 1 else "",
            "session": values[2] if len(values) > 2 else "",
            "course_id": values[3] if len(values) > 3 else "",
            "course": values[4] if len(values) > 4 else "",
            "teacher": values[5] if len(values) > 5 else "",
            "exam_time": values[6] if len(values) > 6 else "",
            "room": values[7] if len(values) > 7 else "",
            "seat": values[8] if len(values) > 8 else "",
            "admission_ticket": values[9] if len(values) > 9 else "",
            "remarks": values[10] if len(values) > 10 else "",
            "cells": values,
            "text": row.text(include_scripts=False),
        }
        match = re.search(
            r"(\d{4}[-/]\d{1,2}[-/]\d{1,2})\s+(\d{1,2}:\d{2})\s*[~～-]\s*(\d{1,2}:\d{2})",
            str(item["exam_time"]),
        )
        if match:
            item.update({"date": match.group(1), "start_time": match.group(2), "end_time": match.group(3)})
        items.append(item)
    return {"url": _safe_url(page_url), "items": items}


def parse_classrooms(source: str, page_url: str) -> dict[str, object]:
    document = parse_html(source)
    table = _table(document, "kbtable")
    if table is None:
        if "未查询到数据" in document.text(include_scripts=False):
            return {"url": _safe_url(page_url), "items": []}
        raise ParseError("未找到空闲教室表")
    items: list[dict[str, object]] = []
    for row, cells in _rows(table):
        values = [cell.text(include_scripts=False) for cell in cells]
        if len(values) < 2 or values[0] in {"教室", "教学楼", "教室名称"}:
            continue
        occupied = any(value.strip() for value in values[1:])
        if not occupied:
            items.append({"index": len(items) + 1, "room": values[0], "available": True, "cells": values, "text": row.text(include_scripts=False)})
    return {"url": _safe_url(page_url), "items": items}


def parse_selection_results(source: str, page_url: str) -> dict[str, object]:
    document = parse_html(source)
    table = _table(document, "dataList")
    if table is None:
        if "未查询到数据" in document.text(include_scripts=False):
            return {"url": _safe_url(page_url), "items": []}
        raise ParseError("未找到选课结果表")
    items: list[dict[str, object]] = []
    for row, cells in _rows(table):
        values = [cell.text(include_scripts=False) for cell in cells]
        if len(values) < 8 or values[0] == "序号":
            continue
        items.append(
            {
                "index": len(items) + 1,
                "sequence": values[0],
                "course": values[1],
                "course_id": values[2],
                "teacher": values[3],
                "hours": values[4],
                "credit": values[5],
                "course_attribute": values[6],
                "course_nature": values[7],
                "cells": values,
                "text": row.text(include_scripts=False),
            }
        )
    return {"url": _safe_url(page_url), "items": items}


def parse_semester_start(source: str, page_url: str) -> dict[str, object]:
    document = parse_html(source)
    table = _table(document, "kbtable")
    if table is None:
        raise ParseError("未找到学期起始日表")
    rows = _rows(table)
    for row, cells in rows:
        values = [cell.text(include_scripts=False) for cell in cells]
        start_date = cells[1].attr("title") if len(cells) > 1 else ""
        if not start_date:
            match = re.search(r"\d{4}年\d{1,2}月\d{1,2}日?", " ".join(values))
            start_date = match.group(0) if match else ""
        if start_date:
            return {"url": _safe_url(page_url), "start_date": start_date, "cells": values, "text": row.text(include_scripts=False)}
    if not rows:
        raise ParseError("学期起始日表没有数据")
    raise ParseError("未找到学期起始日")


def _first_link(row: Element, page_url: str) -> str:
    link = row.first("a")
    if link is None:
        return ""
    href = link.attr("href")
    if href.lower().startswith("javascript:"):
        quoted = re.search(r"['\"]([^'\"]+)", href)
        href = quoted.group(1) if quoted else ""
    return _safe_urljoin(page_url, href) if href else ""


def parse_evaluation_batches(source: str, page_url: str) -> dict[str, object]:
    document = parse_html(source)
    items: list[dict[str, object]] = []
    for table in document.find_all("table"):
        for row, cells in _rows(table):
            values = [cell.text(include_scripts=False) for cell in cells]
            path = _first_link(row, page_url)
            if len(values) < 6 or not path or values[0] == "序号":
                continue
            items.append(
                {
                    "index": len(items) + 1,
                    "sequence": values[0],
                    "semester": values[1],
                    "category": values[2],
                    "name": values[3],
                    "start_date": values[4],
                    "end_date": values[5],
                    "path": path,
                    "cells": values,
                    "text": row.text(include_scripts=False),
                }
            )
    return {"url": _safe_url(page_url), "items": items}


def parse_evaluation_courses(source: str, page_url: str) -> dict[str, object]:
    document = parse_html(source)
    table = _table(document, "dataList")
    if table is None:
        return {"url": _safe_url(page_url), "items": []}
    items: list[dict[str, object]] = []
    for row, cells in _rows(table):
        values = [cell.text(include_scripts=False) for cell in cells]
        path = _first_link(row, page_url)
        if len(values) < 9 or not path or values[0] == "序号":
            continue
        items.append(
            {
                "index": len(items) + 1,
                "sequence": values[0],
                "course_id": values[1],
                "course": values[2],
                "teacher": values[3],
                "category": values[4],
                "total_score": values[5],
                "evaluated": values[6] == "是",
                "submitted": values[7] == "是",
                "hours": values[8],
                "path": path,
                "cells": values,
                "text": row.text(include_scripts=False),
            }
        )
    return {"url": _safe_url(page_url), "items": items}


def parse_evaluation_form(source: str, page_url: str, *, include_sensitive: bool = False) -> dict[str, object]:
    document = parse_html(source)
    form = next(
        (
            candidate
            for candidate in document.find_all("form")
            if "xspj_save.do" in candidate.attr("action")
        ),
        None,
    )
    if form is None:
        raise ParseError("未找到课程评价表")
    hidden = [
        {
            "name": node.attr("name"),
            "value": (
                node.attr("value")
                if include_sensitive
                else "" if _SENSITIVE_FIELD.search(node.attr("name")) else _safe_event(node.attr("value"))
            ),
        }
        for node in form.find_all("input")
        if node.attr("type", "text").lower() == "hidden" and node.attr("name") and not node.is_disabled()
    ]
    questions: list[dict[str, object]] = []
    for row in form.find_all("tr"):
        cells = row.direct("td")
        if len(cells) < 2:
            continue
        question = next(
            (node for node in cells[0].find_all("input") if node.attr("name") == "pj06xh" and not node.is_disabled()),
            None,
        )
        if question is None or not question.attr("value"):
            continue
        options = []
        for node in cells[1].find_all("input"):
            if node.attr("type").lower() != "radio" or not node.attr("value") or node.is_disabled():
                continue
            options.append({"id": node.attr("value"), "label": node.attr("value"), "selected": "checked" in node.attrs})
        if options:
            questions.append(
                {
                    "id": question.attr("value"),
                    "title": cells[0].text(include_scripts=False),
                    "options": options,
                }
            )
    textarea = next((node for node in form.find_all("textarea") if node.attr("name") and not node.is_disabled()), None)
    action = _safe_urljoin(page_url, form.attr("action") or page_url)
    read_only = not any("saveData" in node.attr("onclick") and not node.is_disabled() for node in form.find_all())
    return {
        "url": _safe_url(page_url),
        "action": action if include_sensitive else _safe_url(action, page_url),
        "method": form.attr("method", "post").upper(),
        "course": _heading_value(document, "课程名称"),
        "category": _heading_value(document, "评教大类"),
        "hidden_fields": hidden,
        "questions": questions,
        "suggestion_field": textarea.attr("name") if textarea else "",
        "suggestion": textarea.raw_text() if textarea else "",
        "read_only": read_only,
    }


def _heading_value(document: Element, label: str) -> str:
    text = document.text(include_scripts=False)
    match = re.search(rf"{re.escape(label)}\s*[：:]\s*([^\s]+)", text)
    return match.group(1) if match else ""


def _get_page(client: Client, path: str) -> tuple[Response, Element]:
    response = client.get(path)
    require_logged_in(response)
    _save_cookie_refresh(client, response)
    return response, parse_html(response.body)


def _run_profile(args: argparse.Namespace, client: Client) -> dict[str, object]:
    ensure_session(client)
    response, _ = _get_page(client, "/jsxsd/grxx/xsxx")
    return parse_profile(response.body, response.url)


def _run_exams(args: argparse.Namespace, client: Client) -> dict[str, object]:
    ensure_session(client)
    query, document = _get_page(client, "/jsxsd/xsks/xsksap_query")
    term = args.term or _selected(_options(document, "xnxqid")) or ""
    response = client.post(
        "/jsxsd/xsks/xsksap_list",
        {"xqlbmc": args.category or "", "xnxqid": term, "xqlb": args.category_id or ""},
        headers={"Referer": query.url},
    )
    require_logged_in(response)
    _save_cookie_refresh(client, response)
    data = parse_exams(response.body, response.url)
    data["term"] = term
    return data


def _run_classrooms(args: argparse.Namespace, client: Client) -> dict[str, object]:
    if args.week < 1:
        raise CsustError("--week 必须是正整数", code="invalid_argument")
    campus = str(args.campus).lower()
    campus_id = {"yuntang": "1", "云塘": "1", "1": "1", "jinpenling": "2", "金盆岭": "2", "2": "2"}.get(campus)
    if campus_id is None:
        raise CsustError("--campus 只能是 yuntang、jinpenling、1 或 2", code="invalid_argument")
    section = {1: ("01", "02"), 2: ("03", "04"), 3: ("05", "06"), 4: ("07", "08"), 5: ("09", "10")}.get(args.section)
    if section is None:
        raise CsustError("--section 必须在 1 到 5 之间", code="invalid_argument")
    ensure_session(client)
    response = client.post(
        "/jsxsd/kbcx/kbxx_classroom_ifr",
        {
            "xnxqh": args.term or "",
            "skyx": args.department or "",
            "xqid": campus_id,
            "jzwid": args.building or "",
            "gnq": args.area or "",
            "skjsid": "",
            "skjs": "",
            "zc1": str(args.week),
            "zc2": str(args.week),
            "skxq1": str(args.weekday),
            "skxq2": str(args.weekday),
            "jc1": section[0],
            "jc2": section[1],
        },
    )
    require_logged_in(response)
    _save_cookie_refresh(client, response)
    data = parse_classrooms(response.body, response.url)
    data.update({"campus": campus_id, "week": args.week, "weekday": args.weekday, "section": args.section})
    return data


def _run_selection(args: argparse.Namespace, client: Client) -> dict[str, object]:
    ensure_session(client)
    query, document = _get_page(client, "/jsxsd/xkgl/xsxkjgcx")
    term = args.term or _selected(_options(document, "xnxqid")) or ""
    response = client.post("/jsxsd/xkgl/loadXsxkjgList", {"xnxqid": term}, headers={"Referer": query.url})
    require_logged_in(response)
    _save_cookie_refresh(client, response)
    data = parse_selection_results(response.body, response.url)
    data["term"] = term
    return data


def _run_terms(args: argparse.Namespace, client: Client) -> dict[str, object]:
    ensure_session(client)
    paths = {
        "schedule": ("/jsxsd/xskb/xskb_list.do", "xnxq01id"),
        "grades": ("/jsxsd/kscj/cjcx_query", "kksj"),
        "exams": ("/jsxsd/xsks/xsksap_query", "xnxqid"),
        "selection": ("/jsxsd/xkgl/xsxkjgcx", "xnxqid"),
        "semester-start": ("/jsxsd/jxzl/jxzl_query", "xnxq01id"),
    }
    path, element_id = paths[args.scope]
    response, document = _get_page(client, path)
    options = _options(document, element_id)
    return {"scope": args.scope, "url": _safe_url(response.url), "terms": options, "selected": _selected(options)}


def _run_semester_start(args: argparse.Namespace, client: Client) -> dict[str, object]:
    ensure_session(client)
    query, document = _get_page(client, "/jsxsd/jxzl/jxzl_query")
    term = args.term or _selected(_options(document, "xnxq01id")) or ""
    response = client.post("/jsxsd/jxzl/jxzl_query", {"xnxq01id": term}, headers={"Referer": query.url})
    require_logged_in(response)
    _save_cookie_refresh(client, response)
    data = parse_semester_start(response.body, response.url)
    data["term"] = term
    return data


def _run_course_selection_center(args: argparse.Namespace, client: Client) -> dict[str, object]:
    from .web import inspect_page

    ensure_session(client)
    response, _ = _get_page(client, "/jsxsd/xsxk/xklc_list")
    return inspect_page(response.body, response.url)


def _run_classroom_request(args: argparse.Namespace, client: Client) -> dict[str, object]:
    from .web import inspect_page

    ensure_session(client)
    response, _ = _get_page(client, "/jsxsd/kbxx/jsjy_query")
    return inspect_page(response.body, response.url)


def _run_minor(args: argparse.Namespace, client: Client) -> dict[str, object]:
    from .web import inspect_page

    ensure_session(client)
    response, _ = _get_page(client, "/jsxsd/fxgl/fxbmxx_query")
    return inspect_page(response.body, response.url)


def _ensure_evaluation_session(client: Client) -> None:
    quality_session = getattr(client, "ensure_quality_session", None)
    if callable(quality_session):
        quality_session()
    else:
        ensure_session(client)


def _evaluation_target(client: Client, path: str) -> str:
    quality_url = getattr(client, "web_url", None)
    return quality_url(path) if callable(quality_url) else internal_url(client, path)


def _evaluation_page(client: Client, path: str) -> Response:
    target = _evaluation_target(client, path)
    quality_request = getattr(client, "request_web", None)
    if callable(quality_request):
        response = quality_request(path)
    else:
        response = client.get(target)
        require_logged_in(response)
        _save_cookie_refresh(client, response)
    return response


def _run_evaluation(args: argparse.Namespace, client: Client) -> dict[str, object]:
    command = args.evaluation_command
    if command in {"save", "submit"} and not args.yes:
        raise CsustError("保存或提交评价会修改账号数据，请加 --yes", code="confirmation_required")
    if command != "batches" and not args.path:
        raise CsustError("该评价命令必须提供 --path", code="invalid_argument")
    answers: dict[str, str] = {}
    if command in {"save", "submit"}:
        for answer in args.answer:
            if "=" not in answer:
                raise CsustError("--answer 格式必须为 QUESTION=OPTION", code="invalid_argument")
            question_id, option_id = answer.split("=", 1)
            answers[question_id] = option_id
    if command != "batches":
        _evaluation_target(client, args.path)
    _ensure_evaluation_session(client)
    if command == "batches":
        response = _evaluation_page(client, "/jsxsd/xspj/xspj_find.do")
        return parse_evaluation_batches(response.body, response.url)
    if command == "courses":
        response = _evaluation_page(client, args.path)
        return parse_evaluation_courses(response.body, response.url)
    if command == "form":
        response = _evaluation_page(client, args.path)
        return parse_evaluation_form(response.body, response.url)
    response = _evaluation_page(client, args.path)
    form = parse_evaluation_form(response.body, response.url, include_sensitive=True)
    if form["read_only"]:
        raise CsustError("该评价已经提交，不能再修改", code="already_submitted")
    questions = form["questions"]
    assert isinstance(questions, list)
    question_ids = {str(question["id"]) for question in questions if isinstance(question, dict)}
    unknown = sorted(set(answers) - question_ids)
    if unknown:
        raise CsustError(f"评价题目不存在：{', '.join(unknown)}", code="invalid_answer")
    for question in questions:
        assert isinstance(question, dict)
        question_id = str(question["id"])
        if question_id not in answers:
            raise CsustError(f"缺少评价题目 {question_id} 的答案", code="missing_answer")
        options = question.get("options", [])
        if not any(isinstance(option, dict) and str(option.get("id")) == answers[question_id] for option in options):
            raise CsustError(f"评价题目 {question_id} 的选项无效", code="invalid_answer")
    payload: list[tuple[str, str]] = []
    for field in form["hidden_fields"]:
        assert isinstance(field, dict)
        name = str(field.get("name") or "")
        if name and name not in {"issubmit", "sfxyt"}:
            payload.append((name, str(field.get("value") or "")))
    for question_id, option_id in answers.items():
        payload.append((f"pj0601id_{question_id}", option_id))
    if form["suggestion_field"]:
        payload.append((str(form["suggestion_field"]), args.suggestion or ""))
    payload.extend([("issubmit", "1" if command == "submit" else "0"), ("sfxyt", "0")])
    quality_request = getattr(client, "request_web", None)
    if callable(quality_request):
        result = quality_request(str(form["action"]), method="POST", data=payload, referer=response.url)
    else:
        target = same_origin_url(client, str(form["action"]))
        result = client.post(target, payload, headers={"Referer": response.url})
    require_logged_in(result)
    client.save()
    message = _response_message(result.body)
    if _has_failure_signal(message) or not re.search(r"成功|完成|已保存|已提交|已评价", message):
        raise MutationUnverified(
            "评价已提交但未验证：未从响应中确认成功",
            details={"submitted": True, "confirmed": False, "operation": command},
        )
    return {"ok": True, "operation": command, "submitted": True, "confirmed": True, "message": message}


def _response_message(source: str) -> str:
    document = parse_html(source)
    scripts = " ".join(node.raw_text() for node in document.find_all("script"))
    alerts = re.findall(r"(?:alert|showMsg)\s*\(\s*['\"]([^'\"]+)", scripts, re.I)
    lines = [line.strip() for line in document.raw_text(include_scripts=False).splitlines() if line.strip()]
    messages = alerts + [line for line in lines if re.search(r"成功|失败|错误|已保存|已提交|已评价", line)]
    return "；".join(_safe_event(message) for message in dict.fromkeys(messages) if message)[:500]


def register(subparsers: argparse._SubParsersAction) -> None:
    profile = subparsers.add_parser("profile", aliases=["personal"], help="查询个人信息")
    profile.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    profile.set_defaults(feature_runner=_run_profile, feature_renderer=render)

    exams = subparsers.add_parser("exams", aliases=["exam"], help="查询考试安排")
    exams.add_argument("--term", help="学期，例如 2025-2026-1")
    exams.add_argument("--category", help="考试类别名称")
    exams.add_argument("--category-id", help="考试类别 ID")
    exams.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    exams.set_defaults(feature_runner=_run_exams, feature_renderer=render)

    rooms = subparsers.add_parser("classrooms", aliases=["rooms"], help="查询空闲教室")
    rooms.add_argument("--campus", required=True, help="校区：yuntang、jinpenling、1 或 2")
    rooms.add_argument("--week", required=True, type=int, help="周次")
    rooms.add_argument("--weekday", required=True, type=int, choices=range(1, 8), help="星期，1 到 7")
    rooms.add_argument("--section", required=True, type=int, choices=range(1, 6), help="大节，1 到 5")
    rooms.add_argument("--term", help="学期")
    rooms.add_argument("--department", help="上课院系 ID")
    rooms.add_argument("--building", help="教学楼 ID")
    rooms.add_argument("--area", help="功能区 ID")
    rooms.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    rooms.set_defaults(feature_runner=_run_classrooms, feature_renderer=render)

    selection = subparsers.add_parser("selections", aliases=["selection", "course-results"], help="查询选课结果")
    selection.add_argument("--term", help="学期，例如 2025-2026-1")
    selection.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    selection.set_defaults(feature_runner=_run_selection, feature_renderer=render)

    terms = subparsers.add_parser("terms", aliases=["semesters"], help="查询各功能可用学期")
    terms.add_argument("--scope", choices=("schedule", "grades", "exams", "selection", "semester-start"), required=True)
    terms.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    terms.set_defaults(feature_runner=_run_terms, feature_renderer=render)

    semester_start = subparsers.add_parser("semester-start", help="查询学期起始日")
    semester_start.add_argument("--term", help="学期，例如 2025-2026-1")
    semester_start.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    semester_start.set_defaults(feature_runner=_run_semester_start, feature_renderer=render)

    course_center = subparsers.add_parser("course-selection", aliases=["course-select"], help="查看学生选课中心")
    course_center.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    course_center.set_defaults(feature_runner=_run_course_selection_center, feature_renderer=render)

    classroom_request = subparsers.add_parser("classroom-request", aliases=["room-request"], help="查看教室借用申请")
    classroom_request.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    classroom_request.set_defaults(feature_runner=_run_classroom_request, feature_renderer=render)

    minor = subparsers.add_parser("minor", aliases=["minor-registration"], help="查看辅修报名")
    minor.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    minor.set_defaults(feature_runner=_run_minor, feature_renderer=render)

    evaluation = subparsers.add_parser("evaluation", aliases=["evaluate"], help="查询或提交学生评价")
    evaluation_children = evaluation.add_subparsers(dest="evaluation_command", required=True)
    batches = evaluation_children.add_parser("batches", help="列出评价批次")
    batches.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    batches.set_defaults(feature_runner=_run_evaluation, feature_renderer=render)
    for name, help_text in (("courses", "列出批次中的评价课程"), ("form", "查询评价表"), ("save", "保存评价"), ("submit", "提交评价")):
        child = evaluation_children.add_parser(name, help=help_text)
        child.add_argument("--path", required=name != "batches", help="页面路径或 batches 输出的 path")
        if name in {"save", "submit"}:
            child.add_argument("--answer", action="append", default=[], help="答案，格式 QUESTION=OPTION，可重复")
            child.add_argument("--suggestion", default="", help="学生建议")
            child.add_argument("--yes", action="store_true", help="确认修改远端评价")
        child.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
        child.set_defaults(feature_runner=_run_evaluation, feature_renderer=render)


def render(data: dict[str, object]) -> None:
    if "fields" in data:
        for field in data.get("fields", []):
            if isinstance(field, dict):
                print(f"{_safe_terminal_text(field.get('name'))}: {_safe_terminal_text(field.get('value'))}")
        return
    for item in data.get("items", []):
        if not isinstance(item, dict):
            continue
        label = " ".join(str(item.get(key) or "") for key in ("course", "exam_time", "room", "seat", "name", "semester", "category") if item.get(key))
        print(f"[{_safe_terminal_text(item.get('index'))}] {_safe_terminal_text(label)}".rstrip())
