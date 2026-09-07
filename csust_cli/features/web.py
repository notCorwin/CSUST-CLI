"""Route catalog and data-driven access to the student web application."""

from __future__ import annotations

import argparse
import ast
import hashlib
import json
import re
import time
from pathlib import Path
from urllib.parse import quote, urlparse

from ..core import (
    Client,
    CsustError,
    business_state,
    HttpError,
    MutationUnverified,
    NetworkError,
    result_status,
    Element,
    Response,
    _append_query,
    _commit_stream,
    _decode_body,
    _discard_stream,
    _header_value,
    _image_fields,
    _is_inert_event,
    _input_value,
    _option_value,
    _safe_event,
    _safe_terminal_text,
    _safe_url,
    _SENSITIVE_FIELD,
    _save_cookie_refresh,
    _table_rows,
    _url_origin,
    ensure_session,
    internal_url,
    is_login_page,
    parse_html,
    require_logged_in,
    _safe_urljoin,
    _write_private_file,
)


# (stable CLI command, second-level menu, visible label, data-url)
ROUTE_CATALOG = (
    ("student-evaluation", "教学评价", "学生评价", "/jsxsd/xspj/xspj_find.do"),
    ("deferred-exam-application", "我的申请", "缓考申请", "/jsxsd/kscj/hksq_query"),
    ("exempt-exam-application", "我的申请", "免考申请", "/jsxsd/kscj/mksq_query"),
    ("enrollment-proof-application", "我的申请", "学籍在读证明申请", "/jsxsd/kscj/xjzdzmsq_query"),
    ("exam-schedule", "我的考试", "考试安排查询", "/jsxsd/xsks/xsksap_query"),
    ("graduate-exam-registration", "我的考试", "毕业生插考报名", "/jsxsd/xsks/bysckbm_query"),
    ("in-class-exam", "我的考试", "随堂考试查询", "/jsxsd/xsks/xsstk_query"),
    ("deferred-exam-registration", "我的考试", "缓考考试报名", "/jsxsd/kscj/hkbm_query"),
    ("social-exam-registration", "我的考试", "社会考试报名", "/jsxsd/xsdjks/xsdjks_list"),
    ("make-up-exam-registration", "成绩管理", "补考报名", "/jsxsd/kscj/bkbm_query"),
    ("summer-remedial-registration", "成绩管理", "暑期补修报名", "/jsxsd/kscj/qkbm_query"),
    ("retake-registration-selection", "成绩管理", "重修报名选课", "/jsxsd/kscj/cxbmxk_query"),
    ("teaching-process", "培养方案", "教学进程查询", "/jsxsd/pyfa/pyfajc_query"),
    ("execution-plan", "培养方案", "执行计划", "/jsxsd/pyfa/pyfa_query"),
    ("training-plan-progress", "培养方案", "培养方案及完成情况", "/jsxsd/pyfa/topyfamx"),
    ("minor-execution-plan", "培养方案", "辅修执行计划", "/jsxsd/pyfa/fxpyfa_query"),
    ("minor-training-plan", "培养方案", "辅修培养方案明细", "/jsxsd/pyfa/tofxpyfamx"),
    ("university-timetable", "我的课表", "全校性总课表查询", "/jsxsd/jskb/qxxzkb_find.do"),
    ("semester-timetable", "我的课表", "学期理论课表", "/jsxsd/xskb/xskb_list.do"),
    ("lab-timetable", "我的课表", "实验课表查询", "/jsxsd/syjx/toXskb.do"),
    ("class-timetable", "我的课表", "班级课表查询", "/jsxsd/kbcx/kbxx_xzb"),
    ("teacher-timetable", "我的课表", "教师课表查询", "/jsxsd/kbcx/kbxx_teacher"),
    ("classroom-timetable", "我的课表", "教室课表查询", "/jsxsd/kbcx/kbxx_classroom"),
    ("course-timetable", "我的课表", "课程课表查询", "/jsxsd/kbcx/kbxx_kc"),
    ("class-change", "我的课表", "调停课查询", "/jsxsd/xskb/xskb_ttkmx.do"),
    ("course-selection-center", "选课管理", "学生选课中心", "/jsxsd/xsxk/xklc_list"),
    ("special-course-application", "选课管理", "特殊选课申请", "/jsxsd/tsxk/tsxk_sqlist"),
    ("special-course-query", "选课管理", "特殊选课查询", "/jsxsd/tsxk/tsxk_cxlist"),
    ("preselection-management", "选课管理", "学生预选管理", "/jsxsd/xkgl/xsyxgl"),
    ("preselection-query", "选课管理", "学生预选查询", "/jsxsd/xkgl/xsyxcx"),
    ("classroom-loan-record", "选课管理", "教室借用记录", "/jsxsd/kbxx/jsjyjl_query"),
    ("teaching-progress", "选课管理", "教学进度查询", "/jsxsd/xkgl/skjhQuery.do"),
    ("drop-course-application", "选课管理", "学生退课申请", "/jsxsd/xkgl/xstk_list"),
    ("course-selection-result", "选课管理", "选课结果查询", "/jsxsd/xkgl/xsxkjgcx"),
    ("textbook-account-info", "教材管理", "教材账目信息", "/jsxsd/nxsjc/jczmxx"),
    ("textbook-confirmation", "教材管理", "学生教材确认", "/jsxsd/nxsjc/jccxcslg"),
    ("minor-application", "辅修管理", "辅修报名", "/jsxsd/fxgl/fxbmxx_query"),
    ("lab-booking", "实验教学", "实验预约管理", "/jsxsd/view/syjx/syyy_find.jsp"),
    ("open-lab-booking", "实验教学", "开放实验预约", "/jsxsd/view/syjx/kfsy_find.jsp"),
    ("second-class-credit-application", "第二课堂学分", "第二课堂学分申报", "/jsxsd/pyfa/cxxfsb_query"),
    ("second-class-credit-query", "第二课堂学分", "第二课堂学分查询", "/jsxsd/pyfa/cxxf_query"),
    ("discipline-competition-registration", "学科竞赛", "学科竞赛报名", "/jsxsd/xsxkjs/xkjsbm_query"),
    ("teacher-project-topics", "创新创业", "教师发布课题", "/jsxsd/view/cxcyxm/ktgl_xs_query.jsp"),
    ("project-change", "创新创业", "项目变更管理", "/jsxsd/cxcyxm/queryXmbg.do"),
    ("member-change", "创新创业", "成员变更管理", "/jsxsd/cxcyxm/queryCybg.do"),
    ("project-application", "创新创业", "项目申报管理", "/jsxsd/cxcyxm/querySq.do"),
    ("project-funding", "创新创业", "项目资金发放查看", "/jsxsd/cxcyxm/queryXmzjff.do"),
    ("received-notices", "公告留言", "已收公告", "/jsxsd/ggly/ysgg_query"),
    ("received-messages", "公告留言", "已收留言", "/jsxsd/ggly/ysly_query"),
    ("message-notifications", "公告留言", "消息通知", "/jsxsd/ggly/xxtz_query"),
    ("personal-info", "个人信息", "修改个人信息", "/jsxsd/grsz/grsz_xggrxx.do"),
    ("change-password", "个人信息", "修改密码", "/jsxsd/grsz/grsz_xgmm"),
    ("online-qa", "在线问答", "在线问答", "/jsxsd/zxwd/zxwd_opt"),
    ("teaching-calendar", "教学周历", "教学周历查看", "/jsxsd/jxzl/jxzl_query"),
    ("student-record-card", "学籍管理", "学籍卡片", "/jsxsd/grxx/xsxx"),
    ("graduation-status", "学籍管理", "毕业情况查询", "/jsxsd/xxwcqk/byqkcx.do"),
    ("student-status-management", "学籍管理", "学籍信息管理", "/jsxsd/xsxj/xjxxgl.do"),
    ("status-warning", "学籍管理", "学籍预警查询", "/jsxsd/xsxj/xsyjxx.do"),
    ("status-change", "学籍管理", "学籍异动信息", "/jsxsd/xsxj/xsydxx.do"),
    ("major-streaming", "学籍管理", "专业分流", "/jsxsd/xsxj/toQueryZyfl.do"),
    ("minor-status-change", "学籍管理", "辅修学生异动申请", "/jsxsd/fxxsxj/xsydxx.do"),
    ("direction-streaming", "学籍管理", "方向分流", "/jsxsd/xsxj/toQueryfxfl.do"),
    ("course-grades", "我的成绩", "课程成绩查询", "/jsxsd/kscj/cjcx_frm"),
    ("grade-recognition", "我的成绩", "成绩认定", "/jsxsd/kscj/cjrd_list"),
    ("grade-review-application", "我的成绩", "成绩查卷申请", "/jsxsd/kscj/cjfh_list"),
    ("grade-confirmation", "我的成绩", "成绩确认申请", "/jsxsd/kscj/cjqr_list"),
    ("graduate-info-check", "毕业管理", "毕业生信息核对", "/jsxsd/bygl/bysxx"),
    ("graduation-conclusion", "毕业管理", "毕业结论查看", "/jsxsd/bygl/bygl_ckxsList"),
    ("graduation-course-recognition", "毕业管理", "毕业课程认定查询", "/jsxsd/bygl/bykcrd_query"),
)

PUBLIC_CATALOG = (
    ("login", "登录", "/"),
    ("forgot-password", "忘记密码", "/findmm.jsp"),
    ("account-recovery-step", "找回密码步骤页", "/Logon.do"),
    ("captcha", "登录验证码", "/verifycode.servlet"),
    ("app-qr", "APP 下载/返回登录", "/css/images/codeFrame.png"),
)

MAIN_MENU_CATALOG = (
    ("desktop", "我的桌面", "NEW_XSD_WDZM"),
    ("student-records", "学籍成绩", "NEW_XSD_XJCJ"),
    ("training", "培养管理", "NEW_XSD_PYGL"),
    ("exams", "考试报名", "NEW_XSD_KSBM"),
    ("practice", "实践环节", "NEW_XSD_SJHJ"),
    ("evaluation", "教学评价", "NEW_XSD_JXPJ"),
)

