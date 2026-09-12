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
  personal-info               个人资料设置（更新需要 --yes）
  graduation-conclusion       毕业结论查询
  graduation-info-check       毕业生信息核对
  exams/in-class-exams       考试安排与随堂考查询
  classrooms/selections      教室与选课查询
  course-selection            选课中心/跨专业选修课程
  special-course-query        特殊选课申请查询
  social-exam-registration    社会考试报名状态与可报名项目
  make-up-exam-registration   补考报名状态与可报名课程
  summer-remedial-registration 暑期补修报名状态与课程
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
  vpn                         VPN 门户、工作台/分组、申请、设备、资料、分享、认证与 API
  teaching                    网络教学平台
  quality                     教学质量保障系统
  services                    已映射业务服务目录
  admission-notice            研究生录取通知书查询/打印
  undergraduate-admissions    本科招生计划、历年分数、录取进程和结果查询
  union                       智慧工会模块、角色登录和会话
  journal                     期刊检索
  employment                  云就业信息
  onlinejudge                 OnlineJudge 题目/竞赛/提交
  mooc                        本校网络课程目录、院系筛选和分页查询
  quality-system              教学质量保障系统配置、登录、听评课和教学质量汇总查询
  party-exam                  党校课程与成绩
  archive                     学生/综合档案系统
  student-record              学籍档案预约
  staff-record                教工人事档案预约
  sunshine                    教育阳光服务诉求提交/查询与短信验证
  equipment                   实验室仪器列表、筛选和详情
  recruitment                 人才招聘频道、公告、岗位筛选和详情
  professional-learning       专业技术人员继续教育课程、分类、通知和详情
  institutional-learning      事业单位工作人员继续教育课程、分类、通知和详情
  transport-mobile            交通运输工程综合信息登录、身份、待办、字典、答辩、财务及学院业务查询
  electronic-documents         电子成绩单与在校证明登录、文件类型、申请记录和申请/下载
  campus-network              校园网自助服务资料、账单、详单、缴费、套餐和设备
  student-digital-archive     学生数字档案个人资料、学业、借阅、消费、上网和随手记
  research                    科研管理系统角色登录、验证码和会话
  transport-info              交通学院综合信息服务登录、验证码和会话
  transport-lab               实验室预约用户/教职工登录、注册、找回密码和会话
  continuing-platform         继续教育信息平台三类用户登录和会话
  ehall                       eHall 服务、身份、收藏、消息、邮箱、新闻、评价、服务项收藏、周期与详情
  continuing-education        继续教育学生信息
  virtual-lab                公路交通虚拟实验中心
  library-center              图书馆个人资料、信用记录、联系方式、密码、空间/座位资源和预约
  library                     图书馆馆藏/书目、读者资料、借阅、预约、权限和规则
  library-remote              图书馆远程数据库导航、筛选和资源详情
  campus-map                  校园地图、校区、公共点、地点搜索和全景漫游
  graduate-admissions         研究生招生旧系统
  legacy-mail                 旧邮件改密入口
  security-admin              安全运维管理平台
  cms-admin                   内容后台
  cms-admin-legacy            旧内容后台
  site                        任意 csust.edu.cn 子域名

页面能力使用结构化快照；写操作需要 --yes，并返回 confirmed/evidence。
`)
}
