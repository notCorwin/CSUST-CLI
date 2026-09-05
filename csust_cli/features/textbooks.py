"""Textbook listing and single-row subscribe/unsubscribe actions."""

from __future__ import annotations

import argparse
import re
from urllib.parse import urlparse

from ..core import (
    Client,
    CsustError,
    Element,
    HttpError,
    MutationUnverified,
    ParseError,
    Response,
    _append_query,
    _has_failure_signal,
    _image_fields,
    _is_inert_event,
    _input_value,
    _option_value,
    _SENSITIVE_FIELD,
    _safe_event,
    _safe_terminal_text,
    _safe_url,
    _save_cookie_refresh,
    _table_rows,
    ensure_session,
    parse_html,
    require_logged_in,
    same_origin_url,
    _safe_urljoin,
)


TEXTBOOK_PATHS = (
    "/jsxsd/nxsjc/jccx",
    "/jsxsd/xsjc/showXsjc",
    "/jsxsd/nxsjc/jczmxx",
    "/jsxsd/xsjc/xsjc.do",
)
TEXTBOOK_ACCOUNT_PATH = "/jsxsd/nxsjc/jczmxx"
_HEADER_WORDS = frozenset({"序号", "序", "课程", "课程名称", "课程代码", "课号", "教材", "书名", "状态", "操作"})


def parse_control(node: Element, *, safe: bool = False) -> dict[str, object]:
    kind = node.attr("type") or ("submit" if node.tag == "button" else node.tag)
    name = node.attr("name")
    value = "" if kind.lower() == "file" else node.attr("value")
    href = node.attr("href")
    onclick = node.attr("onclick")
    formaction = node.attr("formaction")
    if safe:
        sensitive = bool(_SENSITIVE_FIELD.search(name) or kind.lower() in {"password", "file"})
        if sensitive:
            value = ""
        href = _safe_url(href)
        onclick = _safe_event(onclick)
        formaction = _safe_url(formaction)
    else:
        sensitive = False
    return {
        "tag": node.tag,
        "type": kind,
        "name": name,
        "value": value,
        "text": "" if sensitive else node.text(include_scripts=False),
        "href": href,
        "onclick": onclick,
        "formaction": formaction,
        "formmethod": node.attr("formmethod"),
        "checked": "checked" in node.attrs,
        "disabled": node.is_disabled(),
    }


def ancestors(node: Element) -> list[Element]:
    result: list[Element] = []
    current = node.parent
    while current is not None:
        result.append(current)
        current = current.parent
    return result


def nearest_form(node: Element) -> Element | None:
    current: Element | None = node
    while current is not None:
        if current.tag == "form":
            return current
        current = current.parent
    return None


def textbook_table(document: Element) -> Element | None:
    candidates: list[tuple[int, Element]] = []
    for table in document.find_all("table"):
        text = table.text(include_scripts=False)
        controls = table.find_all("input") + table.find_all("button") + table.find_all("a")
        action_text = " ".join(
            " ".join(str(parse_control(node).get(key, "")) for key in ("text", "value", "onclick", "href"))
            for node in controls
        )
        score = 0
        if table.attr("id").strip() in {"dataList", "jczmxx", "jccx"}:
            score += 4
        if any(word in text for word in ("教材", "书名", "ISBN", "课程")):
            score += 3
        if re.search(r"选订|订购|退订|取消订|增订", action_text):
            score += 4
        if table.find_all("tr"):
            score += 1
        if score:
            candidates.append((score, table))
    return max(candidates, key=lambda item: item[0])[1] if candidates else None


def textbook_rows(document: Element) -> list[tuple[Element, list[Element]]]:
    table = textbook_table(document)
    if table is None:
        return []
    rows: list[tuple[Element, list[Element]]] = []
    for row in _table_rows(table):
        cells = row.direct("td")
        values = [cell.text(include_scripts=False).strip() for cell in cells]
        is_header = bool(values) and (
            values[0] in {"序号", "序"}
            or sum(value in _HEADER_WORDS for value in values) >= 2
        )
        if len(cells) >= 2 and row.text(include_scripts=False) and not is_header:
            rows.append((row, cells))
    return rows