SECOND_LEVEL_CATALOG = (
    ("教学评价", "教学评价", "NEW_XSD_JXPJ_JXPJ"),
    ("我的申请", "考试报名", "NEW_XSD_KSBM_WDSQ"),
    ("我的考试", "考试报名", "NEW_XSD_KSBM_WDKS"),
    ("成绩管理", "考试报名", "NEW_XSD_KSBM_CJGL"),
    ("培养方案", "培养管理", "NEW_XSD_PYGL_PYFA"),
    ("我的课表", "培养管理", "NEW_XSD_PYGL_WDKB"),
    ("选课管理", "培养管理", "NEW_XSD_PYGL_XKGL"),
    ("教材管理", "培养管理", "NEW_XSD_PYGL_JCGL"),
    ("辅修管理", "培养管理", "NEW_XSD_PYGL_FXGL"),
    ("实验教学", "实践环节", "NEW_XSD_SJHJ_SYJX"),
    ("第二课堂学分", "实践环节", "NEW_XSD_SJHJ_CXXF"),
    ("毕业设计", "实践环节", "NEW_XSD_KSBM_BYSJ"),
    ("学科竞赛", "实践环节", "NEW_XSD_SJHJ_XKJS"),
    ("创新创业", "实践环节", "NEW_XSD_SJHJ_CXCY"),
    ("公告留言", "我的桌面", "NEW_XSD_WDZM_GGLY"),
    ("个人信息", "我的桌面", "NEW_XSD_WDZM_GRXX"),
    ("在线问答", "我的桌面", "NEW_XSD_WDZM_ZXWD"),
    ("教学周历", "我的桌面", "NEW_XSD_WDZM_JXZL"),
    ("学籍管理", "学籍成绩", "NEW_XSD_XJCJ_XJGL"),
    ("我的成绩", "学籍成绩", "NEW_XSD_XJCJ_WDCJ"),
    ("毕业管理", "学籍成绩", "NEW_XSD_BYGL_BYGL"),
)

PAGE_SNAPSHOT_SCHEMA = 1

READ_ONLY_METHODS = frozenset({"GET", "HEAD", "OPTIONS"})
SUPPORTED_METHODS = READ_ONLY_METHODS | {"POST", "PUT", "PATCH", "DELETE"}
_BODY_UNSET = object()
EVENT_ATTRIBUTES = ("onclick", "onchange", "onsubmit", "ondblclick")
_REPORT_TARGETS = {
    ("192.168.3.125", "/FineReport"),
    ("192.168.253.164", "/ReportServer"),
}
_SIDE_EFFECT_GET = re.compile(
    r"(?:/(?:logout|delete|remove|add|join|bind|ignore|favorite|recommend|subscribe|unsubscribe|cancel|submit|save|update|sort)(?:[/?._]|$)|[?&](?:action|op|ACTION|operation)=)",
    re.I,
)
_TRANSIENT_HTTP_STATUSES = frozenset({408, 502, 503, 504})

# Compatibility for callers that used the old tuple.
KNOWN_ROUTES = tuple((command, label, path) for command, _group, label, path in ROUTE_CATALOG)


def _supported_method(value: str) -> str:
    method = value.strip().upper()
    if method not in SUPPORTED_METHODS:
        raise CsustError("不支持的 HTTP 方法", code="invalid_argument")
    return method


def _retry_read(method: str, target: str, call):
    try:
        return call()
    except (HttpError, NetworkError) as exc:
        status = getattr(exc, "status", None)
        if (
            method not in READ_ONLY_METHODS
            or _is_side_effect_get(target)
            or (isinstance(exc, HttpError) and status not in _TRANSIENT_HTTP_STATUSES)
        ):
            raise
        # ponytail: one retry with fixed delay; add backoff only if measurements require it.
        time.sleep(0.1)
        return call()


def _ensure_web_session(client: Client) -> None:
    hook = getattr(client, "ensure_web_session", None)
    if callable(hook):
        hook()
        return
    ensure_session(client)


def _is_side_effect_get(target: str) -> bool:
    try:
        parsed = urlparse(target)
    except ValueError:
        return False
    return bool(_SIDE_EFFECT_GET.search(parsed.path + ("?" + parsed.query if parsed.query else "")))


def _is_report_target(target: str) -> bool:
    try:
        parsed = urlparse(target)
        return (
            parsed.scheme.lower() == "https"
            and parsed.hostname is not None
            and (parsed.hostname.lower(), parsed.path) in _REPORT_TARGETS
            and parsed.port in (None, 443)
            and parsed.username is None
            and parsed.password is None
        )
    except (TypeError, ValueError):
        return False


def _pairs(values: list[str], flag: str) -> list[tuple[str, str]]:
    pairs: list[tuple[str, str]] = []
    for value in values:
        if "=" not in value:
            raise CsustError(f"{flag} 格式必须为 NAME=VALUE", code="invalid_argument")
        name, item = value.split("=", 1)
        if not name:
            raise CsustError(f"{flag} 的名称不能为空", code="invalid_argument")
        pairs.append((name, item))
    return pairs


def _safe_external_url(value: str) -> str:
    try:
        parsed = urlparse(value)
    except ValueError:
        return ""
    return _safe_url(parsed._replace(query="", fragment="").geturl())


def _target(client: Client, path: str, params: list[tuple[str, str]] = ()) -> str:
    quality_url = getattr(client, "web_url", None)
    target = quality_url(path) if callable(quality_url) else _allowed_target(client, client.url(path))
    if params:
        target = _append_query(target, params)
    return target


def _allowed_target(client: Client, target: str) -> str:
    """Keep same-site requests strict, with the two report servers used by print/export."""
    quality_url = getattr(client, "web_url", None)
    if callable(quality_url):
        return quality_url(target)
    try:
        return internal_url(client, target)
    except CsustError:
        try:
            parsed = urlparse(target)
        except ValueError as exc:
            raise CsustError("页面地址格式无效", code="invalid_path") from exc
        if _is_report_target(target):
            return target
        raise


def _public_target(client: Client, path: str, params: list[tuple[str, str]] = ()) -> str:
    quality_url = getattr(client, "web_url", None)
    target = quality_url(path) if callable(quality_url) else client.url(path)
    try:
        parsed = urlparse(target)
        target_origin = _url_origin(target)
        base_origin = _url_origin(client.base_url)
    except ValueError as exc:
        raise CsustError("页面地址格式无效", code="invalid_path") from exc
    allowed_path = parsed.path
    if callable(quality_url):
        service = client.ensure_service()
        prefix = str(service.get("urlPlus") or "").rstrip("/")
        if prefix and (allowed_path == prefix or allowed_path.startswith(prefix + "/")):
            allowed_path = allowed_path[len(prefix) :] or "/"
    allowed = {"/", "/findmm.jsp", "/Logon.do", "/verifycode.servlet", "/verifycode.servlet1", "/css/images/codeFrame.png"}
    if (
        parsed.scheme.lower() not in {"http", "https"}
        or not parsed.netloc
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or target_origin != base_origin
        or allowed_path not in allowed
    ):
        raise CsustError("公开入口只允许登录、找回密码和验证码页面", code="invalid_path")
    if params:
        target = _append_query(target, params)
    return target


def _link(node: Element, page_url: str) -> dict[str, object]:
    href = node.attr("href")
    if href.lower().startswith("javascript:"):
        quoted = re.search(r"['\"]([^'\"]+)", href)
        href = quoted.group(1) if quoted else ""
    target = _safe_urljoin(page_url, href) if href else ""
    try:
        parsed = urlparse(target)
        parsed_origin = _url_origin(target)
        page_origin = _url_origin(page_url)
    except ValueError:
        parsed = urlparse("")
        parsed_origin = ("", "", None)
        page_origin = ("", "", None)
    safe_target = _safe_url(target)
    safe_parsed = urlparse(safe_target) if safe_target else urlparse("")
    safe_path = safe_parsed.path
    if safe_parsed.params:
        safe_path += ";" + safe_parsed.params
    if safe_parsed.query:
        safe_path += "?" + safe_parsed.query
    if safe_parsed.fragment:
        safe_path += "#" + safe_parsed.fragment
    return {
        "text": _display_text(node),
        "href": _safe_url(href, page_url),
        "path": safe_path if parsed_origin == page_origin else safe_target,
        "onclick": _safe_event(node.attr("onclick")),
    }


def _sensitive_control(node: Element) -> bool:
    return bool(_SENSITIVE_FIELD.search(node.attr("name")) or node.attr("type").lower() in {"password", "file"})


def _safe_page_text(node: Element) -> str:
    if node.tag in {"script", "style"} or (node.tag in {"input", "textarea", "select", "button"} and _sensitive_control(node)):
        return ""
    chunks: list[str] = []
    stack: list[tuple[Element | str, bool, bool]] = [(node, False, False)]
    while stack:
        child, closing, add_newline = stack.pop()
        if isinstance(child, str):
            chunks.append(child)
            continue
        if closing:
            if add_newline:
                chunks.append("\n")
            continue
        if child.tag in {"script", "style"} or (child.tag in {"input", "textarea", "select", "button"} and _sensitive_control(child)):
            continue
        stack.append((child, True, child is not node and child.tag in {"br", "p", "div", "tr"}))
        stack.extend((grandchild, False, False) for grandchild in reversed(child.children))
    return "".join(chunks)


def _display_text(node: Element) -> str:
    return _safe_event(" ".join(_safe_page_text(node).replace("\xa0", " ").split()))


def _semantic_text(value: object) -> str:
    return " ".join(str(value or "").replace("\xa0", " ").split()).casefold()


def _control_label(document: Element, node: Element) -> str:
    value = node.attr("aria-label").strip()
    if value:
        return _safe_event(value)
    labelledby = [item for item in node.attr("aria-labelledby").split() if item]
    if labelledby:
        labels = [document.first(element_id=item) for item in labelledby]
        text = " ".join(_display_text(label) for label in labels if label is not None).strip()
        if text:
            return text
    node_id = node.attr("id").strip()
    if node_id:
        label = next((item for item in document.find_all("label") if item.attr("for").strip() == node_id), None)
        if label is not None:
            text = _display_text(label)
            if text:
                return text
    current: Element | None = node.parent
    while current is not None:
        if current.tag == "label":
            text = _display_text(current)
            if text:
                return text
        current = current.parent
    for attribute in ("placeholder", "title"):
        value = node.attr(attribute).strip()
        if value:
            return _safe_event(value)
    return ""


def _control(node: Element) -> dict[str, object]:
    name = node.attr("name")
    sensitive = _sensitive_control(node)
    placeholder = "<redacted>" if re.search(r"execution|flowexecutionkey|\b(?:state|lt)\b", name, re.I) else ""
    value = placeholder if sensitive else _safe_event(node.attr("value"))
    data: dict[str, object] = {
        "tag": node.tag,
        "type": node.attr("type") or node.tag,
        "id": node.attr("id"),
        "name": name,
        "value": value,
        "text": "" if sensitive else _display_text(node),
        "role": node.attr("role"),
        "aria_label": _safe_event(node.attr("aria-label")),
        "placeholder": _safe_event(node.attr("placeholder")),
        "href": _safe_url(node.attr("href")),
        "onclick": _safe_event(node.attr("onclick")),
        "disabled": node.is_disabled(),
        "checked": "checked" in node.attrs,
    }
    events = {name: value for name, value in node.attrs.items() if name in EVENT_ATTRIBUTES}
    if events:
        data["events"] = {name: _safe_event(value) for name, value in events.items()}
    if node.tag == "select":
        data["multiple"] = "multiple" in node.attrs
        data["options"] = [
            {
                "text": "" if sensitive else _display_text(option),
                "value": placeholder if _SENSITIVE_FIELD.search(name) else _safe_event(_option_value(option)),
                "selected": "selected" in option.attrs,
                "disabled": option.is_disabled(),
            }
            for option in node.find_all("option")
        ]
    return data


