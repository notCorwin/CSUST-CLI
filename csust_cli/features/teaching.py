"""CLI access to the web teaching platform behind the VPN SSO service.

The teaching platform is a mixed legacy HTML/iframe application and Vue
homework application.  The catalog is intentionally data-only; ``request``
keeps every discovered page/API open so a changed form does not require a
new wrapper command.
"""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from urllib.parse import unquote, urlsplit

from ..core import (
    Client,
    CsustError,
    HttpError,
    MutationUnverified,
    NetworkError,
    ParseError,
    business_state,
    result_status,
    Response,
    _decode_body,
    _save_cookie_refresh,
    _table_rows,
    _write_private_file,
    _safe_url,
    _safe_terminal_text,
    same_origin_url,
    is_login_page,
    parse_html,
    require_logged_in,
    _SENSITIVE_FIELD,
)
from . import web
from .vpn import (
    VPN_BASE_URL,
    VpnClient,
    _has_session_cookie,
    _json_argument,
    _parse_files,
    _parse_pairs,
    _redact,
    _resolve_path,
)


TEACHING_SERVICE_NAME = "网络教学平台"
TEACHING_SERVICE_GROUP_API = "/api/client/users/service/group?endlessType="
_UNSET = object()


def _route(name: str, section: str, label: str, path: str, description: str) -> dict[str, object]:
    return {"name": name, "section": section, "label": label, "path": path, "description": description}


