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
  terms/semester-start        学期信息
  textbooks                   教材操作
  evaluation                  学生评价
  web/routes                  教务网页入口
  vpn                         VPN 门户与 API
  teaching                    网络教学平台
  quality                     教学质量保障系统
  site                        任意 csust.edu.cn 子域名

页面能力使用结构化快照；写操作需要 --yes，并返回 confirmed/evidence。
`)
}