def _shape_fingerprint(document: Element) -> str:
    """Hash the page shape without volatile values or rendered table data."""
    parts: list[str] = []
    stack: list[tuple[Element, bool]] = [(document, False)]
    shape_attributes = frozenset(
        {"type", "name", "role", "aria-label", "aria-labelledby", "placeholder", "method", "enctype", "multiple"}
    )
    layout_tags = frozenset({"div", "span", "section", "main", "header", "footer", "nav", "article", "aside", "tbody", "thead", "tfoot"})
    while stack:
        node, closing = stack.pop()
        if closing:
            parts.append(f"</{node.tag}>")
            continue
        if node.tag == "#document":
            stack.extend((child, False) for child in reversed(node.children) if isinstance(child, Element))
            continue
        if node.tag in layout_tags:
            stack.extend((child, False) for child in reversed(node.children) if isinstance(child, Element))
            continue
        attributes = ",".join(sorted(key for key in node.attrs if key in shape_attributes))
        parts.append(f"<{node.tag}[{attributes}]>")
        if node.tag in {"script", "style"}:
            continue
        if node.children:
            stack.append((node, True))
            stack.extend((child, False) for child in reversed(node.children) if isinstance(child, Element))
    return hashlib.sha256("\x1f".join(parts).encode("utf-8")).hexdigest()


def _nearest_form(node: Element) -> Element | None:
    current: Element | None = node
    while current is not None:
        if current.tag == "form":
            return current
        current = current.parent
    return None


def _build_form_index(document: Element) -> tuple[list[Element], dict[int, list[Element]]]:
    forms = document.find_all("form")
    form_by_id = {form.attr("id"): form for form in forms if form.attr("id")}
    controls: dict[int, list[Element]] = {id(form): [] for form in forms}
    owners: dict[int, Element | None] = {}
    for node in document.find_all():
        if node.tag not in {"input", "select", "textarea", "button"}:
            continue
        owner = form_by_id.get(node.attr("form")) if node.attr("form") else _nearest_form(node)
        owners[id(node)] = owner
        if owner is not None:
            controls.setdefault(id(owner), []).append(node)
    setattr(document, "_form_index", (forms, controls, owners))
    return forms, controls


def _form_owner(node: Element, document: Element | None = None) -> Element | None:
    if document is not None:
        indexed = getattr(document, "_form_index", None)
        if indexed is not None:
            owners = indexed[2]
            if id(node) in owners:
                return owners[id(node)]
    if node.tag in {"input", "select", "textarea", "button"} and "form" in node.attrs:
        form_id = node.attr("form").strip()
        return document.first("form", element_id=form_id) if document is not None and form_id else None
    return _nearest_form(node)


def _is_form_submitter(node: Element) -> bool:
    kind = node.attr("type").lower()
    return (node.tag == "input" and kind in {"submit", "image"}) or (node.tag == "button" and kind in {"", "submit"})


def _action_submits_form(node: Element, document: Element | None = None) -> bool:
    source = _event_source(node)
    script = f"{source} {node.attr('href')}"
    if node.tag == "form" or _is_form_submitter(node) or re.search(r"\bsubmit\s*\(", script, re.I):
        return True
    if document is not None:
        call = _onclick_call(source or node.attr("href"))
        definition = _function_body(document, call[0]) if call else None
        return bool(definition and re.search(r"\bsubmit\s*\(", definition[1], re.I))
    return False


def _action_nodes(document: Element) -> list[Element]:
    inert = {"", "#", "/", "javascript:void(0)", "javascript:void(0);", "javascript:;"}
    nodes: list[Element] = []
    for node in document.find_all():
        event_source = _event_source(node)
        if _is_inert_event(event_source) or (_is_inert_event(node.attr("href")) and not event_source):
            continue
        if node.is_disabled():
            continue
        form = _form_owner(node, document)
        if form is not None and _is_inert_event(form.attr("onsubmit")):
            continue
        event = any(name in EVENT_ATTRIBUTES for name in node.attrs)
        if node.tag == "a" and (event or node.attr("href") not in inert):
            nodes.append(node)
        elif node.tag in {"input", "button"} and node.attr("type").lower() != "reset" and (
            event
            or (form is not None and (node.tag == "button" or node.attr("type").lower() in {"submit", "button", "image"}))
        ):
            nodes.append(node)
        elif event and node.tag in {"form", "select", "option", "textarea", "td", "tr", "div", "span"}:
            nodes.append(node)
    return nodes


def _event_source(node: Element) -> str:
    return next((node.attr(name) for name in EVENT_ATTRIBUTES if node.attr(name)), "")


def _split_js(value: str, separator: str = ",") -> list[str]:
    parts: list[str] = []
    start = 0
    quote = ""
    escaped = False
    depth = 0
    for index, char in enumerate(value):
        if escaped:
            escaped = False
            continue
        if quote:
            if char == "\\":
                escaped = True
            elif char == quote:
                quote = ""
            continue
        if char in "'\"`":
            quote = char
        elif char in "([{":
            depth += 1
        elif char in ")]}":
            depth = max(0, depth - 1)
        elif char == separator and depth == 0:
            parts.append(value[start:index].strip())
            start = index + 1
    parts.append(value[start:].strip())
    return [part for part in parts if part]


def _js_string(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] in "'\"`" and value[-1] == value[0]:
        try:
            return str(ast.literal_eval(value))
        except (SyntaxError, ValueError):
            return value[1:-1]
    return value


def _js_value(expression: str, variables: dict[str, str]) -> str:
    terms = _split_js(expression, "+")
    values: list[str] = []
    for term in terms:
        term = term.strip()
        if term in variables:
            values.append(variables[term])
        elif re.fullmatch(r"document\.getElementById\([\"'][^\"']+[\"']\)\.value", term):
            key = re.search(r"[\"']([^\"']+)[\"']", term)
            if key is None or key.group(1) not in variables:
                raise CsustError("页面脚本引用了未知字段", code="parse_error")
            values.append(variables[key.group(1)])
        elif re.fullmatch(r"\$\([\"']#[^\"']+[\"']\)\.val\(\)", term):
            key = re.search(r"#[^\"']+", term)
            if key is None or key.group(0)[1:] not in variables:
                raise CsustError("页面脚本引用了未知字段", code="parse_error")
            values.append(variables[key.group(0)[1:]])
        elif term.startswith(("'", '"', "`")) and len(term) >= 2 and term[-1] == term[0] and "${" not in term:
            values.append(_js_string(term))
        elif term.startswith("encodeURIComponent(") and term.endswith(")"):
            values.append(quote(_js_value(term[19:-1], variables), safe="-_.!~*'()"))
        elif re.fullmatch(r"-?\d+(?:\.\d+)?", term):
            values.append(term)
        else:
            raise CsustError("页面脚本包含无法解析的表达式；可使用 web request 显式提供请求参数", code="parse_error")
    return "".join(values)


def _element_value(node: Element) -> str:
    if node.tag == "select":
        options = [option for option in node.find_all("option") if not option.is_disabled()]
        selected = next((option for option in options if "selected" in option.attrs), None)
        if selected is not None:
            return _option_value(selected)
        return "" if "multiple" in node.attrs or not options else _option_value(options[0])
    return _input_value(node)


def _function_body(document: Element, name: str) -> tuple[list[str], str] | None:
    pattern = re.compile(r"function\s+" + re.escape(name) + r"\s*\(([^)]*)\)\s*\{")
    for script in document.find_all("script"):
        source = script.raw_text()
        match = pattern.search(source)
        if not match:
            continue
        start = match.end() - 1
        depth = 0
        quote = ""
        escaped = False
        for index in range(start, len(source)):
            char = source[index]
            if escaped:
                escaped = False
                continue
            if quote:
                if char == "\\":
                    escaped = True
                elif char == quote:
                    quote = ""
                continue
            if char in "'\"`":
                quote = char
            elif char == "{":
                depth += 1
            elif char == "}":
                depth -= 1
                if depth == 0:
                    return [item.strip() for item in match.group(1).split(",") if item.strip()], source[start + 1 : index]
    return None


def _onclick_call(source: str) -> tuple[str, list[str]] | None:
    source = re.sub(r"^\s*javascript:\s*", "", source, flags=re.I)
    ignored = {"if", "alert", "confirm", "return", "void"}
    parentheses: list[tuple[str, int]] = []
    index = 0
    quote = ""
    escaped = False
    while index < len(source):
        char = source[index]
        if escaped:
            escaped = False
            index += 1
            continue
        if quote:
            if char == "\\":
                escaped = True
            elif char == quote:
                quote = ""
            index += 1
            continue
        if char in "'\"`":
            quote = char
        elif char == "(":
            parentheses.append(("", index))
        elif char == ")" and parentheses:
            name, start = parentheses.pop()
            if name and name.lower() not in ignored and not any(parent for parent, _ in parentheses):
                return name, _split_js(source[start + 1 : index])
        elif ("A" <= char <= "Z") or ("a" <= char <= "z") or char in "_$":
            end = index + 1
            while end < len(source) and (("A" <= source[end] <= "Z") or ("a" <= source[end] <= "z") or ("0" <= source[end] <= "9") or source[end] in "_$"):
                end += 1
            cursor = end
            while cursor < len(source) and source[cursor].isspace():
                cursor += 1
            if cursor < len(source) and source[cursor] == "(":
                parentheses.append((source[index:end], cursor))
                index = cursor
        index += 1
    return None


def _resolve_js_action(
    node: Element,
    document: Element,
    page_url: str,
    overrides: dict[str, str] | None = None,
) -> tuple[str, str] | None:
    call = _onclick_call(_event_source(node) or node.attr("href"))
    if call is None:
        return None
    name, arguments = call
    definition = _function_body(document, name)
    if definition is None:
        return None
    formals, body = definition
    variables = dict(overrides or {})
    for formal, argument in zip(formals, arguments):
        if argument.strip() == "this.value":
            variables[formal] = _element_value(node)
        else:
            value = _js_value(argument, variables)
            if not value and argument.strip().startswith(("'", '"', "`")):
                value = _js_string(argument)
            if value or formal not in variables:
                variables[formal] = value

    assignments = re.finditer(
        r"(?:^|[;{}])\s*(?:var\s+|let\s+|const\s+)?([A-Za-z_$][\w$]*)\s*(\+=|=)\s*([^;{}]+)",
        body,
    )
    for assignment in assignments:
        name, operator, value = assignment.group(1), assignment.group(2), _js_value(assignment.group(3), variables)
        if operator == "+=":
            value = variables.get(name, "") + value
        if value or name not in variables:
            variables[name] = value

    expressions: list[tuple[int, str, bool]] = []
    for pattern, navigation in (
        (r"\.action\s*=\s*([^;]+)", False),
        (r"(?:window\.location(?:\.href)?|location(?:\.href)?)\s*=\s*([^;]+)", True),
        (r"(?:window\.)?open\s*\(\s*([^,;)]+)", False),
        (r"(?:openWindow|showWindow|openView)\s*\(\s*([^,;)]+)", False),
        (r"(?:v?JsMod|JsMod)\s*\(\s*([^,;)]+)", False),
        (r"\burl\s*:\s*([^,;}]+)", False),
        (r"\.(?:load|get|post|put|patch|delete)\s*\(\s*([^,;)]+)", False),
        (r"\bfetch\s*\(\s*([^,;)]+)", False),
        (r"\.attr\s*\(\s*[\"']action[\"']\s*,\s*([^,;)]+)", False),
    ):
        expressions.extend((match.start(1), match.group(1).strip(), navigation) for match in re.finditer(pattern, body, re.I))
    expressions.sort(key=lambda item: item[0])
    candidates = expressions
    method_match = re.search(
        r"(?:type|method)\s*:\s*[\"'](GET|POST|PUT|PATCH|DELETE)[\"']|\.(get|post|put|patch|delete)\s*\(|\bfetch\s*\(",
        body,
        re.I,
    )
    method = next((group.upper() for group in (method_match.groups() if method_match else ()) if group), "")
    if not method:
        method = "POST" if re.search(r"\.action\s*=|\.submit\s*\(|type\s*:\s*[\"']post", body, re.I) else "GET"
    for _position, expression, _navigation in candidates:
        target = _js_value(expression, variables)
        if not target.startswith(("/", "http://", "https://")) or target.startswith("javascript:"):
            continue
        return method, _safe_urljoin(page_url, target)
    return None