# Paths observed from the Chrome session.  Query values such as courseId,
# lid, page and resource ids are supplied with --param so one row covers all
# courses and pages of the same kind.
TEACHING_ROUTE_CATALOG = (
    _route("home", "全局", "首页", "/meol/index.do?menuId=1", "个人首页、通知、日历、课程列表"),
    _route("learning", "全局", "学习", "/meol/custom.do?menuId=2", "课程排行与荣誉级别"),
    _route("activities", "全局", "活动", "/meol/custom.do?menuId=3", "研究型教学活动查询"),
    _route("activity-search", "活动", "研究型教学查询", "/meol/activity.do?menuId=3", "按范围和主题查询研究型教学"),
    _route("activity-course-list", "活动", "研究型教学课程列表", "/meol/homepage/V8/include/issue_course_list.jsp", "研究型教学课程列表 iframe"),
    _route("podcast", "全局", "播客", "/meol/custom.do?menuId=4", "微课/播客检索、播放、评论与分页"),
    _route("excellent-courses", "全局", "精品课", "/meol/custom.do?menuId=5", "精品课程申报和国家、省、校级课程"),
    _route("resource-center", "全局", "资源中心", "/meol/respush/res/res_sso.jsp?resourceIndex=1", "公共资源、资源库、个人资源空间"),
    _route("resource-sso", "资源中心", "资源中心 SSO 交接", "/moocresource/eol_sso_new.jsp", "从教学平台进入资源中心的 SSO 交接页"),
    _route("course-union", "全局", "课程联盟", "/meol/courseunion/index.jsp?menuId=8", "课程联盟"),
    _route("personal", "全局", "个人", "/meol/personal.do?menuId=0", "个人资料、提醒、通知、日历和课程"),
    _route("social", "全局", "学习社区", "/meol/social/socialIndex.do?menuId=9", "学习社区"),
    _route("social-friend-search", "学习社区", "找人", "/meol/social/friendSearch.do", "按姓名、性别、学校、年份、班级、专业找人"),
    _route("social-course-search", "学习社区", "找课程", "/meol/social/coursesSearch.do", "社区课程搜索 iframe"),
    _route("social-circle-search", "学习社区", "找圈子", "/meol/social/studyCircleSearch.do", "学习圈搜索"),
    _route("social-circle", "学习社区", "学习圈详情", "/meol/social/studyCircleInfo.do", "学习圈详情和成员动态"),
    _route("social-circle-list", "学习社区", "我的圈子", "/meol/social/studyCircleList.do", "我的学习圈列表"),
    _route("social-person", "学习社区", "个人社区主页", "/meol/social/viewPersonInfo.do", "社区个人信息和动态"),
    _route("social-news", "学习社区", "社区动态", "/meol/social/listNews.do", "个人或圈子动态"),
    _route("social-topics", "学习社区", "我的话题", "/meol/social/listTopics.do", "个人话题列表"),
    _route("social-attention", "学习社区", "关注列表", "/meol/social/listAttention.do", "关注的人和圈子"),
    _route("social-fans", "学习社区", "粉丝列表", "/meol/social/listMyFans.do", "我的粉丝"),
    _route("social-new-circle", "学习社区", "创建圈子", "/meol/social/preAddStudyCircle.do", "创建学习圈表单"),
    _route("social-edit-person", "学习社区", "编辑社区资料", "/meol/social/preUpdPersonInfo.do", "编辑社区个人资料"),
    _route("social-upload-image", "学习社区", "上传社区图片", "/meol/lifelong/social/uploadimg.jsp", "社区图片上传 iframe"),
    _route("social-date-picker", "学习社区", "社区日期选择器", "/meol/lifelong/social/My97DatePicker.htm", "社区日期选择器 iframe"),
    _route("social-download-file", "学习社区", "下载社区文件", "/meol/downloadTheolFile.do", "下载社区图片/文件"),
    _route("course-jump", "学习社区", "进入社区课程", "/meol/lesson/mainJumpPage.jsp", "从社区进入课程"),
    _route("department", "全局", "虚拟教研室", "/meol/department.do?deptId=78401", "虚拟教研室"),
    _route("course-search", "全局", "课程搜索", "/meol/course.do", "按课程名称/编号查询课程"),
    _route("course-departments", "全局", "课程院系列表", "/meol/allDepartment.do", "课程搜索的院系筛选列表"),
    _route("course-curriculum", "全局", "培养方案课程表", "/meol/homepage/V8/include/course_curriculum.jsp", "院系培养方案及课程表"),
    _route("password-recovery", "全局", "找回密码", "/meol/findPasswdIndex.do", "登录页找回密码入口"),
    _route("password-email-pre", "全局", "邮箱找回密码", "/meol/findPasswdMailBoxPreAccount.do", "通过邮箱验证身份"),
    _route("password-email-receive", "全局", "接收重置邮件", "/meol/findPasswdMailBoxReceive.do", "邮箱找回密码结果页"),
    _route("password-question-pre", "全局", "提示问题找回密码", "/meol/findPasswdQuestionPreAccount.do", "通过提示问题验证身份"),
    _route("password-question-reset", "全局", "重置密码", "/meol/findPasswdQuestionPreReset.do", "提示问题验证后的密码重置页"),
    _route("password-captcha", "全局", "找回密码验证码", "/meol/getCaptcha.do", "获取找回密码验证码图片"),
    _route("platform-login", "全局", "旧版登录表单", "/meol/loginCheck.do", "未通过统一认证时的平台登录表单"),
    _route("learning-menu", "学习管理", "学习管理菜单", "/meol/left_v8.jsp", "学习管理隐藏菜单 iframe"),
    _route("research-menu", "研究型教学", "研究型教学菜单", "/meol/issueteach/left.jsp", "研究型教学隐藏菜单 iframe"),
    _route("excellent-menu", "精品课", "精品课程菜单", "/meol/jpk/student/left.jsp", "精品课程隐藏菜单 iframe"),
    _route("podcast-menu", "播客", "教学播客菜单", "/meol/common/vblog/stu_left.jsp", "教学播客隐藏菜单 iframe"),
    _route("podcast-inner", "播客", "教学播客首页", "/meol/common/vblog/inner_index.jsp", "教学播客隐藏首页"),
    _route("consultation-menu", "公共组件", "应用咨询菜单", "/meol/popups/teach_v8/student_popups.jsp", "应用咨询隐藏菜单 iframe"),
    _route("consultation", "公共组件", "应用咨询", "/meol/popups/student_1.htm", "应用咨询内容"),
    _route("department-teachers", "虚拟教研室", "教研室教师", "/meol/teacher.do", "按教研室查看教师"),
    _route("honor-courses", "虚拟教研室", "荣誉课程", "/meol/honLesson.do", "荣誉课程列表"),
    _route("honor-teachers", "虚拟教研室", "荣誉教师", "/meol/honTeacher.do", "荣誉教师列表"),
    _route("global-logout", "全局", "平台退出", "/meol/homepage/V8/include/logout.jsp", "退出教学平台"),
    _route("student-info", "个人", "学生信息", "/meol/popups/viewstudent_info.jsp", "查看和编辑个人资料"),
    _route("student-info-edit", "个人", "编辑学生信息", "/meol/popups/student_info_modify.jsp", "编辑邮箱和电话"),
    _route("security", "个人", "安全设置", "/meol/lifelong/user/security_info.jsp", "修改密码和安全问答"),
    _route("password", "个人", "修改密码", "/meol/popups/password.jsp", "修改密码弹窗"),
    _route("password-update", "个人", "提交新密码", "/meol/popups/password_do.jsp", "提交密码修改"),
    _route("security-question", "个人", "安全问题验证", "/meol/validateQuestion.do", "设置安全问题页面"),
    _route("security-question-update", "个人", "提交安全问题", "/meol/validateQuestionDo.do", "提交安全问题答案"),
    _route("security-email-verify", "个人", "验证邮箱", "/meol/validateEmailSend.do", "发送邮箱验证"),
    _route("security-mobile", "个人", "绑定手机", "/meol/lifelong/user/bind_mobile.jsp", "绑定或更换手机"),
    _route("bind-mobile-reminder", "个人", "绑定手机提醒", "/meol/lifelong/user/bind_mobile_ignore.jsp", "绑定手机提醒弹窗"),
    _route("mobile-login", "个人", "手机验证码登录", "/meol/mobileLogin.do", "使用手机验证码登录教学平台"),
    _route("bind-mobile-ignore", "个人", "暂不绑定手机", "/meol/bindMobileIgnore.do", "关闭绑定手机提醒"),
    _route("notifications", "个人", "系统通知", "/meol/common/inform/index_stu.jsp", "通知排序、分页和详情"),
    _route("notification", "个人", "通知详情", "/meol/common/inform/message_content.jsp", "查看单条通知"),
    _route("home-notifications-all", "全局", "全部通知", "/meol/homepage/common/inform_all.jsp", "首页查看更多系统通知"),
    _route("calendar", "个人", "日历", "/meol/common/calendar/index.jsp", "日历入口"),
    _route("calendar-month", "个人", "月日历", "/meol/common/calendar/month.jsp", "按月查看日历"),
    _route("calendar-date", "个人", "日日历", "/meol/common/calendar/date.jsp", "按日查看日历"),
    _route("all-courses", "个人", "全部课程", "/meol/lesson/blen.student.lesson.list.jsp", "课程搜索和课程顺序调整"),
    _route("course-apply", "学习管理", "申请课程", "/meol/lesson/applynewcourse_stu.jsp", "检索可申请课程"),
    _route("course-summary", "学习管理", "课程简介", "/meol/lesson/coursesum.jsp", "查看可申请课程简介"),
    _route("course-join", "学习管理", "加入课程", "/meol/lesson/applyjoincourse.jsp", "申请加入课程"),
    _route("course-archive", "学习管理", "历史课程", "/meol/archive/archiveCourseListByStu.do", "查看历史课程"),
    _route("mail", "站内邮箱", "邮箱首页", "/meol/common/mail/index.jsp", "站内邮箱入口"),
    _route("mail-inbox", "站内邮箱", "收件箱", "/meol/common/mail/inputbox.jsp", "收件箱和邮件操作"),
    _route("mail-sent", "站内邮箱", "已发送", "/meol/common/mail/sent.jsp", "已发送邮件"),
    _route("mail-draft", "站内邮箱", "草稿箱", "/meol/common/mail/draft.jsp", "草稿邮件"),
    _route("mail-trash", "站内邮箱", "垃圾箱", "/meol/common/mail/garbage.jsp", "垃圾邮件和清空操作"),
    _route("mail-folders", "站内邮箱", "邮箱文件夹", "/meol/common/mail/managefolder.jsp", "管理邮箱文件夹"),
    _route("mail-contacts", "站内邮箱", "邮箱联系人", "/meol/common/mail/managecontact.jsp", "管理邮箱联系人"),
    _route("mail-compose", "站内邮箱", "写邮件", "/meol/common/mail/composemail.jsp", "写信、发送和保存草稿"),
    _route("mail-edit-folder", "站内邮箱", "编辑邮箱文件夹", "/meol/common/mail/editfolder.jsp", "新建或编辑邮箱文件夹"),
    _route("mail-edit-contact", "站内邮箱", "编辑邮箱联系人", "/meol/common/mail/editcontact.jsp", "新建或编辑邮箱联系人"),
    _route("mail-search", "站内邮箱", "搜索邮件", "/meol/common/mail/search.jsp", "搜索站内邮件"),
    _route("mail-message", "站内邮箱", "邮件详情", "/meol/common/mail/mail.jsp", "查看单封邮件"),
    _route("mail-settings", "站内邮箱", "邮箱设置", "/meol/common/mail/mailsetting.jsp", "邮箱通知设置"),
    _route("mail-contact-search", "站内邮箱", "联系人搜索详情", "/meol/common/mail/search_info.jsp", "搜索并选择联系人"),
    _route("daily-top", "学习", "日排行", "/meol/homepage/common/today_lesson_top_list.jsp", "课程日排行"),
    _route("total-top", "学习", "总排行", "/meol/homepage/common/total_lesson_top_list.jsp", "课程总排行"),
    _route("rank", "学习", "荣誉级别", "/meol/homepage/V8/include/rank_lesson_content.jsp", "荣誉级别内容"),
    _route("research-teaching", "活动", "研究型教学", "/meol/issueteach/issue/myissue/issue_list.jsp?_style=new03", "研究型教学主题"),
    _route("research-public", "研究型教学", "校级研究型教学", "/meol/issueteach/issue/pubissue_list.jsp", "校级研究型教学主题"),
    _route("research-course-list", "研究型教学", "课程研究型教学", "/meol/issueteach/issue/courseissue_list_stu.jsp", "按课程查看研究型教学"),
    _route("research-my-all", "研究型教学", "我的研究型教学", "/meol/issueteach/issue/myissue/my_issue_allcourse.jsp", "跨课程查看我的研究型教学"),
    _route("podcast-detail", "播客", "播客详情", "/meol/common/vblog/videodetail.jsp", "播客详情、属性和评论入口"),
    _route("podcast-play", "播客", "播客播放", "/meol/microlessonunit/podcast.do", "播放播客资源"),
    _route("podcast-player", "播客", "播客播放器", "/meol/common/stream/player.jsp", "播放流媒体播客"),
    _route("podcast-recommend", "播客", "播客推荐", "/meol/common/vblog/recommend.jsp", "推荐播客"),
    _route("podcast-favorite", "播客", "播客收藏", "/meol/common/vblog/favorite/add_favorite.jsp", "收藏播客"),
    _route("podcast-review", "播客", "播客评论", "/meol/common/vblog/reviewvideo.jsp", "查看/发布播客评论"),
    _route("podcast-list", "播客", "全校视频", "/meol/common/vblog/listvideo.jsp", "全校视频列表和筛选"),
    _route("podcast-favorites", "播客", "我的播客收藏", "/meol/common/vblog/favorite/favorite_video.jsp", "查看和删除播客收藏"),
    _route("podcast-recent", "播客", "近期点播", "/meol/common/vblog/recentvideo.jsp", "近期点播视频"),
    _route("podcast-favorite-delete", "播客", "删除播客收藏", "/meol/common/vblog/favorite/del_favorite.jsp", "批量删除播客收藏"),
    _route("excellent-current", "精品课", "当前申报", "/meol/jpkContent.do", "当前精品课申报"),
    _route("excellent-history", "精品课", "历次申报", "/meol/jpkContent.do", "历次精品课申报"),
    _route("excellent-national", "精品课", "国家级精品课", "/meol/jpkContent.do?jpkNewsId=3", "国家级精品课"),
    _route("excellent-provincial", "精品课", "省级精品课", "/meol/jpkContent.do?jpkNewsId=4", "省级精品课"),
    _route("excellent-university", "精品课", "校级精品课", "/meol/jpkContent.do?jpkNewsId=5", "校级精品课"),
    _route("excellent-apply", "精品课", "当前精品课程申报", "/meol/jpk/teacher/ela_sect_apply.jsp", "精品课程申报列表"),
    _route("excellent-course-list", "精品课", "历年精品课程", "/meol/jpk/teacher/ela_sect.jsp", "历年精品课程列表"),
    _route("excellent-course-manage", "精品课", "精品课程详情列表", "/meol/jpk/teacher/coursemanage_byselect.jsp", "按申报期查看课程"),
    _route("excellent-course", "精品课", "精品课程浏览", "/meol/jpk/course/index.jsp", "浏览申报课程"),
    _route("resource-home", "资源中心", "资源首页", "/moocresource/index/index.jsp", "资源搜索、推荐和统计"),
    _route("resource-login", "资源中心", "资源中心登录", "/moocresource/servlet/loginServlet", "资源中心登录表单"),
    _route("resource-home-search", "资源中心", "资源首页搜索", "/moocresource/view/reslist.jsp", "资源首页关键词搜索"),
    _route("resource-excellent", "资源中心", "优秀课程", "/moocresource/courses/excellent/indextop.jsp", "优秀课程资源"),
    _route("resource-open", "资源中心", "公开课", "/moocresource/courses/ocw/ocwindex.jsp", "开放课程资源"),
    _route("resource-video", "资源中心", "校友会视频", "/moocresource/courses/xiaoyouhui/video_intro.jsp", "校友会视频"),
    _route("resource-mooc", "资源中心", "校友会 MOOC", "/moocresource/courses/xiaoyouhui/mooc_intro.jsp", "校友会 MOOC"),
    _route("resource-micro", "资源中心", "校友会微课", "/moocresource/courses/xiaoyouhui/micro_intro.jsp", "校友会微课"),
    _route("resource-banks", "资源中心", "资源库", "/moocresource/banks/banksindex.jsp", "资源库和分类资源检索"),
    _route("resource-space", "资源中心", "我的资源", "/moocresource/myres/spaceindex.jsp", "个人资源、文件夹和批量操作"),
    _route("resource-logout", "资源中心", "资源中心退出", "/moocresource/logout.jsp", "退出资源中心会话"),
    _route("resource-left", "资源中心", "资源空间菜单", "/moocresource/myres/left.jsp", "个人资源菜单 iframe"),
    _route("resource-list-frame", "资源中心", "资源列表框架", "/moocresource/resource/resourceInfoList.do", "资源列表 iframe"),
    _route("resource-add", "资源中心", "添加资源", "/moocresource/resource/preAddResourceInfo.do", "添加资源页面"),
    _route("resource-move-batch", "资源中心", "批量移动资源", "/moocresource/resource/moveResourceInfoByBatch.do", "批量移动资源"),
    _route("resource-delete-batch", "资源中心", "批量删除资源", "/moocresource/resource/deleteResourceInfoByBatch.do", "批量删除资源"),
    _route("resource-folders-frame", "资源中心", "资源目录框架", "/moocresource/resource/listMyFolder.do", "资源目录 iframe"),
    _route("resource-favorites-frame", "资源中心", "收藏框架", "/moocresource/myres/favorite/favorite_detail.jsp", "收藏资源 iframe"),
    _route("resource-follows-frame", "资源中心", "关注框架", "/moocresource/myres/attentionres/attentionres_list.jsp", "关注资源 iframe"),
    _route("resource-comments-frame", "资源中心", "评论框架", "/moocresource/myres/myreview/reviewlist.jsp", "资源评论 iframe"),
    _route("resource-shares-frame", "资源中心", "共享资源框架", "/moocresource/resource/listShareRes.do", "共享资源 iframe"),
    _route("resource-personal-frame", "资源中心", "个人信息框架", "/moocresource/personalset/class_personal_info.jsp", "资源个人信息 iframe"),
    _route("resource-view", "资源中心", "共享资源详情", "/moocresource/resource/viewResourceInfo.do", "共享资源详情"),
    _route("resource-favorite", "资源中心", "收藏资源", "/moocresource/myres/favorite/favorite.jsp", "收藏或取消收藏资源"),
    _route("resource-rate", "资源中心", "资源评分", "/moocresource/search/res_evaluate.jsp", "资源一至五星评分"),
    _route("resource-download", "资源中心", "下载资源", "/moocresource/servlet/ResourceDownload", "下载资源文件"),
    _route("resource-tag-exists", "资源中心", "检查资源标签", "/moocresource/taglibrary/tagExist.do", "检查标签是否存在"),
    _route("resource-tag-add", "资源中心", "添加资源标签", "/moocresource/taglibrary/tagAdd.do", "添加资源标签"),
    _route("folder-add", "资源中心", "添加资源目录", "/moocresource/resource/preAddFolder.do", "添加资源目录页面"),
    _route("folder-update", "资源中心", "更新资源目录", "/moocresource/resource/updateMySelfFolder.do", "更新资源目录"),
    _route("folder-share-batch", "资源中心", "批量共享目录", "/moocresource/resource/shareFolderBatch.do", "共享或取消共享目录"),
    _route("folder-list", "资源中心", "收藏夹目录管理", "/moocresource/resource/myfolder_list.jsp", "管理收藏夹目录"),
    _route("follow-add", "资源中心", "添加资源关注", "/moocresource/myres/attentionres/attentionres_add.jsp", "添加资源关注"),
    _route("resource-list", "资源中心", "个人资源列表", "/moocresource/myres/resourceInfoList.do", "个人资源筛选、排序和分页"),
    _route("resource-folders", "资源中心", "资源文件夹", "/moocresource/myres/listMyFolder.do", "文件夹新建、共享和删除"),
    _route("resource-favorites", "资源中心", "资源收藏", "/moocresource/myres/favorite_detail.jsp", "收藏夹和收藏资源"),
    _route("resource-follows", "资源中心", "资源关注", "/moocresource/myres/attentionres_list.jsp", "关注的资源"),
    _route("resource-comments", "资源中心", "资源评论", "/moocresource/myres/reviewlist.jsp", "资源评论"),
    _route("resource-shares", "资源中心", "共享资源", "/moocresource/myres/listShareRes.do", "共享资源筛选、排序和分页"),
    _route("resource-personal", "资源中心", "资源个人信息", "/moocresource/myres/class_personal_info.jsp", "资源积分、等级和统计"),
    _route("resource-notifications", "资源中心", "资源通知", "/moocresource/common/inform/index_all.jsp", "资源通知"),
    _route("resource-updates", "资源中心", "资源最新更新", "/moocresource/statistics/all_new_list.jsp", "最新资源更新"),
    _route("resource-bank-subject", "资源中心", "资源库分类", "/moocresource/banks/banksindex.jsp", "按学科和类型浏览资源"),
    _route("resource-bank-login", "资源中心", "资源库登录提示", "/moocresource/banks/login.jsp", "资源库登录提示和返回入口"),
    _route("resource-bank-subject-list", "资源中心", "资源库分类列表", "/moocresource/banks/search/bank_index.jsp", "按专题库分类浏览资源"),
    _route("resource-bank", "资源中心", "资源库首页", "/moocresource/banks/search/bank.jsp", "专题库详情首页"),
    _route("resource-bank-left", "资源中心", "资源库分类菜单", "/moocresource/banks/search/left.jsp", "专题库分类菜单 iframe"),
    _route("resource-bank-left-submenu", "资源中心", "资源库子分类菜单", "/moocresource/banks/search/left-submenu.jsp", "专题库子分类菜单"),
    _route("resource-bank-list", "资源中心", "资源库资源列表", "/moocresource/banks/search/reslist.jsp", "专题库资源检索列表"),
    _route("resource-excellent-list", "资源中心", "精品课程列表", "/moocresource/courses/excellent/course_list_query.jsp", "精品课程检索结果"),
    _route("resource-open-list", "资源中心", "全球开放课程列表", "/moocresource/courses/ocw/ocwcoursesearch.jsp", "全球开放课程检索结果"),
    _route("resource-video-list", "资源中心", "公开视频列表", "/moocresource/courses/xiaoyouhui/video_list_query.jsp", "公开视频检索结果"),
    _route("resource-mooc-list", "资源中心", "MOOC列表", "/moocresource/courses/xiaoyouhui/mooc_list_query.jsp", "MOOC检索结果"),
    _route("resource-micro-list", "资源中心", "微课程列表", "/moocresource/courses/xiaoyouhui/micro_list_query.jsp", "微课程检索结果"),
    _route("resource-tag-list", "资源中心", "标签资源列表", "/moocresource/courses/xiaoyouhui/searchresourcebytag.jsp", "按标签检索资源"),
    _route("resource-entry-detail", "资源中心", "资源外部条目", "/moocresource/search/browser_by_entry.jsp", "按外部条目标识查看资源"),
    _route("resource-search", "资源中心", "资源检索", "/moocresource/search/browser.jsp", "资源详情和检索结果"),
    _route("resource-detail", "资源中心", "资源详情", "/moocresource/search/browser.jsp", "预览、属性、收藏、下载、评论和标签"),
    _route("resource-attribute", "资源中心", "资源属性", "/moocresource/banks/search/attribute.jsp", "资源属性"),
    _route("resource-related", "资源中心", "相关资源", "/moocresource/banks/relateres.jsp", "相关资源"),
    _route("course", "课程", "课程首页", "/meol/jpk/course/layout/newpage/index.jsp", "课程壳和所有课程菜单"),
    _route("course-default", "课程", "课程介绍首页", "/meol/jpk/course/layout/newpage/default_demonstrate.jsp", "课程介绍和最新动态 iframe"),
    _route("course-unit-blank", "课程", "单元学习容器", "/meol/jpk/course/layout/_blank.jsp", "单元学习容器页面"),
    _route("course-qrcode", "课程", "课程二维码", "/meol/lesson/qrcode.jsp", "显示课程二维码"),
    _route("course-logout", "全局", "教学平台退出", "/meol/homepage/common/logout.jsp", "退出网络教学平台"),
    _route("course-user-stat", "课程", "课程统计", "/meol/jpk/course/userLessonStat.jsp", "课程首页统计数据"),
    _route("course-online-heartbeat", "课程", "在线时长心跳", "/meol/lesson/onlinetime_listener.jsp", "记录课程在线时长"),
    _route("online-help", "公共组件", "在线帮助", "/meol/common/help/help.jsp", "当前页面在线帮助"),
    _route("public-course", "课程", "公开课程首页", "/meol/homepage/course/course_index.jsp", "排行和公开课程入口"),
    _route("course-lesson-home", "课程", "课程学习首页", "/meol/jpk/course/layout/lesson/index.jsp", "课程学习首页"),
    _route("course-column", "课程", "课程栏目", "/meol/jpk/course/course_column_preview_transfer.jsp?tagbug=client", "课程学习/基本信息/课程活动栏目"),
    _route("course-notices", "课程", "课程通知", "/meol/article/ListNews.do", "课程通知列表"),
    _route("course-notice", "课程", "课程通知详情", "/meol/jpk/course/layout/course_meswrap.jsp", "课程通知详情"),
    _route("course-resources", "课程", "课程资源", "/meol/common/script/courseResource.jsp", "课程资源框架和文件夹"),
    _route("course-resource-list", "课程", "课程资源列表", "/meol/common/script/listview.jsp", "课程资源列表、目录属性和分页"),
    _route("course-resource-search", "课程", "课程资源搜索", "/meol/common/script/search.jsp", "课程资源搜索条件"),
    _route("course-resource-preview", "课程", "课程资源预览", "/meol/common/script/preview/download_preview.jsp", "课程资源预览/下载"),
    _route("course-micro-lessons", "课程", "教学播课", "/meol/microlessonunit/viewMicroLesson.do", "播课单元和视频"),
    _route("course-analytics", "课程", "学习分析", "/meol/jpk/course/blended_module/mod_la_stu.jsp", "学习时长、进入次数和课程行为"),
    _route("course-analytics-info", "课程", "学习分析信息", "/meol/la/student/course_info.jsp", "学习分析统计信息"),
    _route("course-analytics-chart", "课程", "学习分析图表", "/meol/la/student/script_view.jsp", "学习分析图表"),
    _route("course-analytics-enter", "课程", "课程进入记录", "/meol/la/student/lesson_enter.jsp", "课程进入次数和学习时间"),
    _route("course-score", "课程", "课程成绩", "/meol/newscoremanagement/scoreView.do", "课程成绩详情"),
    _route("course-score-view", "课程", "课程成绩单", "/meol/scoremanagement/scoreView.do", "课程成绩单入口"),
    _route("course-score-detail", "课程", "个人学习记录", "/meol/common/newscoremanagement/stu_course_detail.jsp", "个人课程学习记录和成绩分析"),
    _route("course-feedback", "课程", "随堂反馈", "/meol/common/unitfeedback/stu/feedback_list.jsp", "随堂建议问卷"),
    _route("course-notes", "课程", "学习笔记", "/meol/common/notebook/column_notebook_share.jsp", "教师/学生笔记集"),
    _route("course-note-write", "课程", "写学习笔记", "/meol/common/notebook/notebook_write.jsp", "创建学习笔记"),
    _route("course-note-compose", "课程", "课程笔记编辑", "/meol/common/notebook/course_write_note.jsp", "课程首页写笔记弹窗"),
    _route("course-note-book", "课程", "课程笔记本", "/meol/common/notebook/course_notebook.jsp", "课程首页查看笔记弹窗"),
    _route("course-mailbox", "课程", "站内邮箱", "/meol/common/mail/inputbox.jsp", "站内邮箱收件箱弹窗"),
    _route("course-forum-frame", "课程", "讨论区弹窗", "/meol/common/faq/forum.jsp", "课程首页讨论区弹窗"),
    _route("course-livevod", "课程", "直播预约", "/meol/livevod/liveVodAppointmentStuList.do", "课程直播预约"),
    _route("course-ccvod", "课程", "CC直播频道", "/meol/ccvod/ccVodChannelStuList.do", "CC直播频道"),
    _route("course-netmeeting", "课程", "网络会议", "/meol/netmeeting/netMeetingPreAddUser.do", "课程网络会议"),
    _route("course-tencent-meeting", "课程", "腾讯会议", "/meol/tencentmeeting/tenCentMeetingPreAddUser.do", "课程腾讯会议"),
    _route("security-send-sms", "个人", "发送手机验证码", "/meol/sendSms.do", "发送绑定手机验证码"),
    _route("security-bind-mobile", "个人", "提交手机绑定", "/meol/bindMobileDo.do", "提交手机绑定验证码"),
    _route("course-faq", "课程", "常见问题", "/meol/common/faq/course_tea_faq.jsp", "FAQ 分类、查询和推荐筛选"),
    _route("course-faq-personal", "课程", "个人答疑问题", "/meol/common/faq/course_tea_maq.jsp", "个人答疑问题"),
    _route("course-faq-detail", "课程", "常见问题详情", "/meol/common/faq/article.jsp", "FAQ 详情和评论"),
    _route("course-forum-discuss", "课程", "讨论区版面", "/meol/common/faq/tea_discuss.jsp", "讨论区版面及主题列表"),
    _route("bug-report", "公共组件", "问题反馈", "/meol/popups/bugreport_do.jsp", "向平台提交页面问题反馈"),
    _route("system-faq", "公共组件", "系统帮助", "/meol/common/faq/sys_faq.jsp", "系统帮助和常见问题"),
    _route("course-forum", "课程", "答疑讨论", "/meol/common/faq/forum_search.jsp", "讨论区搜索、排序和发帖"),
    _route("course-thread", "课程", "讨论主题", "/meol/homepage/threadAction.do", "讨论主题和回复"),
    _route("course-forum-post", "课程", "发表讨论", "/meol/common/faq/postArticle.jsp?opt=topost", "发表新话题/回复"),
    _route("course-surveys", "课程", "课程问卷", "/meol/common/questionaire/stu_survey.jsp", "问卷列表、参加和结果"),
    _route("course-survey", "课程", "问卷填写", "/meol/common/questionaire/survey.jsp", "填写课程问卷"),
    _route("course-survey-result", "课程", "问卷结果", "/meol/common/questionaire/survey_result.jsp", "查看问卷结果"),
    _route("course-questionbank", "课程", "试题试卷库", "/meol/common/question/questionbank/student/list.jsp", "按题型/章节/知识点检索试题"),
    _route("course-questions", "课程", "试题库", "/meol/common/question/questionbank/student/list.jsp", "试题库列表"),
    _route("course-papers", "课程", "试卷库", "/meol/common/question/paperbank/student/list.jsp", "试卷库列表、下载和答案"),
    _route("course-tests", "课程", "在线测试", "/meol/common/question/test/student/list.jsp", "测试列表和结果"),
    _route("course-test", "课程", "开始在线测试", "/meol/common/question/test/student/stu_qtest_navigate.jsp", "在线测试导航"),
    _route("course-test-photo", "课程", "测试照片上传", "/meol/test/userTestPhoto.do", "测试过程照片上传"),
    _route("course-issues", "课程", "研究型教学主题", "/meol/issueteach/issue/myissue/my_issue.jsp", "我的研究型教学主题"),
    _route("course-issue-list", "课程", "研究型教学主题列表", "/meol/issueteach/issue/myissue/issue_list.jsp?_style=new03", "全部研究型教学主题"),
    _route("course-structures", "课程", "播课单元结构", "/meol/microlessonunit/previewListCourseStructure.do", "播课单元结构列表"),
    _route("homework-app", "作业", "课程作业应用", "/meol/common/homework.html?showType=null", "作业 SPA"),
    _route("homework-info", "作业", "作业信息", "/meol/common/homework.html?showType=null", "作业内容和提交规则"),
    _route("homework-submit", "作业", "作业提交", "/meol/common/homework.html?showType=null", "作业答案编辑、提交和订正"),
    _route("homework-result", "作业", "作业结果", "/meol/common/homework.html?showType=null", "成绩、互评和评论"),
    _route("ueditor-content", "公共组件", "富文本内容框架", "/meol/common/ueditor/content.html", "平台富文本内容 iframe"),
)


def _api(name: str, method: str, path: str, mutating: bool, description: str) -> dict[str, object]:
    return {"name": name, "method": method, "path": path, "mutating": mutating, "description": description}