def _table_headers(document: Element) -> list[str]:
    table = textbook_table(document)
    if table is None:
        return []
    for row in _table_rows(table):
        headers = row.direct("th")
        if headers:
            return [header.text(include_scripts=False) for header in headers]
        cells = row.direct("td")
        values = [cell.text(include_scripts=False).strip() for cell in cells]
        if values and (values[0] in {"序号", "序"} or sum(value in _HEADER_WORDS for value in values) >= 2):
            return values
    return []


def _header_value(headers: list[str], cells: list[Element], pattern: str) -> str:
    for index, header in enumerate(headers):
        if re.search(pattern, header, re.I) and index < len(cells):
            return cells[index].text(include_scripts=False)
    return ""


def action_matches(control: dict[str, object], operation: str) -> bool:
    tag = str(control.get("tag") or "").lower()
    kind = str(control.get("type") or "").lower()
    actionable = bool(control.get("formaction") or control.get("href") or control.get("onclick"))
    if (
        control.get("disabled")
        or kind == "reset"
        or (tag == "input" and kind not in {"submit", "image"} and not actionable)
        or (tag == "button" and kind != "submit" and not actionable)
        or _is_inert_event(str(control.get("onclick") or ""))
        or (_is_inert_event(str(control.get("href") or "")) and not control.get("onclick"))
    ):
        return False
    text = " ".join(str(control.get(key, "")) for key in ("text", "value", "onclick", "href"))
    if operation == "unsubscribe":
        return bool(re.search(r"退订|取消订|不订|退购|取消选择", text))
    return bool(re.search(r"选订|订购|增订|订教材|购买", text)) and not re.search(r"退订|取消订|取消选择", text)


def _action_submits_form(control: dict[str, object]) -> bool:
    kind = str(control.get("type") or "").lower()
    script = f"{control.get('onclick') or ''} {control.get('href') or ''}"
    return kind in {"submit", "image"} or bool(re.search(r"\bsubmit\s*\(", script, re.I))


def _row_actions(row: Element) -> list[str]:
    controls = row.find_all("input") + row.find_all("button") + row.find_all("a")
    actions: list[str] = []
    for operation in ("subscribe", "unsubscribe"):
        if any(action_matches(parse_control(node), operation) for node in controls):
            actions.append(operation)
    return actions


def _row_status(row_text: str, actions: list[str]) -> str:
    if re.search(r"已订|已选|已购|订购成功", row_text):
        return "已订"
    if re.search(r"未订|未选|可订|待订", row_text):
        return "未订"
    if "unsubscribe" in actions and "subscribe" not in actions:
        return "已订"
    if "subscribe" in actions and "unsubscribe" not in actions:
        return "未订"
    return ""


def parse_textbooks(source: str, page_url: str) -> dict[str, object]:
    document = parse_html(source)
    headers = _table_headers(document)
    items: list[dict[str, object]] = []
    for row, cells in textbook_rows(document):
        controls = [parse_control(node, safe=True) for node in row.find_all("input") if node.attr("type").lower() != "hidden"]
        controls.extend(parse_control(node, safe=True) for node in row.find_all("button"))
        controls.extend(parse_control(node, safe=True) for node in row.find_all("a") if node.attr("href") or node.attr("onclick"))
        row_text = row.text(include_scripts=False)
        actions = _row_actions(row)
        course = _header_value(headers, cells, r"课程|课程名称|课程代码|课号")
        title = _header_value(headers, cells, r"教材|书名|教材名称|图书")
        isbn = _header_value(headers, cells, r"ISBN")
        item = {
            "index": len(items) + 1,
            "course": course or (cells[0].text(include_scripts=False) if cells else ""),
            "title": title or (cells[1].text(include_scripts=False) if len(cells) > 1 else ""),
            "isbn": isbn,
            "status": _row_status(row_text, actions),
            "actions": actions,
            "cells": [cell.text(include_scripts=False) for cell in cells],
            "text": row_text,
            "controls": controls,
            "form": bool(nearest_form(row)),
        }
        items.append(item)
    return {"url": _safe_url(page_url), "items": items}


def find_textbook_page(client: Client) -> tuple[Response, Element]:
    for path in TEXTBOOK_PATHS:
        try:
            response = client.get(path)
        except HttpError as exc:
            if exc.status == 404:
                continue
            raise
        require_logged_in(response)
        _save_cookie_refresh(client, response)
        document = parse_html(response.body)
        if textbook_table(document) is not None:
            return response, document
    raise ParseError("未找到教材页面；学校可能暂未开放教材确认或已更换页面")