def _js_state_fields(
    node: Element,
    form: Element | None,
    document: Element,
    overrides: dict[str, str] | None = None,
) -> list[tuple[str, str]]:
    call = _onclick_call(_event_source(node) or node.attr("href"))
    if call is None:
        return []
    definition = _function_body(document, call[0])
    if definition is None:
        return []
    formals, body = definition
    variables = dict(_form_fields(form, node, document)) if form is not None else {}
    variables.update(overrides or {})
    for formal, argument in zip(formals, call[1]):
        if argument.strip() == "this.value":
            variables[formal] = _element_value(node)
        else:
            value = _js_value(argument, variables)
            if not value and argument.strip().startswith(("'", '"', "`")):
                value = _js_string(argument)
            if value or formal not in variables:
                variables[formal] = value
    fields: list[tuple[str, str]] = [
        (match.group(1), _js_value(match.group(2), variables))
        for match in re.finditer(r"getElementById\([\"']([^\"']+)[\"']\)\.value\s*=\s*([^;]+)", body)
    ]
    fields.extend(
        (match.group(1), _js_value(match.group(2), variables))
        for match in re.finditer(r"\$\([\"']#([^\"']+)[\"']\)\.val\(\s*([^)]*)\)", body)
        if match.group(2).strip()
    )
    for data_match in re.finditer(r"\bdata\s*:\s*\{([^{}]*)\}", body, re.I | re.S):
        for item in _split_js(data_match.group(1)):
            if ":" not in item:
                continue
            name, expression = item.split(":", 1)
            name = name.strip().strip("'\"")
            if not name:
                continue
            try:
                fields.append((name, _js_value(expression, variables)))
            except CsustError:
                continue
    return [(name, value) for name, value in fields if name]


def _quoted_action(node: Element, page_url: str) -> tuple[str, str] | None:
    source = (node.attr("href") or "") + " " + " ".join(value for name, value in node.attrs.items() if name in EVENT_ATTRIBUTES)
    if re.search(r"['\"]\s*\+|\+\s*['\"]|\$\{", source):
        if re.search(r"location|window\.open|openWindow|v?JsMod|fetch|ajax|\.(?:load|get|post|put|patch|delete)\s*\(|\.action", source, re.I):
            raise CsustError("内联脚本地址包含动态表达式；请显式提供请求路径", code="parse_error")
    candidates: list[tuple[int, str, str]] = []
    patterns = (
        (r"(?:window\.)?location(?:\.href)?\s*=\s*['\"](?P<target>[^'\"]+)['\"]", "GET"),
        (r"(?:window\.)?(?:open|openWindow|showWindow|openView|v?JsMod)\s*\(\s*['\"](?P<target>[^'\"]+)['\"]", "GET"),
        (r"fetch\s*\(\s*['\"](?P<target>[^'\"]+)['\"]", "GET"),
        (r"url\s*:\s*['\"](?P<target>[^'\"]+)['\"]", ""),
        (r"\.(?P<method>load|get|post|put|patch|delete)\s*\(\s*['\"](?P<target>[^'\"]+)['\"]", ""),
        (r"(?:\.attr\s*\(\s*['\"]action['\"]\s*,|\b[A-Za-z_$][\w$]*\s*\.action\s*=)\s*['\"](?P<target>[^'\"]+)['\"]", "POST"),
    )
    for pattern, default_method in patterns:
        for match in re.finditer(pattern, source, re.I):
            method = str(match.groupdict().get("method") or default_method).upper()
            if not method:
                method_match = re.search(r"\b(?:method|type)\s*[:=]\s*['\"]?(GET|POST|PUT|PATCH|DELETE)", source, re.I)
                method = method_match.group(1).upper() if method_match else "GET"
            candidates.append((match.start(), method, match.group("target")))
    for _position, method, value in sorted(candidates):
        if value.startswith(("/", "http://", "https://")) and not value.startswith("//"):
            return method, _safe_urljoin(page_url, value)
    return None


def _action_target(
    node: Element,
    form: Element | None,
    page_url: str,
    document: Element | None = None,
    overrides: dict[str, str] | None = None,
) -> tuple[str, str]:
    formaction = node.attr("formaction")
    formmethod = node.attr("formmethod")
    if document is not None and _event_source(node) and not _is_inert_event(_event_source(node)):
        variables = dict(_form_fields(form, node, document)) if form is not None else {}
        variables.update(overrides or {})
        resolved = _resolve_js_action(node, document, page_url, variables)
        if resolved:
            return resolved
    if formaction and _action_submits_form(node, document):
        method = formmethod or (form.attr("method", "GET") if form is not None else "GET")
        return method.upper(), _safe_urljoin(page_url, formaction)
    href = node.attr("href")
    if href and not href.lower().startswith("javascript:") and href not in {"#", "javascript:void(0)", "javascript:void(0);", "javascript:;"}:
        return "GET", _safe_urljoin(page_url, href)
    if document is not None:
        variables = dict(_form_fields(form, node, document)) if form is not None else {}
        variables.update(overrides or {})
        resolved = _resolve_js_action(node, document, page_url, variables)
        if resolved:
            return resolved
    quoted = _quoted_action(node, page_url)
    if quoted:
        return quoted
    if form is None or not _action_submits_form(node, document):
        return "GET", ""
    return (formmethod or form.attr("method", "GET")).upper(), _safe_urljoin(page_url, form.attr("action") or page_url)


def _form_fields(
    form: Element | None,
    action_node: Element | None = None,
    document: Element | None = None,
) -> list[tuple[str, str]]:
    if form is None:
        return []
    fields: list[tuple[str, str]] = []
    if document is not None:
        indexed = getattr(document, "_form_index", None)
        nodes = indexed[1].get(id(form), []) if indexed is not None else document.find_all()
    else:
        nodes = form.find_all()
    for node in nodes:
        if node.tag not in {"input", "select", "textarea", "button"}:
            continue
        if document is not None and _form_owner(node, document) is not form:
            continue
        if node is action_node and node.is_disabled():
            continue
        if node.tag == "input":
            name = node.attr("name")
            if node.is_disabled():
                continue
            kind = node.attr("type", "text").lower()
            if kind == "image" and node is action_node:
                fields.extend(_image_fields(node))
                continue
            if not name:
                continue
            if kind in {"submit", "button", "image", "reset", "file"} and node is not action_node:
                continue
            if kind in {"checkbox", "radio"} and "checked" not in node.attrs:
                continue
            fields.append((name, _input_value(node)))
        elif node.tag == "select":
            name = node.attr("name")
            if not name or node.is_disabled():
                continue
            options = [option for option in node.find_all("option") if not option.is_disabled()]
            selected = [option for option in options if "selected" in option.attrs]
            if not selected and options and "multiple" not in node.attrs:
                selected = [options[0]]
            if "multiple" in node.attrs:
                fields.extend((name, _option_value(option)) for option in selected)
            elif selected:
                fields.append((name, _option_value(selected[0])))
        elif node.tag == "textarea" and node.attr("name") and not node.is_disabled():
            fields.append((node.attr("name"), node.raw_text()))
        elif node is action_node and node.tag == "button" and node.attr("name"):
            fields.append((node.attr("name"), node.attr("value") if "value" in node.attrs else ""))
    return fields


def _form_file_names(form: Element | None, document: Element | None = None) -> set[str]:
    if form is None:
        return set()
    indexed = getattr(document, "_form_index", None) if document is not None else None
    nodes = indexed[1].get(id(form), []) if indexed is not None else form.find_all()
    return {node.attr("name") for node in nodes if node.tag == "input" and node.attr("type").lower() == "file" and node.attr("name") and not node.is_disabled()}


def _parse_upload_files(values: list[str]) -> list[tuple[str, object]]:
    from .vpn import _parse_files

    return _parse_files(values)


def _safe_arguments(source: str) -> list[str]:
    call = _onclick_call(source)
    if not call:
        return []
    if call[0].lower() == "towptjbs":
        return ["<redacted>"] * len(call[1])
    return [_safe_event(value) for value in call[1]]


def _action_ref(node: Element, method: str, target: str, form: Element | None, fields: list[str]) -> str:
    """Build a position-independent reference from the action's semantics."""
    form_key = "|".join(fields) if fields else (form.attr("name") or form.attr("id") if form else "")
    target_path = ""
    if node.tag == "a" and target:
        try:
            target_path = urlparse(target).path
        except ValueError:
            target_path = ""
    label = _display_text(node) or node.attr("value")
    name = node.attr("name")
    aria_label = node.attr("aria-label")
    node_id = node.attr("id") if not (name or label or aria_label) else ""
    call = _onclick_call(_event_source(node) or node.attr("href"))
    identity = "\x1f".join(
        (
            node.tag,
            _semantic_text(node.attr("type")),
            _semantic_text(name),
            _semantic_text(node_id),
            _semantic_text(node.attr("role")),
            _semantic_text(aria_label),
            _semantic_text(label),
            _semantic_text(call[0] if call else ""),
            method.upper(),
            _semantic_text(form_key),
            target_path,
        )
    )
    return "action:" + hashlib.sha256(identity.encode("utf-8")).hexdigest()[:16]