_TEACHING_API_ROWS = (
    _api("homework-stu-user-login", "GET", "/meol/hw/stu/userLogin.do", False, "作业学生会话初始化"),
    _api("homework-stu-list", "GET", "/meol/hw/stu/hwStuHwtList.do", False, "学生作业列表"),
    _api("homework-stu-submit", "GET", "/meol/hw/stu/hwStuSubmit.do", False, "读取作业提交页"),
    _api("homework-stu-submit-do", "POST", "/meol/hw/stu/hwStuSubmitDo.do", True, "提交或更新作业答案"),
    _api("homework-stu-answer-view", "GET", "/meol/hw/stu/hwTaskAnswerView.do", False, "查看已提交答案/订正"),
    _api("homework-stu-review-list", "GET", "/meol/hw/stu/hwReviewList.do", False, "我的互评任务"),
    _api("homework-stu-mutual-list", "GET", "/meol/hw/stu/hwMutualList.do", False, "互评结果"),
    _api("homework-stu-mutual-review", "GET", "/meol/hw/stu/hwMutualReview.do", False, "互评填写页"),
    _api("homework-stu-review-details", "GET", "/meol/hw/stu/hwStuReviewDetails.do", False, "学生互评详情"),
    _api("homework-stu-review-save", "POST", "/meol/hw/stu/hwStuReviewSave.do", True, "保存自评/互评"),
    _api("homework-stu-score", "GET", "/meol/hw/stu/hwShowScore.do", False, "查看作业成绩"),
    _api("homework-stu-review-show", "GET", "/meol/hw/stu/hwShowReviewDetails.do", False, "查看教师/自评/互评详情"),
    _api("homework-stu-complain", "POST", "/meol/hw/stu/hwShowReviewDetails.do", True, "作业评价申诉"),
    _api("homework-stu-hook-up", "GET", "/meol/hw/stu/hwStuHookUp.do", False, "学生作业关联"),
    _api("homework-vis-list", "GET", "/meol/hw/vis/hwVisHwtList.do", False, "访客作业列表"),
    _api("homework-vis-hook-up", "GET", "/meol/hw/vis/hwVisHookUp.do", False, "访客作业关联"),
    _api("homework-tea-login", "GET", "/meol/hw/tea/userLogin.do", False, "教师作业会话初始化"),
    _api("homework-tea-untreated-complain", "GET", "/meol/hw/tea/hwUntreatedComplainList.do", False, "未处理申诉"),
    _api("homework-tea-task-list", "GET", "/meol/hw/tea/zyHwTaskList.do", False, "教师作业任务列表"),
    _api("homework-tea-task-info", "GET", "/meol/hw/tea/hwTaskInfo.do", False, "作业任务信息"),
    _api("homework-tea-add-task", "POST", "/meol/hw/tea/addHwTask.do", True, "新增作业任务"),
    _api("homework-tea-update-task", "POST", "/meol/hw/tea/updateHwTask.do", True, "更新作业任务"),
    _api("homework-tea-delete-task", "POST", "/meol/hw/tea/delHwTask.do", True, "删除作业任务"),
    _api("homework-tea-package-task", "POST", "/meol/hw/tea/packageTask.do", True, "打包作业"),
    _api("homework-tea-group-list", "GET", "/meol/hw/tea/taskGroupList.do", False, "作业分组"),
    _api("homework-tea-comment-info", "GET", "/meol/hw/tea/taskCommentInfo.do", False, "批语信息"),
    _api("homework-tea-set-comment", "POST", "/meol/hw/tea/setTaskComment.do", True, "设置批语"),
    _api("homework-tea-update-setting", "POST", "/meol/hw/tea/updateTaskSetting.do", True, "更新作业设置"),
    _api("homework-tea-review-manage", "GET", "/meol/hw/tea/hwReviewManage.do", False, "作业批阅管理"),
    _api("homework-tea-search-student", "GET", "/meol/hw/tea/hwSearchStuHwa.do", False, "搜索学生作业"),
    _api("homework-tea-update-review-setting", "POST", "/meol/hw/tea/hwUpdateSett.do", True, "更新批阅设置"),
    _api("homework-tea-import-batch", "POST", "/meol/hw/tea/hwImportBatchHwa.do", True, "批量导入作业"),
    _api("homework-tea-no-submit", "GET", "/meol/hw/tea/hwStuNoSubmit.do", False, "未提交学生"),
    _api("homework-tea-remove-list", "GET", "/meol/hw/tea/hwRemoveList.do", False, "移除列表"),
    _api("homework-tea-update-mark", "POST", "/meol/hw/tea/hwUpdateMark.do", True, "更新成绩"),
    _api("homework-tea-answer-all", "GET", "/meol/hw/tea/hwAnswerAllList.do", False, "全部答案"),
    _api("homework-tea-write", "GET", "/meol/hw/tea/hwWriteByTea.do", False, "教师批阅页"),
    _api("homework-tea-write-do", "POST", "/meol/hw/tea/hwWriteByTeaDo.do", True, "提交教师批阅"),
    _api("homework-tea-review", "GET", "/meol/hw/tea/hwTeaReview.do", False, "教师互评管理"),
    _api("homework-tea-review-details", "GET", "/meol/hw/tea/hwTeaReviewDetails.do", False, "教师批阅详情"),
    _api("homework-tea-review-save", "POST", "/meol/hw/tea/hwTeaReviewSave.do", True, "保存教师互评"),
    _api("homework-tea-mutual-mark", "GET", "/meol/hw/tea/hwStuMutualMarkList.do", False, "学生互评成绩"),
    _api("homework-tea-mutual-details", "GET", "/meol/hw/tea/hwStuMutualDetails.do", False, "学生互评详情"),
    _api("homework-tea-download-zip", "GET", "/meol/hw/tea/downLoadTaskZip.do", False, "下载作业压缩包"),
    _api("homework-tea-standard-list", "GET", "/meol/hw/tea/zyStandardList.do", False, "评分标准列表"),
    _api("homework-tea-standard-delete", "POST", "/meol/hw/tea/delZyStandard.do", True, "删除评分标准"),
    _api("homework-tea-standard-item-delete", "POST", "/meol/hw/tea/delZyStandardItem.do", True, "删除评分标准项"),
    _api("homework-tea-standard-add", "POST", "/meol/hw/tea/addZyStandard.do", True, "新增评分标准"),
    _api("homework-tea-standard-status", "POST", "/meol/hw/tea/changeZyStandardStatus.do", True, "改变评分标准状态"),
    _api("homework-tea-standard-item-add", "POST", "/meol/hw/tea/addZyStandardItem.do", True, "新增评分标准项"),
    _api("homework-tea-standard-info", "GET", "/meol/hw/tea/zyStandardInfo.do", False, "评分标准详情"),
    _api("homework-tea-list", "GET", "/meol/hw/tea/hwList.do", False, "教师作业列表"),
    _api("homework-tea-delete", "POST", "/meol/hw/tea/delHw.do", True, "删除教师作业"),
    _api("homework-tea-view", "GET", "/meol/hw/tea/hwView.do", False, "教师作业详情"),
    _api("homework-tea-add-zy", "POST", "/meol/hw/tea/addZyHw.do", True, "新增作业"),
    _api("homework-tea-standard-score-list", "GET", "/meol/hw/tea/zyStandardByScoreList.do", False, "按分数查看评分标准"),
    _api("homework-tea-select-mark", "GET", "/meol/hw/tea/hwSelectMark.do", False, "选择批阅"),
    _api("homework-student-stat", "GET", "/meol/hw/stu/hwStat.do", False, "作业成绩统计"),
    _api("homework-student-stat-grade", "GET", "/meol/hw/stu/hwStatGrade.do", False, "作业等级统计"),
    _api("homework-export-grade", "GET", "/meol/hw/tea/exportHwTaskGrade.do", False, "导出作业成绩"),
    _api("homework-export-student-score", "GET", "/meol/hw/tea/hwExportStuScore.do", False, "导出学生成绩"),
    _api("homework-export-not-submit", "GET", "/meol/hw/tea/hwExportStuNotSubmit.do", False, "导出未提交学生"),
    _api("homework-answer-export-demo", "GET", "/meol/hw/tea/hwAnswerExportDemo.do", False, "导出答案模板"),
    _api("calendar-month", "GET", "/meol/common/calendar/month.jsp", False, "月日历数据"),
    _api("calendar-date", "GET", "/meol/common/calendar/date.jsp", False, "日日历数据"),
    _api("course-apply", "GET", "/meol/lesson/applynewcourse_stu.jsp", False, "可申请课程查询"),
    _api("course-summary", "GET", "/meol/lesson/coursesum.jsp", False, "可申请课程简介"),
    _api("course-join", "GET", "/meol/lesson/applyjoincourse.jsp", True, "申请加入课程"),
    _api("course-archive", "GET", "/meol/archive/archiveCourseListByStu.do", False, "历史课程"),
    _api("mail", "GET", "/meol/common/mail/index.jsp", False, "站内邮箱入口"),
    _api("mail-inbox", "GET", "/meol/common/mail/inputbox.jsp", False, "收件箱"),
    _api("mail-sent", "GET", "/meol/common/mail/sent.jsp", False, "已发送邮件"),
    _api("mail-draft", "GET", "/meol/common/mail/draft.jsp", False, "草稿箱"),
    _api("mail-trash", "GET", "/meol/common/mail/garbage.jsp", False, "垃圾箱"),
    _api("mail-folders", "GET", "/meol/common/mail/managefolder.jsp", False, "邮箱文件夹管理"),
    _api("mail-contacts", "GET", "/meol/common/mail/managecontact.jsp", False, "邮箱联系人管理"),
    _api("mail-compose", "GET", "/meol/common/mail/composemail.jsp", False, "写邮件页面"),
    _api("mail-edit-folder", "GET", "/meol/common/mail/editfolder.jsp", False, "编辑邮箱文件夹页面"),
    _api("mail-edit-contact", "GET", "/meol/common/mail/editcontact.jsp", False, "编辑邮箱联系人页面"),
    _api("mail-search", "POST", "/meol/common/mail/search.jsp", False, "搜索邮件"),
    _api("mail-message-action", "POST", "/meol/common/mail/inputbox.jsp", True, "邮件删除、标记和移动"),
    _api("mail-trash-empty", "GET", "/meol/common/mail/garbage.jsp?action=empty", True, "清空垃圾箱"),
    _api("mail-folder-action", "POST", "/meol/common/mail/managefolder.jsp", True, "邮箱文件夹操作"),
    _api("mail-folder-empty", "GET", "/meol/common/mail/managefolder.jsp?action=_empty", True, "清空邮箱文件夹"),
    _api("mail-contact-action", "POST", "/meol/common/mail/managecontact.jsp", True, "联系人删除和移动"),
    _api("mail-send", "POST", "/meol/common/mail/composemail.jsp", True, "发送邮件"),
    _api("mail-save", "POST", "/meol/common/mail/composemail.jsp", True, "保存邮件草稿"),
    _api("mail-folder-save", "POST", "/meol/common/mail/editfolder.jsp", True, "保存邮箱文件夹"),
    _api("mail-contact-save", "POST", "/meol/common/mail/editcontact.jsp", True, "保存邮箱联系人"),
    _api("mail-message", "GET", "/meol/common/mail/mail.jsp", False, "邮件详情"),
    _api("mail-settings", "GET", "/meol/common/mail/mailsetting.jsp", False, "邮箱设置"),
    _api("mail-settings-save", "POST", "/meol/common/mail/mailsetting.jsp", True, "保存邮箱设置"),
    _api("mail-contact-search", "GET", "/meol/common/mail/search_info.jsp", False, "联系人搜索详情"),
    _api("student-info-edit", "GET", "/meol/popups/student_info_modify.jsp", False, "编辑学生信息页面"),
    _api("student-info-update", "POST", "/meol/popups/student_info_modify_do.jsp", True, "更新邮箱和电话"),
    _api("password", "GET", "/meol/popups/password.jsp", False, "修改密码页面"),
    _api("password-update", "POST", "/meol/popups/password_do.jsp", True, "提交密码修改"),
    _api("security-question", "GET", "/meol/validateQuestion.do", False, "安全问题设置页面"),
    _api("security-question-update", "POST", "/meol/validateQuestionDo.do", True, "提交安全问题"),
    _api("security-email-verify", "GET", "/meol/validateEmailSend.do", True, "发送邮箱验证"),
    _api("security-mobile", "GET", "/meol/lifelong/user/bind_mobile.jsp", False, "绑定手机页面"),
    _api("bind-mobile-reminder", "GET", "/meol/lifelong/user/bind_mobile_ignore.jsp", False, "绑定手机提醒页面"),
    _api("security-send-sms", "POST", "/meol/sendSms.do", True, "发送手机验证码"),
    _api("security-bind-mobile", "POST", "/meol/bindMobileDo.do", True, "提交手机绑定"),
    _api("mobile-login", "POST", "/meol/mobileLogin.do", True, "手机验证码登录"),
    _api("bind-mobile-ignore", "GET", "/meol/bindMobileIgnore.do", True, "暂不绑定手机"),
    _api("home-notifications-all", "GET", "/meol/homepage/common/inform_all.jsp", False, "首页全部通知"),
    _api("course-search", "POST", "/meol/course.do", False, "课程名称/编号查询"),
    _api("course-departments", "GET", "/meol/allDepartment.do", False, "课程院系列表"),
    _api("course-curriculum", "GET", "/meol/homepage/V8/include/course_curriculum.jsp", False, "培养方案课程表"),
    _api("password-recovery", "GET", "/meol/findPasswdIndex.do", False, "找回密码页面"),
    _api("password-email-pre", "GET", "/meol/findPasswdMailBoxPreAccount.do", False, "邮箱找回密码页面"),
    _api("password-email-account", "POST", "/meol/findPasswdMailBoxAccount.do", True, "提交邮箱身份验证"),
    _api("password-email-receive", "GET", "/meol/findPasswdMailBoxReceive.do", False, "接收重置邮件页面"),
    _api("password-question-pre", "GET", "/meol/findPasswdQuestionPreAccount.do", False, "提示问题找回密码页面"),
    _api("password-question-account", "POST", "/meol/findPasswdQuestionAccount.do", True, "提交提示问题验证"),
    _api("password-question-reset", "GET", "/meol/findPasswdQuestionPreReset.do", False, "提示问题重置密码页面"),
    _api("password-captcha", "GET", "/meol/getCaptcha.do", False, "获取找回密码验证码"),
    _api("platform-login", "POST", "/meol/loginCheck.do", True, "旧版平台登录表单"),
    _api("learning-menu", "GET", "/meol/left_v8.jsp", False, "学习管理菜单"),
    _api("research-menu", "GET", "/meol/issueteach/left.jsp", False, "研究型教学菜单"),
    _api("excellent-menu", "GET", "/meol/jpk/student/left.jsp", False, "精品课程菜单"),
    _api("podcast-menu", "GET", "/meol/common/vblog/stu_left.jsp", False, "教学播客菜单"),
    _api("podcast-inner", "GET", "/meol/common/vblog/inner_index.jsp", False, "教学播客首页"),
    _api("consultation-menu", "GET", "/meol/popups/teach_v8/student_popups.jsp", False, "应用咨询菜单"),
    _api("consultation", "GET", "/meol/popups/student_1.htm", False, "应用咨询内容"),
    _api("department-teachers", "GET", "/meol/teacher.do", False, "教研室教师列表"),
    _api("honor-courses", "GET", "/meol/honLesson.do", False, "荣誉课程列表"),
    _api("honor-teachers", "GET", "/meol/honTeacher.do", False, "荣誉教师列表"),
    _api("global-logout", "GET", "/meol/homepage/V8/include/logout.jsp", False, "退出教学平台"),
    _api("public-course", "GET", "/meol/homepage/course/course_index.jsp", False, "公开课程首页"),
    _api("course-lesson-home", "GET", "/meol/jpk/course/layout/lesson/index.jsp", False, "课程学习首页"),
    _api("course-default", "GET", "/meol/jpk/course/layout/newpage/default_demonstrate.jsp", False, "课程介绍首页"),
    _api("course-unit-blank", "GET", "/meol/jpk/course/layout/_blank.jsp", False, "单元学习容器"),
    _api("course-qrcode", "GET", "/meol/lesson/qrcode.jsp", False, "课程二维码"),
    _api("course-logout", "GET", "/meol/homepage/common/logout.jsp", False, "退出教学平台"),
    _api("course-user-stat", "POST", "/meol/jpk/course/userLessonStat.jsp", False, "课程首页统计数据"),
    _api("course-online-heartbeat", "POST", "/meol/lesson/onlinetime_listener.jsp", True, "记录课程在线时长"),
    _api("online-help", "GET", "/meol/common/help/help.jsp", False, "在线帮助"),
    _api("course-note-compose", "GET", "/meol/common/notebook/course_write_note.jsp", False, "课程首页写笔记"),
    _api("course-note-book", "GET", "/meol/common/notebook/course_notebook.jsp", False, "课程首页笔记本"),
    _api("course-mailbox", "GET", "/meol/common/mail/inputbox.jsp", False, "站内邮箱"),
    _api("course-forum-frame", "GET", "/meol/common/faq/forum.jsp", False, "课程首页讨论区"),
    _api("course-livevod", "GET", "/meol/livevod/liveVodAppointmentStuList.do", False, "直播预约"),
    _api("course-ccvod", "GET", "/meol/ccvod/ccVodChannelStuList.do", False, "CC直播频道"),
    _api("course-netmeeting", "GET", "/meol/netmeeting/netMeetingPreAddUser.do", False, "网络会议"),
    _api("course-tencent-meeting", "GET", "/meol/tencentmeeting/tenCentMeetingPreAddUser.do", False, "腾讯会议"),
    _api("podcast-search", "POST", "/meol/custom.do?menuId=4", False, "播客筛选查询"),
    _api("activity-search", "POST", "/meol/activity.do?menuId=3", False, "研究型教学查询"),
    _api("activity-course-list", "GET", "/meol/homepage/V8/include/issue_course_list.jsp", False, "研究型教学课程列表"),
    _api("research-public", "GET", "/meol/issueteach/issue/pubissue_list.jsp", False, "校级研究型教学主题"),
    _api("research-course-list", "POST", "/meol/issueteach/issue/courseissue_list_stu.jsp", False, "课程研究型教学"),
    _api("research-my-all", "POST", "/meol/issueteach/issue/myissue/my_issue_allcourse.jsp", False, "我的研究型教学"),
    _api("excellent-apply", "GET", "/meol/jpk/teacher/ela_sect_apply.jsp", False, "精品课程申报列表"),
    _api("excellent-course-list", "GET", "/meol/jpk/teacher/ela_sect.jsp", False, "历年精品课程列表"),
    _api("excellent-course-manage", "POST", "/meol/jpk/teacher/coursemanage_byselect.jsp", False, "申报期课程列表"),
    _api("excellent-course", "GET", "/meol/jpk/course/index.jsp", False, "申报课程浏览"),
    _api("social-friend-search", "POST", "/meol/social/friendSearch.do", False, "社区找人"),
    _api("social-course-search", "GET", "/meol/social/coursesSearch.do", False, "社区找课程"),
    _api("social-circle-search", "GET", "/meol/social/studyCircleSearch.do", False, "社区找圈子"),
    _api("social-circle", "GET", "/meol/social/studyCircleInfo.do", False, "学习圈详情"),
    _api("social-circle-list", "GET", "/meol/social/studyCircleList.do", False, "我的学习圈"),
    _api("social-person", "GET", "/meol/social/viewPersonInfo.do", False, "社区个人主页"),
    _api("social-news", "GET", "/meol/social/listNews.do", False, "社区动态"),
    _api("social-news-more", "POST", "/meol/social/listNewsMore.do", False, "加载更多社区动态"),
    _api("social-topics", "GET", "/meol/social/listTopics.do", False, "个人话题"),
    _api("social-attention", "GET", "/meol/social/listAttention.do", False, "关注列表"),
    _api("social-attention-more", "POST", "/meol/social/listAttentionMore.do", False, "加载更多关注"),
    _api("social-fans", "GET", "/meol/social/listMyFans.do", False, "粉丝列表"),
    _api("social-new-circle", "GET", "/meol/social/preAddStudyCircle.do", False, "创建圈子页面"),
    _api("social-edit-person", "GET", "/meol/social/preUpdPersonInfo.do", False, "编辑社区资料页面"),
    _api("social-update-person", "POST", "/meol/social/updPersonInfo.do", True, "更新社区个人资料"),
    _api("social-add-message", "POST", "/meol/social/addMessage.do", True, "发布社区动态"),
    _api("social-add-topic-reply", "POST", "/meol/social/addTopicReply.do", True, "发表评论或回复"),
    _api("social-add-attention", "POST", "/meol/social/addAttention.do", True, "添加关注"),
    _api("social-delete-attention", "POST", "/meol/social/deleteAttention.do", True, "取消关注"),
    _api("social-add-circle-user", "POST", "/meol/social/addCircleUser.do", True, "加入学习圈"),
    _api("social-exit-circle", "POST", "/meol/social/circleUserExit.do", True, "退出学习圈"),
    _api("social-add-circle", "POST", "/meol/social/addStudyCircle.do", True, "创建学习圈"),
    _api("social-delete-circle", "POST", "/meol/social/deleteStudyCircle.do", True, "删除学习圈"),
    _api("social-delete-topic", "POST", "/meol/social/deleteMessage.do", True, "删除社区话题"),
    _api("social-delete-topic-reply", "POST", "/meol/social/deleteTopicReply.do", True, "删除话题回复"),
    _api("social-delete-news-topic", "POST", "/meol/social/deleteNewsAndTopic.do", True, "删除动态和话题"),
    _api("social-delete-fan", "POST", "/meol/social/deleteMyFans.do", True, "删除粉丝"),
    _api("social-upload-photo", "POST", "/meol/social/newUploadPhotos.do", True, "上传社区头像/图片"),
    _api("social-upload-temp", "POST", "/meol/social/uploadImg.do", True, "上传社区图片临时文件"),
    _api("social-delete-temp", "POST", "/meol/social/deleteImg.do", True, "删除社区图片临时文件"),
    _api("social-date-picker", "GET", "/meol/lifelong/social/My97DatePicker.htm", False, "社区日期选择器"),
    _api("social-download-file", "GET", "/meol/downloadTheolFile.do", False, "下载社区文件"),
    _api("course-jump", "GET", "/meol/lesson/mainJumpPage.jsp", False, "从社区进入课程"),
    _api("social-help", "GET", "/meol/common/help/help.jsp", False, "社区在线帮助"),
    _api("course-notices", "GET", "/meol/common/inform/index_stu.jsp", False, "课程通知列表"),
    _api("course-notice-detail", "GET", "/meol/common/inform/message_content.jsp", False, "通知详情"),
    _api("course-forum-search", "POST", "/meol/common/faq/forum_search.jsp", False, "讨论区搜索"),
    _api("course-forum-post", "POST", "/meol/common/faq/postArticle.jsp?opt=topost", True, "发帖/回复"),
    _api("course-forum-sort", "GET", "/meol/homepage/forumAction.do", False, "讨论排序/分页"),
    _api("course-thread", "GET", "/meol/homepage/threadAction.do", False, "讨论主题详情"),
    _api("course-faq", "GET", "/meol/common/faq/course_tea_faq.jsp", False, "FAQ 查询"),
    _api("course-faq-personal", "GET", "/meol/common/faq/course_tea_maq.jsp", False, "个人答疑"),
    _api("course-faq-detail", "GET", "/meol/common/faq/article.jsp", False, "FAQ 详情"),
    _api("course-forum-discuss", "GET", "/meol/common/faq/tea_discuss.jsp", False, "讨论区版面"),
    _api("bug-report", "POST", "/meol/popups/bugreport_do.jsp", True, "提交问题反馈"),
    _api("system-faq", "POST", "/meol/common/faq/sys_faq.jsp", False, "系统帮助"),
    _api("course-survey-list", "POST", "/meol/common/questionaire/stu_survey.jsp", False, "问卷列表"),
    _api("course-survey", "GET", "/meol/common/questionaire/survey.jsp", False, "填写问卷"),
    _api("course-survey-result", "GET", "/meol/common/questionaire/survey_result.jsp", False, "问卷结果"),
    _api("course-question-list", "POST", "/meol/common/question/questionbank/student/list.jsp", False, "题库查询"),
    _api("course-paper-list", "GET", "/meol/common/question/paperbank/student/list.jsp", False, "试卷库查询"),
    _api("course-test-list", "GET", "/meol/common/question/test/student/list.jsp", False, "在线测试查询"),
    _api("course-test-navigate", "GET", "/meol/common/question/test/student/stu_qtest_navigate.jsp", False, "在线测试导航"),
    _api("course-test-photo", "POST", "/meol/test/userTestPhoto.do", True, "上传测试照片"),
    _api("course-resource-search", "GET", "/meol/common/script/search.jsp", False, "课程资源搜索"),
    _api("course-resource-list", "GET", "/meol/common/script/listview.jsp", False, "课程资源列表"),
    _api("course-resource-preview", "GET", "/meol/common/script/preview/download_preview.jsp", False, "预览/下载课程资源"),
    _api("course-resource-folder", "GET", "/meol/common/script/left.jsp", False, "课程资源文件夹"),
    _api("course-micro-lessons", "GET", "/meol/microlessonunit/viewMicroLesson.do", False, "教学播课"),
    _api("course-structure", "POST", "/meol/microlessonunit/deleteCourseStructure.do", True, "播课单元结构变更"),
    _api("course-structure-list", "GET", "/meol/microlessonunit/previewListCourseStructure.do", False, "播课单元结构列表"),
    _api("course-analytics", "GET", "/meol/jpk/course/blended_module/mod_la_stu.jsp", False, "学习分析"),
    _api("course-analytics-info", "GET", "/meol/la/student/course_info.jsp", False, "学习分析信息"),
    _api("course-analytics-chart", "GET", "/meol/la/student/script_view.jsp", False, "学习分析图表"),
    _api("course-analytics-enter", "GET", "/meol/la/student/lesson_enter.jsp", False, "学习分析记录"),
    _api("course-score", "GET", "/meol/newscoremanagement/scoreView.do", False, "课程成绩"),
    _api("course-score-detail", "GET", "/meol/common/newscoremanagement/stu_course_detail.jsp", False, "个人学习记录"),
    _api("course-feedback", "GET", "/meol/common/unitfeedback/stu/feedback_list.jsp", False, "随堂反馈"),
    _api("course-notes", "POST", "/meol/common/notebook/column_notebook_share.jsp", False, "学习笔记查询"),
    _api("course-note-write", "POST", "/meol/common/notebook/notebook_write.jsp", True, "保存学习笔记"),
    _api("podcast-detail", "GET", "/meol/common/vblog/videodetail.jsp", False, "播客详情"),
    _api("podcast-play", "GET", "/meol/microlessonunit/podcast.do", False, "播客播放"),
    _api("podcast-recommend", "GET", "/meol/common/vblog/recommend.jsp", True, "推荐播客"),
    _api("podcast-favorite", "GET", "/meol/common/vblog/favorite/add_favorite.jsp", True, "收藏播客"),
    _api("podcast-review", "POST", "/meol/common/vblog/reviewvideo.jsp", True, "播客评论"),
    _api("podcast-list", "GET", "/meol/common/vblog/listvideo.jsp", False, "全校视频列表"),
    _api("podcast-favorites", "GET", "/meol/common/vblog/favorite/favorite_video.jsp", False, "我的播客收藏"),
    _api("podcast-recent", "GET", "/meol/common/vblog/recentvideo.jsp", False, "近期点播"),
    _api("podcast-favorite-delete", "POST", "/meol/common/vblog/favorite/del_favorite.jsp", True, "删除播客收藏"),
    _api("resource-detail", "GET", "/moocresource/search/browser.jsp", False, "资源详情"),
    _api("resource-sso", "POST", "/moocresource/eol_sso_new.jsp", True, "资源中心 SSO 交接"),
    _api("resource-attribute", "GET", "/moocresource/banks/search/attribute.jsp", False, "资源属性"),
    _api("resource-related", "GET", "/moocresource/banks/relateres.jsp", False, "相关资源"),
    _api("resource-personal-list", "GET", "/moocresource/myres/resourceInfoList.do", False, "我的资源"),
    _api("resource-login", "POST", "/moocresource/servlet/loginServlet", True, "资源中心登录"),
    _api("resource-home-search", "POST", "/moocresource/view/reslist.jsp", False, "资源首页搜索"),
    _api("resource-list-frame", "GET", "/moocresource/resource/resourceInfoList.do", False, "资源列表框架"),
    _api("resource-list-search", "POST", "/moocresource/resource/resourceInfoList.do", False, "我的资源筛选"),
    _api("resource-list-page", "POST", "/moocresource/resource/resourceInfoList.do", False, "我的资源分页"),
    _api("resource-add", "GET", "/moocresource/resource/preAddResourceInfo.do", False, "添加资源页面"),
    _api("resource-move-batch", "POST", "/moocresource/resource/moveResourceInfoByBatch.do", True, "批量移动资源"),
    _api("resource-delete-batch", "POST", "/moocresource/resource/deleteResourceInfoByBatch.do", True, "批量删除资源"),
    _api("resource-left", "GET", "/moocresource/myres/left.jsp", False, "资源空间菜单"),
    _api("resource-folders-frame", "GET", "/moocresource/resource/listMyFolder.do", False, "资源目录框架"),
    _api("resource-favorites-frame", "GET", "/moocresource/myres/favorite/favorite_detail.jsp", False, "收藏资源框架"),
    _api("resource-follows-frame", "GET", "/moocresource/myres/attentionres/attentionres_list.jsp", False, "关注资源框架"),
    _api("resource-comments-frame", "GET", "/moocresource/myres/myreview/reviewlist.jsp", False, "资源评论框架"),
    _api("resource-shares-frame", "GET", "/moocresource/resource/listShareRes.do", False, "共享资源框架"),
    _api("resource-personal-frame", "GET", "/moocresource/personalset/class_personal_info.jsp", False, "资源个人信息框架"),
    _api("resource-view", "GET", "/moocresource/resource/viewResourceInfo.do", False, "共享资源详情"),
    _api("resource-favorite", "POST", "/moocresource/myres/favorite/favorite.jsp", True, "收藏或取消收藏资源"),
    _api("resource-rate", "POST", "/moocresource/search/res_evaluate.jsp", True, "资源评分"),
    _api("resource-download", "GET", "/moocresource/servlet/ResourceDownload", False, "下载资源文件"),
    _api("resource-tag-exists", "POST", "/moocresource/taglibrary/tagExist.do", False, "检查资源标签"),
    _api("resource-tag-add", "POST", "/moocresource/taglibrary/tagAdd.do", True, "添加资源标签"),
    _api("folder-add", "GET", "/moocresource/resource/preAddFolder.do", False, "添加资源目录页面"),
    _api("folder-update", "POST", "/moocresource/resource/updateMySelfFolder.do", True, "更新资源目录"),
    _api("folder-share-batch", "POST", "/moocresource/resource/shareFolderBatch.do", True, "共享或取消共享目录"),
    _api("folder-list", "GET", "/moocresource/resource/myfolder_list.jsp", False, "收藏夹目录管理"),
    _api("follow-add", "GET", "/moocresource/myres/attentionres/attentionres_add.jsp", False, "添加资源关注页面"),
    _api("resource-logout", "GET", "/moocresource/logout.jsp", False, "退出资源中心会话"),
    _api("resource-folders", "GET", "/moocresource/myres/listMyFolder.do", False, "我的资源文件夹"),
    _api("resource-favorites", "GET", "/moocresource/myres/favorite_detail.jsp", False, "资源收藏"),
    _api("resource-follows", "GET", "/moocresource/myres/attentionres_list.jsp", False, "资源关注"),
    _api("resource-comments", "GET", "/moocresource/myres/reviewlist.jsp", False, "资源评论"),
    _api("resource-shares", "GET", "/moocresource/myres/listShareRes.do", False, "共享资源"),
    _api("resource-personal-info", "GET", "/moocresource/myres/class_personal_info.jsp", False, "资源个人统计"),
    _api("resource-notifications", "GET", "/moocresource/common/inform/index_all.jsp", False, "资源通知"),
    _api("resource-updates", "GET", "/moocresource/statistics/all_new_list.jsp", False, "资源更新"),
    _api("resource-bank-subject-list", "GET", "/moocresource/banks/search/bank_index.jsp", False, "资源库分类列表"),
    _api("resource-bank-login", "GET", "/moocresource/banks/login.jsp", False, "资源库登录提示"),
    _api("resource-bank", "GET", "/moocresource/banks/search/bank.jsp", False, "资源库首页"),
    _api("resource-bank-left", "GET", "/moocresource/banks/search/left.jsp", False, "资源库分类菜单"),
    _api("resource-bank-left-submenu", "GET", "/moocresource/banks/search/left-submenu.jsp", False, "资源库子分类菜单"),
    _api("resource-bank-list", "POST", "/moocresource/banks/search/reslist.jsp", False, "专题库资源查询"),
    _api("resource-bank-keyword", "POST", "/moocresource/courses/searchres.jsp", False, "专题库关键词查询"),
    _api("resource-excellent-list", "POST", "/moocresource/courses/excellent/course_list_query.jsp", False, "精品课程查询"),
    _api("resource-open-list", "POST", "/moocresource/courses/ocw/ocwcoursesearch.jsp", False, "全球开放课程查询"),
    _api("resource-video-list", "POST", "/moocresource/courses/xiaoyouhui/video_list_query.jsp", False, "公开视频查询"),
    _api("resource-mooc-list", "POST", "/moocresource/courses/xiaoyouhui/mooc_list_query.jsp", False, "MOOC查询"),
    _api("resource-micro-list", "POST", "/moocresource/courses/xiaoyouhui/micro_list_query.jsp", False, "微课程查询"),
    _api("resource-tag-list", "GET", "/moocresource/courses/xiaoyouhui/searchresourcebytag.jsp", False, "标签资源列表"),
    _api("resource-entry-detail", "GET", "/moocresource/search/browser_by_entry.jsp", False, "外部条目资源详情"),
)


