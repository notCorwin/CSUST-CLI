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
  exams/classrooms/selections 教务查询
  course-selection            选课中心/跨专业选修课程
  terms/semester-start        学期信息
  textbooks                   教材操作
  evaluation                  学生评价
  web/routes                  教务网页入口
  vpn                         VPN 门户与 API
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