def _describe_action(
    node: Element,
    page_url: str,
    index: int,
    document: Element | None = None,
    overrides: dict[str, str] | None = None,
) -> dict[str, object]:
    form = _form_owner(node, document)
    submits_form = _action_submits_form(node, document)
    parse_error = ""
    try:
        method, target = _action_target(node, form, page_url, document, overrides)
        state = (
            _js_state_fields(node, form, document, {**dict(_form_fields(form, node, document)), **(overrides or {})})
            if document is not None else []
        )
    except CsustError as exc:
        if exc.code != "parse_error":
            raise
        method, target, state = "", "", []
        parse_error = str(exc)
    form_index = 0
    if document is not None and form is not None:
        forms = getattr(document, "_form_index", (document.find_all("form"), {}, {}))[0]
        form_index = forms.index(form) + 1 if form in forms else 0
    fields = sorted(
        ({name for name, _ in _form_fields(form, node, document)} if submits_form else set())
        | {name for name, _ in state}
    )
    text = "" if _sensitive_control(node) else (_display_text(node) or _safe_event(node.attr("value")))
    return {
        **({"parse_error": parse_error} if parse_error else {}),
        "index": index,
        "ref": _action_ref(node, method, target, form, fields),
        "text": text,
        "tag": node.tag,
        "method": method,
        "target": _safe_url(target, page_url),
        "name": node.attr("name"),
        "id": node.attr("id"),
        "role": node.attr("role"),
        "aria_label": _safe_event(node.attr("aria-label")),
        "form": form.attr("id") if form else "",
        "form_index": form_index,
        "fields": fields,
        "state_fields": [name for name, _ in state],
        "href": _safe_url(node.attr("href"), page_url),
        "onclick": _safe_event(node.attr("onclick")),
        "events": {name: _safe_event(value) for name, value in node.attrs.items() if name in EVENT_ATTRIBUTES},
        "arguments": _safe_arguments(_event_source(node) or node.attr("href")),
    }


def _script_messages(document: Element) -> list[str]:
    messages = []
    for script in document.find_all("script"):
        for statement in _split_js(script.raw_text(), ";"):
            match = re.fullmatch(r"\s*(?:window\.)?(?:alert|showMsg)\s*\(\s*(['\"])(.*?)\1\s*\)\s*", statement, re.S)
            if match:
                messages.append(_safe_event(match.group(2).strip()))
    return list(dict.fromkeys(message for message in messages if message))


def _inspect_document(document: Element, source: str, page_url: str) -> dict[str, object]:
    forms_index, controls_index = _build_form_index(document)
    messages = _script_messages(document)
    title = document.first("title")
    links = [_link(node, page_url) for node in document.find_all("a") if node.attr("href") or node.attr("onclick")]
    forms: list[dict[str, object]] = []
    for index, form in enumerate(forms_index, start=1):
        controls = controls_index.get(id(form), [])
        fields: list[dict[str, object]] = []
        for node in controls:
            field = _control(node)
            label = _control_label(document, node)
            if label:
                field["label"] = label
            fields.append(field)
        forms.append(
            {
                "index": index,
                "action": _safe_url(_safe_urljoin(page_url, form.attr("action") or page_url), page_url),
                "method": form.attr("method", "get").upper(),
                "name": form.attr("name"),
                "id": form.attr("id"),
                "enctype": form.attr("enctype", "application/x-www-form-urlencoded"),
                "fields": fields,
            }
        )
    tables: list[dict[str, object]] = []
    for table in document.find_all("table"):
        table_rows = _table_rows(table)
        rows: list[list[str]] = []
        for row in table_rows:
            cells = row.direct("th") + row.direct("td")
            if cells:
                rows.append([_display_text(cell) for cell in cells])
        if rows:
            table_data: dict[str, object] = {"id": table.attr("id"), "class": table.attr("class"), "rows": rows}
            header_cells = table_rows[0].direct("th") if table_rows else []
            if header_cells:
                table_data["headers"] = [_display_text(cell) for cell in header_cells]
                table_data["data_rows"] = rows[1:]
            tables.append(table_data)
    actions = [
        _describe_action(node, page_url, index, document)
        for index, node in enumerate(_action_nodes(document), start=1)
    ]
    functions: list[str] = []
    endpoints: list[str] = []
    for script in document.find_all("script"):
        script_source = script.raw_text()
        functions.extend(re.findall(r"function\s+([A-Za-z_$][\w$]*)\s*\(", script_source))
        endpoints.extend(
            _safe_url(value, page_url)
            for value in re.findall(r"['\"]((?:/jsxsd|https?://)[^'\"\s<>]*)['\"]", script_source)
        )
        endpoints.extend(
            _safe_url(value, page_url)
            for value in re.findall(r"['\"]((?:/meol|/moocresource)[^'\"\s<>]*)['\"]", script_source)
        )
    text = _display_text(document)[:12000]
    script_sources = [_safe_url(node.attr("src"), page_url) for node in document.find_all("script") if node.attr("src")]
    kind = "dynamic" if document.find_all("script") and not any((links, forms, tables, actions)) and not text else "html"
    capabilities = [
        name
        for name, present in (
            ("links", bool(links)),
            ("forms", bool(forms)),
            ("tables", bool(tables)),
            ("actions", bool(actions)),
            ("api_candidates", bool(endpoints)),
            ("dynamic", kind == "dynamic"),
        )
        if present
    ]
    return {
        "schema_version": PAGE_SNAPSHOT_SCHEMA,
        "kind": kind,
        "url": _safe_url(page_url),
        "fingerprint": hashlib.sha256(source.encode("utf-8", errors="replace")).hexdigest(),
        "shape_fingerprint": _shape_fingerprint(document),
        "title": _display_text(title) if title else "",
        "text": text,
        "capabilities": capabilities,
        "messages": messages,
        "links": links,
        "forms": forms,
        "tables": tables,
        "actions": actions,
        "scripts": {"src": script_sources, "functions": sorted(set(functions))},
        "endpoints": sorted(set(endpoints)),
    }


def inspect_page(source: str, page_url: str) -> dict[str, object]:
    return _inspect_document(parse_html(source), source, page_url)


def _response_text(response: Response) -> str:
    return _decode_body(response.body, response.headers)


def _write_download(response: Response, output: str, *, require_session: bool, mutating: bool = False) -> dict[str, object]:
    body = response.body
    if isinstance(body, str):
        body = body.encode("utf-8")
    try:
        if require_session and is_login_page(Response(response.url, response.status, response.headers, _response_text(response))):
            raise CsustError("下载响应是登录页，会话可能已失效", code="login_required")
        if not output or not output.strip():
            raise CsustError("下载路径不能为空", code="invalid_argument")
        try:
            target = Path(output).expanduser()
        except (OSError, RuntimeError, ValueError) as exc:
            raise CsustError("下载路径无效", code="file_write_failed") from exc
        payload = _feedback(response)
        if business_state(payload) is False:
            result_status(payload, mutating=mutating)
        streamed = response.stream_temp_path is not None or response.stream_path == str(target)
        if streamed:
            _commit_stream(response)
        else:
            _write_private_file(target, body, code="file_write_failed", label="下载文件")
        byte_count = target.stat().st_size if streamed else len(body)
        content_type = _header_value(response.headers, "Content-Type")
        return {"ok": True, "downloaded": True, "output": str(target), "bytes": byte_count, "content_type": content_type, "url": _safe_url(response.url)}
    except BaseException:
        _discard_stream(response)
        raise


def _request(
    client: Client,
    method: str,
    target: str,
    data: list[tuple[str, str]] | None = None,
    *,
    referer: str = "",
    output: str | None = None,
    require_session: bool = True,
    multipart: list[tuple[str, object]] | None = None,
    mutating: bool | None = None,
    headers: dict[str, str] | None = None,
    json_body: object = _BODY_UNSET,
    _retry_auth: bool = True,
) -> tuple[Response, dict[str, object] | None]:
    method = _supported_method(method)
    if multipart is not None and method not in READ_ONLY_METHODS:
        multipart = [*(data or []), *multipart]
        data = None
    request_headers = dict(headers or {})
    if referer:
        try:
            same_origin = _url_origin(referer) == _url_origin(target)
        except ValueError:
            same_origin = False
        if same_origin:
            request_headers["Referer"] = referer
    request_options: dict[str, object] = {"headers": request_headers or None}
    if json_body is not _BODY_UNSET:
        if data is not None or multipart is not None:
            raise CsustError("不能同时使用表单、multipart 和 JSON 参数", code="invalid_argument")
        request_options["json_body"] = json_body
    request_target = target
    if (data or multipart) and method in READ_ONLY_METHODS:
        request_target = _append_query(request_target, data)
    effective_mutating = method not in READ_ONLY_METHODS if mutating is None else mutating
    quality_request = getattr(client, "request_web", None)
    if callable(quality_request):
        if json_body is not _BODY_UNSET or headers:
            raise CsustError("当前网页网关不支持自定义 JSON/请求头", code="invalid_argument")
        try:
            result = _retry_read(
                method,
                request_target,
                lambda: quality_request(
                    request_target,
                    method=method,
                    data=data if method not in READ_ONLY_METHODS else None,
                    multipart=multipart if method not in READ_ONLY_METHODS else None,
                    output=output or False,
                    referer=referer if request_headers else "",
                    mutating=effective_mutating,
                ),
            )
        except (HttpError, NetworkError) as exc:
            if require_session and _retry_auth and not effective_mutating and method in READ_ONLY_METHODS and isinstance(exc, HttpError) and exc.status == 401:
                _ensure_web_session(client)
                return _request(client, method, target, data, referer=referer, output=output, require_session=require_session, multipart=multipart, mutating=mutating, headers=headers, json_body=json_body, _retry_auth=False)
            if not effective_mutating:
                raise
            raise _mutation_transport_error(exc, {"method": method, "path": urlparse(target).path}) from exc
        assert isinstance(result, Response)
        try:
            if require_session:
                if effective_mutating and is_login_page(result):
                    raise MutationUnverified(
                        "网页写请求已发送但返回登录页，结果未知",
                        details={"submitted": True, "confirmed": False, "request": {"method": method, "path": urlparse(target).path}, "cause": "login_required"},
                    )
                require_logged_in(result)
            if not effective_mutating:
                result_status(_feedback(result), mutating=False)
            if output is not None:
                return result, _write_download(result, output, require_session=False, mutating=effective_mutating)
            return result, None
        except BaseException:
            _discard_stream(result)
            raise
    if output is not None:
        try:
            result = _retry_read(
                method,
                request_target,
                lambda: client.request(
                    request_target,
                    method=method,
                    data=data if method not in READ_ONLY_METHODS else None,
                    multipart=multipart if method not in READ_ONLY_METHODS else None,
                    **request_options,
                    binary=True,
                    with_metadata=True,
                    stream_to=output,
                    defer_stream_commit=True,
                ),
            )
        except (HttpError, NetworkError) as exc:
            if require_session and _retry_auth and not effective_mutating and method in READ_ONLY_METHODS and isinstance(exc, HttpError) and exc.status == 401:
                _ensure_web_session(client)
                return _request(client, method, target, data, referer=referer, output=output, require_session=require_session, multipart=multipart, mutating=mutating, headers=headers, json_body=json_body, _retry_auth=False)
            if not effective_mutating:
                raise
            raise _mutation_transport_error(exc, {"method": method, "path": urlparse(target).path}) from exc
        assert isinstance(result, Response)
        try:
            if require_session:
                if effective_mutating and is_login_page(result):
                    raise MutationUnverified(
                        "网页写请求已发送但返回登录页，结果未知",
                        details={"submitted": True, "confirmed": False, "request": {"method": method, "path": urlparse(target).path}, "cause": "login_required"},
                    )
                require_logged_in(result)
            if not effective_mutating:
                _save_cookie_refresh(client, result)
            try:
                saved = _write_download(result, output, require_session=False, mutating=effective_mutating)
            except CsustError:
                if effective_mutating:
                    _save_cookie_refresh(client, result)
                raise
            return result, saved
        except BaseException:
            _discard_stream(result)
            raise
    try:
        result = _retry_read(
            method,
            request_target,
            lambda: client.request(
                request_target,
                method=method,
                data=data if method not in READ_ONLY_METHODS and multipart is None else None,
                multipart=multipart if method not in READ_ONLY_METHODS else None,
                **request_options,
            ),
        )
    except (HttpError, NetworkError) as exc:
        if require_session and _retry_auth and not effective_mutating and method in READ_ONLY_METHODS and isinstance(exc, HttpError) and exc.status == 401:
            _ensure_web_session(client)
            return _request(client, method, target, data, referer=referer, output=output, require_session=require_session, multipart=multipart, mutating=mutating, headers=headers, json_body=json_body, _retry_auth=False)
        if not effective_mutating:
            raise
        raise _mutation_transport_error(exc, {"method": method, "path": urlparse(target).path}) from exc
    assert isinstance(result, Response)
    if require_session:
        if effective_mutating and is_login_page(result):
            raise MutationUnverified(
                "网页写请求已发送但返回登录页，结果未知",
                details={"submitted": True, "confirmed": False, "request": {"method": method, "path": urlparse(target).path}, "cause": "login_required"},
            )
        require_logged_in(result)
    if not effective_mutating:
        _save_cookie_refresh(client, result)
        result_status(_feedback(result), mutating=False)
    return result, None