TEACHING_ACTION_CATALOG = (
    {"name": "global.search", "section": "全局", "label": "课程名称/编号搜索", "method": "POST", "path": "/meol/course.do", "mutating": False, "fields": ["deptId", "s_keywordrealation", "s_keyword"]},
    {"name": "global.course.departments", "section": "全局", "label": "读取课程院系列表", "method": "GET", "path": "/meol/allDepartment.do", "mutating": False, "fields": []},
    {"name": "global.course.curriculum", "section": "全局", "label": "查看培养方案课程表", "method": "GET", "path": "/meol/homepage/V8/include/course_curriculum.jsp", "mutating": False, "fields": ["deptId", "majorid", "curriculumid"]},
    {"name": "password.recovery.open", "section": "全局", "label": "打开找回密码", "method": "GET", "path": "/meol/findPasswdIndex.do", "mutating": False, "fields": []},
    {"name": "password.email.open", "section": "全局", "label": "打开邮箱找回密码", "method": "GET", "path": "/meol/findPasswdMailBoxPreAccount.do", "mutating": False, "fields": []},
    {"name": "password.email.verify", "section": "全局", "label": "提交邮箱身份验证", "method": "POST", "path": "/meol/findPasswdMailBoxAccount.do", "mutating": True, "fields": ["username", "email", "imgcode"]},
    {"name": "password.email.receive", "section": "全局", "label": "查看重置邮件结果", "method": "GET", "path": "/meol/findPasswdMailBoxReceive.do", "mutating": False, "fields": []},
    {"name": "password.question.open", "section": "全局", "label": "打开提示问题找回密码", "method": "GET", "path": "/meol/findPasswdQuestionPreAccount.do", "mutating": False, "fields": []},
    {"name": "password.question.verify", "section": "全局", "label": "提交提示问题验证", "method": "POST", "path": "/meol/findPasswdQuestionAccount.do", "mutating": True, "fields": ["username", "questionId", "questionVal", "questionId2", "questionVal2", "questionId3", "questionVal3", "imgcode"]},
    {"name": "password.question.reset", "section": "全局", "label": "打开密码重置页", "method": "GET", "path": "/meol/findPasswdQuestionPreReset.do", "mutating": False, "fields": ["code"]},
    {"name": "password.captcha", "section": "全局", "label": "获取找回密码验证码", "method": "GET", "path": "/meol/getCaptcha.do", "mutating": False, "fields": []},
    {"name": "personal.profile.update", "section": "个人", "label": "提交个人资料", "method": "POST", "path": "/meol/popups/student_info_modify_do.jsp", "mutating": True, "fields": ["SID", "from", "rd", "IPT_EMAIL", "IPT_PHONE", "description"]},
    {"name": "personal.password.update", "section": "个人", "label": "修改密码", "method": "POST", "path": "/meol/popups/password_do.jsp", "mutating": True, "fields": ["uid", "oldpass", "newpass", "rnewpass"]},
    {"name": "personal.security-question.open", "section": "个人", "label": "设置安全问题", "method": "GET", "path": "/meol/validateQuestion.do", "mutating": False, "fields": []},
    {"name": "personal.security-question.update", "section": "个人", "label": "提交安全问题", "method": "POST", "path": "/meol/validateQuestionDo.do", "mutating": True, "fields": ["secret", "questionId", "questionVal", "questionId2", "questionVal2", "questionId3", "questionVal3"]},
    {"name": "personal.email.verify", "section": "个人", "label": "发送邮箱验证", "method": "GET", "path": "/meol/validateEmailSend.do", "mutating": True, "fields": []},
    {"name": "personal.mobile.send-sms", "section": "个人", "label": "发送绑定手机验证码", "method": "POST", "path": "/meol/sendSms.do", "mutating": True, "fields": ["mobile", "opertionType"]},
    {"name": "personal.mobile.login-send-sms", "section": "个人", "label": "发送手机登录验证码", "method": "POST", "path": "/meol/sendSms.do", "mutating": True, "fields": ["mobile", "opertionType=loginoperation"]},
    {"name": "personal.mobile.bind", "section": "个人", "label": "绑定手机", "method": "POST", "path": "/meol/bindMobileDo.do", "mutating": True, "fields": ["mobile", "randomcode"]},
    {"name": "personal.mobile.login", "section": "个人", "label": "手机验证码登录", "method": "POST", "path": "/meol/mobileLogin.do", "mutating": True, "fields": ["mobile", "randomcode"]},
    {"name": "personal.mobile.ignore", "section": "个人", "label": "暂不绑定手机", "method": "GET", "path": "/meol/bindMobileIgnore.do", "mutating": True, "fields": []},
    {"name": "personal.notifications.sort", "section": "个人", "label": "通知排序/分页", "method": "GET", "path": "/meol/common/inform/index_stu.jsp", "mutating": False, "fields": ["s_order", "s_page", "s_gotopage"]},
    {"name": "global.notifications.all", "section": "全局", "label": "查看全部通知", "method": "GET", "path": "/meol/homepage/common/inform_all.jsp", "mutating": False, "fields": ["s_page", "s_gotopage"]},
    {"name": "global.course.search", "section": "全局", "label": "课程名称/编号查询", "method": "POST", "path": "/meol/course.do", "mutating": False, "fields": ["deptId", "s_keywordrealation", "s_keyword"]},
    {"name": "department.teachers", "section": "虚拟教研室", "label": "教研室教师", "method": "GET", "path": "/meol/teacher.do", "mutating": False, "fields": ["deptId", "pagingPage", "pagingNumberPer"]},
    {"name": "honor.courses", "section": "虚拟教研室", "label": "荣誉课程", "method": "GET", "path": "/meol/honLesson.do", "mutating": False, "fields": ["cpiCode", "cpId"]},
    {"name": "honor.teachers", "section": "虚拟教研室", "label": "荣誉教师", "method": "GET", "path": "/meol/honTeacher.do", "mutating": False, "fields": ["cpi", "cpicode"]},
    {"name": "activity.search", "section": "活动", "label": "研究型教学查询", "method": "POST", "path": "/meol/activity.do?menuId=3", "mutating": False, "fields": ["radio", "issuename"]},
    {"name": "activity.course-list", "section": "活动", "label": "研究型教学课程列表", "method": "GET", "path": "/meol/homepage/V8/include/issue_course_list.jsp", "mutating": False, "fields": ["radiovalue", "issuename"]},
    {"name": "platform.login", "section": "全局", "label": "旧版平台登录", "method": "POST", "path": "/meol/loginCheck.do", "mutating": True, "fields": ["logintoken", "IPT_LOGINUSERNAME", "IPT_LOGINPASSWORD"]},
    {"name": "research.course.search", "section": "研究型教学", "label": "筛选课程研究型教学", "method": "POST", "path": "/meol/issueteach/issue/courseissue_list_stu.jsp", "mutating": False, "fields": ["selectcourse"]},
    {"name": "research.my.search", "section": "研究型教学", "label": "筛选我的研究型教学", "method": "POST", "path": "/meol/issueteach/issue/myissue/my_issue_allcourse.jsp", "mutating": False, "fields": ["selectcourse"]},
    {"name": "excellent.apply.sort", "section": "精品课", "label": "排序当前申报", "method": "GET", "path": "/meol/jpk/teacher/ela_sect_apply.jsp", "mutating": False, "fields": ["sortColumn", "sortDirection", "open", "pagingPage", "pagingNumberPer"]},
    {"name": "excellent.course.sort", "section": "精品课", "label": "排序历年精品课", "method": "GET", "path": "/meol/jpk/teacher/ela_sect.jsp", "mutating": False, "fields": ["sortColumn", "sortDirection", "pagingPage", "pagingNumberPer"]},
    {"name": "excellent.course.search", "section": "精品课", "label": "筛选申报课程", "method": "POST", "path": "/meol/jpk/teacher/coursemanage_byselect.jsp", "mutating": False, "fields": ["sessionId", "courseName", "instructorName", "diciplineId", "departmentId"]},
    {"name": "excellent.course.open", "section": "精品课", "label": "浏览申报课程", "method": "GET", "path": "/meol/jpk/course/index.jsp", "mutating": False, "fields": ["courseId"]},
    {"name": "social.friend.search", "section": "学习社区", "label": "找人", "method": "POST", "path": "/meol/social/friendSearch.do", "mutating": False, "fields": ["nickName", "gender", "school", "entranceYear", "className", "speciality", "pagingPage", "pagingNumberPer"]},
    {"name": "social.friend.page", "section": "学习社区", "label": "找人分页", "method": "GET", "path": "/meol/social/friendSearch.do", "mutating": False, "fields": ["sortColumn", "sortDirection", "nickName", "pagingPage", "pagingNumberPer"]},
    {"name": "social.course.search", "section": "学习社区", "label": "找课程", "method": "GET", "path": "/meol/social/coursesSearch.do", "mutating": False, "fields": ["attentionType", "keyword", "pagingPage", "pagingNumberPer"]},
    {"name": "social.course.search-submit", "section": "学习社区", "label": "筛选课程", "method": "POST", "path": "/meol/social/coursesSearch.do", "mutating": False, "fields": ["attentionType", "name", "numb", "teacherName", "pagingPage", "pagingNumberPer"]},
    {"name": "social.circle.search", "section": "学习社区", "label": "找圈子", "method": "GET", "path": "/meol/social/studyCircleSearch.do", "mutating": False, "fields": ["keyword", "pagingPage", "pagingNumberPer"]},
    {"name": "social.circle.search-submit", "section": "学习社区", "label": "筛选圈子", "method": "POST", "path": "/meol/social/studyCircleSearch.do", "mutating": False, "fields": ["title", "creator", "pagingPage", "pagingNumberPer"]},
    {"name": "social.circle.open", "section": "学习社区", "label": "打开学习圈", "method": "GET", "path": "/meol/social/studyCircleInfo.do", "mutating": False, "fields": ["studyCircleId"]},
    {"name": "social.circle.list", "section": "学习社区", "label": "我的学习圈", "method": "GET", "path": "/meol/social/studyCircleList.do", "mutating": False, "fields": ["userId", "genre", "pagingPage", "pagingNumberPer"]},
    {"name": "social.person.open", "section": "学习社区", "label": "打开社区个人主页", "method": "GET", "path": "/meol/social/viewPersonInfo.do", "mutating": False, "fields": ["userId", "genre"]},
    {"name": "social.news.list", "section": "学习社区", "label": "读取社区动态", "method": "GET", "path": "/meol/social/listNews.do", "mutating": False, "fields": ["userId", "genre", "type"]},
    {"name": "social.news.more", "section": "学习社区", "label": "加载更多社区动态", "method": "POST", "path": "/meol/social/listNewsMore.do", "mutating": False, "fields": ["logTime", "type", "userId"]},
    {"name": "social.topics.list", "section": "学习社区", "label": "读取个人话题", "method": "GET", "path": "/meol/social/listTopics.do", "mutating": False, "fields": ["userId", "genre", "pagingPage", "pagingNumberPer"]},
    {"name": "social.attention.list", "section": "学习社区", "label": "读取关注列表", "method": "GET", "path": "/meol/social/listAttention.do", "mutating": False, "fields": ["userId", "genre", "type"]},
    {"name": "social.attention.more", "section": "学习社区", "label": "加载更多关注", "method": "POST", "path": "/meol/social/listAttentionMore.do", "mutating": False, "fields": ["logTime", "type", "userId"]},
    {"name": "social.fans.list", "section": "学习社区", "label": "读取粉丝列表", "method": "GET", "path": "/meol/social/listMyFans.do", "mutating": False, "fields": ["userId", "genre", "pagingPage", "pagingNumberPer"]},
    {"name": "social.message.add", "section": "学习社区", "label": "发布社区动态", "method": "POST", "path": "/meol/social/addMessage.do", "mutating": True, "fields": ["type", "studyCircleId", "saytext", "imgId"]},
    {"name": "social.topic.reply", "section": "学习社区", "label": "评论/回复话题", "method": "POST", "path": "/meol/social/addTopicReply.do", "mutating": True, "fields": ["topicId", "topicReplyId", "content"]},
    {"name": "social.attention.add", "section": "学习社区", "label": "添加关注", "method": "POST", "path": "/meol/social/addAttention.do", "mutating": True, "fields": ["objId", "type"]},
    {"name": "social.attention.delete", "section": "学习社区", "label": "取消关注", "method": "POST", "path": "/meol/social/deleteAttention.do", "mutating": True, "fields": ["objId", "type", "attentionId"]},
    {"name": "social.circle.join", "section": "学习社区", "label": "加入学习圈", "method": "POST", "path": "/meol/social/addCircleUser.do", "mutating": True, "fields": ["userId", "studyCircleId"]},
    {"name": "social.circle.exit", "section": "学习社区", "label": "退出学习圈", "method": "POST", "path": "/meol/social/circleUserExit.do", "mutating": True, "fields": ["userId", "studyCircleId", "circleUserId"]},
    {"name": "social.circle.create", "section": "学习社区", "label": "创建学习圈", "method": "POST", "path": "/meol/social/addStudyCircle.do", "mutating": True, "fields": ["id", "genre", "creatorId", "from", "title", "content", "photoFile"]},
    {"name": "social.person.edit", "section": "学习社区", "label": "编辑社区资料", "method": "GET", "path": "/meol/social/preUpdPersonInfo.do", "mutating": False, "fields": []},
    {"name": "social.person.update", "section": "学习社区", "label": "更新社区资料", "method": "POST", "path": "/meol/social/updPersonInfo.do", "mutating": True, "fields": ["ticket", "id", "nickName", "gender", "birthday", "mobile", "email", "school", "entranceYear", "className", "speciality", "unit", "provice", "city", "briefIntro"]},
    {"name": "social.circle.delete", "section": "学习社区", "label": "删除学习圈", "method": "POST", "path": "/meol/social/deleteStudyCircle.do", "mutating": True, "fields": ["id", "genre", "userId"]},
    {"name": "social.topic.delete", "section": "学习社区", "label": "删除话题", "method": "POST", "path": "/meol/social/deleteMessage.do", "mutating": True, "fields": ["topicId"]},
    {"name": "social.topic.reply-delete", "section": "学习社区", "label": "删除话题回复", "method": "POST", "path": "/meol/social/deleteTopicReply.do", "mutating": True, "fields": ["id"]},
    {"name": "social.news-topic.delete", "section": "学习社区", "label": "删除动态和话题", "method": "POST", "path": "/meol/social/deleteNewsAndTopic.do", "mutating": True, "fields": ["topicId", "newsId"]},
    {"name": "social.fan.delete", "section": "学习社区", "label": "删除粉丝", "method": "POST", "path": "/meol/social/deleteMyFans.do", "mutating": True, "fields": ["userId"]},
    {"name": "social.photo.upload", "section": "学习社区", "label": "上传社区图片", "method": "POST", "path": "/meol/social/newUploadPhotos.do", "mutating": True, "fields": ["file"]},
    {"name": "social.photo.upload-temp", "section": "学习社区", "label": "上传社区图片临时文件", "method": "POST", "path": "/meol/social/uploadImg.do", "mutating": True, "fields": ["file", "uid", "a"]},
    {"name": "social.photo.delete-temp", "section": "学习社区", "label": "删除社区图片临时文件", "method": "POST", "path": "/meol/social/deleteImg.do", "mutating": True, "fields": ["fileId"]},
    {"name": "social.file.download", "section": "学习社区", "label": "下载社区文件", "method": "GET", "path": "/meol/downloadTheolFile.do", "mutating": False, "fields": ["id"]},
    {"name": "course.community.open", "section": "学习社区", "label": "进入社区课程", "method": "GET", "path": "/meol/lesson/mainJumpPage.jsp", "mutating": False, "fields": ["courseId"]},
    {"name": "personal.calendar.month", "section": "个人", "label": "按月日历", "method": "GET", "path": "/meol/common/calendar/month.jsp", "mutating": False, "fields": ["date"]},
    {"name": "personal.calendar.date", "section": "个人", "label": "按日日历", "method": "GET", "path": "/meol/common/calendar/date.jsp", "mutating": False, "fields": ["date"]},
    {"name": "course.apply.search", "section": "学习管理", "label": "检索可申请课程", "method": "GET", "path": "/meol/lesson/applynewcourse_stu.jsp", "mutating": False, "fields": ["CATID", "SEL_LESSON_CAT", "s_keyword", "s_keywordrealation", "button"]},
    {"name": "course.apply.sort", "section": "学习管理", "label": "排序可申请课程", "method": "GET", "path": "/meol/lesson/applynewcourse_stu.jsp", "mutating": False, "fields": ["s_order", "CATID"]},
    {"name": "course.apply.summary", "section": "学习管理", "label": "查看课程简介", "method": "GET", "path": "/meol/lesson/coursesum.jsp", "mutating": False, "fields": ["lid", "fancybox"]},
    {"name": "course.apply.join", "section": "学习管理", "label": "申请加入课程", "method": "GET", "path": "/meol/lesson/applyjoincourse.jsp", "mutating": True, "fields": ["lid"]},
    {"name": "course.archive.open", "section": "学习管理", "label": "查看历史课程", "method": "GET", "path": "/meol/archive/archiveCourseListByStu.do", "mutating": False, "fields": []},
    {"name": "mail.message.action", "section": "站内邮箱", "label": "删除/标记/移动邮件", "method": "POST", "path": "/meol/common/mail/inputbox.jsp", "mutating": True, "fields": ["_action", "allbox"]},
    {"name": "mail.message.delete", "section": "站内邮箱", "label": "删除邮件", "method": "POST", "path": "/meol/common/mail/inputbox.jsp", "mutating": True, "fields": ["_action", "allbox", "ids"]},
    {"name": "mail.message.mark-unread", "section": "站内邮箱", "label": "标记未读", "method": "POST", "path": "/meol/common/mail/inputbox.jsp", "mutating": True, "fields": ["_action", "allbox", "ids"]},
    {"name": "mail.message.move", "section": "站内邮箱", "label": "移动邮件", "method": "POST", "path": "/meol/common/mail/inputbox.jsp", "mutating": True, "fields": ["_action", "allbox", "ids"]},
    {"name": "mail.search", "section": "站内邮箱", "label": "搜索邮件", "method": "POST", "path": "/meol/common/mail/search.jsp", "mutating": False, "fields": ["_action", "search", "InFolder", "Search", "allbox"]},
    {"name": "mail.trash.empty", "section": "站内邮箱", "label": "清空垃圾箱", "method": "GET", "path": "/meol/common/mail/garbage.jsp?action=empty", "mutating": True, "fields": []},
    {"name": "mail.folder.action", "section": "站内邮箱", "label": "邮箱文件夹操作", "method": "POST", "path": "/meol/common/mail/managefolder.jsp", "mutating": True, "fields": ["_action", "_HMaction", "allbox"]},
    {"name": "mail.folder.delete", "section": "站内邮箱", "label": "删除邮箱文件夹", "method": "POST", "path": "/meol/common/mail/managefolder.jsp", "mutating": True, "fields": ["_action", "_HMaction", "allbox"]},
    {"name": "mail.folder.empty", "section": "站内邮箱", "label": "清空邮箱文件夹", "method": "GET", "path": "/meol/common/mail/managefolder.jsp?action=_empty", "mutating": True, "fields": []},
    {"name": "mail.contact.action", "section": "站内邮箱", "label": "联系人操作", "method": "POST", "path": "/meol/common/mail/managecontact.jsp", "mutating": True, "fields": ["_action", "allbox"]},
    {"name": "mail.contact.delete", "section": "站内邮箱", "label": "删除联系人", "method": "POST", "path": "/meol/common/mail/managecontact.jsp", "mutating": True, "fields": ["_action", "allbox", "ids"]},
    {"name": "mail.trash.delete-forever", "section": "站内邮箱", "label": "永久删除邮件", "method": "POST", "path": "/meol/common/mail/garbage.jsp", "mutating": True, "fields": ["_action", "allbox", "ids"]},
    {"name": "mail.send", "section": "站内邮箱", "label": "发送邮件", "method": "POST", "path": "/meol/common/mail/composemail.jsp", "mutating": True, "fields": ["operation", "importance", "action", "from", "fID", "to", "subject", "sentbox"]},
    {"name": "mail.save", "section": "站内邮箱", "label": "保存邮件草稿", "method": "POST", "path": "/meol/common/mail/composemail.jsp", "mutating": True, "fields": ["operation", "importance", "action", "from", "fID", "to", "subject", "sentbox"]},
    {"name": "mail.folder.save", "section": "站内邮箱", "label": "保存邮箱文件夹", "method": "POST", "path": "/meol/common/mail/editfolder.jsp", "mutating": True, "fields": ["action", "folderID", "foldername", "from"]},
    {"name": "mail.contact.save", "section": "站内邮箱", "label": "保存邮箱联系人", "method": "POST", "path": "/meol/common/mail/editcontact.jsp", "mutating": True, "fields": ["operation", "contactID", "addanother", "from", "contactUsername", "name", "email", "vip", "department", "houseAddress", "telephone", "mobile", "note"]},
    {"name": "mail.settings.save", "section": "站内邮箱", "label": "保存邮箱设置", "method": "POST", "path": "/meol/common/mail/mailsetting.jsp", "mutating": True, "fields": ["_action", "inforWay", "saveProp"]},
    {"name": "mail.message.open", "section": "站内邮箱", "label": "查看邮件详情", "method": "GET", "path": "/meol/common/mail/mail.jsp", "mutating": False, "fields": ["id"]},
    {"name": "mail.contact.search", "section": "站内邮箱", "label": "搜索联系人", "method": "GET", "path": "/meol/common/mail/search_info.jsp", "mutating": False, "fields": ["contactUsername"]},
    {"name": "courses.search", "section": "个人", "label": "课程搜索", "method": "POST", "path": "/meol/lesson/blen.student.lesson.list.jsp", "mutating": False, "fields": ["name", "tutorName"]},
    {"name": "courses.move-up", "section": "个人", "label": "课程上移", "method": "GET", "path": "/meol/lesson/blen.student.lesson.list.jsp", "mutating": True, "fields": ["ACTION", "lid"]},
    {"name": "courses.move-down", "section": "个人", "label": "课程下移", "method": "GET", "path": "/meol/lesson/blen.student.lesson.list.jsp", "mutating": True, "fields": ["ACTION", "lid"]},
    {"name": "course.open", "section": "课程", "label": "打开课程", "method": "GET", "path": "/meol/jpk/course/layout/newpage/index.jsp", "mutating": False, "fields": ["courseId"]},
    {"name": "course.public.open", "section": "课程", "label": "打开公开课程", "method": "GET", "path": "/meol/homepage/course/course_index.jsp", "mutating": False, "fields": ["courseId"]},
    {"name": "course.lesson-home.open", "section": "课程", "label": "打开课程学习首页", "method": "GET", "path": "/meol/jpk/course/layout/lesson/index.jsp", "mutating": False, "fields": ["courseId"]},
    {"name": "course.default.open", "section": "课程", "label": "打开课程介绍首页", "method": "GET", "path": "/meol/jpk/course/layout/newpage/default_demonstrate.jsp", "mutating": False, "fields": ["courseId"]},
    {"name": "course.unit-blank.open", "section": "课程", "label": "打开单元学习容器", "method": "GET", "path": "/meol/jpk/course/layout/_blank.jsp", "mutating": False, "fields": []},
    {"name": "course.qrcode.open", "section": "课程", "label": "查看课程二维码", "method": "GET", "path": "/meol/lesson/qrcode.jsp", "mutating": False, "fields": ["lessonId"]},
    {"name": "course.stat", "section": "课程", "label": "读取课程统计", "method": "POST", "path": "/meol/jpk/course/userLessonStat.jsp", "mutating": False, "fields": ["courseId"]},
    {"name": "course.online-heartbeat", "section": "课程", "label": "记录在线时长", "method": "POST", "path": "/meol/lesson/onlinetime_listener.jsp", "mutating": True, "fields": ["lessId"]},
    {"name": "online-help.open", "section": "公共组件", "label": "打开在线帮助", "method": "GET", "path": "/meol/common/help/help.jsp", "mutating": False, "fields": ["qstr"]},
    {"name": "course.mailbox.open", "section": "课程", "label": "打开站内邮箱", "method": "GET", "path": "/meol/common/mail/inputbox.jsp", "mutating": False, "fields": []},
    {"name": "course.note.compose", "section": "课程", "label": "写课程笔记", "method": "GET", "path": "/meol/common/notebook/course_write_note.jsp", "mutating": False, "fields": ["uid", "lid"]},
    {"name": "course.note.book", "section": "课程", "label": "查看课程笔记本", "method": "GET", "path": "/meol/common/notebook/course_notebook.jsp", "mutating": False, "fields": ["tag", "lid"]},
    {"name": "course.forum.frame", "section": "课程", "label": "打开讨论区弹窗", "method": "GET", "path": "/meol/common/faq/forum.jsp", "mutating": False, "fields": ["lid"]},
    {"name": "course.livevod.open", "section": "课程", "label": "打开直播预约", "method": "GET", "path": "/meol/livevod/liveVodAppointmentStuList.do", "mutating": False, "fields": ["courseId"]},
    {"name": "course.ccvod.open", "section": "课程", "label": "打开 CC 直播频道", "method": "GET", "path": "/meol/ccvod/ccVodChannelStuList.do", "mutating": False, "fields": ["courseId"]},
    {"name": "course.netmeeting.open", "section": "课程", "label": "打开网络会议", "method": "GET", "path": "/meol/netmeeting/netMeetingPreAddUser.do", "mutating": False, "fields": ["courseId"]},
    {"name": "course.tencent-meeting.open", "section": "课程", "label": "打开腾讯会议", "method": "GET", "path": "/meol/tencentmeeting/tenCentMeetingPreAddUser.do", "mutating": False, "fields": ["courseId", "meetingStatus"]},
    {"name": "podcast.search", "section": "播客", "label": "播客筛选查询", "method": "POST", "path": "/meol/custom.do?menuId=4", "mutating": False, "fields": ["title", "typeid", "pagingNumberPer"]},
    {"name": "podcast.play", "section": "播客", "label": "播放播客", "method": "GET", "path": "/meol/common/stream/player.jsp", "mutating": False, "fields": ["fileId", "vId"]},
    {"name": "podcast.recommend", "section": "播客", "label": "推荐播客", "method": "GET", "path": "/meol/common/vblog/recommend.jsp", "mutating": True, "fields": ["id"]},
    {"name": "podcast.favorite", "section": "播客", "label": "收藏播客", "method": "GET", "path": "/meol/common/vblog/favorite/add_favorite.jsp", "mutating": True, "fields": ["id"]},
    {"name": "podcast.list.search", "section": "播客", "label": "筛选全校视频", "method": "GET", "path": "/meol/common/vblog/listvideo.jsp", "mutating": False, "fields": ["title", "typeid"]},
    {"name": "podcast.list.sort", "section": "播客", "label": "排序全校视频", "method": "GET", "path": "/meol/common/vblog/listvideo.jsp", "mutating": False, "fields": ["sortColumn", "sortDirection", "pagingPage", "pagingNumberPer"]},
    {"name": "podcast.favorite.delete", "section": "播客", "label": "删除播客收藏", "method": "POST", "path": "/meol/common/vblog/favorite/del_favorite.jsp", "mutating": True, "fields": ["ids"]},
    {"name": "course.column.open", "section": "课程", "label": "打开课程栏目", "method": "GET", "path": "/meol/jpk/course/course_column_preview_transfer.jsp?tagbug=client", "mutating": False, "fields": ["columnId"]},
    {"name": "course.notice.list", "section": "课程", "label": "课程通知列表", "method": "GET", "path": "/meol/article/ListNews.do", "mutating": False, "fields": ["courseId", "s_order", "s_page"]},
    {"name": "course.notice.detail", "section": "课程", "label": "课程通知详情", "method": "GET", "path": "/meol/jpk/course/layout/course_meswrap.jsp", "mutating": False, "fields": ["nid", "courseId"]},
    {"name": "course.resource.search", "section": "课程", "label": "课程资源查询", "method": "GET", "path": "/meol/common/script/search.jsp", "mutating": False, "fields": ["folderid", "lid", "groupid", "keyword", "resourceType", "chapter", "knowledge"]},
    {"name": "course.resource.open-folder", "section": "课程", "label": "进入资源文件夹", "method": "GET", "path": "/meol/common/script/listview.jsp", "mutating": False, "fields": ["acttype", "folderid", "lid"]},
    {"name": "course.resource.preview", "section": "课程", "label": "预览/下载课程资源", "method": "GET", "path": "/meol/common/script/preview/download_preview.jsp", "mutating": False, "fields": ["fileid", "resid", "lid"]},
    {"name": "course.analytics", "section": "课程", "label": "学习分析", "method": "GET", "path": "/meol/jpk/course/blended_module/mod_la_stu.jsp", "mutating": False, "fields": ["lid"]},
    {"name": "course.score", "section": "课程", "label": "课程成绩", "method": "GET", "path": "/meol/newscoremanagement/scoreView.do", "mutating": False, "fields": ["lid"]},
    {"name": "course.learning-record", "section": "课程", "label": "课程学习记录", "method": "GET", "path": "/meol/common/newscoremanagement/stu_course_detail.jsp", "mutating": False, "fields": ["lid", "uid"]},
    {"name": "course.notes.search", "section": "课程", "label": "学习笔记查询", "method": "POST", "path": "/meol/common/notebook/column_notebook_share.jsp", "mutating": False, "fields": ["lid", "tag", "s_keyword", "userId", "listLimit"]},
    {"name": "course.faq.search", "section": "课程", "label": "FAQ 查询", "method": "GET", "path": "/meol/common/faq/course_tea_faq.jsp", "mutating": False, "fields": ["lid", "kid", "s_keyword", "isRecommend"]},
    {"name": "course.forum.discuss", "section": "课程", "label": "打开讨论区版面", "method": "GET", "path": "/meol/common/faq/tea_discuss.jsp", "mutating": False, "fields": ["forumid"]},
    {"name": "course.faq.report", "section": "课程", "label": "提交问题反馈", "method": "POST", "path": "/meol/popups/bugreport_do.jsp", "mutating": True, "fields": ["BUG_SUBJECT", "BUG_HTMLFORMAT", "BUG_CONTENT", "BUG_EMAIL"]},
    {"name": "course.forum.search", "section": "课程", "label": "讨论区搜索", "method": "POST", "path": "/meol/common/faq/forum_search.jsp", "mutating": False, "fields": ["forumid", "s_keyword", "username"]},
    {"name": "course.forum.post", "section": "课程", "label": "发表话题/回复", "method": "POST", "path": "/meol/common/faq/postArticle.jsp?opt=topost", "mutating": True, "fields": ["forumid", "threadid", "parentid", "subject", "content"]},
    {"name": "system.faq.search", "section": "公共组件", "label": "搜索系统帮助", "method": "POST", "path": "/meol/common/faq/sys_faq.jsp", "mutating": False, "fields": ["lid", "kid", "s_keyword", "s_keywordrealation", "search", "s_gotopage"]},
    {"name": "course.survey.list", "section": "课程", "label": "课程问卷列表", "method": "POST", "path": "/meol/common/questionaire/stu_survey.jsp", "mutating": False, "fields": ["lid", "moduleType", "s_order", "s_page"]},
    {"name": "course.survey.take", "section": "课程", "label": "参加问卷", "method": "GET", "path": "/meol/common/questionaire/survey.jsp", "mutating": False, "fields": ["paperId", "lid", "moduleType"]},
    {"name": "course.survey.result", "section": "课程", "label": "查看问卷结果", "method": "GET", "path": "/meol/common/questionaire/survey_result.jsp", "mutating": False, "fields": ["paperid", "moduleType"]},
    {"name": "course.question.search", "section": "课程", "label": "题库查询", "method": "POST", "path": "/meol/common/question/questionbank/student/list.jsp", "mutating": False, "fields": ["cateId", "questionType", "chapterId", "sectionId", "knowledgeKey", "keywords", "pagingNumberPer"]},
    {"name": "course.paper.search", "section": "课程", "label": "试卷库查询", "method": "GET", "path": "/meol/common/question/paperbank/student/list.jsp", "mutating": False, "fields": ["cateId", "keywords", "pagingNumberPer"]},
    {"name": "course.test.list", "section": "课程", "label": "在线测试列表", "method": "GET", "path": "/meol/common/question/test/student/list.jsp", "mutating": False, "fields": ["cateId", "pagingNumberPer", "pagingPage"]},
    {"name": "course.test.open", "section": "课程", "label": "打开在线测试", "method": "GET", "path": "/meol/common/question/test/student/stu_qtest_navigate.jsp", "mutating": False, "fields": ["testId"]},
    {"name": "course.micro-lessons", "section": "课程", "label": "教学播课", "method": "GET", "path": "/meol/microlessonunit/viewMicroLesson.do", "mutating": False, "fields": ["courseId", "ustatus"]},
    {"name": "resource.search", "section": "资源中心", "label": "资源检索", "method": "GET", "path": "/moocresource/search/browser.jsp", "mutating": False, "fields": ["id", "keyword"]},
    {"name": "resource.home.search", "section": "资源中心", "label": "资源首页搜索", "method": "POST", "path": "/moocresource/view/reslist.jsp", "mutating": False, "fields": ["inputkeyword", "domain"]},
    {"name": "resource.bank.subject", "section": "资源中心", "label": "资源库分类浏览", "method": "GET", "path": "/moocresource/banks/search/bank_index.jsp", "mutating": False, "fields": ["bankId", "viewtype", "subjectId", "resourceType", "mediaType", "pagingPage", "pagingNumberPer"]},
    {"name": "resource.bank.open", "section": "资源中心", "label": "打开资源库", "method": "GET", "path": "/moocresource/banks/search/bank.jsp", "mutating": False, "fields": ["bankId"]},
    {"name": "resource.bank.list", "section": "资源中心", "label": "查询专题库资源", "method": "POST", "path": "/moocresource/banks/search/reslist.jsp", "mutating": False, "fields": ["bankId", "subjectid", "courseName", "keyWord", "courseType", "mediaType", "courseLevel", "pagingPage", "pagingNumberPer"]},
    {"name": "resource.bank.keyword", "section": "资源中心", "label": "关键词查询资源", "method": "POST", "path": "/moocresource/courses/searchres.jsp", "mutating": False, "fields": ["keyword"]},
    {"name": "resource.excellent.search", "section": "资源中心", "label": "精品课程搜索", "method": "POST", "path": "/moocresource/courses/excellent/course_list_query.jsp", "mutating": False, "fields": ["inputkeyword", "universityId", "domain"]},
    {"name": "resource.open.search", "section": "资源中心", "label": "全球开放课程搜索", "method": "POST", "path": "/moocresource/courses/ocw/ocwcoursesearch.jsp", "mutating": False, "fields": ["inputkeyword", "universityId", "domain"]},
    {"name": "resource.video.search", "section": "资源中心", "label": "公开视频搜索", "method": "POST", "path": "/moocresource/courses/xiaoyouhui/video_list_query.jsp", "mutating": False, "fields": ["inputkeyword", "bankType", "domain"]},
    {"name": "resource.mooc.search", "section": "资源中心", "label": "MOOC搜索", "method": "POST", "path": "/moocresource/courses/xiaoyouhui/mooc_list_query.jsp", "mutating": False, "fields": ["inputkeyword", "bankType", "domain"]},
    {"name": "resource.micro.search", "section": "资源中心", "label": "微课程搜索", "method": "POST", "path": "/moocresource/courses/xiaoyouhui/micro_list_query.jsp", "mutating": False, "fields": ["inputkeyword", "bankType", "domain"]},
    {"name": "resource.tag.search", "section": "资源中心", "label": "标签资源搜索", "method": "GET", "path": "/moocresource/courses/xiaoyouhui/searchresourcebytag.jsp", "mutating": False, "fields": ["tagName"]},
    {"name": "resource.entry.open", "section": "资源中心", "label": "打开外部资源条目", "method": "GET", "path": "/moocresource/search/browser_by_entry.jsp", "mutating": False, "fields": ["entry"]},
    {"name": "resource.bank.login", "section": "资源中心", "label": "打开资源库登录提示", "method": "GET", "path": "/moocresource/banks/login.jsp", "mutating": False, "fields": ["url"]},
    {"name": "resource.sso.open", "section": "资源中心", "label": "进入资源中心 SSO", "method": "POST", "path": "/moocresource/eol_sso_new.jsp", "mutating": True, "fields": ["user", "time", "verify", "columnId", "moduleType", "shareResId", "bankId", "sso", "resourceIndex"]},
    {"name": "resource.attribute", "section": "资源中心", "label": "资源属性", "method": "GET", "path": "/moocresource/banks/search/attribute.jsp", "mutating": False, "fields": ["erid"]},
    {"name": "resource.related", "section": "资源中心", "label": "相关资源", "method": "GET", "path": "/moocresource/banks/relateres.jsp", "mutating": False, "fields": ["resId"]},
    {"name": "resource.my-list", "section": "资源中心", "label": "我的资源", "method": "GET", "path": "/moocresource/myres/resourceInfoList.do", "mutating": False, "fields": ["resourceName", "keyword", "resourceType", "directory"]},
    {"name": "resource.list.search", "section": "资源中心", "label": "筛选我的资源", "method": "POST", "path": "/moocresource/resource/resourceInfoList.do", "mutating": False, "fields": ["title", "keyWord", "type", "mySelfFolderId"]},
    {"name": "resource.list.page", "section": "资源中心", "label": "我的资源分页", "method": "POST", "path": "/moocresource/resource/resourceInfoList.do", "mutating": False, "fields": ["folderId", "oldFolderId", "pagingNumberPer", "pagingPage"]},
    {"name": "resource.add", "section": "资源中心", "label": "添加资源", "method": "GET", "path": "/moocresource/resource/preAddResourceInfo.do", "mutating": False, "fields": ["mySelfFolderId", "oldFolderId"]},
    {"name": "resource.move-batch", "section": "资源中心", "label": "批量移动资源", "method": "POST", "path": "/moocresource/resource/moveResourceInfoByBatch.do", "mutating": True, "fields": ["beanIds", "oldFolderId"]},
    {"name": "resource.delete-batch", "section": "资源中心", "label": "批量删除资源", "method": "POST", "path": "/moocresource/resource/deleteResourceInfoByBatch.do", "mutating": True, "fields": ["beanIds", "oldFolderId"]},
    {"name": "resource.my-folders", "section": "资源中心", "label": "我的资源文件夹", "method": "GET", "path": "/moocresource/myres/listMyFolder.do", "mutating": False, "fields": ["folderName"]},
    {"name": "resource.my-folders-frame", "section": "资源中心", "label": "读取资源目录", "method": "GET", "path": "/moocresource/resource/listMyFolder.do", "mutating": False, "fields": ["folderName", "pagingPage", "pagingNumberPer"]},
    {"name": "resource.favorites", "section": "资源中心", "label": "资源收藏", "method": "GET", "path": "/moocresource/myres/favorite_detail.jsp", "mutating": False, "fields": ["folderId", "resourceName"]},
    {"name": "resource.favorites-frame", "section": "资源中心", "label": "读取收藏资源", "method": "GET", "path": "/moocresource/myres/favorite/favorite_detail.jsp", "mutating": False, "fields": ["folderId", "resourceName"]},
    {"name": "resource.follows", "section": "资源中心", "label": "资源关注", "method": "GET", "path": "/moocresource/myres/attentionres_list.jsp", "mutating": False, "fields": []},
    {"name": "resource.follows-frame", "section": "资源中心", "label": "读取关注资源", "method": "GET", "path": "/moocresource/myres/attentionres/attentionres_list.jsp", "mutating": False, "fields": []},
    {"name": "resource.comments", "section": "资源中心", "label": "资源评论", "method": "GET", "path": "/moocresource/myres/reviewlist.jsp", "mutating": False, "fields": []},
    {"name": "resource.comments-frame", "section": "资源中心", "label": "读取资源评论", "method": "GET", "path": "/moocresource/myres/myreview/reviewlist.jsp", "mutating": False, "fields": []},
    {"name": "resource.shares", "section": "资源中心", "label": "共享资源", "method": "GET", "path": "/moocresource/myres/listShareRes.do", "mutating": False, "fields": ["resourceName", "sharer", "keyword", "pagingPage"]},
    {"name": "resource.shares-frame", "section": "资源中心", "label": "读取共享资源", "method": "GET", "path": "/moocresource/resource/listShareRes.do", "mutating": False, "fields": ["resourceName", "sharer", "keyword", "pagingPage"]},
    {"name": "resource.personal-info", "section": "资源中心", "label": "资源个人统计", "method": "GET", "path": "/moocresource/myres/class_personal_info.jsp", "mutating": False, "fields": []},
    {"name": "resource.personal-info-frame", "section": "资源中心", "label": "读取资源个人统计", "method": "GET", "path": "/moocresource/personalset/class_personal_info.jsp", "mutating": False, "fields": []},
    {"name": "resource.view", "section": "资源中心", "label": "查看共享资源详情", "method": "GET", "path": "/moocresource/resource/viewResourceInfo.do", "mutating": False, "fields": ["id"]},
    {"name": "resource.favorite", "section": "资源中心", "label": "收藏/取消收藏资源", "method": "POST", "path": "/moocresource/myres/favorite/favorite.jsp", "mutating": True, "fields": ["resId"]},
    {"name": "resource.rate", "section": "资源中心", "label": "资源评分", "method": "POST", "path": "/moocresource/search/res_evaluate.jsp", "mutating": True, "fields": ["score", "resId"]},
    {"name": "resource.download", "section": "资源中心", "label": "下载资源", "method": "GET", "path": "/moocresource/servlet/ResourceDownload", "mutating": False, "fields": ["erid"]},
    {"name": "resource.tag-exists", "section": "资源中心", "label": "检查资源标签", "method": "POST", "path": "/moocresource/taglibrary/tagExist.do", "mutating": False, "fields": ["tagName", "objId", "objType"]},
    {"name": "resource.tag-add", "section": "资源中心", "label": "添加资源标签", "method": "POST", "path": "/moocresource/taglibrary/tagAdd.do", "mutating": True, "fields": ["tagName", "objId", "objType"]},
    {"name": "folder.add", "section": "资源中心", "label": "添加资源目录", "method": "GET", "path": "/moocresource/resource/preAddFolder.do", "mutating": False, "fields": ["fid"]},
    {"name": "folder.update", "section": "资源中心", "label": "更新资源目录", "method": "POST", "path": "/moocresource/resource/updateMySelfFolder.do", "mutating": True, "fields": ["id", "level", "title"]},
    {"name": "folder.share-batch", "section": "资源中心", "label": "共享/取消共享目录", "method": "POST", "path": "/moocresource/resource/shareFolderBatch.do", "mutating": True, "fields": ["folderIds", "shareType"]},
    {"name": "folder.list", "section": "资源中心", "label": "管理收藏夹目录", "method": "GET", "path": "/moocresource/resource/myfolder_list.jsp", "mutating": False, "fields": ["indexStatus"]},
    {"name": "follow.add", "section": "资源中心", "label": "添加资源关注", "method": "GET", "path": "/moocresource/myres/attentionres/attentionres_add.jsp", "mutating": False, "fields": []},
    {"name": "homework.list", "section": "作业", "label": "作业列表", "method": "GET", "path": "/meol/hw/stu/hwStuHwtList.do", "mutating": False, "fields": ["courseId", "title", "pagingNumberPer", "um", "pagingPage", "sortColumn", "sortDirection"]},
    {"name": "homework.submit", "section": "作业", "label": "提交作业", "method": "POST", "path": "/meol/hw/stu/hwStuSubmitDo.do", "mutating": True, "fields": ["hwaId", "courseId", "um", "hwtId", "answer"]},
    {"name": "homework.reviews", "section": "作业", "label": "互评任务", "method": "GET", "path": "/meol/hw/stu/hwReviewList.do", "mutating": False, "fields": ["hwtId", "courseId", "um"]},
    {"name": "homework.review.save", "section": "作业", "label": "保存互评", "method": "POST", "path": "/meol/hw/stu/hwStuReviewSave.do", "mutating": True, "fields": ["hwtId", "mutualId", "um"]},
    {"name": "homework.score", "section": "作业", "label": "作业成绩", "method": "GET", "path": "/meol/hw/stu/hwShowScore.do", "mutating": False, "fields": ["hwtId", "um"]},
    {"name": "homework.result", "section": "作业", "label": "互评结果", "method": "GET", "path": "/meol/hw/stu/hwMutualList.do", "mutating": False, "fields": ["hwtId", "um"]},
    {"name": "homework.complain", "section": "作业", "label": "作业评价申诉", "method": "POST", "path": "/meol/hw/stu/hwShowReviewDetails.do", "mutating": True, "fields": ["hwtId", "um", "type"]},
)


