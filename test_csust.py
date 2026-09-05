from __future__ import annotations

import contextlib
from email.message import Message
from http.client import HTTPException
from http.cookiejar import Cookie
import io
import json
import os
import stat
import sys
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError
from pathlib import Path
from types import SimpleNamespace
from unittest import mock
from urllib.parse import parse_qs, urlparse
from urllib.request import Request

from csust import (
    Response,
    fetch_captcha,
    generate_encoded,
    parse_classrooms,
    parse_evaluation_batches,
    parse_evaluation_courses,
    parse_evaluation_form,
    parse_exams,
    parse_grade_detail,
    parse_grades,
    parse_html,
    parse_profile,
    parse_schedule,
    parse_selection_results,
    parse_semester_start,
    parse_textbooks,
    submit_textbook_action,
)
from csust_cli.cli import _run_logout, main
from csust_cli.core import Client, CsustError, HttpError, LoginRequired, MutationUnverified, NetworkError, _SafeRedirectHandler, _append_query, _credentials, _decode_body, _safe_terminal_text, _safe_url, _safe_urljoin, _save_cookie_refresh, ensure_session, internal_url, login, require_logged_in, same_origin_url, solve_captcha
from csust_cli.features.academic import _options, _response_message, _run_evaluation, _selected, render as render_academic
from csust_cli.features.grades import render as render_grades, run_detail
from csust_cli.features.schedule import render as render_schedule, selected_option
from csust_cli.features.web import ROUTE_CATALOG, _element_value, _form_button, _form_fields, _graduation_design_url, _js_state_fields, _mutation_verified, _public_target, _request, _run_action_common, _run_graduation_design, _run_post, _run_public_get, _run_public_post, _run_request, _run_form, _run_route, _safe_external_url, _write_download, inspect_page, render as render_web
from csust_cli.features.textbooks import _same_item, _state_confirms, action_url, form_fields, render as render_textbooks, response_message


LOGIN_PAGE = '<form id="loginForm"><input name="userAccount"><input name="userPassword"></form>'


def schedule_page() -> str:
    weekdays = "".join(f"<th>星期{day}</th>" for day in "一二三四五六日")
    return f"""
    <select id="xnxq01id"><option value="2025-2026-1" selected>2025-2026-1</option></select>
    <table id="kbtable"><tr><th></th>{weekdays}</tr>
    <tr><th>1、2节</th><td><div class="kbcontent">线性代数<br>
    <font title="老师">张老师</font><br><font title="周次(节次)">1-2(周)[01-02节]</font>
    <br><font title="教室">A101</font></div></td><td></td><td></td><td></td><td></td><td></td><td></td></tr>
    </table>
    """


def textbook_page(subscribed: bool) -> str:
    action = ("unsubscribe", "退订", "已订") if subscribed else ("subscribe", "选订", "未订")
    return f"""
    <form action="/save" method="post"><input type="hidden" name="token" value="server-token">
    <table id="jccx"><tr><th>课程</th><th>教材</th><th>状态</th><th>操作</th></tr>
    <tr><td>线代</td><td>教材 A</td><td>{action[2]}</td><td>
    <input type="hidden" name="id" value="7"><button name="op" value="{action[0]}">{action[1]}</button>
    </td></tr></table></form>
    """


GRADE_PAGE = """
<table id="dataList"><tr><td></td><td>2025-2026-1</td><td>C1</td><td>线代</td><td></td>
<td>95</td><td>主修</td><td></td><td>3</td><td>48</td><td>4.0</td><td></td><td>考试</td>
<td></td><td></td><td>必修</td><td></td></tr></table>
"""


class AcademicHandler(BaseHTTPRequestHandler):
    captcha_requests = 0
    login_attempts = 0
    schedule_requests = 0
    grades_requests = 0
    textbook_requests = 0
    subscribed = False
    invalidate_next = False

    @classmethod
    def reset(cls) -> None:
        cls.captcha_requests = 0
        cls.login_attempts = 0
        cls.schedule_requests = 0
        cls.grades_requests = 0
        cls.textbook_requests = 0
        cls.subscribed = False
        cls.invalidate_next = False

    def log_message(self, *_args: object) -> None:
        return

    def _authenticated(self) -> bool:
        return "AUTH=1" in self.headers.get("Cookie", "")

    def _send(self, body: str | bytes, *, content_type: str = "text/html", cookie: str | None = None) -> None:
        payload = body.encode("utf-8") if isinstance(body, str) else body
        self.send_response(200)
        self.send_header("Content-Type", f"{content_type}; charset=utf-8")
        if cookie:
            self.send_header("Set-Cookie", cookie)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def _send_login_or(self, body: str) -> None:
        if not self._authenticated():
            self._send(LOGIN_PAGE)
        else:
            self._send(body)

    def do_GET(self) -> None:  # noqa: N802 - stdlib handler API
        parsed = urlparse(self.path)
        if parsed.path == "/":
            self._send(LOGIN_PAGE)
        elif parsed.path == "/verifycode.servlet":
            type(self).captcha_requests += 1
            self._send(b"fake-captcha", content_type="image/png")
        elif parsed.path == "/jsxsd/xskb/xskb_list.do":
            if type(self).invalidate_next and self._authenticated():
                type(self).invalidate_next = False
                self._send(LOGIN_PAGE)
            else:
                type(self).schedule_requests += 1
                self._send_login_or(schedule_page())
        elif parsed.path == "/jsxsd/nxsjc/jccx":
            type(self).textbook_requests += 1
            self._send_login_or(textbook_page(type(self).subscribed))
        else:
            self.send_error(404)

    def do_POST(self) -> None:  # noqa: N802 - stdlib handler API
        parsed = urlparse(self.path)
        length = int(self.headers.get("Content-Length", "0"))
        values = parse_qs(self.rfile.read(length).decode("utf-8"), keep_blank_values=True)
        if parsed.path == "/Logon.do" and parse_qs(parsed.query).get("flag") == ["sess"]:
            self._send("abcdefghijklmnop#12345", content_type="text/plain")
        elif parsed.path == "/Logon.do":
            type(self).login_attempts += 1
            if values.get("RANDOMCODE") != ["good"]:
                self._send("验证码错误")
            else:
                self._send("登录成功", cookie="AUTH=1; Path=/")
        elif parsed.path == "/jsxsd/xskb/xskb_list.do" and self._authenticated():
            type(self).schedule_requests += 1
            self._send(schedule_page())
        elif parsed.path == "/jsxsd/kscj/cjcx_list" and self._authenticated():
            type(self).grades_requests += 1
            self._send(GRADE_PAGE)
        elif parsed.path == "/save" and self._authenticated():
            if values.get("id") == ["7"] and values.get("op") == ["subscribe"]:
                type(self).subscribed = True
                self._send("操作成功")
            else:
                self._send("操作失败")
        else:
            self._send(LOGIN_PAGE)