def _get_page(client: Client, path: str, params: list[tuple[str, str]], *, public: bool = False) -> tuple[Response, dict[str, object]]:
    target = _public_target(client, path, params) if public else _target(client, path, params)
    if not public:
        quality_session = getattr(client, "ensure_quality_session", None)
        if callable(quality_session):
            quality_session()
        else:
            _ensure_web_session(client)
    quality_request = getattr(client, "request_web", None)
    response = _retry_read("GET", target, lambda: quality_request(target) if callable(quality_request) else client.get(target))
    if not public and not getattr(client, "allow_anonymous_pages", False):
        require_logged_in(response)
    _save_cookie_refresh(client, response)
    result_status(_feedback(response), mutating=False)
    page = _page_payload(response)
    if not isinstance(page, dict):
        raise CsustError("页面响应不是可操作的 HTML 页面", code="parse_error")
    return response, page


def _get(client: Client, path: str, params: list[tuple[str, str]], *, public: bool = False) -> dict[str, object]:
    _response, page = _get_page(client, path, params, public=public)
    return page


def _feedback(response: Response) -> object:
    text = _response_text(response)
    if not text.strip():
        return None
    content_type = _header_value(response.headers, "Content-Type").lower()
    if isinstance(response.body, bytes) and content_type and not any(kind in content_type for kind in ("text/", "json", "xml")):
        return None
    if "json" in content_type or text.lstrip().startswith(("{", "[")):
        try:
            return _redact_payload(json.loads(text))
        except ValueError as exc:
            raise CsustError("远端返回了无效 JSON", code="parse_error") from exc
    if "html" in content_type or text.lstrip().startswith("<"):
        return _inspect_document(_response_document(response), text, response.url)
    return text


def _logout_result(response: Response) -> dict[str, object]:
    """Require a business or HTML signal before claiming remote logout."""
    details = {"status": response.status}
    if response.status == 204:
        return {"ok": True, "logged_out": True, **details, "response": None}
    if response.status >= 400:
        raise CsustError("远端退出请求失败", code="business_rejected", details=details)
    if is_login_page(response):
        return {"ok": True, "logged_out": True, **details, "response": {"logged_out": True, "evidence": "login_page"}}
    payload = _feedback(response)
    state = business_state(payload)
    if state is None and re.search(r"(?:退出|注销|登出).*(?:成功|完成)|(?:成功|完成).*(?:退出|注销|登出)|logged\s*out|logout\s+(?:success|ok)", _response_text(response), re.I | re.S):
        payload = {"ok": True, "logged_out": True, "evidence": "html_message"}
    status = result_status(payload, mutating=True, details=details)
    return {**status, "logged_out": True, **details, "response": payload}


def _redact_payload(value: object) -> object:
    if isinstance(value, dict):
        return {str(key): "<redacted>" if _SENSITIVE_FIELD.search(str(key)) else _redact_payload(item) for key, item in value.items()}
    if isinstance(value, list):
        return [_redact_payload(item) for item in value]
    if isinstance(value, str) and ("?" in value or ";" in value or value.startswith(("http://", "https://"))):
        return _safe_url(value)
    return value


def _response_document(response: Response) -> Element:
    document = getattr(response, "_csust_document", None)
    if isinstance(document, Element):
        return document
    document = parse_html(_response_text(response))
    setattr(response, "_csust_document", document)
    return document


def _page_payload(response: Response) -> object:
    text = _response_text(response)
    if not text.strip():
        raise CsustError("页面响应为空", code="parse_error")
    content_type = _header_value(response.headers, "Content-Type").lower()
    if "json" in content_type or text.lstrip().startswith(("{", "[")):
        return _feedback(response)
    document = _response_document(response)
    return _inspect_document(document, text, response.url)


def _mutation_verified(page: dict[str, object]) -> bool:
    return business_state(page) is True


def _mutation_result(response: Response, page: dict[str, object], request: dict[str, object], *, action: dict[str, object] | None = None) -> dict[str, object]:
    details = {"request": request, **({"action": action} if action else {})}
    return {**result_status(_feedback(response), mutating=True, details=details), **details, "page": page}


def _save_mutation(client: Client, result: dict[str, object]) -> None:
    try:
        client.save()
    except CsustError as exc:
        raise MutationUnverified(
            "远端写请求已完成但本地会话保存失败，结果未知",
            details={
                "submitted": True,
                "confirmed": False,
                "request": result,
                **result,
                "ok": False,
                "save_error": exc.code,
                "verification": "unknown",
            },
        ) from exc


def _mutation_transport_error(exc: CsustError, request: dict[str, object]) -> MutationUnverified:
    return MutationUnverified(
        "网页写请求已发送但结果未知",
        details={"submitted": True, "confirmed": False, "request": request, "cause": exc.code},
    )


def _download_result(response: Response, saved: dict[str, object], mutating: bool) -> dict[str, object]:
    payload = _feedback(response)
    return {**saved, **result_status(payload, mutating=mutating, details=saved)}


def _discovered_routes(page: dict[str, object]) -> list[dict[str, object]]:
    routes: list[dict[str, object]] = []
    seen: set[tuple[str, str]] = set()
    page_url = str(page.get("url") or "")
    for link in page.get("links", []):
        if not isinstance(link, dict):
            continue
        target = str(link.get("path") or "")
        if not target.startswith("/jsxsd/"):
            continue
        item = {"name": link.get("text") or "", "path": target, "method": "GET", "source": "link"}
        key = (str(item["name"]), target)
        if key not in seen:
            seen.add(key)
            routes.append(item)
    for action in page.get("actions", []):
        if not isinstance(action, dict) or str(action.get("method") or "GET").upper() != "GET":
            continue
        target = str(action.get("target") or "")
        if not target.startswith(("/jsxsd/", "http://", "https://")):
            continue
        if target.startswith(("http://", "https://")):
            try:
                if _url_origin(target) != _url_origin(page_url):
                    continue
            except ValueError:
                continue
        item = {
            "name": action.get("text") or "",
            "path": target,
            "method": "GET",
            "ref": action.get("ref") or "",
            "source": "action",
        }
        key = (str(item["name"]), target)
        if key not in seen:
            seen.add(key)
            routes.append(item)
    return routes


def _resolve_route_name(client: Client, name: str) -> str:
    requested = _semantic_text(name)
    if not requested:
        raise CsustError("页面名称不能为空", code="invalid_argument")
    response, page = _get_page(client, "/jsxsd/framework/xsMain.jsp", [])
    known_labels = {
        _semantic_text(command): label
        for command, _group, label, _path in ROUTE_CATALOG
    }
    label = known_labels.get(requested, name)
    wanted = _semantic_text(label)
    candidates = [
        route
        for route in _discovered_routes(page)
        if _semantic_text(route.get("name")) == wanted
    ]
    if not candidates and label != name:
        candidates = [
            route
            for route in _discovered_routes(page)
            if wanted and wanted in _semantic_text(route.get("name"))
        ]
    if len(candidates) > 1:
        raise CsustError(
            "页面名称对应多个入口，请改用 --path",
            code="ambiguous_target",
            details={"name": name, "candidates": candidates, "url": _safe_url(response.url)},
        )
    if not candidates:
        raise CsustError(
            "当前页面未发现该入口",
            code="route_not_found",
            details={"name": name, "url": _safe_url(response.url)},
        )
    return str(candidates[0]["path"])


def _run_routes(_args: argparse.Namespace, client: Client) -> dict[str, object]:
    response, page = _get_page(client, _args.path, [])
    discovered = _discovered_routes(page)
    group_codes = {name: (top, code) for name, top, code in SECOND_LEVEL_CATALOG}
    catalog = [
        {"command": command, "group": group, "group_code": group_codes[group][1], "menu": group_codes[group][0], "name": label, "path": path}
        for command, group, label, path in ROUTE_CATALOG
    ]
    return {
        "url": _safe_url(response.url),
        "menus": [{"command": c, "name": n, "code": code} for c, n, code in MAIN_MENU_CATALOG],
        "groups": [{"name": n, "menu": m, "code": c} for n, m, c in SECOND_LEVEL_CATALOG],
        "catalog": catalog,
        "known": [{"name": command, "title": label, "path": path} for command, _group, label, path in ROUTE_CATALOG],
        "public": [{"command": command, "name": label, "path": path} for command, label, path in PUBLIC_CATALOG],
        "conditional": [{"command": "graduation-design", "name": "毕业设计", "kind": "external-sso", "path": "https://oauth.fanyu.com/sso/cas/10536/1004"}],
        "discovered": discovered,
        "page": page,
    }


def _run_get(args: argparse.Namespace, client: Client) -> dict[str, object]:
    params = _pairs(args.param, "--param")
    name = getattr(args, "name", None)
    path = _resolve_route_name(client, name) if name is not None else args.path
    target = _target(client, path, params)
    ensure_session(client)
    response, saved = _request(client, "GET", target, require_session=True, output=args.output)
    if saved:
        return saved
    return _page_payload(response)