TEACHING_ROUTE_BY_NAME = {str(item["name"]): item for item in TEACHING_ROUTE_CATALOG}
TEACHING_API_BY_NAME = {str(item["name"]): item for item in _TEACHING_API_ROWS}
TEACHING_ACTION_BY_NAME = {str(item["name"]): item for item in TEACHING_ACTION_CATALOG}


class TeachingClient(VpnClient):
    """VPN-cookie client that discovers the current teaching gateway prefix."""

    def __init__(
        self,
        base_url: str | None = None,
        cookie_file: Path | None = None,
        session_file: Path | None = None,
        *,
        prefix: str | None = None,
        service_name: str = TEACHING_SERVICE_NAME,
        load_cookies: bool = True,
    ) -> None:
        super().__init__(base_url, cookie_file, session_file, load_cookies=load_cookies)
        self.web_prefix = (prefix or "").rstrip("/")
        self.service_name = service_name
        self.service: dict[str, object] | None = None

    def ensure_service(self, *, recover: bool = True) -> dict[str, object]:
        self._validate_prefix(self.web_prefix or os_environ("CSUST_TEACHING_PREFIX"))
        recovered = False
        if not self.session.get("token") and not _has_session_cookie(self):
            if not recover:
                raise LoginRequired("VPN 会话不存在")
            self._recover_vpn_session()
            recovered = True
        if self.web_prefix:
            return self.service or {"name": self.service_name, "urlPlus": self.web_prefix}
        configured = os_environ("CSUST_TEACHING_PREFIX") if self.service_name == TEACHING_SERVICE_NAME else ""
        if configured:
            self.web_prefix = configured.rstrip("/")
            return {"name": self.service_name, "urlPlus": self.web_prefix}
        try:
            _response, value = self.request_api(TEACHING_SERVICE_GROUP_API, method="GET", retry_refresh=False)
        except HttpError as exc:
            if exc.status != 401:
                raise
            if not recover:
                raise
            if not recovered:
                self._recover_vpn_session()
                recovered = True
            _response, value = self.request_api(TEACHING_SERVICE_GROUP_API, method="GET", retry_refresh=False)
        if isinstance(value, dict) and str(value.get("code")) == "3010":
            from .vpn import login

            if not recover:
                raise LoginRequired("VPN 会话已失效")
            if not recovered:
                if self.session.get("refreshToken"):
                    if not self.refresh():
                        raise CsustError("VPN 会话刷新失败，请重新登录", code="login_required")
                else:
                    login(argparse.Namespace(auth="cas", username=None, password_stdin=False, captcha_info=None), self)
                recovered = True
            _response, value = self.request_api(TEACHING_SERVICE_GROUP_API, method="GET", retry_refresh=False)
        result_status(value, mutating=False, success_codes=("200",))
        service = _find_teaching_service(value, self.service_name)
        if service is None:
            raise CsustError(f"VPN 当前没有可用服务：{self.service_name}", code="service_unavailable")
        service_url = service.get("url")
        if isinstance(service_url, str) and service_url.strip():
            opened = self.request(
                self._api_path(service_url),
                method="GET",
                headers={
                    **self._headers(self._api_path(service_url)),
                    "Accept": "text/html,application/xhtml+xml,application/json,text/plain,*/*",
                    "Accept-Encoding": "identity",
                },
                with_metadata=True,
            )
            assert isinstance(opened, Response)
            _save_cookie_refresh(self, opened)
            opened_payload = web._feedback(opened)
            if business_state(opened_payload) is False:
                result_status(opened_payload, mutating=False)
        url_plus = str(service.get("urlPlus") or "")
        match = re.match(r"^(/(?:http|https)/[^/]+)", url_plus, re.I)
        if not match:
            raise CsustError("无法解析网络教学平台网关地址", code="service_unavailable")
        self.web_prefix = match.group(1).rstrip("/")
        self.service = {"name": self.service_name, "id": service.get("id"), "type": service.get("type"), "urlPlus": self.web_prefix}
        return self.service

    def _validate_prefix(self, prefix: str) -> None:
        if not prefix:
            return
        try:
            parsed = urlsplit(prefix)
            path = unquote(unquote(parsed.path))
        except (TypeError, ValueError) as exc:
            raise CsustError("教学平台网关前缀格式无效", code="invalid_path") from exc
        if parsed.scheme or parsed.netloc or not path.startswith(("/http/", "/https/")) or "\\" in path or any(part in {".", ".."} for part in path.split("/")):
            raise CsustError("教学平台网关前缀必须是当前 VPN 的 /http/... 或 /https/... 路径", code="invalid_path")

    def _recover_vpn_session(self) -> None:
        from .vpn import login

        if self.session.get("refreshToken") and self.refresh():
            return
        login(argparse.Namespace(auth="cas", username=None, password_stdin=False, captcha_info=None), self)

    def web_url(self, path: str, *, _recover_session: bool = True) -> str:
        if not isinstance(path, str) or not path.strip() or any(ord(c) < 0x20 for c in path):
            raise CsustError("教学平台路径格式无效", code="invalid_path")
        value = path.strip()
        try:
            parsed_value = urlsplit(value)
            request_path = unquote(unquote(parsed_value.path))
        except (TypeError, ValueError) as exc:
            raise CsustError("教学平台路径格式无效", code="invalid_path") from exc
        if "\\" in request_path or any(part in {".", ".."} for part in request_path.split("/") if part):
            raise CsustError("教学平台路径不能包含目录跳转", code="invalid_path")
        absolute_target = same_origin_url(self, value) if value.lower().startswith(("http://", "https://")) else ""
        service = self.ensure_service(recover=_recover_session)
        prefix = str(service.get("urlPlus") or self.web_prefix).rstrip("/")
        self._validate_prefix(prefix)
        if value.lower().startswith(("http://", "https://")):
            target = absolute_target
            if not urlsplit(target).path.startswith(prefix + "/") and urlsplit(target).path != prefix:
                raise CsustError("教学平台地址不属于当前服务", code="invalid_path")
            return target
        if value.startswith("/http/") or value.startswith("/https/"):
            target_path = value
            if not target_path.startswith(prefix + "/") and target_path != prefix:
                raise CsustError("教学平台地址不属于当前服务", code="invalid_path")
        else:
            target_path = prefix + "/" + value.lstrip("/")
        return self.url(target_path)

    def request_web(
        self,
        path: str,
        *,
        method: str = "GET",
        params: list[tuple[str, str]] | tuple[tuple[str, str], ...] = (),
        data: list[tuple[str, str]] | None = None,
        json_body: object = _UNSET,
        multipart: list[tuple[str, object]] | None = None,
        output: bool | str = False,
        referer: str = "",
        _recover_session: bool = True,
        mutating: bool | None = None,
    ) -> Response:
        method = method.strip().upper()
        if method not in web.SUPPORTED_METHODS:
            raise CsustError("不支持的 HTTP 方法", code="invalid_argument")
        effective_mutating = method not in web.READ_ONLY_METHODS if mutating is None else mutating
        raw = _resolve_path(path, params, ())
        target = self.web_url(raw, _recover_session=_recover_session)
        headers = self._headers(target)
        headers["Accept"] = "text/html,application/xhtml+xml,application/json,text/plain,*/*"
        headers["Accept-Encoding"] = "identity"
        # Legacy THEOL SSO expects a browser navigation.  The EnUES bearer
        # header is valid for the portal JSON API but makes the web gateway
        # redirect its own SSO request in a loop.
        for key in ("Authorization", "Content-Type", "Origin", "ajax-Flow", "UseMode", "userProtocolState"):
            headers.pop(key, None)
        if referer and urlsplit(referer).netloc.lower() == urlsplit(target).netloc.lower():
            headers["Referer"] = referer
        options: dict[str, object] = {"method": method, "headers": headers, "with_metadata": True, "binary": bool(output)}
        if isinstance(output, str):
            options["stream_to"] = output
            options["defer_stream_commit"] = True
        if method in web.READ_ONLY_METHODS:
            if data is not None or multipart is not None or json_body is not _UNSET:
                raise CsustError("只读请求请使用 --param", code="invalid_argument")
        elif multipart is not None:
            options["multipart"] = multipart
        elif json_body is not _UNSET:
            options["json_body"] = json_body
        else:
            options["data"] = data or []
        try:
            result = self.request(target, **options)
        except HttpError as exc:
            if exc.status != 401 or effective_mutating or not _recover_session:
                if effective_mutating:
                    raise MutationUnverified(
                        "教学平台写请求已发送但结果未知",
                        details={"submitted": True, "confirmed": False, "request": {"method": method, "path": urlsplit(target).path}, "cause": exc.code},
                    ) from exc
                raise
            self._recover_vpn_session()
            result = self.request(target, **options)
        except NetworkError as exc:
            if not effective_mutating:
                raise
            raise MutationUnverified(
                "教学平台写请求已发送但结果未知",
                details={"submitted": True, "confirmed": False, "request": {"method": method, "path": urlsplit(target).path}, "cause": exc.code},
            ) from exc
        assert isinstance(result, Response)
        try:
            if effective_mutating and is_login_page(result):
                raise MutationUnverified(
                    "教学平台写请求已发送但返回登录页，结果未知",
                    details={"submitted": True, "confirmed": False, "request": {"method": method, "path": urlsplit(target).path}, "cause": "login_required"},
                )
            _save_cookie_refresh(self, result)
        except CsustError as exc:
            _discard_stream(result)
            if isinstance(exc, MutationUnverified):
                raise
            if effective_mutating:
                raise MutationUnverified(
                    "教学平台写请求已完成但会话保存失败，结果未知",
                    details={"submitted": True, "confirmed": False, "request": {"method": method, "path": urlsplit(target).path}, "save_error": exc.code},
                ) from exc
            raise
        return result


