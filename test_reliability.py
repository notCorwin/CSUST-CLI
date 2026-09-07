"""Regression checks for CLI outcomes and real request encoding."""
import argparse
import contextlib
import io
import json
import tempfile
import unittest
from email.parser import BytesParser
from email.policy import default
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from csust_cli import cli
from csust_cli.core import Client, CsustError, NetworkError, Response, business_state, result_status
from csust_cli.features import teaching, quality, vpn, web
from test_teaching import TeachingHandler, teaching_server


class ReliabilityTests(unittest.TestCase):
    def test_outcome_contract(self):
        cases = [
            ({"code": 200, "messages": "操作成功"}, True),
            ({"code": 200, "messages": ["failure"]}, False),
            (["操作成功"], None),
            ({"code": 200}, True), ({"response": {"code": 0}}, True),
            ({"success": True, "response": {"success": False}}, False),
            ({"code": 200, "msg": "提交失败"}, False),
            ({"response": {"code": 200, "message": "操作未成功"}}, False),
            ({"code": 403}, False), ({"data": {"success": True}}, None),
            ("操作成功", True), ("邮件发送成功", True), ("收到请求", None),
            ("历史记录：提交成功", None),
            (web.inspect_page('<a>成功</a><form><button>提交</button></form>', 'https://example.test'), None),
            (web.inspect_page("<script>function later(){alert('操作成功');}</script>", 'https://example.test'), None),
            (web.inspect_page("<script>if(false){alert('操作成功');}</script>", 'https://example.test'), None),
        ]
        for payload, state in cases:
            with self.subTest(payload=payload):
                self.assertIs(business_state(payload), state)
                for mutating in (False, True):
                    if state is False or (mutating and state is None):
                        with self.assertRaises(CsustError) as raised:
                            result_status(payload, mutating=mutating)
                        expected = ('mutation_rejected' if mutating else 'business_rejected') if state is False else 'mutation_unverified'
                        self.assertEqual(raised.exception.code, expected)
                        self.assertFalse(raised.exception.details['confirmed'])
                    else:
                        self.assertTrue(result_status(payload, mutating=mutating)['ok'])

    def test_mixed_upload_keeps_repeated_and_empty_fields(self):
        original = TeachingHandler.do_POST
        captured = []
        def receive(handler):
            body = handler.rfile.read(int(handler.headers['Content-Length']))
            captured.append(BytesParser(policy=default).parsebytes(
                ('Content-Type: ' + handler.headers['Content-Type'] + '\r\n\r\n').encode() + body))
            handler._body('{"code":200}', 'application/json')
        with teaching_server() as base, tempfile.TemporaryDirectory() as directory, mock.patch.object(TeachingHandler, 'do_POST', receive):
            file = Path(directory) / 'upload.bin'
            file.write_bytes(b'\x00\xfffile')
            for module, cls in ((teaching, teaching.TeachingClient), (quality, quality.QualityClient)):
                client = cls(base_url=base, prefix='/http/gateway', cookie_file=Path(directory)/'cookies', session_file=Path(directory)/'session', load_cookies=False)
                client.session['token'] = 'vpn-token'
                if module is quality:
                    client.quality_logged_in = True
                args = SimpleNamespace(name=None, path='/meol/json', method='POST', data=['tag=a', 'tag=', 'courseId=1'], file=[f'file={file}'], param=[], data_json=None, yes=True, output=None, raw=False, referer='')
                self.assertTrue(module._run_request(args, client)['confirmed'])
                parts = list(captured[-1].iter_parts())
                self.assertEqual([part.get_param('name', header='content-disposition') for part in parts], ['tag', 'tag', 'courseId', 'file'])
                self.assertEqual([part.get_payload(decode=True) for part in parts], [b'a', b'', b'1', b'\x00\xfffile'])
                args.data_json = '{}'
                with mock.patch.object(client, 'request_web') as request:
                    with self.assertRaises(CsustError) as raised:
                        module._run_request(args, client)
                    self.assertEqual(raised.exception.code, 'invalid_argument')
                    request.assert_not_called()
        self.assertIs(TeachingHandler.do_POST, original)

    def test_unresolved_scripts_remain_inspectable_but_do_not_submit(self):
        source = '<script>function go(){var url="/jsxsd/save?id="+missing; location.href=url;}</script><a onclick="go()">保存</a>'
        page = web.inspect_page(source, 'https://example.test/jsxsd/page')
        self.assertEqual(page['actions'][0]['target'], '')
        self.assertIn('parse_error', page['actions'][0])
        response = Response('https://example.test/jsxsd/page', 200, {}, source)
        args = SimpleNamespace(data=[], index=1, param=[], yes=True, output=None, fingerprint=page['fingerprint'])
        with mock.patch.object(web, '_get_page', return_value=(response, page)), mock.patch.object(web, '_request') as request:
            with self.assertRaises(CsustError) as raised:
                web._run_action_common(args, object(), '/jsxsd/page')
            self.assertEqual(raised.exception.code, 'parse_error')
            request.assert_not_called()
        self.assertEqual(web._js_value("'/path?id='+id", {'id': ''}), '/path?id=')

    def test_form_and_action_apply_script_state(self):
        source = """<form action='/jsxsd/save' method='post'><input id='term' name='term' value='old'><button onclick='save()'>保存</button></form><script>function save(){document.getElementById('term').value='';document.forms[0].submit();}</script>"""
        page = Response('https://example.test/jsxsd/page', 200, {}, source)
        saved = Response('https://example.test/jsxsd/save', 200, {}, "<script>alert('操作成功')</script>")
        with tempfile.TemporaryDirectory() as directory:
            client = Client(base_url='https://example.test', cookie_file=Path(directory)/'cookies', load_cookies=False)
            for runner in (web._run_form, web._run_action_common):
                args = SimpleNamespace(data=[], param=[], form=1, button=1, index=1, yes=True, output=None, fingerprint='fp')
                with mock.patch.object(web, '_get_page', return_value=(page, {'fingerprint': 'fp'})), mock.patch.object(web, '_request', return_value=(saved, None)) as request:
                    self.assertTrue(runner(args, client, '/jsxsd/page')['confirmed'])
                    self.assertEqual(request.call_args.args[3], [('term', '')])

    def test_download_error_does_not_replace_existing_file(self):
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory)/'download'
            target.write_bytes(b'previous')
            for body in ('<form id="loginForm"></form>', '{"code":403}', '<script>alert("操作失败")</script>'):
                response = Response('https://example.test', 200, {}, body)
                with self.subTest(body=body), self.assertRaises(CsustError):
                    web._write_download(response, str(target), require_session=True)
                self.assertEqual(target.read_bytes(), b'previous')
            temporary = Path(directory) / 'stream.part'
            temporary.write_text('<form id="loginForm"></form>', encoding='utf-8')
            response = Response(
                'https://example.test',
                200,
                {'Content-Type': 'application/octet-stream'},
                temporary.read_bytes(),
                str(target),
                str(temporary),
            )
            with self.assertRaises(CsustError):
                web._write_download(response, str(target), require_session=True)
            self.assertEqual(target.read_bytes(), b'previous')
            self.assertFalse(temporary.exists())

    def test_login_and_bad_json_are_not_business_pages(self):
        for body, content_type, code in (
            ('<form action="/login"><input name="username"><input type="password"></form>', 'text/html', 'login_required'),
            ('{"broken":', 'application/json', 'parse_error'),
        ):
            with self.assertRaises(CsustError) as raised:
                teaching._response_payload(Response('https://example.test', 200, {'Content-Type':content_type}, body))
            self.assertEqual(raised.exception.code, code)
        self.assertIsNone(web._feedback(Response('https://example.test', 200, {'Content-Type':'application/octet-stream'}, b'file with error documentation')))

    def test_vpn_refresh_is_bounded_and_transport_errors_are_not_retried(self):
        with tempfile.TemporaryDirectory() as directory:
            client = vpn.VpnClient(base_url='https://example.test', cookie_file=Path(directory)/'cookies', session_file=Path(directory)/'session', load_cookies=False)
            expired = Response('https://example.test/api/save', 200, {}, '{"code":3010}')
            for refreshed, count in ((True, 1), (False, 1)):
                with mock.patch.object(client, 'request', return_value=expired) as request, mock.patch.object(client, 'refresh', return_value=refreshed) as refresh:
                    client.request_api('/api/save', method='POST', data={})
                    self.assertEqual(request.call_count, count)
                    refresh.assert_not_called()
            with mock.patch.object(client, 'request', side_effect=NetworkError('timeout')) as request, mock.patch.object(client, 'refresh') as refresh:
                with self.assertRaises(CsustError) as raised:
                    client.request_api('/api/save', method='POST', data={})
                self.assertEqual(raised.exception.code, 'mutation_unverified')
                request.assert_called_once()
                refresh.assert_not_called()

    def test_cli_exit_and_json_match_outcomes(self):
        for body, mutating, expected_code in (
            ('{"code":200}', True, None), ('{"code":500}', True, 'mutation_rejected'),
            ('{"data":{}}', True, 'mutation_unverified'), ('{"code":500}', False, 'business_rejected'),
        ):
            args = ['teaching', 'request', '--path', '/meol/test', '--method', 'POST' if mutating else 'GET', '--yes', '--json']
            out = io.StringIO()
            with mock.patch.object(teaching.TeachingClient, 'request_web', return_value=Response('https://example.test', 200, {'Content-Type':'application/json'}, body)), contextlib.redirect_stdout(out):
                status = cli.main(args)
            payload = json.loads(out.getvalue())
            self.assertEqual(status, 2 if expected_code else 0)
            self.assertEqual(payload.get('code'), expected_code)
            if expected_code:
                self.assertIn('error', payload)

    def test_dynamic_query_keeps_duplicates_and_suffix_before_query(self):
        self.assertEqual(vpn._resolve_path('/api/item?tag=old&keep=', [('tag', 'a'), ('tag', '')], ['a/b']), '/api/item/a%2Fb?keep=&tag=a&tag=')
        self.assertEqual(teaching._entry(None, '/meol/page', None)[1:3], ('GET', False))
        self.assertEqual(teaching._entry('personal', None, 'POST')[1:3], ('POST', True))

    def test_service_recovery_happens_only_once(self):
        expired = (Response('https://example.test', 200, {}, ''), {'code':3010})
        with tempfile.TemporaryDirectory() as directory:
            client = teaching.TeachingClient(base_url='https://example.test', cookie_file=Path(directory)/'cookies', session_file=Path(directory)/'session', load_cookies=False)
            with mock.patch.object(client, 'request_api', return_value=expired) as request, mock.patch.object(vpn, 'login', return_value={'ok':True}) as login:
                with self.assertRaises(CsustError):
                    client.ensure_service()
                login.assert_called_once()
                self.assertEqual(request.call_count, 2)
                self.assertTrue(all(call.kwargs['retry_refresh'] is False for call in request.call_args_list))

    def test_read_business_errors_and_invalid_outputs_are_nonzero(self):
        response = Response('https://example.test/jsxsd/page', 200, {'Content-Type':'application/json'}, '{"code":403}')
        for json_mode in (True, False):
            out, err = io.StringIO(), io.StringIO()
            with mock.patch.object(web, 'ensure_session'), mock.patch.object(Client, 'request', return_value=response), contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
                status = cli.main(['web', 'get', '--path', '/jsxsd/page'] + (['--json'] if json_mode else []))
            self.assertEqual(status, 2)
            if json_mode:
                self.assertEqual(json.loads(out.getvalue())['code'], 'business_rejected')
            else:
                self.assertEqual(out.getvalue(), '')
                self.assertIn('错误:', err.getvalue())
        with mock.patch.object(Client, 'request') as request, contextlib.redirect_stdout(io.StringIO()) as out:
            self.assertEqual(cli.main(['web','get','--path','/jsxsd/page','--output','','--json']), 2)
            self.assertEqual(json.loads(out.getvalue())['code'], 'invalid_argument')
            request.assert_not_called()

    def test_all_command_help_and_catalogs_are_offline(self):
        parser = cli.build_parser()
        def visit(parser):
            self.assertTrue(parser.format_help())
            for action in parser._actions:
                if isinstance(action, argparse._SubParsersAction):
                    for child in {id(p):p for p in action.choices.values()}.values():
                        visit(child)
        visit(parser)
        for command in ('web', 'vpn', 'teaching', 'quality'):
            with mock.patch.object(Client, 'request', side_effect=AssertionError('catalog must be offline')), contextlib.redirect_stdout(io.StringIO()) as out:
                self.assertEqual(cli.main([command, 'catalog', '--json']), 0)
            self.assertIsInstance(json.loads(out.getvalue()), dict)


if __name__ == '__main__':
    unittest.main()