def choose_textbook_item(items: list[dict[str, object]], index: int | None, match: str | None) -> tuple[int, dict[str, object]]:
    if (index is None) == (match is None):
        raise CsustError("必须且只能提供 --index 或 --match", code="invalid_target")
    if index is not None:
        candidates = [(index - 1, items[index - 1])] if 1 <= index <= len(items) else []
    else:
        needle = (match or "").strip().casefold()
        if not needle:
            raise CsustError("--match 不能为空", code="invalid_target")
        candidates = [(position, item) for position, item in enumerate(items) if needle in str(item["text"]).casefold()]
    if not candidates:
        raise CsustError("没有匹配的教材条目", code="target_not_found")
    if len(candidates) > 1:
        raise CsustError("匹配到多个教材条目，请改用 --index", code="target_ambiguous")
    return candidates[0]


def form_fields(form: Element, target_row: Element, action_node: Element | None) -> list[tuple[str, str]]:
    """Keep form-level fields and the selected row, never sibling rows."""
    fields: list[tuple[str, str]] = []
    for node in form.find_all():
        if node is not action_node and node.tag not in {"input", "select", "textarea"}:
            continue
        if node.is_disabled():
            continue
        node_ancestors = ancestors(node)
        in_target = target_row in node_ancestors
        in_any_row = any(ancestor.tag == "tr" for ancestor in node_ancestors)
        if in_any_row and not in_target:
            continue
        if node is action_node and node.tag in {"input", "button"}:
            if node.tag == "input" and node.attr("type").lower() == "image":
                fields.extend(_image_fields(node))
            elif node.attr("name"):
                fields.append((node.attr("name"), node.attr("value") if "value" in node.attrs else ""))
            continue
        if node.tag == "input":
            name = node.attr("name")
            if not name:
                continue
            kind = node.attr("type", "text").lower()
            if kind in {"submit", "button", "image", "reset", "file"}:
                continue
            if kind in {"checkbox", "radio"} and "checked" not in node.attrs:
                continue
            fields.append((name, _input_value(node)))
        elif node.tag == "select":
            name = node.attr("name")
            if not name:
                continue
            options = [option for option in node.find_all("option") if not option.is_disabled()]
            selected = [option for option in options if "selected" in option.attrs]
            if "multiple" in node.attrs:
                fields.extend((name, _option_value(option)) for option in selected)
            elif selected:
                fields.append((name, _option_value(selected[0])))
            elif options:
                fields.append((name, _option_value(options[0])))
        elif node.tag == "textarea" and node.attr("name"):
            fields.append((node.attr("name"), node.raw_text()))
    return fields


def action_url(control: dict[str, object], form: Element | None, page_url: str) -> tuple[str, str]:
    explicit_url = str(control.get("formaction") or "")
    explicit_method = str(control.get("formmethod") or "")
    href = str(control.get("href") or "")
    onclick = str(control.get("onclick") or "")
    if explicit_url and ("type" not in control or _action_submits_form(control)):
        method = explicit_method or (form.attr("method", "GET") if form is not None else "GET")
        return method.upper(), _safe_urljoin(page_url, explicit_url)
    if href and not href.lower().startswith("javascript:") and href != "#":
        return "GET", _safe_urljoin(page_url, href)
    script = href + " " + onclick
    quoted = re.findall(r"['\"]([^'\"]+)['\"]", script)
    for value in quoted:
        if value.startswith(("/", "http://", "https://")) and not value.startswith("//"):
            method = "GET" if re.search(r"location|window\.open", script, re.I) else "POST"
            return method, _safe_urljoin(page_url, value)
    if form is None:
        return "GET", ""
    if "type" in control and not _action_submits_form(control):
        return "GET", ""
    return (explicit_method or form.attr("method", "GET")).upper(), _safe_urljoin(page_url, form.attr("action") or page_url)