def os_environ(name: str) -> str:
    # Kept as a tiny indirection so catalog tests never need to mutate the
    # process environment while constructing a client.
    import os

    return os.environ.get(name, "").strip()


def _find_teaching_service(value: object, service_name: str = TEACHING_SERVICE_NAME) -> dict[str, object] | None:
    if not isinstance(value, dict):
        return None
    data = value.get("data")
    if not isinstance(data, dict):
        return None
    children = data.get("children")
    if not isinstance(children, list):
        return None
    for group in children:
        if not isinstance(group, dict):
            continue
        services = group.get("serviceList")
        if not isinstance(services, list):
            continue
        for item in services:
            if isinstance(item, dict) and item.get("name") == service_name:
                return item
    return None


def _response_text(response: Response) -> str:
    return _decode_body(response.body, response.headers)


def _redact_raw_text(value: str, page_url: str = "") -> str:
    def redact_control(match: re.Match[str]) -> str:
        tag = match.group(0)
        identity = re.search(
            r"\b(?:name|id)\s*=\s*(['\"]?)([^'\"\s>]+)\1", tag, re.I
        )
        if identity and _SENSITIVE_FIELD.search(identity.group(2)):
            tag = re.sub(
                r"(\bvalue\s*=\s*)(['\"])(.*?)(\2)",
                r"\1\2<redacted>\4",
                tag,
                count=1,
                flags=re.I | re.S,
            )
        return tag

    value = re.sub(
        r"<(?:input|textarea|select|option|button)\b[^>]*>",
        redact_control,
        value,
        flags=re.I | re.S,
    )
    value = re.sub(
        r"(\b(?:href|src|action)\s*=\s*)(['\"])(.*?)(\2)",
        lambda match: (
            match.group(1)
            + match.group(2)
            + (
                _safe_url(match.group(3), page_url)
                if any(token in match.group(3) for token in ("?", ";"))
                or match.group(3).lower().startswith(("http://", "https://"))
                else match.group(3)
            )
            + match.group(4)
        ),
        value,
        flags=re.I | re.S,
    )
    return re.sub(
        rf"(?i)((?:{_SENSITIVE_FIELD.pattern})\s*[=:]\s*)([^&\s,;<>\"']+)",
        r"\1<redacted>",
        value,
    )


