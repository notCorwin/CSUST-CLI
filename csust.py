#!/usr/bin/env python3
"""Compatibility launcher and public parser imports for the CSUST CLI."""

from csust_cli.cli import build_parser, main
from csust_cli.core import (
    CaptchaError,
    CaptchaRequired,
    Client,
    CredentialsRequired,
    CsustError,
    DEFAULT_BASE_URL,
    DAY_NAMES,
    Element,
    AuthenticationFailed,
    HttpError,
    LoginRequired,
    MutationUnverified,
    NetworkError,
    OcrUnavailable,
    ParseError,
    Response,
    ensure_session,
    fetch_captcha,
    generate_encoded,
    env_value,
    is_login_page,
    internal_url,
    login,
    parse_html,
    require_logged_in,
    save_captcha,
    same_origin_url,
    solve_captcha,
)
from csust_cli.features.grades import parse_grade_detail, parse_grades
from csust_cli.features.academic import (
    parse_classrooms,
    parse_evaluation_batches,
    parse_evaluation_courses,
    parse_evaluation_form,
    parse_exams,
    parse_profile,
    parse_selection_results,
    parse_semester_start,
)
from csust_cli.features.schedule import parse_schedule
from csust_cli.features.site import SITE_CATALOG, SiteClient
from csust_cli.features.textbooks import (
    TEXTBOOK_PATHS,
    action_url,
    choose_textbook_item,
    parse_textbooks,
    submit_textbook_action,
)


if __name__ == "__main__":
    raise SystemExit(main())