def response_message(source: str) -> str:
    document = parse_html(source)
    text = document.raw_text(include_scripts=False)
    scripts = " ".join(node.raw_text() for node in document.find_all("script"))
    alerts = re.findall(r"(?:alert|showMsg)\s*\(\s*['\"]([^'\"]+)['\"]", scripts, flags=re.I)
    messages = alerts + [line.strip() for line in text.splitlines() if re.search(r"成功|失败|错误|不能|不允许|已订|退订|操作", line)]
    return "；".join(_safe_event(message) for message in dict.fromkeys(messages) if message)[:500]


def _success_message(message: str) -> bool:
    return bool(re.search(r"成功|已订|退订|操作完成|提交成功|保存成功", message))


def _failure_message(message: str) -> bool:
    return _has_failure_signal(message)


def _same_item(left: dict[str, object], right: dict[str, object]) -> bool:
    left_text = str(left.get("text") or "").strip()
    right_text = str(right.get("text") or "").strip()
    if left_text and right_text and left_text == right_text:
        return True
    isbn_left = str(left.get("isbn") or "").strip()
    isbn_right = str(right.get("isbn") or "").strip()
    if isbn_left or isbn_right:
        return bool(isbn_left and isbn_right and isbn_left == isbn_right)
    return all(
        str(left.get(key) or "").strip()
        and str(right.get(key) or "").strip()
        and str(left.get(key)).strip() == str(right.get(key)).strip()
        for key in ("course", "title")
    )


def _state_confirms(item: dict[str, object], operation: str) -> bool:
    actions = set(item.get("actions", []))
    status = str(item.get("status") or "")
    if operation == "subscribe":
        return bool(re.search(r"已订|已选|已购", status)) or actions == {"unsubscribe"}
    return bool(re.search(r"未订|未选|可订|待订", status)) or actions == {"subscribe"}


def _verify_state(client: Client, before: dict[str, object], operation: str) -> tuple[bool, str]:
    try:
        response, _ = find_textbook_page(client)
        data = parse_textbooks(response.body, response.url)
    except CsustError as exc:
        return False, f"提交后的教材页面无法复查：{exc}"
    items = data["items"]
    assert isinstance(items, list)
    candidates = [item for item in items if isinstance(item, dict) and _same_item(item, before)]
    if len(candidates) != 1:
        return False, "提交后的教材条目无法唯一定位"
    candidate = candidates[0]
    if not _state_confirms(candidate, operation):
        return False, "提交后的教材状态没有显示预期变化"
    return True, ""


def submit_textbook_action(
    client: Client,
    response: Response,
    document: Element,
    operation: str,
    index: int | None,
    match: str | None,
    *,
    verify: bool = True,
) -> dict[str, object]:
    parsed = parse_textbooks(response.body, response.url)
    items = parsed["items"]
    assert isinstance(items, list)
    position, item = choose_textbook_item(items, index, match)
    rows = textbook_rows(document)
    if position >= len(rows):
        raise ParseError("教材条目与页面行数不一致")
    row = rows[position][0]
    form = nearest_form(row)
    if form is not None and _is_inert_event(form.attr("onsubmit")):
        raise ParseError("教材表单提交被页面脚本禁用")
    descriptors = [(node, parse_control(node)) for node in row.find_all("input") + row.find_all("button") + row.find_all("a")]
    action_node, descriptor = next(((node, data) for node, data in descriptors if action_matches(data, operation)), (None, None))
    if action_node is None:
        raise ParseError(f"未找到“{operation}”按钮；为避免误操作，已拒绝提交")
    assert descriptor is not None
    method, target = action_url(descriptor, form, response.url)
    if not target:
        raise ParseError("教材操作没有可解析的目标；为避免误操作，已拒绝提交")
    if method not in {"GET", "POST"}:
        raise CsustError(f"教材操作不支持 HTTP 方法 {method}", code="invalid_argument")
    target = same_origin_url(client, target)
    if form is None and method != "GET":
        raise ParseError("教材条目没有可提交的表单")
    payload = form_fields(form, row, action_node) if form is not None and _action_submits_form(descriptor) else []
    if method == "GET":
        target = _append_query(target, payload)
        result = client.get(target)
    else:
        result = client.post(target, payload, headers={"Referer": response.url})
    require_logged_in(result)
    client.save()
    message = response_message(result.body)
    safe_request = {"method": method, "path": urlparse(target).path}
    if _failure_message(message):
        raise CsustError(
            f"教材{operation}失败：{message}",
            code="mutation_rejected",
            details={"operation": operation, "index": position + 1},
        )
    base_result: dict[str, object] = {
        "ok": True,
        "operation": operation,
        "index": position + 1,
        "item": item,
        "request": safe_request,
        "message": message,
        "submitted": True,
        "confirmed": False,
    }
    if verify:
        verified, reason = _verify_state(client, item, operation)
        if not verified:
            suffix = "" if _success_message(message) else "且未从响应中确认成功"
            raise MutationUnverified(
                "教材操作已提交但未验证" + suffix + "：" + reason,
                details={**base_result, "ok": False, "verification": "unknown"},
            )
    elif not _success_message(message):
        raise MutationUnverified(
            "教材操作已提交但未从响应中确认成功",
            details={**base_result, "ok": False, "verification": "unknown"},
        )
    base_result["confirmed"] = True
    base_result["verification"] = "confirmed"
    return base_result