def _response_payload(response: Response, *, raw: bool = False, mutating: bool = False) -> object:
    require_logged_in(response)
    if not _response_text(response).strip():
        return None
    payload = _redact(web._feedback(response))
    if isinstance(payload, str):
        payload = _redact_raw_text(payload, response.url)
    if not mutating and business_state(payload) is False:
        result_status(payload, mutating=False)
    if raw and isinstance(payload, dict):
        body = _redact_raw_text(_response_text(response), response.url)
        payload = {**payload, "body": body}
    return payload


def _entry(name: str | None, path: str | None, method: str | None) -> tuple[str, str, bool, str]:
    if name and path:
        raise CsustError("--name 与 --path 不能同时使用", code="invalid_argument")
    if name:
        key = name.strip()
        matches = [catalog.get(key) for catalog in (TEACHING_ACTION_BY_NAME, TEACHING_API_BY_NAME, TEACHING_ROUTE_BY_NAME) if key in catalog]
        if not matches:
            raise CsustError(f"未知教学平台目录项：{name}；先运行 csust teaching catalog", code="unknown_route")
        if len(matches) > 1:
            raise CsustError(f"教学平台目录名称有歧义：{name}；请改用 --path", code="ambiguous_route")
        item = matches[0]
        method = str(item.get("method") or method or "GET").strip().upper()
        return str(item["path"]), method, bool(item.get("mutating", method not in web.READ_ONLY_METHODS)), key
    if not path:
        raise CsustError("--name 与 --path 至少指定一个", code="invalid_argument")
    method = (method or "GET").strip().upper()
    return path, method, method not in web.READ_ONLY_METHODS, ""


