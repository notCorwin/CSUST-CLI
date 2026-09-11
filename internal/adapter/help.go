package adapter

import "strings"

func nativeUsage(args []string) []byte {
	command := "csust"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command += " " + args[0]
	}
	return []byte(command + `：Go 原生协议 CLI

用法：csust <命令> [选项]

命令：
  login/logout                 教务会话
  schedule/grades/profile     教务查询
  graduation-conclusion       毕业结论查询
  graduation-info-check       毕业生信息核对
  exams/in-class-exams       考试安排与随堂考查询
  classrooms/selections      教室与选课查询
  course-selection            选课中心/跨专业选修课程
  training-plan               培养方案执行计划
  training-progress           培养方案完成情况
  teaching-calendar           学期教学周历
  deferred-exam-applications  缓考申请记录（按学期/课程/状态查询）
  deferred-exam-registration  缓考报名状态/记录查询
  exempt-exam-applications    免考申请记录（按学期/课程/考试方式查询）
  graduate-exam-registration  毕业生插考报名状态/记录查询
  grade-recognition-applications 成绩认定申请记录
  grade-confirmation           成绩确认时间和状态
  class-changes                调停课记录
  enrollment-proof-applications 在读证明申请记录
  enrollment-status-changes  学籍异动历史
  drop-course-applications    可退课程及退课审核状态
  student-status-changes      个人信息修改审核历史
  second-class-credits        第二课堂学分查询
  second-class-credit-applications 第二课堂学分申报记录
  second-class-credit-application --id ID 查看申报审核流程
  status-warnings             学籍预警查询
  announcements               已收公告与详情
  announcement --id ID        查看公告正文
  messages                    已收留言与详情
  message --id ID             查看留言正文
  message reply --id ID       回复留言（需要 --content 和 --yes）
  retake-courses               重修报名可报课程
  terms/semester-start        学期信息
  textbooks                   教材操作
  evaluation                  学生评价
  web/routes                  教务网页入口
  vpn                         VPN 门户、工作台/分组、申请、设备、资料、分享与 API
  teaching                    网络教学平台
  quality                     教学质量保障系统
  services                    已映射业务服务目录
  admission-notice            研究生录取通知书查询/打印
  journal                     期刊检索
  employment                  云就业信息
  onlinejudge                 OnlineJudge 题目/竞赛/提交
  party-exam                  党校课程与成绩
  archive                     学生/综合档案系统
  student-record              学籍档案预约
  staff-record                教工人事档案预约
  sunshine                    教育阳光服务诉求查询与短信验证
  ehall                       eHall 服务、身份、收藏、消息、邮箱、新闻、评价、服务周期与详情
  continuing-education        继续教育学生信息
  virtual-lab                公路交通虚拟实验中心
  library-center              图书馆个人中心
  graduate-admissions         研究生招生旧系统
  legacy-mail                 旧邮件改密入口
  security-admin              安全运维管理平台
  cms-admin                   内容后台
  cms-admin-legacy            旧内容后台
  site                        任意 csust.edu.cn 子域名

页面能力使用结构化快照；写操作需要 --yes，并返回 confirmed/evidence。
`)
}