def register(subparsers: argparse._SubParsersAction) -> None:
    parent = subparsers.add_parser("textbooks", aliases=["textbook"], help="教材查询与订退")
    children = parent.add_subparsers(dest="textbooks_command", required=True)
    list_parser = children.add_parser("list", help="列出教材")
    list_parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    list_parser.set_defaults(feature_runner=run, feature_renderer=render)
    account_parser = children.add_parser("account", help="查看教材账目")
    account_parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    account_parser.set_defaults(feature_runner=run, feature_renderer=render)
    for name, aliases, help_text in (
        ("subscribe", ["order"], "选订教材"),
        ("unsubscribe", ["cancel"], "退订教材"),
    ):
        action_parser = children.add_parser(name, aliases=aliases, help=help_text)
        action_parser.add_argument("--index", type=int, help="list 输出的条目序号")
        action_parser.add_argument("--match", help="按教材条目文本匹配；必须唯一")
        action_parser.add_argument("--yes", action="store_true", help="确认执行远端变更")
        action_parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
        action_parser.set_defaults(feature_runner=run, feature_renderer=render)
    for alias, command in (("order", "subscribe"), ("cancel", "unsubscribe")):
        alias_parser = subparsers.add_parser(alias, help=f"{('选订' if command == 'subscribe' else '退订')}教材")
        alias_parser.add_argument("--index", type=int, help="list 输出的条目序号")
        alias_parser.add_argument("--match", help="按教材条目文本匹配；必须唯一")
        alias_parser.add_argument("--yes", action="store_true", help="确认执行远端变更")
        alias_parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
        alias_parser.set_defaults(textbooks_command=command, feature_runner=run, feature_renderer=render)


def run(args: argparse.Namespace, client: Client) -> dict[str, object]:
    if args.textbooks_command == "list":
        ensure_session(client)
        response, document = find_textbook_page(client)
        return parse_textbooks(response.body, response.url)
    if args.textbooks_command == "account":
        ensure_session(client)
        response = client.get(TEXTBOOK_ACCOUNT_PATH)
        require_logged_in(response)
        _save_cookie_refresh(client, response)
        return parse_textbooks(response.body, response.url)
    if not args.yes:
        raise CsustError("远端订退教材会修改账号数据，请加 --yes", code="confirmation_required")
    if (args.index is None) == (args.match is None):
        raise CsustError("必须且只能提供 --index 或 --match", code="invalid_target")
    if args.index is not None and args.index < 1:
        raise CsustError("--index 必须是正整数", code="invalid_target")
    if args.match is not None and not args.match.strip():
        raise CsustError("--match 不能为空", code="invalid_target")
    ensure_session(client)
    response, document = find_textbook_page(client)
    operation = "unsubscribe" if args.textbooks_command in {"unsubscribe", "cancel"} else "subscribe"
    return submit_textbook_action(client, response, document, operation, args.index, args.match)


def render(data: dict[str, object]) -> None:
    if "operation" in data:
        print(_safe_terminal_text(data.get("message") or "教材操作已完成"))
        return
    for item in data.get("items", []):
        course = str(item.get("course") or "")
        title = str(item.get("title") or "")
        status = str(item.get("status") or "")
        label = " ".join(value for value in (course, title, status) if value) or str(item.get("text") or "")
        print(f"[{_safe_terminal_text(item.get('index'))}] {_safe_terminal_text(label)}")