def _request_info(name: str, path: str, method: str, data: list[tuple[str, str]]) -> dict[str, object]:
    return {"name": name or None, "method": method, "path": urlsplit(path).path, "fields": [key for key, _ in data], "service": TEACHING_SERVICE_NAME}


def _mutation_confirmed(payload: object) -> bool:
    return business_state(payload) is True


def _run_request(args: argparse.Namespace, client: TeachingClient) -> dict[str, object]:
    path, method, catalog_mutating, name = _entry(getattr(args, "name", None), getattr(args, "path", None), getattr(args, "method", None))
    mutating = catalog_mutating if name else method not in web.READ_ONLY_METHODS
    if mutating and not args.yes:
        raise CsustError("教学平台请求可能改变远端状态，请加 --yes", code="confirmation_required")
    params = _parse_pairs(getattr(args, "param", []), "--param")
    data = _parse_pairs(getattr(args, "data", []), "--data")
    files = _parse_files(getattr(args, "file", []))
    data_json = getattr(args, "data_json", None)
    if (files or data) and data_json is not None:
        raise CsustError("--data-json 不能与 --data/--file 同时使用", code="invalid_argument")
    if method in web.READ_ONLY_METHODS and (data or files or data_json is not None):
        raise CsustError("GET/HEAD/OPTIONS 只能使用 --param", code="invalid_argument")
    json_body = _UNSET if data_json is None else _json_argument(data_json)
    response = client.request_web(
        path,
        method=method,
        params=params,
        data=None if json_body is not _UNSET or files else (data or None),
        json_body=json_body,
        multipart=(data + files) if files else None,
        output=args.output or False,
        referer=getattr(args, "referer", ""),
        mutating=mutating,
    )
    request = _request_info(name, path, method, data)
    payload = None if args.output and response.stream_path else _response_payload(response, raw=bool(args.raw), mutating=mutating)
    if business_state(payload) is False:
        result_status(payload, mutating=mutating, details={"request": request})
    if args.output:
        body = response.body if isinstance(response.body, bytes) else str(response.body).encode("utf-8")
        output = Path(args.output).expanduser()
        saved = web._write_download(response, output, require_session=False, mutating=mutating)
        saved.update({"status": response.status, "request": request})
        return {**saved, **result_status(payload, mutating=mutating, details=saved)}
    return {
        **result_status(payload, mutating=mutating, details={"request": request}),
        "status": response.status,
        "request": request,
        "response": payload,
    }


def _course_rows(source: str, page_url: str) -> list[dict[str, object]]:
    document = parse_html(source)
    rows: list[dict[str, object]] = []
    for table in document.find_all("table"):
        for row in _table_rows(table):
            cells = row.direct("th") + row.direct("td")
            links = row.find_all("a")
            course = next(
                (link for link in links if "courseId=" in (link.attr("href") + " " + link.attr("onclick"))),
                None,
            )
            if course is None:
                continue
            source = course.attr("href") + " " + course.attr("onclick")
            course_id_match = re.search(r"courseId=([A-Za-z0-9_-]+)", source)
            query = {"courseId": course_id_match.group(1)} if course_id_match else {}
            values = [cell.text(include_scripts=False) for cell in cells]
            order: dict[str, object] = {
                "course_id": query.get("courseId", ""),
                "name": course.text(include_scripts=False),
                "href": _safe_url(re.sub(r"^javascript:.*?(['\"])([^'\"]*courseId=[^'\"]+)\1.*$", r"\2", source).strip(), page_url),
                "columns": values,
            }
            for direction in ("up", "down"):
                link = next((item for item in links if f"LESS{direction.upper()}" in item.attr("href")), None)
                if link is not None:
                    order[direction] = _safe_url(link.attr("href"), page_url)
            if len(values) >= 4:
                order.update({"course_number": values[0], "department": values[2], "tutor": values[3]})
            rows.append(order)
    return rows


def run_courses(args: argparse.Namespace, client: TeachingClient) -> dict[str, object]:
    data = []
    if args.name:
        data.append(("name", args.name))
    if args.tutor:
        data.append(("tutorName", args.tutor))
    personal = client.request_web("/meol/personal.do", params=[("menuId", "0")])
    response = client.request_web(
        "/meol/lesson/blen.student.lesson.list.jsp",
        method="POST" if data else "GET",
        data=data or None,
        referer=personal.url,
    )
    if not _response_text(response).strip():
        raise ParseError("教学平台课程页面为空")
    payload = _response_payload(response, raw=bool(args.raw))
    rows = _course_rows(_response_text(response), response.url)
    return {"ok": True, "courses": rows, "course_count": len(rows), "page": payload}


def run_course(args: argparse.Namespace, client: TeachingClient) -> dict[str, object]:
    if not args.course_id:
        raise CsustError("--course-id 不能为空", code="invalid_argument")
    if args.column_id:
        path = "/meol/jpk/course/course_column_preview_transfer.jsp?tagbug=client"
        params = [("columnId", args.column_id)]
    else:
        path = "/meol/jpk/course/layout/newpage/index.jsp"
        params = [("courseId", args.course_id)]
    response = client.request_web(path, params=params)
    if not _response_text(response).strip():
        raise ParseError("教学平台课程页面为空")
    return {"ok": True, "course_id": args.course_id, "column_id": args.column_id or None, "page": _response_payload(response, raw=bool(args.raw))}


def run_course_order(args: argparse.Namespace, client: TeachingClient) -> dict[str, object]:
    if not args.yes:
        raise CsustError("调整课程顺序会改变远端状态，请加 --yes", code="confirmation_required")
    action = "LESSUP" if args.direction == "up" else "LESSDOWN"
    response = client.request_web(
        "/meol/lesson/blen.student.lesson.list.jsp",
        params=[("ACTION", action), ("lid", args.course_id)],
        mutating=True,
    )
    payload = _response_payload(response, mutating=True)
    return {**result_status(payload, mutating=True), "page": payload}


def run_service(_args: argparse.Namespace, client: TeachingClient) -> dict[str, object]:
    service = client.ensure_service()
    return {"ok": True, "service": service}


def run_catalog(_args: argparse.Namespace, _client: Client | None = None) -> dict[str, object]:
    return {
        "service": TEACHING_SERVICE_NAME,
        "routes": list(TEACHING_ROUTE_CATALOG),
        "actions": list(TEACHING_ACTION_CATALOG),
        "apis": list(_TEACHING_API_ROWS),
        "route_count": len(TEACHING_ROUTE_CATALOG),
        "action_count": len(TEACHING_ACTION_CATALOG),
        "api_count": len(_TEACHING_API_ROWS),
        "request": "csust teaching request --name NAME --param NAME=VALUE --data NAME=VALUE --yes",
    }


def _add_json(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")


def _request_args(parser: argparse.ArgumentParser, *, get_only: bool = False) -> None:
    parser.add_argument("--name", help="目录项名称；可来自 catalog")
    parser.add_argument("--path", help="教学平台同源路径")
    if not get_only:
        parser.add_argument("--method", default="GET", help="GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS")
        parser.add_argument("--data", action="append", default=[], help="表单字段 NAME=VALUE，可重复")
        parser.add_argument("--data-json", help="JSON 请求体；可用 @FILE 或 -")
        parser.add_argument("--file", action="append", default=[], help="multipart 文件字段 NAME=PATH，可重复")
        parser.add_argument("--yes", action="store_true", help="确认执行可能改变远端状态的请求")
        parser.add_argument("--referer", default="", help="同源 Referer")
    else:
        parser.set_defaults(method="GET", data=[], data_json=None, file=[], yes=False, referer="")
    parser.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    parser.add_argument("--output", help="原样保存响应")
    parser.add_argument("--raw", action="store_true", help="在结构化页面外附带原始正文")
    _add_json(parser)


def register(subparsers: argparse._SubParsersAction) -> None:
    teaching = subparsers.add_parser("teaching", aliases=["theol"], help="通过 VPN 统一认证访问网络教学平台")
    children = teaching.add_subparsers(dest="teaching_command", required=True)

    catalog = children.add_parser("catalog", help="列出网页、控件动作和 SPA/API 完整目录")
    _add_json(catalog)
    catalog.set_defaults(feature_runner=run_catalog, feature_renderer=render)

    service = children.add_parser("service", help="解析当前 VPN 会话中的网络教学平台服务")
    _add_json(service)
    service.set_defaults(feature_runner=run_service, feature_renderer=render)

    request = children.add_parser("request", aliases=["api", "page"], help="调用任意教学平台页面或目录 API")
    _request_args(request)
    request.set_defaults(feature_runner=_run_request, feature_renderer=render)

    get = children.add_parser("get", help="GET 教学平台页面并输出结构化内容")
    _request_args(get, get_only=True)
    get.set_defaults(feature_runner=_run_request, feature_renderer=render)

    courses = children.add_parser("courses", help="列出、搜索全部课程")
    courses.add_argument("--name", help="课程名称/编号关键词")
    courses.add_argument("--tutor", help="主讲教师关键词")
    courses.add_argument("--raw", action="store_true", help="附带原始正文")
    _add_json(courses)
    courses.set_defaults(feature_runner=run_courses, feature_renderer=render)

    course = children.add_parser("course", help="读取课程首页或指定课程栏目")
    course.add_argument("--course-id", required=True)
    course.add_argument("--column-id", help="栏目或章节 columnId")
    course.add_argument("--raw", action="store_true", help="附带原始正文")
    _add_json(course)
    course.set_defaults(feature_runner=run_course, feature_renderer=render)

    order = children.add_parser("course-order", help="上移或下移课程")
    order.add_argument("--course-id", required=True)
    order.add_argument("--direction", choices=("up", "down"), required=True)
    order.add_argument("--yes", action="store_true", help="确认改变课程顺序")
    _add_json(order)
    order.set_defaults(feature_runner=run_course_order, feature_renderer=render)


def render(data: dict[str, object]) -> None:
    if "routes" in data and "apis" in data:
        print(f"教学平台：{data.get('service')}；页面 {data.get('route_count')}；动作 {data.get('action_count')}；API {data.get('api_count')}")
        return
    if "courses" in data:
        for item in data.get("courses", []):
            if isinstance(item, dict):
                print("\t".join(_safe_terminal_text(item.get(key, "")) for key in ("course_id", "course_number", "name", "department", "tutor")))
        return
    if data.get("downloaded"):
        print(f"已保存：{data.get('output')}（{data.get('bytes')} bytes）")
        return
    print(json.dumps(data, ensure_ascii=False))


if __name__ == "__main__":
    raise SystemExit("use csust teaching ...")