@contextlib.contextmanager
def academic_server():
    AcademicHandler.reset()
    server = ThreadingHTTPServer(("127.0.0.1", 0), AcademicHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


class CsustParserTests(unittest.TestCase):
    def test_local_ocr_adapter_is_lazy_and_normalizes_result(self):
        class FakeEngine:
            def classification(self, image: bytes) -> str:
                self.image = image
                return " A B 12 "

        fake_module = SimpleNamespace(DdddOcr=lambda show_ad=False: FakeEngine())
        with mock.patch.dict(sys.modules, {"ddddocr": fake_module}), mock.patch("csust_cli.core._ocr_engine", None):
            self.assertEqual(solve_captcha(b"captcha"), "AB12")

    def test_login_encoding_matches_page_algorithm(self):
        self.assertEqual(generate_encoded("a", "b", "abcdefghijklmnop#12345"), "aa%bc%def%ghijbklmno")

    def test_schedule_parser(self):
        source = """
        <table id="kbtable"><tr><th></th><th>星期一</th><th>星期二</th></tr>
        <tr><th>1、2节</th><td><div class="kbcontent">高等数学<br>
        <font title="老师">张老师</font><br><font title="周次(节次)">1-2(周)[01-02节]</font>
        <br><font title="教室">A101</font></div></td><td></td></tr></table>
        """
        items = parse_schedule(source, "2025-2026-1")
        self.assertEqual(items[0]["weekday"], "星期一")
        self.assertEqual(items[0]["course"], "高等数学")
        self.assertEqual(items[0]["weeks"], [1, 2])
        self.assertEqual(items[0]["sections"], [1, 2])

    def test_data_parsers_ignore_script_text(self):
        secret = "parser-script-secret"
        profile = parse_profile(
            f"<table id='xjkpTable'><tr><td>姓名：<script>{secret}</script>张三</td></tr></table>",
            "https://example.test/jsxsd/grxx/xsxx",
        )
        self.assertNotIn(secret, str(profile))
        self.assertEqual(profile["fields"][0]["value"], "张三")

        grades = parse_grades(
            "<table id='dataList'><tr>"
            + "".join(
                f"<td>{('<script>' + secret + '</script>') if index == 3 else ''}{value}</td>"
                for index, value in enumerate(("1", "2025-2026-1", "C1", "线代", "", "95"))
            )
            + "</tr></table>"
        )
        self.assertNotIn(secret, str(grades))

        textbooks = parse_textbooks(
            f"<table id='jccx'><tr><th>课程</th><th>教材</th></tr>"
            f"<tr><td><script>{secret}</script>线代</td><td>教材 A</td></tr></table>",
            "https://example.test/jsxsd/nxsjc/jccx",
        )
        self.assertNotIn(secret, str(textbooks))

        schedule = parse_schedule(
            f"<table id='kbtable'><tr><th></th><th>星期一</th></tr><tr><th>1节</th>"
            f"<td><div class='kbcontent'>高数<script>{secret}</script>"
            "<font title='周次(节次)'>1(周)[01节]</font></div></td></tr></table>",
            "2025-2026-1",
        )
        self.assertNotIn(secret, str(schedule))
        with self.assertRaises(CsustError):
            parse_grades("<script>var message = '未查询到数据';</script>")
        require_logged_in(Response("https://example.test/jsxsd/page", 200, {}, f"<script>请输入账号 用户登录 {secret}</script><div>正常页面</div>"))

    def test_html_parser_survives_malformed_prefix(self):
        document = parse_html("<!broken <div>可见</div><!bad")
        self.assertIn("可见", document.text(include_scripts=False))

    def test_html_parser_survives_malformed_declarations(self):
        document = parse_html("<div>可见</div><![broken")
        self.assertIn("可见", document.text(include_scripts=False))
        with self.assertRaises(TypeError):
            parse_html(b"<div>bytes</div>")

    def test_html_parser_closes_common_unclosed_controls(self):
        document = parse_html("<select><option value='a'>甲<option value='b'>乙</select>")
        select = document.first("select")
        assert select is not None
        self.assertEqual([option.text(include_scripts=False) for option in select.find_all("option")], ["甲", "乙"])

        document = parse_html("<table><tr><td>A<td>B<tr><td>C<td>D</table>")
        table = document.first("table")
        assert table is not None
        self.assertEqual([[cell.text() for cell in row.direct("td")] for row in table.find_all("tr")], [["A", "B"], ["C", "D"]])

        source = "<video><track src='captions.vtt'><wbr><div>可见</div></video>"
        document = parse_html(source)
        self.assertEqual([node.tag for node in document.find_all()], ["video", "track", "wbr", "div"])
        self.assertEqual(document.to_html(), '<video><track src="captions.vtt"><wbr><div>可见</div></video>')

        nested = "<table id='outer'><tr><td>before<table id='inner'><tr><td>inside</td></tr></table>after</td></tr></table>"
        document = parse_html(nested)
        outer = document.first("table", element_id="outer")
        inner = document.first("table", element_id="inner")
        assert outer is not None and inner is not None
        self.assertIs(inner.parent, outer.first("td"))
        self.assertEqual(document.to_html(), nested.replace("'", '"'))

        nested_data = (
            '<form><table id="jccx"><tr><th>课程</th><th>教材</th></tr>'
            '<tr><td>线代</td><td>教材 A<table><tr><td>布局</td><td>值</td></tr></table></td></tr>'
            '</table></form>'
        )
        self.assertEqual(len(parse_textbooks(nested_data, "https://example.test/x")["items"]), 1)

        nested_forms = parse_html(
            '<form id="outer"><input name="a"><form id="inner"><input name="b">'
            '<button name="go">提交</button></form><input name="c"></form>'
        )
        forms = nested_forms.find_all("form")
        self.assertEqual(len(forms), 1)
        self.assertEqual([node.attr("name") for node in forms[0].find_all("input")], ["a", "b"])

        duplicate_attributes = parse_html("<form action='/first' action='/second'><input name='x' value='one' value='two'></form>")
        form = duplicate_attributes.first("form")
        input_node = duplicate_attributes.first("input")
        assert form is not None and input_node is not None
        self.assertEqual(form.attr("action"), "/first")
        self.assertEqual(input_node.attr("value"), "one")

    def test_deep_html_does_not_overflow_dom_helpers(self):
        source = "<div>" * 2000 + "页面" + "</div>" * 2000
        document = parse_html(source)
        self.assertEqual(len(document.find_all("div")), 2000)
        self.assertEqual(document.text(include_scripts=False), "页面")
        self.assertEqual(document.to_html(), source)
        self.assertEqual(inspect_page(source, "https://example.test/jsxsd/page")["text"], "页面")

    def test_grade_parser(self):
        cells = "".join(f"<td>{value}</td>" for value in ["", "2025-2026-1", "C1", "线代", "", "95", "主修", "", "3", "48", "4.0", "", "考试", "", "", "必修", ""])
        items = parse_grades(f'<table id="dataList"><tr>{cells}</tr></table>')
        self.assertEqual(items[0]["course"], "线代")
        self.assertEqual(items[0]["score"], "95")
        self.assertEqual(items[0]["credit"], "3")

        header = "".join(f"<td>{value}</td>" for value in ["序号", "学期", "课程代码", "课程", "", "成绩"])
        items = parse_grades(f'<table id="dataList"><tr>{header}</tr><tr>{cells}</tr></table>')
        self.assertEqual(len(items), 1)

    def test_current_grade_columns_and_detail_parser(self):
        values = [str(index) for index in range(20)]
        values[0] = "1"
        values[3] = "线代"
        values[5] = "95"
        values[7] = "3"
        values[9] = "4.0"
        items = parse_grades("<table id='dataList'><tr>" + "".join(f"<td>{value}</td>" for value in values) + "</tr></table>")
        self.assertEqual(items[0]["course"], "线代")
        self.assertEqual(items[0]["credit"], "3")
        self.assertEqual(items[0]["grade_point"], "4.0")
        detail = parse_grade_detail(
            "<table id='dataList'><tr><th>平时</th><th>总评</th></tr><tr><td>90</td><td>95</td></tr></table>",
            "http://example.test/jsxsd/kscj/detail",
        )
        self.assertEqual(detail["fields"]["总评"], "95")

        relative_detail = parse_grades(
            "<table id='dataList'><tr>"
            "<td>1</td><td>2025-2026-1</td><td>C1</td><td>线代</td><td></td>"
            "<td><a href='JAVASCRIPT:go(\"detail.do?id=1\")'>95</a></td></tr></table>",
            "https://example.test/jsxsd/kscj/cjcx_list",
        )
        self.assertEqual(relative_detail[0]["grade_detail_url"], "https://example.test/jsxsd/kscj/detail.do?id=1")

    def test_remaining_academic_page_parsers(self):
        profile = parse_profile(
            "<table id='xjkpTable'><tr><td>院系：计算机</td><td>专业：软件工程</td></tr>"
            "<tr><td>学号</td><td>2025001</td></tr></table>",
            "http://example.test/jsxsd/grxx/xsxx",
        )
        self.assertEqual(profile["values"]["院系"], "计算机")
        self.assertIn("2025001", profile["values"].values())

        exams = parse_exams(
            "<table id='dataList'><tr><th>序号</th></tr>"
            "<tr>" + "".join(f"<td>{value}</td>" for value in ["1", "云塘", "上午", "C1", "线代", "张老师", "2025-06-01 09:00~11:00", "A101", "1", "T1", ""])
            + "</tr></table>",
            "http://example.test/jsxsd/xsks/xsksap_list",
        )
        self.assertEqual(exams["items"][0]["course"], "线代")
        self.assertEqual(exams["items"][0]["start_time"], "09:00")

        classrooms = parse_classrooms(
            "<table id='kbtable'><tr><th>教室</th><th>一</th></tr>"
            "<tr><td>A101</td><td></td></tr><tr><td>A102</td><td>线代</td></tr></table>",
            "http://example.test/jsxsd/kbcx/kbxx_classroom_ifr",
        )
        self.assertEqual([item["room"] for item in classrooms["items"]], ["A101"])

        selection = parse_selection_results(
            "<table id='dataList'><tr>" + "".join(f"<th>{i}</th>" for i in range(8)) + "</tr>"
            "<tr>" + "".join(f"<td>{value}</td>" for value in ["1", "线代", "C1", "张老师", "48", "3", "必修", "主修"]) + "</tr></table>",
            "http://example.test/jsxsd/xkgl/loadXsxkjgList",
        )
        self.assertEqual(selection["items"][0]["course_id"], "C1")

        semester_start = parse_semester_start(
            "<table id='kbtable'><tr><td>学期</td><td>起始日</td></tr>"
            "<tr><td>2025-2026-1</td><td>2025年9月1日</td></tr></table>",
            "https://example.test/jsxsd/jxzl/jxzl_query",
        )
        self.assertEqual(semester_start["start_date"], "2025年9月1日")

        batches = parse_evaluation_batches(
            "<table><tr><th>序号</th></tr><tr>" + "".join(f"<td>{value}</td>" for value in ["1", "2025-2026-1", "教师", "期末评价", "2025-06-01", "2025-06-30"]) + "<td><a href='/batch'>进入</a></td></tr></table>",
            "http://example.test/jsxsd/xspj/xspj_find.do",
        )
        self.assertEqual(batches["items"][0]["path"], "http://example.test/batch")

        courses = parse_evaluation_courses(
            "<table id='dataList'><tr>" + "".join(f"<th>{i}</th>" for i in range(9)) + "</tr><tr>"
            + "".join(f"<td>{value}</td>" for value in ["1", "C1", "线代", "张老师", "教师", "100", "否", "否", "48"])
            + "<td><a href='/form'>评价</a></td></tr></table>",
            "http://example.test/jsxsd/xspj/xspj_list",
        )
        self.assertEqual(courses["items"][0]["course"], "线代")

        malformed_grade = parse_grades(
            "<table id='dataList'><tr>"
            "<td>1</td><td>线代</td><td></td><td></td><td></td><td><a href='http://[bad'>详情</a></td>"
            "</tr></table>",
            "https://example.test",
        )
        self.assertNotIn("grade_detail_url", malformed_grade[0])

        form = parse_evaluation_form(
            "<div>课程名称：线代 评教大类：教师</div><form action='/xspj_save.do' method='post'>"
            "<input type='hidden' name='token' value='t'><input type='hidden' name='state' value='unknown-secret-value-123456789'>"
            "<table><tr><td><input name='pj06xh' value='q1'>题目</td>"
            "<td><input type='radio' name='pj0601id_q1' value='1'>好<input disabled type='radio' name='pj0601id_q1' value='2'>一般</td></tr></table>"
            "<textarea name='jy'>建议</textarea><button onclick='saveData()'>保存</button></form>",
            "http://example.test/jsxsd/xspj/form",
        )
        self.assertFalse(form["read_only"])
        self.assertEqual(form["questions"][0]["id"], "q1")
        self.assertEqual([option["id"] for option in form["questions"][0]["options"]], ["1"])
        self.assertEqual(form["hidden_fields"][0]["value"], "")
        self.assertEqual(form["hidden_fields"][1]["value"], "<redacted>")
        self.assertEqual(
            parse_evaluation_form(
                "<form action='http://[bad/xspj_save.do'><input type='hidden' name='token' value='t'></form>",
                "https://example.test/jsxsd/xspj/form",
            )["action"],
            "",
        )
        self.assertEqual(parse_evaluation_form(
            "<form action='/xspj_save.do'><input type='hidden' name='token' value='t'></form>",
            "http://example.test/jsxsd/xspj/form",
            include_sensitive=True,
        )["hidden_fields"][0]["value"], "t")

        start = parse_semester_start(
            "<table id='kbtable'><tr><th>项目</th><th>日期</th></tr><tr><td>开学</td><td title='2025年09月01'>起始</td></tr></table>",
            "http://example.test/jsxsd/jxzl/jxzl_query",
        )
        self.assertEqual(start["start_date"], "2025年09月01")

    def test_dotenv_credentials_are_noninteractive_and_process_values_win(self):
        with tempfile.TemporaryDirectory() as directory:
            env_file = Path(directory) / ".env"
            env_file.write_text("username=from-file\npassword='file password'\n", encoding="utf-8")
            with mock.patch.dict(os.environ, {"CSUST_ENV_FILE": str(env_file)}, clear=True):
                self.assertEqual(_credentials(), ("from-file", "file password"))
            with mock.patch.dict(
                os.environ,
                {"CSUST_ENV_FILE": str(env_file), "CSUST_USERNAME": "from-process", "CSUST_PASSWORD": "process"},
                clear=True,
            ):
                self.assertEqual(_credentials(), ("from-process", "process"))
        with mock.patch("csust_cli.core.os.environ", {"CSUST_ENV_FILE": "bad\0"}):
            with self.assertRaises(CsustError) as raised:
                _credentials()
        self.assertEqual(raised.exception.code, "credentials_required")
        with tempfile.TemporaryDirectory() as directory:
            env_file = Path(directory) / ".env"
            env_file.write_bytes(b"username=\xff\n")
            with mock.patch.dict(os.environ, {"CSUST_ENV_FILE": str(env_file)}, clear=True):
                with self.assertRaises(CsustError) as raised:
                    _credentials()
            self.assertEqual(raised.exception.code, "credentials_required")

    def test_client_validates_base_url_and_warns_once_for_http(self):
        for value in (
            "file:///tmp/csust",
            "http://[bad",
            "https://example.test:bad/",
            "http://:80",
            "https://:@example.test",
            "https://@example.test",
            "https://example.test\n/next",
        ):
            with self.assertRaises(CsustError) as raised:
                Client(base_url=value, load_cookies=False)
            self.assertEqual(raised.exception.code, "invalid_base_url")

        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="http://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            with mock.patch.object(client.opener, "open", side_effect=OSError("offline")), contextlib.redirect_stderr(io.StringIO()) as error:
                with self.assertRaises(NetworkError):
                    client.get("/")
                with self.assertRaises(NetworkError):
                    client.get("/")
            self.assertEqual(error.getvalue().count("警告："), 1)

            for target in (
                "file:///tmp/csust-secret",
                "http://[bad",
                "https://:443/",
                "https://example.test:bad/",
                "https://:@example.test/",
            ):
                with self.assertRaises(CsustError) as raised:
                    client.request(target)
                self.assertEqual(raised.exception.code, "invalid_path")
            for invalid in (None, 123, "https://example.test\n/next"):
                with self.assertRaises(CsustError) as raised:
                    client.url(invalid)
                self.assertEqual(raised.exception.code, "invalid_path")
            with self.assertRaises(CsustError) as raised:
                client.request("/jsxsd/page", method=None)
            self.assertEqual(raised.exception.code, "invalid_argument")
            with self.assertRaises(CsustError) as raised:
                client.request("/jsxsd/page", headers="not-a-mapping")
            self.assertEqual(raised.exception.code, "invalid_argument")

        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            self.assertEqual(same_origin_url(client, "/save"), "https://example.test/save")
            self.assertEqual(
                same_origin_url(client, "HTTPS://EXAMPLE.TEST:443/save"),
                "HTTPS://EXAMPLE.TEST:443/save",
            )
            self.assertEqual(
                _public_target(client, "HTTPS://EXAMPLE.TEST:443/findmm.jsp"),
                "HTTPS://EXAMPLE.TEST:443/findmm.jsp",
            )
            with self.assertRaises(CsustError) as raised:
                same_origin_url(client, "https://evil.example/save")
            self.assertEqual(raised.exception.code, "invalid_path")
            for path in (
                "https://example.test/jsxsd/../outside",
                "https://example.test/jsxsd/%2e%2e/outside",
                "https://example.test/jsxsd/%252e%252e/outside",
            ):
                with self.assertRaises(CsustError) as raised:
                    internal_url(client, path)
                self.assertEqual(raised.exception.code, "invalid_path")

    def test_http_error_body_read_failure_keeps_stable_error(self):
        class BrokenBody:
            closed = False

            def read(self):
                raise OSError("connection closed")

            def close(self):
                type(self).closed = True
                return None

        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            error = HTTPError("https://example.test/jsxsd/page", 503, "Unavailable", {}, BrokenBody())
            with mock.patch.object(client.opener, "open", side_effect=error):
                with self.assertRaises(NetworkError) as raised:
                    client.get("/jsxsd/page")
            self.assertEqual(raised.exception.code, "http_error")
            self.assertTrue(BrokenBody.closed)

            with mock.patch.object(client.opener, "open", side_effect=HTTPException("connection reset")):
                with self.assertRaises(NetworkError) as raised:
                    client.get("/jsxsd/page")
            self.assertEqual(raised.exception.code, "network_error")

    def test_unknown_response_charset_falls_back_for_login_detection(self):
        headers = Message()
        headers["Content-Type"] = "text/html; charset=not-a-real-codec"
        with self.assertRaises(LoginRequired):
            require_logged_in(Response("https://example.test/", 200, headers, b"<form id='loginForm'></form>"))
        with self.assertRaises(LoginRequired):
            require_logged_in(
                Response(
                    "https://example.test/",
                    200,
                    {"content-type": "text/html; charset=gbk"},
                    "请输入账号 用户登录".encode("gbk"),
                )
            )
        with self.assertRaises(LoginRequired):
            require_logged_in(
                Response(
                    "https://example.test/",
                    200,
                    {"Content-Type": "text/html"},
                    "<meta charset='gbk'><div>用户登录 请输入账号</div>".encode("gbk"),
                )
            )

    def test_response_charset_header_lookup_is_case_insensitive_for_mappings(self):
        body = "请输入账号 用户登录".encode("gbk")
        self.assertEqual(
            _decode_body(body, {"CONTENT-TYPE": "text/html; charset=gbk"}),
            "请输入账号 用户登录",
        )

    def test_cookie_save_keeps_previous_file_when_atomic_write_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            cookie_file = Path(directory) / "cookies.txt"
            cookie_file.write_text("old-cookie", encoding="utf-8")
            client = Client(base_url="https://example.test", cookie_file=cookie_file, load_cookies=False)
            with mock.patch("csust_cli.core.MozillaCookieJar.save", side_effect=OSError("disk full")):
                with self.assertRaises(CsustError) as raised:
                    client.save()
            self.assertEqual(raised.exception.code, "cookie_write_failed")
            self.assertEqual(cookie_file.read_text(encoding="utf-8"), "old-cookie")

            invalid = Client(base_url="https://example.test", cookie_file=Path(directory) / "bad\0", load_cookies=False)
            with self.assertRaises(CsustError) as raised:
                invalid.save()
            self.assertEqual(raised.exception.code, "cookie_write_failed")

            with self.assertRaises(CsustError) as raised:
                Client(base_url="https://example.test", cookie_file=Path(directory) / "bad\0")
            self.assertEqual(raised.exception.code, "cookie_read_failed")

            target = Path(directory) / "cookie-target.txt"
            target.write_text("# Netscape HTTP Cookie File\n", encoding="utf-8")
            target.chmod(0o644)
            link = Path(directory) / "cookies-link.txt"
            link.symlink_to(target)
            with self.assertRaises(CsustError) as raised:
                Client(base_url="https://example.test", cookie_file=link)
            self.assertEqual(raised.exception.code, "cookie_read_failed")
            self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o644)
            writable_client = Client(base_url="https://example.test", cookie_file=link, load_cookies=False)
            with self.assertRaises(CsustError) as raised:
                writable_client.save()
            self.assertEqual(raised.exception.code, "cookie_write_failed")
            self.assertEqual(target.read_text(encoding="utf-8"), "# Netscape HTTP Cookie File\n")

            broken = Path(directory) / "broken-cookies.txt"
            broken.symlink_to(Path(directory) / "missing-cookies.txt")
            with self.assertRaises(CsustError) as raised:
                Client(base_url="https://example.test", cookie_file=broken)
            self.assertEqual(raised.exception.code, "cookie_read_failed")

            with mock.patch.dict(os.environ, {"CSUST_COOKIE_FILE": "~/csust-cli-expanded-cookies.txt"}, clear=False):
                client = Client(base_url="https://example.test", load_cookies=False)
            self.assertEqual(client.cookie_file, Path.home() / "csust-cli-expanded-cookies.txt")

    def test_session_probe_persists_cookie_refresh_only_when_present(self):
        with tempfile.TemporaryDirectory() as directory:
            cookie_file = Path(directory) / "cookies.txt"
            cookie_file.write_text("existing", encoding="utf-8")
            client = Client(base_url="https://example.test", cookie_file=cookie_file, load_cookies=False)
            with mock.patch.object(client, "get", return_value=Response("https://example.test/probe", 200, {"Set-Cookie": "AUTH=2"}, "ok")), mock.patch.object(client, "save") as save:
                ensure_session(client)
            save.assert_called_once()

            with mock.patch.object(client, "get", return_value=Response("https://example.test/probe", 200, {}, "ok")), mock.patch.object(client, "save") as save:
                ensure_session(client)
            save.assert_not_called()

    def test_cookie_state_change_is_persisted_without_final_set_cookie(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            client.cookies.set_cookie(
                Cookie(
                    version=0,
                    name="INTERMEDIATE",
                    value="1",
                    port=None,
                    port_specified=False,
                    domain="example.test",
                    domain_specified=False,
                    domain_initial_dot=False,
                    path="/",
                    path_specified=True,
                    secure=False,
                    expires=None,
                    discard=True,
                    comment=None,
                    comment_url=None,
                    rest={},
                    rfc2109=False,
                )
            )
            with mock.patch.object(client, "save") as save:
                _save_cookie_refresh(client, Response("https://example.test/final", 200, {}, "ok"))
            save.assert_called_once()

    def test_cookie_save_excludes_external_domains(self):
        def make_cookie(name: str, value: str, domain: str) -> Cookie:
            return Cookie(
                version=0,
                name=name,
                value=value,
                port=None,
                port_specified=False,
                domain=domain,
                domain_specified=False,
                domain_initial_dot=domain.startswith("."),
                path="/",
                path_specified=True,
                secure=False,
                expires=None,
                discard=True,
                comment=None,
                comment_url=None,
                rest={},
                rfc2109=False,
            )

        with tempfile.TemporaryDirectory() as directory:
            cookie_file = Path(directory) / "cookies.txt"
            client = Client(base_url="https://example.test", cookie_file=cookie_file, load_cookies=False)
            client.cookies.set_cookie(make_cookie("LOCAL", "ok", "example.test"))
            client.cookies.set_cookie(make_cookie("PARENT", "ok", ".test"))
            client.cookies.set_cookie(make_cookie("EXTERNAL", "secret", "evil.example"))
            client.save()
            saved = cookie_file.read_text(encoding="utf-8")
            self.assertIn("LOCAL", saved)
            self.assertIn("PARENT", saved)
            self.assertNotIn("EXTERNAL", saved)

            reloaded = Client(base_url="https://example.test", cookie_file=cookie_file)
            self.assertEqual({cookie.name for cookie in reloaded.cookies}, {"LOCAL", "PARENT"})

            localhost_file = Path(directory) / "localhost-cookies.txt"
            localhost_client = Client(base_url="http://localhost", cookie_file=localhost_file, load_cookies=False)
            localhost_client.cookies.set_cookie(make_cookie("LOCALHOST", "ok", "localhost.local"))
            localhost_client.save()
            localhost_reloaded = Client(base_url="http://localhost", cookie_file=localhost_file)
            self.assertEqual({cookie.name for cookie in localhost_reloaded.cookies}, {"LOCALHOST"})

    def test_session_probe_reauthenticates_on_401_only(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            client.cookie_file.write_text("existing", encoding="utf-8")
            with mock.patch.object(client, "get", side_effect=HttpError(401, "Unauthorized")), mock.patch(
                "csust_cli.core.login"
            ) as reauth:
                ensure_session(client)
            reauth.assert_called_once_with(client=client, ocr=None)

            with mock.patch.object(client, "get", side_effect=HttpError(403, "Forbidden")), mock.patch(
                "csust_cli.core.login"
            ) as reauth:
                with self.assertRaises(HttpError):
                    ensure_session(client)
            reauth.assert_not_called()

    def test_session_cookie_path_errors_are_machine_readable(self):
        class BrokenPath:
            def is_symlink(self):
                return False

            def exists(self):
                raise OSError("permission denied")

            def __str__(self):
                return "/private/cookies.txt"

        with self.assertRaises(CsustError) as raised:
            ensure_session(SimpleNamespace(cookie_file=BrokenPath()))
        self.assertEqual(raised.exception.code, "cookie_read_failed")

    def test_read_only_web_request_persists_cookie_refresh_only_when_present(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            for headers, expected in (({"set-cookie": "AUTH=2"}, True), ({"SET-COOKIE": "AUTH=3"}, True), ({}, False)):
                with self.subTest(headers=headers), mock.patch.object(
                    client,
                    "request",
                    return_value=Response("https://example.test/jsxsd/page", 200, headers, "页面"),
                ), mock.patch.object(client, "save") as save:
                    _request(client, "GET", "https://example.test/jsxsd/page")
                if expected:
                    save.assert_called_once()
                else:
                    save.assert_not_called()

    def test_cross_origin_web_request_drops_referer(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            response = Response("https://192.168.3.125/FineReport", 200, {}, "页面")
            referer = "https://example.test/jsxsd/page?ticket=private-ticket"
            with mock.patch.object(client, "request", return_value=response) as request:
                _request(client, "GET", response.url, referer=referer)
            self.assertIsNone(request.call_args.kwargs["headers"])
            with mock.patch.object(client, "request", return_value=Response("https://example.test/jsxsd/page", 200, {}, "页面")) as request:
                _request(client, "GET", "https://example.test/jsxsd/page", referer=referer)
            self.assertEqual(request.call_args.kwargs["headers"], {"Referer": referer})

    def test_public_download_persists_cookie_refresh(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            args = SimpleNamespace(path="/", param=[], output=str(Path(directory) / "page.html"))
            with mock.patch.object(
                client,
                "request",
                return_value=Response("https://example.test/", 200, {"Set-Cookie": "PUBLIC=1"}, b"page"),
            ), mock.patch.object(client, "save") as save:
                result = _run_public_get(args, client)
            self.assertTrue(result["downloaded"])
            save.assert_called_once()

    def test_unsafe_cross_origin_redirect_is_rejected(self):
        request = Request("http://example.test/save", data=b"token=secret", method="POST")
        with self.assertRaises(NetworkError):
            _SafeRedirectHandler().redirect_request(request, None, 307, "Temporary Redirect", {}, "http://evil.test/save")

        request = Request("https://example.test/read", data=b"token=secret", method="GET")
        with self.assertRaises(NetworkError):
            _SafeRedirectHandler().redirect_request(request, None, 307, "Temporary Redirect", {}, "https://evil.test/read")

        request = Request("https://example.test/options", headers={"Cookie": "AUTH=secret"}, method="OPTIONS")
        redirected = _SafeRedirectHandler().redirect_request(
            request, None, 302, "Found", {}, "https://evil.test/options"
        )
        assert redirected is not None
        self.assertEqual(redirected.get_method(), "OPTIONS")
        self.assertEqual(dict(redirected.header_items()), {})

        request = Request(
            "https://example.test/save",
            data=b"token=secret",
            headers={"Content-Type": "application/x-www-form-urlencoded"},
            method="POST",
        )
        redirected = _SafeRedirectHandler().redirect_request(
            request, None, 308, "Permanent Redirect", {}, "https://example.test/save-new"
        )
        assert redirected is not None
        self.assertEqual(redirected.get_method(), "POST")
        self.assertEqual(redirected.data, b"token=secret")

        request = Request(
            "https://example.test/page",
            headers={
                "Accept": "text/html",
                "User-Agent": "test-agent",
                "Cookie": "AUTH=secret",
                "Referer": "https://example.test/private?token=secret",
                "Authorization": "Bearer secret",
                "X-Csrf-Token": "secret",
            },
        )
        redirected = _SafeRedirectHandler().redirect_request(request, None, 302, "Found", {}, "https://evil.test/next")
        assert redirected is not None
        self.assertEqual(dict(redirected.header_items()), {"Accept": "text/html", "User-agent": "test-agent"})

    def test_cross_origin_cookie_processor_drops_base_domain_cookie(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(
                base_url="https://example.test",
                cookie_file=Path(directory) / "cookies.txt",
                load_cookies=False,
            )
            client.cookies.set_cookie(
                Cookie(
                    version=0,
                    name="AUTH",
                    value="secret",
                    port=None,
                    port_specified=False,
                    domain=".test",
                    domain_specified=True,
                    domain_initial_dot=True,
                    path="/",
                    path_specified=True,
                    secure=True,
                    expires=None,
                    discard=True,
                    comment=None,
                    comment_url=None,
                    rest={},
                    rfc2109=False,
                )
            )
            same_origin = Request("https://example.test/private")
            client.cookies.add_cookie_header(same_origin)
            self.assertEqual(same_origin.get_header("Cookie"), "AUTH=secret")

            http_client = Client(base_url="http://example.test", cookie_file=Path(directory) / "http-cookies.txt", load_cookies=False)
            http_client.cookies.set_cookie(next(iter(client.cookies)))
            upgrade = Request("https://example.test/secure")
            http_client.cookies.add_cookie_header(upgrade)
            self.assertEqual(upgrade.get_header("Cookie"), "AUTH=secret")

            cross_origin = Request("https://evil.test/next")
            client.cookies.add_cookie_header(cross_origin)
            self.assertIsNone(cross_origin.get_header("Cookie"))

            client.cookies.set_cookie(Cookie(0, "SSO", "ok", None, False, "evil.test", True, False, "/", True, False, None, True, None, None, {}, False))
            external_cookie = Request("https://evil.test/sso")
            client.cookies.add_cookie_header(external_cookie)
            self.assertEqual(external_cookie.get_header("Cookie"), "SSO=ok")

            https_client = Client(base_url="https://example.test", cookie_file=Path(directory) / "https-cookies.txt", load_cookies=False)
            https_client.cookies.set_cookie(
                Cookie(
                    version=0,
                    name="AUTH",
                    value="secret",
                    port=None,
                    port_specified=False,
                    domain="example.test",
                    domain_specified=False,
                    domain_initial_dot=False,
                    path="/",
                    path_specified=True,
                    secure=False,
                    expires=None,
                    discard=True,
                    comment=None,
                    comment_url=None,
                    rest={},
                    rfc2109=False,
                )
            )
            downgrade = Request("http://example.test/insecure")
            https_client.cookies.add_cookie_header(downgrade)
            self.assertIsNone(downgrade.get_header("Cookie"))

    def test_relative_redirects_are_resolved_before_request_creation(self):
        handler = _SafeRedirectHandler()
        request = Request("https://example.test/save/original", data=b"token=secret", method="POST")
        redirected = handler.redirect_request(request, None, 308, "Permanent Redirect", {}, "../save-next")
        assert redirected is not None
        self.assertEqual(redirected.full_url, "https://example.test/save-next")
        self.assertEqual(redirected.get_method(), "POST")
        self.assertEqual(redirected.data, b"token=secret")

        request = Request("https://example.test/options", headers={"Cookie": "AUTH=secret"}, method="OPTIONS")
        redirected = handler.redirect_request(request, None, 302, "Found", {}, "next")
        assert redirected is not None
        self.assertEqual(redirected.full_url, "https://example.test/next")
        self.assertEqual(redirected.get_method(), "OPTIONS")

        request = Request("https://example.test/page", method="HEAD")
        redirected = handler.redirect_request(request, None, 302, "Found", {}, "next")
        assert redirected is not None
        self.assertEqual(redirected.full_url, "https://example.test/next")
        self.assertEqual(redirected.get_method(), "HEAD")

        request = Request("https://example.test/page")
        for location, expected in (("?next=1", "https://example.test/page?next=1"), ("#fragment", "https://example.test/page#fragment")):
            with self.subTest(location=location):
                redirected = handler.redirect_request(request, None, 302, "Found", {}, location)
                assert redirected is not None
                self.assertEqual(redirected.full_url, expected)

        for location in ("\nhttps://evil.test/next", "https://evil.test/\nnext"):
            with self.subTest(location=location), self.assertRaises(NetworkError):
                handler.redirect_request(Request("https://example.test/page"), None, 302, "Found", {}, location)

    def test_redirect_rejects_userinfo_and_invalid_port(self):
        request = Request("https://example.test/page")
        handler = _SafeRedirectHandler()
        for location in (
            "",
            "   ",
            "\t\n",
            "https://user:pass@example.test/next",
            "https://:@example.test/next",
            "https://example.test:bad/next",
            "http://example.test/next",
        ):
            with self.subTest(location=location):
                with self.assertRaises(NetworkError):
                    handler.redirect_request(request, None, 302, "Found", {}, location)

    def test_generic_page_inspection_redacts_credentials(self):
        page = inspect_page(
            "<title>页面</title><form action='/save'><input name='userPassword' value='secret'>"
            "<input name='token' value='t'><input name='csrf' value='csrf-secret'><input name='nonce' value='nonce-secret'>"
            "<input name='state' value='unknown-secret-value-123456789'>"
            "<textarea name='csrf'>textarea-csrf-secret</textarea>"
            "<button><textarea name='csrf'>button-csrf-secret</textarea>保存</button></form>"
            "<a href='/jsxsd/save'><textarea name='csrf'>link-csrf-secret</textarea>链接</a>"
            "<table><tr><td>数据<textarea name='csrf'>table-csrf-secret</textarea></td></tr></table>",
            "http://example.test/jsxsd/page",
        )
        fields = page["forms"][0]["fields"]
        self.assertEqual(next(field for field in fields if field["name"] == "userPassword")["value"], "")
        self.assertEqual(next(field for field in fields if field["name"] == "csrf")["value"], "")
        self.assertEqual(next(field for field in fields if field["name"] == "nonce")["value"], "")
        self.assertEqual(next(field for field in fields if field["name"] == "state")["value"], "<redacted>")
        self.assertNotIn("unknown-secret-value-123456789", str(page))
        textarea = next(field for field in fields if field["tag"] == "textarea")
        self.assertEqual(textarea["name"], "csrf")
        self.assertEqual(textarea["text"], "")
        self.assertNotIn("textarea-csrf-secret", page["text"])
        self.assertNotIn("button-csrf-secret", str(page["actions"]))
        self.assertNotIn("link-csrf-secret", str(page["links"]))
        self.assertNotIn("table-csrf-secret", str(page["tables"]))
        self.assertEqual(page["tables"][0]["rows"], [["数据"]])
        self.assertEqual(page["actions"][0]["text"], "保存")

        option_page = inspect_page(
            "<form><select name='csrf'><option value='short'>option-secret</option></select></form>",
            "https://example.test/jsxsd/page",
        )
        option = option_page["forms"][0]["fields"][0]["options"][0]
        self.assertEqual(option["text"], "")
        self.assertEqual(option["value"], "")
        self.assertNotIn("option-secret", str(option_page))
        self.assertIn("token", page["actions"][0]["fields"])

    def test_specialized_result_urls_redact_display_tokens(self):
        page_url = "https://example.test/jsxsd/page?ticket=private-ticket"
        profile = parse_profile(
            "<table id='xjkpTable'><tr><td>姓名：张三</td></tr></table>",
            page_url,
        )
        self.assertEqual(profile["url"], "https://example.test/jsxsd/page?ticket=%3Credacted%3E")

        source = (
            "<form action='/jsxsd/xspj_save.do?ticket=private-ticket'><table><tr>"
            "<td><input name='pj06xh' value='q1'></td><td><input type='radio' name='answer' value='1'></td>"
            "</tr></table></form>"
        )
        safe = parse_evaluation_form(source, page_url)
        self.assertNotIn("private-ticket", str(safe))
        internal = parse_evaluation_form(source, page_url, include_sensitive=True)
        self.assertIn("private-ticket", internal["action"])

    def test_disabled_controls_are_not_exposed_as_actions(self):
        page = inspect_page(
            "<form><button disabled>不可用</button><button onclick='return false'>无操作</button><button>可用</button></form>",
            "http://example.test/jsxsd/page",
        )
        self.assertEqual([action["text"] for action in page["actions"]], ["可用"])
        page = inspect_page(
            "<form onsubmit='return false'><button>表单禁用</button></form>",
            "http://example.test/jsxsd/page",
        )
        self.assertEqual(page["actions"], [])

        reset_document = parse_html("<form><button type='reset'>重置</button><button type='submit'>提交</button></form>")
        reset_form = reset_document.first("form")
        assert reset_form is not None
        self.assertEqual(_form_button(reset_form, 1).text(), "提交")
        inert_form = parse_html("<form><button onclick='return false'>无操作</button><button>提交</button></form>").first("form")
        assert inert_form is not None
        self.assertEqual(_form_button(inert_form, 1).text(), "提交")
        reset_page = inspect_page(
            "<form><button type='reset'>重置</button><button type='submit'>提交</button></form>",
            "http://example.test/jsxsd/page",
        )
        self.assertEqual([action["text"] for action in reset_page["actions"]], ["提交"])

        non_submit_page = inspect_page(
            "<form action='/save' method='post'><input type='button' value='普通输入按钮'>"
            "<button type='button'>普通按钮</button><button>提交</button></form>",
            "http://example.test/jsxsd/page",
        )
        self.assertEqual(
            [action["text"] for action in non_submit_page["actions"]],
            ["普通输入按钮", "普通按钮", "提交"],
        )
        self.assertEqual([action["target"] for action in non_submit_page["actions"][:2]], ["", ""])
        formaction_page = inspect_page(
            "<form action='/save' method='post'><button type='button' formaction='/danger'>普通按钮</button>"
            "<button formaction='/save'>提交</button></form>",
            "https://example.test/jsxsd/page",
        )
        self.assertEqual([action["target"] for action in formaction_page["actions"]], ["", "https://example.test/save"])
        non_submit_document = parse_html(
            "<form><input type='button' value='普通输入按钮'><button type='button'>普通按钮</button>"
            "<button>提交</button></form>"
        )
        non_submit_form = non_submit_document.first("form")
        assert non_submit_form is not None
        self.assertEqual(_form_button(non_submit_form, 1).text(), "提交")

        unresolved_submitter = inspect_page(
            "<form action='/save' method='post'><button type='button' onclick='doSomething()'>伪提交</button></form>",
            "https://example.test/jsxsd/page",
        )
        self.assertEqual(unresolved_submitter["actions"][0]["target"], "")
        submitted_by_script = inspect_page(
            "<form action='/save' method='post'><select onchange='this.form.submit()'><option>提交</option></select></form>",
            "https://example.test/jsxsd/page",
        )
        self.assertEqual(submitted_by_script["actions"][0]["target"], "https://example.test/save")

        link_page = inspect_page(
            "<form action='/save' method='post'><input type='hidden' name='token' value='secret'>"
            "<a href='/jsxsd/link'>普通链接</a></form>",
            "https://example.test/jsxsd/page",
        )
        self.assertEqual(link_page["actions"][0]["fields"], [])

        self.assertEqual(inspect_page("<button>无表单按钮</button>", "https://example.test/jsxsd/page")["actions"], [])
        unresolved = inspect_page(
            "<button onclick=\"alert('提示')\">只提示</button>",
            "https://example.test/jsxsd/page",
        )
        self.assertEqual(unresolved["actions"][0]["target"], "")

    def test_disabled_fieldset_and_optgroup_follow_form_rules(self):
        source = """
        <form><fieldset disabled><legend><button type='button'>例外</button></legend>
        <input name='blocked' value='bad'><button type='button'>禁用</button></fieldset>
        <select name='term'><optgroup disabled><option selected value='old'>旧</option></optgroup>
        <option value='new'>新</option></select></form>
        """
        document = parse_html(source)
        form = document.first("form")
        assert form is not None
        self.assertEqual(_form_fields(form), [("term", "new")])
        page = inspect_page(source, "http://example.test/jsxsd/page")
        self.assertEqual([action["text"] for action in page["actions"]], ["例外"])
        fields = page["forms"][0]["fields"]
        self.assertTrue(next(field for field in fields if field["name"] == "blocked")["disabled"])
        term = next(field for field in fields if field["name"] == "term")
        self.assertTrue(term["options"][0]["disabled"])

    def test_web_catalog_and_javascript_action_mapping(self):
        self.assertEqual(len(ROUTE_CATALOG), 69)
        self.assertEqual(len({item[0] for item in ROUTE_CATALOG}), 69)
        page = inspect_page(
            """<script>
            function apply(id) { document.forms[0].action = '/jsxsd/demo/save?id=' + id; document.forms[0].submit(); }
            </script><form method='post'><input name='token' value='hidden'>
            <button onclick='apply(7)'>提交</button></form>""",
            "http://example.test/jsxsd/demo",
        )
        action = page["actions"][0]
        self.assertEqual(action["method"], "POST")
        self.assertEqual(action["target"], "http://example.test/jsxsd/demo/save?id=7")

    def test_web_action_uses_form_method_and_encodes_javascript_values(self):
        page = inspect_page(
            """<script>
            function openPage(value) { location.href = '/jsxsd/demo?name=' + encodeURIComponent(value); }
            </script><form method='get' action='/fallback'><button formaction='/jsxsd/demo'>打开</button></form>
            <a onclick=\"openPage('a b')\">打开页面</a>""",
            "http://example.test/jsxsd/page",
        )
        form_action, javascript_action = page["actions"]
        self.assertEqual(form_action["method"], "GET")
        self.assertEqual(form_action["target"], "http://example.test/jsxsd/demo")
        self.assertEqual(javascript_action["target"], "http://example.test/jsxsd/demo?name=a%20b")

        indirect_submit = inspect_page(
            "<script>function apply(){document.forms[0].action='/jsxsd/demo/save'; document.forms[0].submit();}</script>"
            "<form method='post'><input type='hidden' name='token' value='secret'>"
            "<button type='button' onclick='apply()'>提交</button></form>",
            "https://example.test/jsxsd/demo",
        )
        self.assertEqual(indirect_submit["actions"][0]["target"], "https://example.test/jsxsd/demo/save")
        self.assertEqual(indirect_submit["actions"][0]["fields"], ["token"])
        href_submit = inspect_page(
            "<form action='/save' method='post'><input type='hidden' name='token' value='secret'>"
            "<a href='javascript:this.form.submit()'>提交链接</a></form>",
            "https://example.test/jsxsd/page",
        )
        self.assertEqual(href_submit["actions"][0]["target"], "https://example.test/save")
        self.assertEqual(href_submit["actions"][0]["fields"], ["token"])
        inert_href_action = inspect_page(
            "<a href='javascript:void(0)' onclick=\"location.href='/jsxsd/link'\">打开链接</a>",
            "https://example.test/jsxsd/page",
        )
        self.assertEqual(inert_href_action["actions"][0]["target"], "https://example.test/jsxsd/link")

        nested = inspect_page(
            """<script>
            function apply(id, name) { document.forms[0].action = '/jsxsd/demo?id=' + id + '&name=' + name; document.forms[0].submit(); }
            </script><form method='post'><button onclick=\"apply(7, encodeURIComponent('a b'))\">提交</button></form>""",
            "http://example.test/jsxsd/page",
        )
        self.assertEqual(nested["actions"][0]["target"], "http://example.test/jsxsd/demo?id=7&name=a%20b")

        response = Response(
            "https://example.test/jsxsd/page",
            200,
            {},
            "<button onclick=\"alert('提示')\">只提示</button>",
        )
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            args = SimpleNamespace(index=1, data=[], param=[], yes=True, output=None)
            with mock.patch("csust_cli.features.web._get_page", return_value=(response, {})), mock.patch.object(client, "request") as request:
                with self.assertRaises(CsustError) as raised:
                    _run_action_common(args, client, "/jsxsd/page")
            self.assertEqual(raised.exception.code, "action_not_found")
            request.assert_not_called()

            response = Response(
                "https://example.test/jsxsd/page",
                200,
                {},
                "<form action='/save' method='post'><button type='button' onclick='doSomething()'>伪提交</button></form>",
            )
            form_args = SimpleNamespace(form=1, button=1, data=[], param=[], yes=True, output=None)
            with mock.patch("csust_cli.features.web._get_page", return_value=(response, {})), mock.patch.object(client, "request") as request:
                with self.assertRaises(CsustError) as raised:
                    _run_form(form_args, client, "/jsxsd/page")
            self.assertEqual(raised.exception.code, "button_not_found")
            request.assert_not_called()

            link_response = Response(
                "https://example.test/jsxsd/page",
                200,
                {},
                "<form action='/save' method='post'><input type='hidden' name='token' value='secret'>"
                "<a href='/jsxsd/link'>普通链接</a></form>",
            )
            link_args = SimpleNamespace(index=1, data=[], param=[], yes=True, output=None)
            with mock.patch("csust_cli.features.web._get_page", return_value=(link_response, {})), mock.patch.object(
                client, "request", return_value=link_response
            ) as request:
                _run_action_common(link_args, client, "/jsxsd/page")
            self.assertEqual(request.call_args.args[0], "https://example.test/jsxsd/link")
            self.assertIsNone(request.call_args.kwargs["data"])

            direct_button_response = Response(
                "https://example.test/jsxsd/page",
                200,
                {},
                "<form action='/save' method='post'><input type='hidden' name='token' value='secret'>"
                "<button type='button' onclick=\"location='/jsxsd/link'\">打开链接</button></form>",
            )
            direct_button_args = SimpleNamespace(form=1, button=1, data=[], param=[], yes=True, output=None)
            with mock.patch("csust_cli.features.web._get_page", return_value=(direct_button_response, {})), mock.patch.object(
                client, "request", return_value=direct_button_response
            ) as request:
                _run_form(direct_button_args, client, "/jsxsd/page")
            self.assertEqual(request.call_args.args[0], "https://example.test/jsxsd/link")
            self.assertIsNone(request.call_args.kwargs["data"])

    def test_query_parameters_stay_before_url_fragments(self):
        self.assertEqual(_append_query("https://example.test/jsxsd/page?old=1#section", []), "https://example.test/jsxsd/page?old=1#section")
        self.assertEqual(
            _append_query("https://example.test/jsxsd/page?old=1#section", [("new", "two words")]),
            "https://example.test/jsxsd/page?old=1&new=two+words#section",
        )

    def test_disabled_select_options_are_not_submitted(self):
        source = """<form><select name='term'><option disabled selected value='old'>旧</option>
        <option value='new'>新</option></select></form>"""
        document = parse_html(source)
        form = document.first("form")
        assert form is not None
        select = form.first("select")
        assert select is not None
        self.assertEqual(_form_fields(form), [("term", "new")])
        self.assertEqual(_form_fields(form, select), [("term", "new")])
        self.assertEqual(form_fields(form, form, None), [("term", "new")])
        option = inspect_page(source, "http://example.test/jsxsd/page")["forms"][0]["fields"][0]["options"][0]
        self.assertTrue(option["disabled"])

        ordered_document = parse_html(
            "<form><textarea name='first'>1</textarea><input name='second' value='2'>"
            "<button name='submit' value='3'>提交</button><select name='last'>"
            "<option selected value='4'>四</option></select></form>"
        )
        ordered_form = ordered_document.first("form")
        ordered_button = ordered_form.first("button") if ordered_form else None
        assert ordered_form is not None and ordered_button is not None
        self.assertEqual(
            _form_fields(ordered_form, ordered_button),
            [("first", "1"), ("second", "2"), ("submit", "3"), ("last", "4")],
        )
        self.assertEqual(
            form_fields(ordered_form, ordered_form, ordered_button),
            [("first", "1"), ("second", "2"), ("submit", "3"), ("last", "4")],
        )

        button_document = parse_html("<form><button>第一个</button><input type='submit' value='第二个'></form>")
        button_form = button_document.first("form")
        assert button_form is not None
        self.assertEqual(_form_button(button_form, 1).text(), "第一个")
        self.assertEqual(_form_button(button_form, 2).attr("value"), "第二个")
        ordered_page = inspect_page(
            "<form><textarea name='first'>1</textarea><input name='second' value='2'>"
            "<button name='submit' value='3'>提交</button><select name='last'>"
            "<option selected value='4'>四</option></select></form>",
            "https://example.test/jsxsd/page",
        )
        self.assertEqual(
            [field["name"] for field in ordered_page["forms"][0]["fields"]],
            ["first", "second", "submit", "last"],
        )

        associated_source = (
            '<form id="target" action="/save" method="post"><input name="inside" value="1"></form>'
            '<input form="target" name="outside" value="2">'
            '<button form="target" name="submit" value="yes">提交</button>'
            '<input form="" name="orphan" value="3">'
        )
        associated_document = parse_html(associated_source)
        associated_form = associated_document.first("form")
        associated_button = associated_document.first("button")
        assert associated_form is not None and associated_button is not None
        self.assertEqual(
            _form_fields(associated_form, associated_button, associated_document),
            [("inside", "1"), ("outside", "2"), ("submit", "yes")],
        )
        self.assertIs(_form_button(associated_form, 1, associated_document), associated_button)
        associated_page = inspect_page(associated_source, "https://example.test/jsxsd/page")
        self.assertEqual(
            [field["name"] for field in associated_page["forms"][0]["fields"]],
            ["inside", "outside", "submit"],
        )

        self.assertEqual(_selected(_options(document, "term")), "new")
        self.assertEqual(selected_option(document, "term"), "new")

        disabled_select_document = parse_html(
            "<select id='term' disabled><option selected value='blocked'>禁用</option></select>"
        )
        self.assertEqual(_options(disabled_select_document, "term"), [])
        self.assertIsNone(selected_option(disabled_select_document, "term"))

        empty_value_document = parse_html(
            "<form><select name='term'><option selected value=''>请选择</option>"
            "<option>备用</option></select></form>"
        )
        empty_value_form = empty_value_document.first("form")
        assert empty_value_form is not None
        empty_value_select = empty_value_form.first("select")
        assert empty_value_select is not None
        self.assertEqual(_form_fields(empty_value_form), [("term", "")])
        self.assertEqual(form_fields(empty_value_form, empty_value_form, None), [("term", "")])
        self.assertIsNone(selected_option(empty_value_document, "term"))
        self.assertEqual(_element_value(empty_value_select), "")

        default_select_document = parse_html(
            "<form><select name='term'><option value='first'>第一项</option><option value='second'>第二项</option></select></form>"
        )
        default_select_form = default_select_document.first("form")
        assert default_select_form is not None
        self.assertEqual(form_fields(default_select_form, default_select_form, None), [("term", "first")])

        missing_value_document = parse_html(
            "<form><select name='term'><option selected>显示值</option></select></form>"
        )
        missing_value_select = missing_value_document.first("select")
        assert missing_value_select is not None
        self.assertEqual(_form_fields(missing_value_document.first("form")), [("term", "显示值")])
        self.assertEqual(_element_value(missing_value_select), "显示值")

        checkbox_document = parse_html(
            "<form><input type='checkbox' name='flag' checked>"
            "<input type='radio' name='choice' checked>"
            "<input type='checkbox' name='empty' value='' checked></form>"
        )
        checkbox_form = checkbox_document.first("form")
        assert checkbox_form is not None
        self.assertEqual(
            _form_fields(checkbox_form),
            [("flag", "on"), ("choice", "on"), ("empty", "")],
        )
        self.assertEqual(form_fields(checkbox_form, checkbox_form, None), [("flag", "on"), ("choice", "on"), ("empty", "")])

        image_document = parse_html(
            "<form><input type='image' name='submit' src='submit.png'></form>"
        )
        image_form = image_document.first("form")
        assert image_form is not None
        image = _form_button(image_form, 1)
        assert image is not None
        self.assertEqual(_form_fields(image_form, image), [("submit.x", "0"), ("submit.y", "0")])
        self.assertEqual(form_fields(image_form, image_form, image), [("submit.x", "0"), ("submit.y", "0")])

        button_document = parse_html("<form><button name='operation'>显示文字</button></form>")
        button_form = button_document.first("form")
        assert button_form is not None
        button = _form_button(button_form, 1)
        assert button is not None
        self.assertEqual(_form_fields(button_form, button), [("operation", "")])
        self.assertEqual(form_fields(button_form, button_form, button), [("operation", "")])

        file_document = parse_html("<form><input type='file' name='local' value='/private/file'></form>")
        file_form = file_document.first("form")
        assert file_form is not None
        self.assertEqual(_form_fields(file_form), [])
        self.assertEqual(form_fields(file_form, file_form, None), [])

        multiple_document = parse_html(
            "<form><select name='tag' multiple><option value='a'>甲</option>"
            "<option value='b'>乙</option></select></form>"
        )
        multiple_form = multiple_document.first("form")
        multiple_select = multiple_document.first("select")
        assert multiple_form is not None and multiple_select is not None
        self.assertEqual(_form_fields(multiple_form), [])
        self.assertEqual(form_fields(multiple_form, multiple_form, None), [])
        self.assertEqual(_element_value(multiple_select), "")
        multiple_select.first("option").attrs["selected"] = ""
        self.assertEqual(_form_fields(multiple_form), [("tag", "a")])

    def test_javascript_action_preserves_explicit_empty_state(self):
        source = """<script>
        function clearField() { document.getElementById('filter').value = ''; }
        </script><form><input name='filter' value='old'><button onclick='clearField()'>清空</button></form>"""
        document = parse_html(source)
        button = document.first("button")
        assert button is not None
        self.assertEqual(_js_state_fields(button, button.parent, document), [("filter", "")])

    def test_download_rejects_login_html_before_writing(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "export.html"
            response = Response(
                "http://example.test/export",
                200,
                {},
                "<html><form id='loginForm'><input name='userPassword'></form></html>",
            )
            with self.assertRaises(CsustError) as raised:
                _write_download(response, str(output), require_session=True)
            self.assertEqual(raised.exception.code, "login_required")
            self.assertFalse(output.exists())

            with self.assertRaises(CsustError) as raised:
                _write_download(Response("http://example.test/export", 200, {}, b"data"), directory, require_session=False)
            self.assertEqual(raised.exception.code, "file_write_failed")

            output = Path(directory) / "data.bin"
            result = _write_download(Response("http://example.test/export", 200, {}, b"data"), str(output), require_session=False)
            self.assertEqual(result["bytes"], 4)
            self.assertEqual(stat.S_IMODE(output.stat().st_mode), 0o600)

            metadata_output = Path(directory) / "metadata.bin"
            result = _write_download(
                Response("http://example.test/export", 200, {"cOnTeNt-TyPe": "application/pdf"}, b"data"),
                str(metadata_output),
                require_session=False,
            )
            self.assertEqual(result["content_type"], "application/pdf")

            output.write_bytes(b"old")
            with mock.patch("csust_cli.core.os.replace", side_effect=OSError("disk full")):
                with self.assertRaises(CsustError) as raised:
                    _write_download(Response("http://example.test/export", 200, {}, b"new"), str(output), require_session=False)
            self.assertEqual(raised.exception.code, "file_write_failed")
            self.assertEqual(output.read_bytes(), b"old")

            with self.assertRaises(CsustError) as raised:
                _write_download(Response("http://example.test/export", 200, {}, b"new"), str(Path(directory) / "bad\0"), require_session=False)
            self.assertEqual(raised.exception.code, "file_write_failed")

            victim = Path(directory) / "victim.bin"
            victim.write_bytes(b"old")
            link = Path(directory) / "link.bin"
            link.symlink_to(victim)
            with self.assertRaises(CsustError) as raised:
                _write_download(Response("http://example.test/export", 200, {}, b"new"), str(link), require_session=False)
            self.assertEqual(raised.exception.code, "file_write_failed")
            self.assertEqual(victim.read_bytes(), b"old")

            class FakeCaptchaClient:
                cookie_file = Path(directory) / "cookies.txt"

                def request(self, *_args, **_kwargs):
                    return b"captcha"

            with self.assertRaises(CsustError) as raised:
                fetch_captcha(FakeCaptchaClient(), str(link))
            self.assertEqual(raised.exception.code, "captcha_write_failed")
            self.assertEqual(victim.read_bytes(), b"old")

            with self.assertRaises(CsustError) as raised:
                fetch_captcha(FakeCaptchaClient(), "~nonexistent/captcha.png")
            self.assertEqual(raised.exception.code, "captcha_write_failed")

            with self.assertRaises(CsustError) as raised:
                _write_download(Response("http://example.test/export", 200, {}, b"data"), "~nonexistent/export.bin", require_session=False)
            self.assertEqual(raised.exception.code, "file_write_failed")

    def test_empty_output_is_not_silently_ignored(self):
        class FakeClient:
            def request(self, *_args, **_kwargs):
                return Response("https://example.test/jsxsd/page", 200, {}, b"data")

        with self.assertRaises(CsustError) as raised:
            _request(FakeClient(), "GET", "https://example.test/jsxsd/page", output="", require_session=False)
        self.assertEqual(raised.exception.code, "invalid_argument")

    def test_output_failure_keeps_cookie_refresh(self):
        class FakeClient:
            def __init__(self, response):
                self.response = response
                self.saves = 0

            def request(self, *_args, **_kwargs):
                return self.response

            def save(self):
                self.saves += 1

        with tempfile.TemporaryDirectory() as directory:
            client = FakeClient(Response("https://example.test/jsxsd/page", 200, {"Set-Cookie": "SESSION=2"}, b"data"))
            with self.assertRaises(CsustError) as raised:
                _request(client, "POST", "https://example.test/jsxsd/page", output=directory, require_session=False)
        self.assertEqual(raised.exception.code, "file_write_failed")
        self.assertEqual(client.saves, 1)

        login_client = FakeClient(Response("https://example.test/", 200, {"Set-Cookie": "SESSION=3"}, LOGIN_PAGE))
        with self.assertRaises(LoginRequired):
            _request(login_client, "GET", "https://example.test/jsxsd/page", output="ignored.bin", require_session=True)
        self.assertEqual(login_client.saves, 0)

    def test_unverified_mutation_download_saves_once(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            output = Path(directory) / "data.bin"
            response = Response("https://example.test/jsxsd/page", 200, {"Set-Cookie": "SESSION=2"}, b"data")
            args = SimpleNamespace(path="/jsxsd/page", param=[], data=[], yes=True, output=str(output))
            with mock.patch.object(client, "request", return_value=response), mock.patch(
                "csust_cli.features.web.ensure_session"
            ), mock.patch.object(client, "save") as save:
                with self.assertRaises(MutationUnverified) as raised:
                    _run_post(args, client)
            self.assertTrue(raised.exception.details["downloaded"])
            self.assertEqual(output.read_bytes(), b"data")
            save.assert_called_once()

    def test_web_inspection_redacts_sensitive_events(self):
        page = inspect_page(
            '<a onclick="towptjbs(\'12345678901\',\'time\',\'secret-sign-value-123456789\')">毕业设计</a>',
            "http://xk.csust.edu.cn/jsxsd/framework/xsMain.jsp",
        )
        self.assertNotIn("12345678901", str(page))
        self.assertNotIn("secret-sign-value-123456789", str(page))

        page = inspect_page(
            "<script>var token='script-secret-value-123456789';</script><div>可见文本</div>",
            "http://example.test/jsxsd/page",
        )
        self.assertNotIn("script-secret-value-123456789", page["text"])
        self.assertEqual(page["text"], "可见文本")

        page = inspect_page(
            "<a href='https://user:pass@example.test/page#access_token=fragment-secret-value-123456789'>链接</a>",
            "http://example.test/jsxsd/page",
        )
        self.assertNotIn("user:pass", str(page))
        self.assertNotIn("fragment-secret-value-123456789", str(page))
        self.assertIn("example.test/page", page["links"][0]["href"])

        page = inspect_page(
            "<a href='https://example.test/jsxsd/secure'>跨协议链接</a>",
            "http://example.test/jsxsd/page",
        )
        self.assertEqual(page["links"][0]["path"], "https://example.test/jsxsd/secure")

        page = inspect_page(
            "<a href='/jsxsd/page?term=2025&ticket=private-ticket-value'>带参数链接</a>",
            "https://example.test/jsxsd/home",
        )
        self.assertEqual(page["links"][0]["path"], "/jsxsd/page?term=2025&ticket=%3Credacted%3E")
        self.assertNotIn("private-ticket-value", str(page))

        page = inspect_page(
            "<a href='/jsxsd/page;jsessionid=short-session?session=short-session'>会话参数链接</a>",
            "https://example.test/jsxsd/home",
        )
        self.assertNotIn("short-session", str(page))
        self.assertIn("/jsxsd/page;jsessionid=%3Credacted%3E", page["links"][0]["path"])

        page = inspect_page(
            "<a href='/jsxsd/page;token=short-secret;state=visible'>分号参数链接</a>",
            "https://example.test/jsxsd/home",
        )
        self.assertEqual(page["links"][0]["path"], "/jsxsd/page;token=%3Credacted%3E;state=visible")
        self.assertNotIn("short-secret", str(page))

        page = inspect_page(
            "<a href='/jsxsd/page;state=opaque-path-token-123456789'>无关键词路径令牌</a>",
            "https://example.test/jsxsd/home",
        )
        self.assertEqual(page["links"][0]["path"], "/jsxsd/page;state=%3Credacted%3E")
        self.assertNotIn("opaque-path-token-123456789", str(page))

        page = inspect_page("<div>可见\x1b[31m文本\x07\b</div>", "https://example.test/jsxsd/home")
        self.assertNotIn("\x1b", page["text"])
        self.assertNotIn("\x07", page["text"])
        self.assertNotIn("\b", page["text"])

        page = inspect_page(
            "<a href='/jsxsd/page?t%6fken=short-secret&nonce=nonce-secret'>编码查询参数链接</a>",
            "https://example.test/jsxsd/home",
        )
        self.assertNotIn("short-secret", str(page))
        self.assertNotIn("nonce-secret", str(page))

        page = inspect_page(
            "<a href='/jsxsd/page#eyJhbGciOiJIUzI1NiJ9.opaque-fragment-token'>无关键词片段</a>",
            "https://example.test/jsxsd/home",
        )
        self.assertNotIn("opaque-fragment-token", str(page))

        page = inspect_page(
            "<a href='/jsxsd/page?state=opaque-query-token-123456789'>无关键词查询令牌</a>",
            "https://example.test/jsxsd/home",
        )
        self.assertEqual(page["links"][0]["path"], "/jsxsd/page?state=%3Credacted%3E")
        self.assertNotIn("opaque-query-token-123456789", str(page))

        page = inspect_page(
            "<title>ticket=short-secret</title><script>alert('ticket=short-secret')</script>",
            "https://example.test/jsxsd/home",
        )
        self.assertNotIn("short-secret", str(page))
        self.assertEqual(page["messages"], ["ticket=<redacted>"])

        page = inspect_page(
            "<a onclick=\"window.location='/jsxsd/page?ticket=short-secret'\">带事件参数链接</a>",
            "https://example.test/jsxsd/home",
        )
        self.assertNotIn("short-secret", str(page))

        page = inspect_page(
            "<form><input type='file' name='upload' value='/Users/private/report.pdf'></form>",
            "https://example.test/jsxsd/page",
        )
        self.assertEqual(page["forms"][0]["fields"][0]["value"], "")

    def test_text_renderers_strip_terminal_controls(self):
        payload = "\x1b[31m危险\x07\b"
        self.assertEqual(_safe_terminal_text("a\nb\tc\rd"), "a b c d")
        self.assertEqual(_safe_terminal_text("\x1b[31m危险\x1b[0m"), "危险")
        self.assertEqual(_safe_terminal_text("\x1b]0;标题\x07危险"), "危险")
        cases = (
            (render_schedule, {"items": [{"weekday": payload, "sections": [1], "weeks": [1], "course": payload, "teacher": payload, "room": payload}]}),
            (render_grades, {"items": [{"semester": payload, "course": payload, "score": payload, "credit": payload, "grade_point": payload}]}),
            (render_textbooks, {"items": [{"index": 1, "course": payload, "title": payload, "status": payload}]}),
            (render_academic, {"items": [{"index": 1, "course": payload, "exam_time": payload, "room": payload, "seat": payload}]}),
            (render_web, {"page": {"title": payload, "url": payload, "forms": [], "actions": []}}),
        )
        for renderer, data in cases:
            with self.subTest(renderer=renderer.__module__), contextlib.redirect_stdout(io.StringIO()) as output:
                renderer(data)
            for character in ("\x1b", "\x07", "\b"):
                self.assertNotIn(character, output.getvalue())

    def test_safe_url_helpers_reject_malformed_inputs(self):
        self.assertEqual(_safe_url("https://example.test:bad/page"), "")
        self.assertEqual(_safe_urljoin("https://example.test/page", "\n/next"), "")

    def test_web_inspection_ignores_malformed_page_urls(self):
        page = inspect_page(
            "<a href='http://[bad'>坏链接</a><form action='http://[bad'><button>提交</button></form>"
            "<table><tr><td>可见<script>var token='table-secret-value-123456789';</script></td></tr></table>",
            "http://example.test/jsxsd/page",
        )
        self.assertEqual(page["links"][0]["href"], "")
        self.assertEqual(page["forms"][0]["action"], "")
        self.assertEqual(page["actions"][0]["target"], "")
        self.assertEqual(page["tables"][0]["rows"], [["可见"]])
        self.assertNotIn("table-secret-value-123456789", str(page))

    def test_mutation_verification_rejects_explicit_failure(self):
        self.assertTrue(_mutation_verified({"text": "操作成功"}))
        self.assertFalse(_mutation_verified({"text": "提交失败；历史记录显示提交成功"}))
        self.assertFalse(_mutation_verified({"text": "提交未成功"}))
        self.assertFalse(_mutation_verified({"text": "提交不成功"}))
        self.assertFalse(_mutation_verified({"text": "操作未完成"}))
        page = inspect_page("<script>alert('操作成功')</script>", "http://example.test/jsxsd/save")
        self.assertEqual(page["messages"], ["操作成功"])
        self.assertTrue(_mutation_verified(page))
        self.assertFalse(_mutation_verified(inspect_page("<script>showMsg('提交失败')</script>", "http://example.test/jsxsd/save")))
        message = response_message("<script>var token='script-secret-value-123456789'; alert('操作成功')</script>")
        self.assertEqual(message, "操作成功")
        self.assertEqual(response_message("<script>alert('ticket=short-secret')</script>"), "ticket=<redacted>")
        self.assertEqual(_response_message("<script>alert('ticket=short-secret')</script>"), "ticket=<redacted>")

    def test_web_request_rejects_unsupported_method(self):
        with self.assertRaises(CsustError) as raised:
            _request(object(), "TRACE", "http://example.test/jsxsd/page")
        self.assertEqual(raised.exception.code, "invalid_argument")

        response = Response("https://example.test/jsxsd/page", 200, {}, "<form method='TRACE'><button>提交</button></form>")
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            args = SimpleNamespace(param=[], data=[], form=1, button=None, yes=False, output=None)
            with mock.patch("csust_cli.features.web._get_page", return_value=(response, {})):
                with self.assertRaises(CsustError) as raised:
                    _run_form(args, client, "/jsxsd/page")
            self.assertEqual(raised.exception.code, "invalid_argument")

            action_response = Response(
                "https://example.test/jsxsd/page",
                200,
                {},
                "<form><button formaction='/jsxsd/page' formmethod='TRACE'>提交</button></form>",
            )
            action_args = SimpleNamespace(param=[], data=[], index=1, yes=False, output=None)
            with mock.patch("csust_cli.features.web._get_page", return_value=(action_response, {})):
                with self.assertRaises(CsustError) as raised:
                    _run_action_common(action_args, client, "/jsxsd/page")
            self.assertEqual(raised.exception.code, "invalid_argument")

    def test_evaluation_rejects_unknown_answer_ids(self):
        response = Response(
            "http://example.test/jsxsd/xspj/form",
            200,
            {},
            """<form action='/xspj_save.do'><input type='hidden' name='token' value='t'>
            <table><tr><td><input name='pj06xh' value='q1'>题目</td>
            <td><input type='radio' name='pj0601id_q1' value='1'>好</td></tr></table>
            <button onclick='saveData()'>保存</button></form>""",
        )
        args = SimpleNamespace(
            evaluation_command="save",
            path="/jsxsd/xspj/form",
            yes=True,
            answer=["q1=1", "typo=1"],
            suggestion="",
        )
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            with mock.patch("csust_cli.features.academic.ensure_session"), mock.patch(
                "csust_cli.features.academic._evaluation_page", return_value=response
            ):
                with self.assertRaises(CsustError) as raised:
                    _run_evaluation(args, client)
        self.assertEqual(raised.exception.code, "invalid_answer")

    def test_disabled_evaluation_save_control_is_read_only(self):
        form = parse_evaluation_form(
            "<form action='/xspj_save.do'><input disabled type='hidden' name='stale' value='bad'>"
            "<table><tr><td><input disabled name='pj06xh' value='q1'></td>"
            "<td><input type='radio' name='answer' value='1'>好</td></tr></table>"
            "<textarea disabled name='suggestion'>旧建议</textarea>"
            "<button disabled onclick='saveData()'>保存</button></form>",
            "https://example.test/jsxsd/xspj/form",
        )
        self.assertTrue(form["read_only"])
        self.assertEqual(form["hidden_fields"], [])
        self.assertEqual(form["questions"], [])
        self.assertEqual(form["suggestion_field"], "")

    def test_evaluation_argument_errors_do_not_start_login(self):
        args = SimpleNamespace(evaluation_command="form", path="")
        with mock.patch("csust_cli.features.academic.ensure_session") as ensure:
            with self.assertRaises(CsustError) as raised:
                _run_evaluation(args, object())
        self.assertEqual(raised.exception.code, "invalid_argument")
        ensure.assert_not_called()

    def test_evaluation_and_grade_paths_validate_before_login(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            evaluation_args = SimpleNamespace(evaluation_command="form", path="https://evil.example/form")
            detail_args = SimpleNamespace(path="/outside")
            with mock.patch("csust_cli.features.academic.ensure_session") as evaluation_ensure:
                with self.assertRaises(CsustError) as raised:
                    _run_evaluation(evaluation_args, client)
            self.assertEqual(raised.exception.code, "invalid_path")
            evaluation_ensure.assert_not_called()
            with mock.patch("csust_cli.features.grades.ensure_session") as detail_ensure:
                with self.assertRaises(CsustError) as raised:
                    run_detail(detail_args, client)
            self.assertEqual(raised.exception.code, "invalid_path")
            detail_ensure.assert_not_called()

    def test_generic_path_rejects_external_url_as_json_error(self):
        output = io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(main(["--json", "web", "get", "--path", "https://example.com/"]), 2)
        self.assertEqual(json.loads(output.getvalue())["code"], "invalid_path")

    def test_generic_path_rejects_file_and_malformed_urls(self):
        for path in ("file://192.168.3.125/FineReport", "http://[bad", "//[bad"):
            with self.subTest(path=path):
                output = io.StringIO()
                with contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                    self.assertEqual(main(["--json", "web", "get", "--path", path]), 2)
                self.assertEqual(json.loads(output.getvalue())["code"], "invalid_path")

    def test_json_value_is_not_treated_as_json_flag(self):
        output = io.StringIO()
        error = io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(error):
            self.assertEqual(main(["web", "get", "--path", "https://example.com/", "--param", "note=--json"]), 2)
        self.assertEqual(output.getvalue(), "")
        self.assertIn("错误:", error.getvalue())

    def test_static_catalog_does_not_read_cookie_file(self):
        with tempfile.TemporaryDirectory() as directory, mock.patch.dict(
            os.environ, {"CSUST_BASE_URL": "file:///invalid", "CSUST_COOKIE_FILE": directory}, clear=False
        ):
            output = io.StringIO()
            with contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["--json", "web", "catalog"]), 0)
        self.assertEqual(len(json.loads(output.getvalue())["catalog"]), 69)

    def test_request_argument_errors_do_not_start_login(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            args = SimpleNamespace(method="GET", yes=False, path="/jsxsd/page", param=[], data=["malformed"], output=None)
            with mock.patch("csust_cli.features.web.ensure_session") as ensure:
                with self.assertRaises(CsustError) as raised:
                    _run_request(args, client)
            self.assertEqual(raised.exception.code, "invalid_argument")
            ensure.assert_not_called()

    def test_form_action_data_errors_precede_page_fetch(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            form_args = SimpleNamespace(param=[], data=["malformed"], form=1, button=None, yes=False, output=None)
            action_args = SimpleNamespace(param=[], data=["malformed"], index=1, yes=False, output=None)
            with mock.patch("csust_cli.features.web._get_page") as get_page:
                for runner, args in ((_run_form, form_args), (_run_action_common, action_args)):
                    with self.assertRaises(CsustError) as raised:
                        runner(args, client, "/jsxsd/page")
                    self.assertEqual(raised.exception.code, "invalid_argument")
            get_page.assert_not_called()

    def test_route_rejects_button_with_action_before_page_fetch(self):
        args = SimpleNamespace(form=None, action=1, button=2, data=[], param=[], output=None, route_path="/jsxsd/page")
        with mock.patch("csust_cli.features.web._get_page") as get_page:
            with self.assertRaises(CsustError) as raised:
                _run_route(args, object())
        self.assertEqual(raised.exception.code, "invalid_argument")
        get_page.assert_not_called()

    def test_web_indices_reject_non_positive_values_before_page_fetch(self):
        form_args = SimpleNamespace(form=0, button=None, data=[], param=[], yes=False, output=None)
        button_args = SimpleNamespace(form=1, button=0, data=[], param=[], yes=False, output=None)
        action_args = SimpleNamespace(index=0, data=[], param=[], yes=False, output=None)
        with mock.patch("csust_cli.features.web._get_page") as get_page:
            for runner, args, code in (
                (_run_form, form_args, "form_not_found"),
                (_run_form, button_args, "button_not_found"),
                (_run_action_common, action_args, "action_not_found"),
            ):
                with self.subTest(code=code), self.assertRaises(CsustError) as raised:
                    runner(args, object(), "/jsxsd/page")
                self.assertEqual(raised.exception.code, code)
        get_page.assert_not_called()

    def test_generic_mutations_persist_updated_session(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            response = Response("https://example.test/jsxsd/page", 200, {}, "<script>alert('操作成功')</script>")
            with mock.patch("csust_cli.features.web.ensure_session"), mock.patch(
                "csust_cli.features.web._request", return_value=(response, None)
            ), mock.patch.object(client, "save") as save:
                post_result = _run_post(SimpleNamespace(path="/jsxsd/page", param=[], data=[], yes=True, output=None), client)
                request_result = _run_request(SimpleNamespace(method="POST", path="/jsxsd/page", param=[], data=[], yes=True, output=None), client)
            self.assertEqual(save.call_count, 2)
            self.assertTrue(post_result["confirmed"])
            self.assertTrue(request_result["confirmed"])

    def test_form_and_action_download_mutations_persist_updated_session(self):
        response = Response(
            "https://example.test/jsxsd/page",
            200,
            {},
            '<form action="/jsxsd/save" method="post"><input name="id" value="7"><button name="op" value="save">保存</button></form>',
        )
        saved = {"ok": True, "downloaded": True}
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            with mock.patch("csust_cli.features.web._get_page", return_value=(response, {})), mock.patch(
                "csust_cli.features.web._request", return_value=(response, saved)
            ), mock.patch.object(client, "save") as save:
                for runner, args in (
                    (_run_form, SimpleNamespace(param=[], data=[], form=1, button=None, yes=True, output="export.bin")),
                    (_run_action_common, SimpleNamespace(param=[], data=[], index=1, yes=True, output="export.bin")),
                ):
                    with self.assertRaises(MutationUnverified) as raised:
                        runner(args, client, "/jsxsd/page")
                    self.assertTrue(raised.exception.details["downloaded"])
                    self.assertTrue(raised.exception.details["submitted"])
                self.assertEqual(save.call_count, 2)

    def test_public_post_requires_success_signal(self):
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            args = SimpleNamespace(path="/findmm.jsp", param=[], data=[], yes=True, output=None)
            success = Response("https://example.test/findmm.jsp", 200, {"Set-Cookie": "PUBLIC=1"}, "<script>alert('邮件发送成功')</script>")
            with mock.patch("csust_cli.features.web._request", return_value=(success, None)), mock.patch.object(client, "save") as save:
                self.assertTrue(_run_public_post(args, client)["confirmed"])
            save.assert_called_once()
            failure = Response("https://example.test/findmm.jsp", 200, {"Set-Cookie": "PUBLIC=2"}, "<script>alert('发送失败')</script>")
            with mock.patch("csust_cli.features.web._request", return_value=(failure, None)), mock.patch.object(client, "save") as save:
                with self.assertRaises(CsustError) as raised:
                    _run_public_post(args, client)
                self.assertEqual(raised.exception.code, "mutation_rejected")
            save.assert_called_once()

    def test_graduation_design_requires_https_sso_target(self):
        self.assertEqual(
            _safe_external_url(
                "https://user:pass@oauth.fanyu.com/sso/cas/10536/1004;jsessionid=external-secret?ticket=secret#state"
            ),
            "https://oauth.fanyu.com/sso/cas/10536/1004;jsessionid=%3Credacted%3E",
        )
        source = """<a onclick="towptjbs('id'); window.location='file://oauth.fanyu.com/sso/cas/10536/1004'">毕业设计</a>"""
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url="https://example.test", cookie_file=Path(directory) / "cookies.txt", load_cookies=False)
            response = Response("https://example.test/jsxsd/framework/xsMain.jsp", 200, {}, source)
            with mock.patch("csust_cli.features.web.ensure_session"), mock.patch.object(client, "get", return_value=response):
                with self.assertRaises(CsustError) as raised:
                    _graduation_design_url(client)
        self.assertEqual(raised.exception.code, "invalid_path")

    def test_graduation_design_download_keeps_binary_response(self):
        class FakeClient:
            def __init__(self):
                self.calls = []

            def request(self, target, **kwargs):
                self.calls.append((target, kwargs))
                return Response(target, 200, {}, bytes((0x80, 0x81)))

        with tempfile.TemporaryDirectory() as directory, mock.patch(
            "csust_cli.features.web._graduation_design_url",
            return_value="https://oauth.fanyu.com/sso/cas/10536/1004",
        ) as get_url:
            client = FakeClient()
            with self.assertRaises(CsustError) as raised:
                _run_graduation_design(SimpleNamespace(fetch=False, output="ignored.bin"), client)
            self.assertEqual(raised.exception.code, "invalid_argument")
            get_url.assert_not_called()
            output = Path(directory) / "export.bin"
            result = _run_graduation_design(SimpleNamespace(fetch=True, output=str(output)), client)
            self.assertEqual(output.read_bytes(), bytes((0x80, 0x81)))
            self.assertTrue(result["download"]["downloaded"])
            self.assertEqual(client.calls[0][1], {"method": "GET", "binary": True, "with_metadata": True})
            with self.assertRaises(CsustError) as raised:
                _run_graduation_design(SimpleNamespace(fetch=True, output=""), client)
            self.assertEqual(raised.exception.code, "invalid_argument")

    def test_textbook_parser_exposes_normalized_fields_and_controls(self):
        source = textbook_page(False)
        result = parse_textbooks(source, "http://xk.csust.edu.cn/jsxsd/nxsjc/jccx")
        item = result["items"][0]
        self.assertEqual(item["index"], 1)
        self.assertEqual(item["course"], "线代")
        self.assertEqual(item["title"], "教材 A")
        self.assertEqual(item["status"], "未订")
        self.assertEqual(item["actions"], ["subscribe"])
        self.assertEqual(item["controls"][0]["text"], "选订")
        default_form = parse_html("<form action='/save'><button>选订</button></form>").first("form")
        assert default_form is not None
        self.assertEqual(action_url({"formaction": "/save"}, default_form, "https://example.test/page"), ("GET", "https://example.test/save"))
        self.assertEqual(
            action_url({"type": "button", "formaction": "/danger"}, default_form, "https://example.test/page"),
            ("GET", ""),
        )
        self.assertEqual(action_url({}, default_form, "https://example.test/page"), ("GET", "https://example.test/save"))
        self.assertEqual(
            action_url({"type": "button", "onclick": "this.form.submit()"}, default_form, "https://example.test/page"),
            ("GET", "https://example.test/save"),
        )
        self.assertEqual(
            action_url({"tag": "a", "type": "a", "href": "javascript:this.form.submit()"}, default_form, "https://example.test/page"),
            ("GET", "https://example.test/save"),
        )
        disabled = parse_textbooks(
            textbook_page(False).replace("<button name=", "<button disabled name=", 1),
            "http://xk.csust.edu.cn/jsxsd/nxsjc/jccx",
        )
        self.assertEqual(disabled["items"][0]["actions"], [])
        self.assertFalse(_same_item({"course": "线代", "title": "教材 A"}, {"course": "线代", "title": "教材 B"}))
        self.assertFalse(_state_confirms({"actions": ["subscribe", "unsubscribe"], "status": ""}, "subscribe"))
        self.assertFalse(_state_confirms({"actions": ["subscribe", "unsubscribe"], "status": ""}, "unsubscribe"))
        self.assertTrue(_state_confirms({"actions": ["subscribe", "unsubscribe"], "status": "已订"}, "subscribe"))
        legacy = parse_textbooks(
            "<table id='dataList'><tr><td>线代</td><td>教材 A</td><td>已订</td>"
            "<td><a href='/save?id=7'>退订</a></td></tr></table>",
            "http://xk.csust.edu.cn/jsxsd/xsjc/showXsjc",
        )
        self.assertEqual(legacy["items"][0]["status"], "已订")
        self.assertEqual(legacy["items"][0]["actions"], ["unsubscribe"])

        td_header = parse_textbooks(
            "<table id='jccx'><tr><td>序号</td><td>课程</td><td>教材</td><td>状态</td><td>操作</td></tr>"
            "<tr><td>1</td><td>线代</td><td>教材 A</td><td>未订</td><td><a href='/save'>选订</a></td></tr></table>",
            "https://example.test/jsxsd/nxsjc/jccx",
        )
        self.assertEqual(len(td_header["items"]), 1)
        self.assertEqual(td_header["items"][0]["course"], "线代")

        safe = parse_textbooks(
            "<table id='jccx'><tr><td>线代</td><td>教材 A</td><td>未订</td>"
            "<td><a href='/save?ticket=private-ticket-value' onclick=\"towptjbs('private-long-signature-value')\">选订</a></td></tr></table>",
            "https://example.test/jsxsd/nxsjc/jccx?token=private-page-token",
        )
        self.assertNotIn("private-ticket-value", str(safe))
        self.assertNotIn("private-long-signature-value", str(safe))
        self.assertEqual(safe["items"][0]["actions"], ["subscribe"])

        hidden_action = parse_textbooks(
            "<table id='jccx'><tr><td>线代</td><td>教材 A</td><td>未订</td>"
            "<td><input type='hidden' name='operation' value='subscribe'><button>选订</button></td></tr></table>",
            "https://example.test/jsxsd/nxsjc/jccx",
        )
        self.assertEqual(hidden_action["items"][0]["actions"], ["subscribe"])

        reset = parse_textbooks(
            "<table id='jccx'><tr><td>线代</td><td>教材 A</td><td>未订</td>"
            "<td><button type='reset'>选订</button></td></tr></table>",
            "https://example.test/jsxsd/nxsjc/jccx",
        )
        self.assertEqual(reset["items"][0]["actions"], [])

        non_submit = parse_textbooks(
            "<form action='/save' method='post'><table id='jccx'><tr><td>线代</td><td>教材 A</td><td>未订</td>"
            "<td><input type='button' value='选订'><button type='button'>选订</button></td></tr></table></form>",
            "https://example.test/jsxsd/nxsjc/jccx",
        )
        self.assertEqual(non_submit["items"][0]["actions"], [])

        inert = parse_textbooks(
            "<table id='jccx'><tr><td>线代</td><td>教材 A</td><td>未订</td>"
            "<td><a href='javascript:return false'>选订</a></td></tr></table>",
            "https://example.test/jsxsd/nxsjc/jccx",
        )
        self.assertEqual(inert["items"][0]["actions"], [])
        javascript_void = parse_textbooks(
            "<table id='jccx'><tr><td>线代</td><td>教材 A</td><td>未订</td>"
            "<td><a href='javascript:void(0)' onclick=\"location='/save'\">选订</a></td></tr></table>",
            "https://example.test/jsxsd/nxsjc/jccx",
        )
        self.assertEqual(javascript_void["items"][0]["actions"], ["subscribe"])

    def test_textbook_action_targets_selected_row_and_verifies_state(self):
        before = """
        <form action="/save" method="post"><input type="hidden" name="token" value="t"><table>
        <tr><th>课程</th><th>教材</th><th>操作</th></tr>
        <tr><td>高数</td><td>教材 B</td><td><input type="hidden" name="id" value="6"><button name="op" value="subscribe">选订</button></td></tr>
        <tr><td>线代</td><td>教材 A</td><td><input type="hidden" name="id" value="7"><button name="op" value="subscribe">选订</button></td></tr>
        </table></form>
        """
        after = before.replace('value="subscribe">选订', 'value="unsubscribe">退订', 1).replace('value="subscribe">选订', 'value="unsubscribe">退订', 1)

        class FakeClient:
            def __init__(self):
                self.base_url = "http://xk.csust.edu.cn"
                self.calls = []
                self.saved = False

            def url(self, path):
                return path

            def post(self, url, data, **kwargs):
                self.calls.append(("POST", url, data))
                return Response(url, 200, {}, "操作成功")

            def get(self, url, **kwargs):
                self.calls.append(("GET", url, None))
                return Response(url, 200, {}, after)

            def save(self):
                self.saved = True

        fake = FakeClient()
        response = Response("http://xk.csust.edu.cn/page", 200, {}, before)
        unsupported = before.replace(
            '<button name="op" value="subscribe">选订</button>',
            '<button formaction="/save" formmethod="TRACE" name="op" value="subscribe">选订</button>',
            1,
        )
        with self.assertRaises(CsustError) as raised:
            submit_textbook_action(fake, Response(response.url, 200, {}, unsupported), parse_html(unsupported), "subscribe", 1, None)
        self.assertEqual(raised.exception.code, "invalid_argument")

        blocked = before.replace('<form action="/save"', '<form onsubmit="return false" action="/save"', 1)
        with self.assertRaises(CsustError) as raised:
            submit_textbook_action(fake, Response(response.url, 200, {}, blocked), parse_html(blocked), "subscribe", 1, None)
        self.assertEqual(raised.exception.code, "parse_error")

        unresolved = before.replace(
            '<button name="op" value="subscribe">选订</button>',
            '<button type="button" onclick="subscribeNow()">选订</button>',
            1,
        )
        with self.assertRaises(CsustError) as raised:
            submit_textbook_action(fake, Response(response.url, 200, {}, unresolved), parse_html(unresolved), "subscribe", 1, None)
        self.assertEqual(raised.exception.code, "parse_error")

        result = submit_textbook_action(fake, response, parse_html(before), "subscribe", 2, None)
        self.assertTrue(result["ok"])
        self.assertTrue(result["confirmed"])
        self.assertIn(("id", "7"), fake.calls[0][2])
        self.assertNotIn(("id", "6"), fake.calls[0][2])
        self.assertTrue(fake.saved)

        shifted_after = """
        <form action="/save" method="post"><table>
        <tr><th>课程</th><th>教材</th><th>操作</th></tr>
        <tr><td>高数</td><td>教材 B</td><td><button name="op" value="unsubscribe">退订</button></td></tr>
        <tr><td>物理</td><td>教材 C</td><td><button name="op" value="unsubscribe">退订</button></td></tr>
        </table></form>
        """
        fake.get = lambda url, **kwargs: Response(url, 200, {}, shifted_after)
        with self.assertRaises(MutationUnverified):
            submit_textbook_action(fake, response, parse_html(before), "subscribe", 2, None)

        ambiguous_after = """
        <form action="/save" method="post"><table>
        <tr><th>课程</th><th>教材</th><th>操作</th></tr>
        <tr><td>线代</td><td>教材 A</td><td><button name="op" value="unsubscribe">退订</button></td></tr>
        <tr><td>线代</td><td>教材 A</td><td><button name="op" value="unsubscribe">退订</button></td></tr>
        </table></form>
        """
        fake.get = lambda url, **kwargs: Response(url, 200, {}, ambiguous_after)
        with self.assertRaises(MutationUnverified):
            submit_textbook_action(fake, response, parse_html(before), "subscribe", 2, None)

    def test_textbook_action_reports_unverified_without_retrying(self):
        source = textbook_page(False)

        class FakeClient:
            def __init__(self):
                self.base_url = "http://xk.csust.edu.cn"
                self.post_calls = 0
                self.get_calls = 0
                self.saved = False

            def url(self, path):
                return path

            def post(self, url, data, **kwargs):
                self.post_calls += 1
                return Response(url, 200, {}, "操作成功")

            def get(self, url, **kwargs):
                self.get_calls += 1
                return Response(url, 200, {}, "页面暂时不可用")

            def save(self):
                self.saved = True

        fake = FakeClient()
        with self.assertRaises(MutationUnverified) as raised:
            submit_textbook_action(fake, Response("http://xk.csust.edu.cn/page", 200, {}, source), parse_html(source), "subscribe", 1, None)
        self.assertEqual(raised.exception.code, "mutation_unverified")
        self.assertEqual(fake.post_calls, 1)
        self.assertTrue(fake.saved)

        class FailedClient(FakeClient):
            def post(self, url, data, **kwargs):
                self.post_calls += 1
                return Response(url, 200, {}, "操作未成功")

        failed = FailedClient()
        with self.assertRaises(CsustError) as raised:
            submit_textbook_action(
                failed,
                Response("http://xk.csust.edu.cn/page", 200, {}, source),
                parse_html(source),
                "subscribe",
                1,
                None,
            )
        self.assertEqual(raised.exception.code, "mutation_rejected")
        self.assertTrue(failed.saved)

    def test_login_and_queries_work_against_local_server(self):
        with tempfile.TemporaryDirectory() as directory, academic_server() as base_url:
            cookie_file = os.path.join(directory, "cookies.txt")
            environment = {"CSUST_BASE_URL": base_url, "CSUST_COOKIE_FILE": cookie_file, "CSUST_USERNAME": "a", "CSUST_PASSWORD": "b", "CSUST_CAPTCHA": ""}
            ocr_values = iter(("bad", "good"))

            def fake_ocr(_image: bytes) -> str:
                return next(ocr_values)

            with mock.patch.dict(os.environ, environment, clear=False), contextlib.redirect_stderr(io.StringIO()):
                result = login(
                    SimpleNamespace(username="a", captcha=None, captcha_image=None),
                    Client(base_url=base_url, cookie_file=Path(cookie_file), load_cookies=False),
                    ocr=fake_ocr,
                )
            self.assertEqual(result["attempts"], 2)
            self.assertEqual(AcademicHandler.login_attempts, 2)
            self.assertEqual(stat.S_IMODE(os.stat(cookie_file).st_mode), 0o600)

            output = io.StringIO()
            with mock.patch.dict(os.environ, environment, clear=False), contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["schedule", "--json"]), 0)
            schedule_result = json.loads(output.getvalue())
            self.assertEqual(schedule_result["items"][0]["course"], "线性代数")

            AcademicHandler.invalidate_next = True
            output = io.StringIO()
            with mock.patch.dict(os.environ, environment, clear=False), mock.patch("csust_cli.core.solve_captcha", return_value="good"), contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["grades", "--json"]), 0)
            grades_result = json.loads(output.getvalue())
            self.assertEqual(grades_result["items"][0]["score"], "95")
            self.assertGreaterEqual(AcademicHandler.login_attempts, 3)

            output = io.StringIO()
            with mock.patch.dict(os.environ, environment, clear=False), contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["textbooks", "subscribe", "--index", "1", "--yes", "--json"]), 0)
            textbook_result = json.loads(output.getvalue())
            self.assertTrue(textbook_result["confirmed"])
            self.assertTrue(AcademicHandler.subscribed)

    def test_missing_credentials_and_confirmation_are_machine_readable(self):
        with tempfile.TemporaryDirectory() as directory:
            environment = {"CSUST_COOKIE_FILE": os.path.join(directory, "cookies.txt"), "CSUST_USERNAME": "", "CSUST_PASSWORD": ""}
            output = io.StringIO()
            with mock.patch.dict(os.environ, environment, clear=False), contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["--json", "login"]), 2)
            self.assertEqual(json.loads(output.getvalue())["code"], "credentials_required")

            output = io.StringIO()
            with mock.patch.dict(os.environ, environment, clear=False), contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["--json", "textbooks", "subscribe", "--index", "1"]), 2)
            self.assertEqual(json.loads(output.getvalue())["code"], "confirmation_required")

            output = io.StringIO()
            with mock.patch.dict(os.environ, environment, clear=False), contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["--json", "textbooks", "subscribe", "--index", "1", "--match", "x", "--yes"]), 2)
            self.assertEqual(json.loads(output.getvalue())["code"], "invalid_target")

            output = io.StringIO()
            with mock.patch.dict(os.environ, environment, clear=False), contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["--json", "evaluation", "save", "--path", "/jsxsd/xspj/form"]), 2)
            self.assertEqual(json.loads(output.getvalue())["code"], "confirmation_required")

    def test_logout_clears_local_session_when_remote_logout_fails(self):
        class FakeClient:
            cookie_file = Path("/tmp/csust-test-cookies.txt")

            def __init__(self):
                self.cleared = False
                self.saved = False

            def get(self, _path):
                raise NetworkError("offline")

            def clear_cookies(self):
                self.cleared = True

            def save(self):
                self.saved = True

        client = FakeClient()
        with self.assertRaises(NetworkError):
            _run_logout(SimpleNamespace(), client)
        self.assertTrue(client.cleared)
        self.assertTrue(client.saved)

    def test_week_arguments_reject_non_positive_values(self):
        for arguments in (
            ["--json", "schedule", "--week", "0"],
            [
                "--json",
                "classrooms",
                "--campus",
                "yuntang",
                "--week",
                "-1",
                "--weekday",
                "1",
                "--section",
                "1",
            ],
        ):
            output = io.StringIO()
            with contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(arguments), 2)
            self.assertEqual(json.loads(output.getvalue())["code"], "invalid_argument")


if __name__ == "__main__":
    unittest.main()