def _run_post(args: argparse.Namespace, client: Client) -> dict[str, object]:
    if not args.yes:
        raise CsustError("通用 POST 可能修改账号数据，请加 --yes", code="confirmation_required")
    data = _pairs(args.data, "--data")
    files = _parse_upload_files(getattr(args, "file", []))
    target = _target(client, args.path, _pairs(args.param, "--param"))
    ensure_session(client)
    response, saved = _request(client, "POST", target, data, require_session=True, output=args.output, multipart=files or None, mutating=True)
    request = {"method": "POST", "path": urlparse(response.url).path, "fields": [name for name, _ in data]}
    _save_mutation(client, request)
    if saved:
        return _download_result(response, {**saved, "request": request}, True)
    page = _page_payload(response)
    return _mutation_result(response, page, request)


def _select_form(document: Element, index: int) -> Element:
    forms = document.find_all("form")
    if not 1 <= index <= len(forms):
        raise CsustError("表单序号超出页面范围", code="form_not_found")
    return forms[index - 1]


def _form_button(form: Element, index: int | None, document: Element | None = None) -> Element | None:
    nodes = document.find_all() if document is not None else form.find_all()
    buttons = [
        node
        for node in nodes
        if node.tag in {"input", "button"}
        if document is None or _form_owner(node, document) is form
        if node.attr("type").lower() != "reset"
        if not node.is_disabled()
        and not _is_inert_event(node.attr("onclick"))
        and (
            _is_form_submitter(node)
            or any(name in EVENT_ATTRIBUTES for name in node.attrs)
        )
    ]
    if index is None:
        return None
    if not 1 <= index <= len(buttons):
        raise CsustError("表单按钮序号超出页面范围", code="button_not_found")
    return buttons[index - 1]


def _run_form(args: argparse.Namespace, client: Client, path: str, *, public: bool = False) -> dict[str, object]:
    overrides = _pairs(args.data, "--data")
    if args.form < 1:
        raise CsustError("表单序号必须是正整数", code="form_not_found")
    if args.button is not None and args.button < 1:
        raise CsustError("表单按钮序号必须是正整数", code="button_not_found")
    response, discovery_page = _get_page(client, path, _pairs(args.param, "--param"), public=public)
    expected_fingerprint = getattr(args, "fingerprint", None)
    if expected_fingerprint and expected_fingerprint != discovery_page.get("fingerprint"):
        raise CsustError("页面已变化，操作序号已失效；请重新执行 web get", code="stale_page")
    document = parse_html(_response_text(response))
    _build_form_index(document)
    form = _select_form(document, args.form)
    if _is_inert_event(form.attr("onsubmit")):
        raise CsustError("表单提交被页面脚本禁用", code="action_blocked")
    button = _form_button(form, args.button, document)
    override_map = dict(overrides)
    if button is None:
        method, target = form.attr("method", "GET").upper(), _safe_urljoin(response.url, form.attr("action") or response.url)
    else:
        method, target = _action_target(button, form, response.url, document, override_map)
    method = _supported_method(method)
    if not target:
        raise CsustError("表单按钮没有可解析的目标", code="button_not_found")
    target = _public_target(client, target) if public else _allowed_target(client, target)
    data = _form_fields(form, button, document) if button is None or _action_submits_form(button, document) else []
    if button is not None and _action_submits_form(button, document):
        state = _js_state_fields(button, form, document, override_map)
        state_names = {name for name, _ in state}
        data = [(name, value) for name, value in data if name not in state_names] + state
    override_names = {name for name, _ in overrides}
    data = [(name, value) for name, value in data if name not in override_names] + overrides
    files = _parse_upload_files(getattr(args, "file", []))
    required_files = _form_file_names(form, document)
    if required_files - {name for name, _value in files}:
        raise CsustError("表单包含文件字段，请使用 --file NAME=PATH 提供文件", code="invalid_argument")
    mutating = method not in READ_ONLY_METHODS or _is_side_effect_get(target)
    if mutating and not args.yes:
        raise CsustError("提交网页表单可能修改账号数据，请加 --yes", code="confirmation_required")
    result, saved = _request(client, method, target, data, referer=response.url, output=args.output, require_session=not public and getattr(args, "require_login", True), multipart=files or None, mutating=mutating)
    mutation_request = {"method": method, "path": urlparse(result.url).path, "fields": [name for name, _ in data]}
    if mutating:
        _save_mutation(client, mutation_request)
    if saved:
        return _download_result(result, saved, mutating)
    page = _page_payload(result)
    if not mutating:
        return {"ok": True, "submitted": False, "confirmed": True, "form": args.form, "page": page}
    return _mutation_result(result, page, mutation_request)


def _run_action_common(args: argparse.Namespace, client: Client, path: str, *, public: bool = False) -> dict[str, object]:
    overrides = _pairs(args.data, "--data")
    index = getattr(args, "index", None)
    ref = str(getattr(args, "ref", "") or "").strip()
    if (index is None) == (not ref):
        raise CsustError("必须且只能指定 --index 或 --ref", code="invalid_argument")
    if index is not None and index < 1:
        raise CsustError("操作序号必须是正整数", code="action_not_found")
    response, discovery_page = _get_page(client, path, _pairs(args.param, "--param"), public=public)
    expected_fingerprint = getattr(args, "fingerprint", None)
    if index is not None and not expected_fingerprint:
        raise CsustError("执行页面动作必须提供页面快照 --fingerprint", code="fingerprint_required")
    if expected_fingerprint and expected_fingerprint != discovery_page.get("fingerprint"):
        raise CsustError("页面已变化，操作序号已失效；请重新执行 web get", code="stale_page")
    document = _response_document(response)
    _build_form_index(document)
    nodes = _action_nodes(document)
    selected_index: int
    if index is not None:
        if not 1 <= index <= len(nodes):
            raise CsustError("操作序号超出页面范围", code="action_not_found")
        selected_index = index - 1
    else:
        candidates = [
            (candidate_index, _describe_action(node, response.url, candidate_index + 1, document))
            for candidate_index, node in enumerate(nodes)
        ]
        matches = [(candidate_index, descriptor) for candidate_index, descriptor in candidates if descriptor.get("ref") == ref]
        if len(matches) > 1:
            raise CsustError(
                "页面动作 ref 不唯一，请使用 --index 配合最新 fingerprint",
                code="ambiguous_target",
                details={"ref": ref, "candidates": [descriptor for _candidate_index, descriptor in matches]},
            )
        if not matches:
            raise CsustError(
                "页面动作 ref 已失效，请重新执行 web get",
                code="stale_page",
                details={"ref": ref},
            )
        selected_index = matches[0][0]
    node = nodes[selected_index]
    form = _form_owner(node, document)
    override_map = dict(overrides)
    method, target = _action_target(node, form, response.url, document, override_map)
    method = _supported_method(method)
    if not target:
        raise CsustError("页面动作没有可解析的目标", code="action_not_found")
    target = _public_target(client, target) if public else _allowed_target(client, target)
    if method not in READ_ONLY_METHODS and not args.yes:
        raise CsustError("网页操作可能修改账号数据，请加 --yes", code="confirmation_required")
    submits_form = _action_submits_form(node, document)
    data = _form_fields(form, node, document) if submits_form else []
    state = _js_state_fields(node, form, document, override_map)
    state_names = {name for name, _ in state}
    data = [(name, value) for name, value in data if name not in state_names]
    data.extend(state)
    override_names = {name for name, _ in overrides}
    data = [(name, value) for name, value in data if name not in override_names] + overrides
    files = _parse_upload_files(getattr(args, "file", []))
    required_files = _form_file_names(form, document) if submits_form else set()
    if required_files - {name for name, _value in files}:
        raise CsustError("表单包含文件字段，请使用 --file NAME=PATH 提供文件", code="invalid_argument")
    mutating = method not in READ_ONLY_METHODS or _is_side_effect_get(target)
    if mutating and not args.yes:
        raise CsustError("网页操作可能修改账号数据，请加 --yes", code="confirmation_required")
    result, saved = _request(client, method, target, data, referer=response.url, output=args.output, require_session=not public and getattr(args, "require_login", True), multipart=files or None, mutating=mutating)
    descriptor = _describe_action(node, response.url, selected_index + 1, document, override_map)
    mutation_request = {"method": method, "path": urlparse(result.url).path, "fields": [name for name, _ in data]}
    if mutating:
        _save_mutation(client, mutation_request)
    if saved:
        return _download_result(result, {**saved, "action": descriptor}, mutating)
    page = _page_payload(result)
    if not mutating:
        return {"ok": True, "submitted": False, "confirmed": True, "action": descriptor, "page": page}
    return _mutation_result(result, page, mutation_request, action=descriptor)


def _run_action(args: argparse.Namespace, client: Client) -> dict[str, object]:
    return _run_action_common(args, client, args.path)


def _run_route(args: argparse.Namespace, client: Client) -> dict[str, object]:
    if args.form is not None and args.action is not None:
        raise CsustError("--form 与 --action 不能同时使用", code="invalid_argument")
    if args.action is not None and args.button is not None:
        raise CsustError("--button 只能与 --form 一起使用", code="invalid_argument")
    if args.form is not None:
        return _run_form(args, client, args.route_path)
    if args.action is not None:
        args.index = args.action
        return _run_action_common(args, client, args.route_path)
    if args.data or getattr(args, "file", []) or args.button is not None:
        raise CsustError("--data/--button 需要同时指定 --form 或 --action", code="invalid_argument")
    return _run_get(type("Args", (), {"path": args.route_path, "param": args.param, "output": args.output})(), client)


def _run_public_get(args: argparse.Namespace, client: Client) -> dict[str, object]:
    if args.output is not None:
        target = _public_target(client, args.path, _pairs(args.param, "--param"))
        _response, saved = _request(client, "GET", target, output=args.output, require_session=False)
        assert saved is not None
        return saved
    response, page = _get_page(client, args.path, _pairs(args.param, "--param"), public=True)
    return page


def _run_public_post(args: argparse.Namespace, client: Client) -> dict[str, object]:
    if not args.yes:
        raise CsustError("公开 POST 可能发送找回密码邮件，请加 --yes", code="confirmation_required")
    target = _public_target(client, args.path, _pairs(args.param, "--param"))
    data = _pairs(args.data, "--data")
    response, saved = _request(client, "POST", target, data, output=args.output, require_session=False)
    mutation_request = {"method": "POST", "path": urlparse(response.url).path, "fields": [name for name, _ in data]}
    _save_mutation(client, mutation_request)
    if saved:
        return _download_result(response, saved, True)
    page = _page_payload(response)
    return _mutation_result(
        response,
        page,
        mutation_request,
    )


def _run_public_action(args: argparse.Namespace, client: Client) -> dict[str, object]:
    return _run_action_common(args, client, args.path, public=True)


def _run_request(args: argparse.Namespace, client: Client) -> dict[str, object]:
    method = _supported_method(args.method)
    if method not in READ_ONLY_METHODS and not args.yes:
        raise CsustError("通用请求可能修改账号数据，请加 --yes", code="confirmation_required")
    data = _pairs(args.data, "--data")
    files = _parse_upload_files(getattr(args, "file", []))
    if method in READ_ONLY_METHODS and files:
        raise CsustError("GET/HEAD/OPTIONS 不能上传文件", code="invalid_argument")
    target = _target(client, args.path, _pairs(args.param, "--param"))
    ensure_session(client)
    mutating = method not in READ_ONLY_METHODS
    response, saved = _request(client, method, target, data, output=args.output, require_session=True, multipart=files or None, mutating=mutating)
    request = {"method": method, "path": urlparse(response.url).path, "fields": [name for name, _ in data]}
    if method not in READ_ONLY_METHODS:
        _save_mutation(client, request)
    if saved:
        return _download_result(response, {**saved, "request": request}, method not in READ_ONLY_METHODS)
    page = _page_payload(response)
    if method not in READ_ONLY_METHODS:
        return _mutation_result(response, page, request)
    return {"ok": True, "submitted": False, "confirmed": True, "request": request, "page": page}


def _graduation_design_url(client: Client) -> str:
    quality_session = getattr(client, "ensure_quality_session", None)
    if callable(quality_session):
        quality_session()
        response = client.request_web("/jsxsd/framework/xsMain.jsp")
    else:
        ensure_session(client)
        response = client.get(_target(client, "/jsxsd/framework/xsMain.jsp"))
    require_logged_in(response)
    _save_cookie_refresh(client, response)
    document = parse_html(_response_text(response))
    node = next((item for item in document.find_all("a") if "towptjbs" in item.attr("onclick")), None)
    if node is None:
        raise CsustError("当前账号没有毕业设计入口", code="feature_unavailable")
    _method, target = _action_target(node, None, response.url, document)
    try:
        parsed = urlparse(target)
    except ValueError as exc:
        raise CsustError("毕业设计入口目标未通过安全校验", code="invalid_path") from exc
    if parsed.scheme.lower() != "https" or parsed.netloc != "oauth.fanyu.com" or parsed.path != "/sso/cas/10536/1004":
        raise CsustError("毕业设计入口目标未通过安全校验", code="invalid_path")
    return target


def _run_graduation_design(args: argparse.Namespace, client: Client) -> dict[str, object]:
    if args.output is not None and not args.fetch:
        raise CsustError("--output 需要同时指定 --fetch", code="invalid_argument")
    target = _graduation_design_url(client)
    result: dict[str, object] = {"ok": True, "external": True, "url": _safe_external_url(target), "method": "GET"}
    if args.fetch:
        if args.output is not None:
            response = client.request(target, method="GET", binary=True, with_metadata=True, stream_to=args.output, defer_stream_commit=True)
            assert isinstance(response, Response)
        else:
            response = client.get(target)
        try:
            result["response"] = _page_payload(response)
            if args.output is not None:
                download = _write_download(response, args.output, require_session=False)
                download["url"] = _safe_external_url(response.url)
                result["download"] = download
        except BaseException:
            _discard_stream(response)
            raise
    return result


def _add_page_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--param", action="append", default=[], help="初始页面查询参数 NAME=VALUE，可重复")
    parser.add_argument("--form", type=int, help="按 1 起始序号提交页面表单")
    parser.add_argument("--button", type=int, help="表单内按 1 起始序号选择提交按钮")
    parser.add_argument("--action", type=int, help="按 1 起始序号执行页面动作")
    parser.add_argument("--data", action="append", default=[], help="覆盖/附加字段 NAME=VALUE，可重复")
    parser.add_argument("--file", action="append", default=[], help="multipart 文件字段 NAME=PATH，可重复")
    parser.add_argument("--fingerprint", help="web get 返回的页面指纹")
    parser.add_argument("--yes", action="store_true", help="确认执行可能改变账号数据的操作")
    parser.add_argument("--output", help="把响应原样保存到文件，适用于打印/导出/下载")
    parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")


def register(subparsers: argparse._SubParsersAction) -> None:
    routes = subparsers.add_parser("routes", aliases=["menu", "discover"], help="列出全部网页菜单、公开入口和条件入口")
    routes.add_argument("--path", default="/jsxsd/framework/xsMain.jsp", help="登录后主页路径")
    routes.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    routes.set_defaults(feature_runner=_run_routes, feature_renderer=render)

    web = subparsers.add_parser("web", help="访问教务系统页面、表单、动作和下载")
    children = web.add_subparsers(dest="web_command", required=True)

    get = children.add_parser("get", help="GET 页面并输出结构化内容")
    path_or_name = get.add_mutually_exclusive_group(required=True)
    path_or_name.add_argument("--path", help="页面路径；直接请求的快速路径")
    path_or_name.add_argument("--name", help="按登录后主页实时菜单文本或稳定命令名发现页面")
    get.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    get.add_argument("--output", help="保存响应文件")
    get.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    get.set_defaults(feature_runner=_run_get, feature_renderer=render)

    post = children.add_parser("post", help="显式 POST /jsxsd 页面")
    post.add_argument("--path", required=True, help="/jsxsd/ 下的页面路径")
    post.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    post.add_argument("--data", action="append", default=[], help="表单字段 NAME=VALUE，可重复")
    post.add_argument("--file", action="append", default=[], help="multipart 文件字段 NAME=PATH，可重复")
    post.add_argument("--yes", action="store_true", help="确认可能产生远端变更的请求")
    post.add_argument("--output", help="保存响应文件")
    post.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    post.set_defaults(feature_runner=_run_post, feature_renderer=render)

    action = children.add_parser("action", aliases=["run"], help="按页面动作 ref 或序号提交表单或打开链接")
    action.add_argument("--path", required=True, help="/jsxsd/ 下的页面路径")
    action.add_argument("--index", type=int, help="兼容模式：web get 输出的动作序号")
    action.add_argument("--ref", help="web get 输出的稳定动作 ref；页面重新排列后仍可解析")
    action.add_argument("--param", action="append", default=[], help="初始页面查询参数 NAME=VALUE，可重复")
    action.add_argument("--data", action="append", default=[], help="覆盖表单字段 NAME=VALUE，可重复")
    action.add_argument("--file", action="append", default=[], help="multipart 文件字段 NAME=PATH，可重复")
    action.add_argument("--fingerprint", help="使用 --index 时校验 web get 返回的页面指纹")
    action.add_argument("--yes", action="store_true", help="确认执行网页动作")
    action.add_argument("--output", help="保存响应文件")
    action.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    action.set_defaults(feature_runner=_run_action, feature_renderer=render)

    request = children.add_parser("request", help="发送任意受限 HTTP 请求")
    request.add_argument("--path", required=True, help="/jsxsd/ 下的路径")
    request.add_argument("--method", default="GET", help="GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS")
    request.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    request.add_argument("--data", action="append", default=[], help="请求字段 NAME=VALUE，可重复")
    request.add_argument("--file", action="append", default=[], help="multipart 文件字段 NAME=PATH，可重复")
    request.add_argument("--yes", action="store_true", help="确认非只读请求")
    request.add_argument("--output", help="保存响应文件")
    request.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    request.set_defaults(feature_runner=_run_request, feature_renderer=render)

    catalog = children.add_parser("catalog", help="只输出固化的 69 条登录后菜单映射")
    catalog.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")
    catalog.set_defaults(
        feature_runner=lambda _args, _client: {
            "menus": [{"command": c, "name": n, "code": code} for c, n, code in MAIN_MENU_CATALOG],
            "groups": [{"name": n, "menu": m, "code": c} for n, m, c in SECOND_LEVEL_CATALOG],
            "catalog": [{"command": c, "group": g, "name": n, "path": p} for c, g, n, p in ROUTE_CATALOG],
        },
        feature_renderer=render,
    )

    public = children.add_parser("public", help="访问登录和找回密码公开页面")
    public_children = public.add_subparsers(dest="public_command", required=True)
    public_get = public_children.add_parser("get", help="GET 公开页面")
    public_get.add_argument("--path", required=True, choices=[item[2] for item in PUBLIC_CATALOG])
    public_get.add_argument("--param", action="append", default=[])
    public_get.add_argument("--output")
    public_get.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    public_get.set_defaults(feature_runner=_run_public_get, feature_renderer=render)
    public_post = public_children.add_parser("post", help="POST 找回密码等公开表单")
    public_post.add_argument("--path", required=True, choices=["/Logon.do"])
    public_post.add_argument("--param", action="append", default=[])
    public_post.add_argument("--data", action="append", default=[])
    public_post.add_argument("--yes", action="store_true")
    public_post.add_argument("--output")
    public_post.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    public_post.set_defaults(feature_runner=_run_public_post, feature_renderer=render)
    public_action = public_children.add_parser("action", help="按序号执行找回密码页面动作")
    public_action.add_argument("--path", required=True, choices=["/findmm.jsp", "/Logon.do"])
    public_action.add_argument("--index", type=int, help="兼容模式：页面动作序号")
    public_action.add_argument("--ref", help="页面动作 ref")
    public_action.add_argument("--param", action="append", default=[])
    public_action.add_argument("--data", action="append", default=[])
    public_action.add_argument("--fingerprint", help="使用 --index 时校验页面指纹")
    public_action.add_argument("--yes", action="store_true")
    public_action.add_argument("--output")
    public_action.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    public_action.set_defaults(feature_runner=_run_public_action, feature_renderer=render)

    graduation = children.add_parser("graduation-design", help="打开条件显示的毕业设计外部 SSO 入口")
    graduation.add_argument("--fetch", action="store_true", help="跟随 SSO 入口并返回目标页面")
    graduation.add_argument("--output", help="与 --fetch 一起保存目标响应")
    graduation.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    graduation.set_defaults(feature_runner=_run_graduation_design, feature_renderer=render)

    for command, _group, label, path in ROUTE_CATALOG:
        page = children.add_parser(command, help=f"{label}：{path}")
        _add_page_args(page)
        page.set_defaults(feature_runner=_run_route, feature_renderer=render, route_path=path)


def render(data: dict[str, object]) -> None:
    if "catalog" in data:
        for item in data.get("catalog", []):
            if isinstance(item, dict):
                print("\t".join(_safe_terminal_text(item.get(key)) for key in ("command", "group", "name", "path")))
        return
    if data.get("downloaded"):
        print(f"已保存：{_safe_terminal_text(data.get('output'))}（{_safe_terminal_text(data.get('bytes', 0))} bytes）")
        return
    page = data.get("page", data)
    if isinstance(page, dict):
        print(f"{_safe_terminal_text(page.get('title'))}\t{_safe_terminal_text(page.get('url'))}".strip())
        for form in page.get("forms", []):
            if isinstance(form, dict):
                print("表单 " + "\t".join(_safe_terminal_text(form.get(key)) for key in ("index", "method", "action")))
        for action in page.get("actions", []):
            if isinstance(action, dict):
                print(("动作 " + "\t".join(_safe_terminal_text(action.get(key)) for key in ("ref", "index", "text", "method", "target"))).rstrip())
