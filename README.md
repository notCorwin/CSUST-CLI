# CSUST CLI

[![Tests](https://github.com/notCorwin/csust-cli/actions/workflows/tests.yml/badge.svg)](https://github.com/notCorwin/csust-cli/actions/workflows/tests.yml)

面向智能体的长沙理工大学服务 CLI。项目使用 Go 直接调用学校服务的 HTTP、CAS、JSON 和传统表单协议，把常用网页能力转换为语义化命令，并为脚本和智能体提供统一的 JSON 结果。

项目不是无头浏览器，也不是爬虫客户端。`site discover` 和 `site scripts` 只用于开发阶段的能力发现；正式命令直接通过 adapter 调用服务协议。

## 能做什么

- 教务：课表、成绩、个人信息、考试、空教室、选课结果、培养方案、第二课堂学分、学期信息、教材和教学评价。
- 学籍：学籍预警、学籍卡片及毕业相关页面的结构化入口。
- 公告：已收公告列表及详情入口。
- 考试报名：重修报名可报课程及资格状态。
- 网页映射：教务菜单、页面快照、表单和动作调用、公开入口及毕业设计跳转。
- VPN：登录、状态、退出、工作台应用/分组、消息、审批、页面/控件/API 目录、文件资源和已映射 API 调用。
- 网络教学与教学质量保障：课程、课程详情、页面请求、教学评价和页面目录。
- eHall：当前账号可用服务目录、服务详情、权限和统一跳转入口。
- 其他业务服务：录取通知书、期刊、云就业、OnlineJudge、党校考试、学生/教工档案、教育阳光服务、继续教育、虚拟实验中心、图书馆个人中心、研究生招生、旧邮件及后台入口。
- 全站适配：对 `csust.edu.cn` 根域名和子域名提供结构化页面、表单、动作和通用请求能力。

业务命令使用语义参数；需要保留网页特有能力时，再使用 `web`、`teaching`、`quality`、`vpn` 或 `site` 的通用映射命令。服务目录和页面/API 目录可通过 CLI 自身查看，不在 README 中复制易变的端点清单。

## 快速开始

### 环境要求

- Go 1.27 或更高版本
- 能访问目标学校服务的网络环境；部分内部地址只能在校内网或相应 VPN 环境中访问
- 需要登录的功能使用学校账号，部分业务服务使用独立账号或验证码

### 构建

```bash
git clone https://github.com/notCorwin/csust-cli.git
cd csust-cli

go build -o csust .
./csust --help
```

开发时也可以直接运行：

```bash
go run . --help
```

### 配置账号

最简单的方式是设置环境变量；密码优先使用标准输入，避免进入 shell 历史：

```bash
export CSUST_USERNAME='学号'
printf '%s\n' '密码' | ./csust login --auth sso --password-stdin --json
```

也可以在当前目录使用未提交的 `.env`：

```dotenv
username=学号
password=密码
```

`.env` 默认要求权限为 `600`；只有确实需要兼容旧环境时才设置 `CSUST_ALLOW_INSECURE_ENV=1`。推荐把 `.env` 保持在 `.gitignore` 中，并优先使用 `--password-stdin`。

登录后即可执行查询：

```bash
./csust schedule --json
./csust grades --term 2025-2026-1 --json
./csust grades detail --term 2025-2026-1 --course-name 课程名称 --json
./csust profile --json
./csust graduation-conclusion --json
./csust exams --json
./csust course-selection --scope cross-major --json
./csust training-plan --keyword 专业核心 --json
./csust training-progress --json
./csust deferred-exam-applications --term 2025-2026-1 --status approved --json
./csust second-class-credits --json
./csust second-class-credit-applications --json
./csust second-class-credit-application --id APPLICATION_ID --json
./csust status-warnings --json
./csust announcements --json
./csust announcement --id ANNOUNCEMENT_ID --json
./csust messages --json
./csust message --id MESSAGE_ID --json
./csust message reply --id MESSAGE_ID --content 回复内容 --yes --json
./csust retake-courses --json
```

## 常用命令

| 命令 | 用途 |
| --- | --- |
| `login` / `logout` | 教务统一认证或旧登录会话 |
| `schedule`, `grades`, `profile`, `exams` | 教务查询；成绩包含学分/绩点汇总和成绩构成详情 |
| `graduation-conclusion` | 毕业结论、学位结论和学生基本信息 |
| `classrooms`, `selections`, `course-selection`, `terms`, `semester-start` | 教室、选课和学期信息；跨专业选修使用 `--scope cross-major` |
| `training-plan` | 培养方案执行计划课程 |
| `training-progress` | 培养方案课程完成情况和学分汇总 |
| `deferred-exam-applications` | 按学期、缓考活动、课程和审核状态查询缓考申请记录 |
| `second-class-credits` | 第二课堂学分认定查询 |
| `second-class-credit-applications` | 第二课堂学分申报及审核状态，包含 `application_id` 和流程详情路径 |
| `second-class-credit-application --id` | 查看申报项目获得时间、审核历史和认定历史 |
| `status-warnings` | 学籍预警及处理结果 |
| `announcements` | 已收公告及详情路径 |
| `announcement --id` | 查看单条公告正文 |
| `messages` | 已收留言及详情路径 |
| `message --id` | 查看单条留言正文 |
| `message reply --id --content --yes` | 回复单条留言，并验证服务端成功反馈 |
| `retake-courses` | 重修报名可报课程、资格和缴费状态 |
| `textbooks` | 教材列表、账目和选订/退订 |
| `staff-record` | 教职工人事档案预约（个人/单位）及介绍信上传 |
| `sunshine` | 教育阳光服务公开诉求、详情、部门、统计和短信验证 |
| `evaluation` | 学生评价批次、课程和保存/提交 |
| `web` / `routes` | 教务页面目录、快照、表单和动作 |
| `vpn` | VPN 门户、工作台/分组、申请、设备、会话、目录和 API |
| `teaching` | 网络教学平台页面和课程 |
| `quality` | 教学质量保障系统及评价 |
| `ehall` | eHall 当前可用服务及服务详情 |
| `services` | 已映射业务服务及依据 |
| `site` | 任意官方子域名的通用适配器 |

更多业务命令可先查看目录：

```bash
./csust services catalog --json
./csust web catalog --json
./csust vpn catalog --json
./csust site catalog --json
```

一些完整用法示例：

```bash
# 教务页面快照与结构化动作
./csust web get --name course-selection-center --json
./csust web semester-timetable --output timetable.html --json

# 教材写操作必须显式确认，成功还会回读验证
./csust textbooks list --json
./csust textbooks subscribe --index 1 --yes --json
./csust textbooks unsubscribe --index 1 --yes --json

# 教职工人事档案预约；单位预约的介绍信可在提交前自动上传
./csust staff-record form --kind personal --json
./csust staff-record request --kind personal --subject-name 姓名 --birth-date 1980-01-02 --employee-id 工号 --subject-unit 单位 --applicant-name 姓名 --phone 手机 --usage 查阅 --reason 业务办理 --appointment-date 2026-09-15 --captcha 验证码 --yes --json

# VPN 和网络教学
./csust vpn login --auth cas --password-stdin --json
./csust vpn status --json
./csust vpn apps --tab all --json
./csust vpn groups --json
./csust vpn groups create --name 常用 --yes --json
./csust vpn messages --type approve --read-status unread --json
./csust vpn approvals --view pending --json
./csust vpn devices --json
./csust vpn apply list --search 教务 --json
./csust vpn apply request --service-id SERVICE_ID --service-name 服务名 --reason 申请原因 --start "2026-09-15 09:00" --end "2026-09-16 18:00" --yes --json
./csust vpn apply cancel-account --reason 注销原因 --yes --json
./csust vpn shares --view received --search 文件名 --json
./csust vpn links --search 文件名 --json
./csust vpn api --name users-info --json
./csust teaching courses --json
./csust quality status --json

# eHall 的当前 SSO 回调由 adapter 处理，成功后再访问门户
./csust site login --service ehall --auth sso --password-stdin --json
./csust site get --service ehall --path /index.html --require-login --json
./csust ehall services --json
./csust ehall service --id SERVICE_ID --json
./csust ehall health --id SERVICE_ID --json

./csust ehall me --json
./csust ehall favorites --json
./csust ehall message-count --json
./csust ehall notifications --json
./csust ehall favorite add --service-id SERVICE_ID --yes --json
./csust ehall favorite remove --service-id SERVICE_ID --yes --json

# 无密码认证：扫码，或先发送动态码再登录
./csust site login --service ehall --auth qr --qr-image ./ehall-qr.png --json
./csust site login --service ehall --auth dynamic --mobile 手机号 --send-code --yes --json
./csust site login --service ehall --auth dynamic --mobile 手机号 --dynamic-code 动态码 --captcha 验证码 --json

# 语义化业务服务
./csust journal search --journal transport --query 软岩 --page-size 20 --json
./csust employment list --kind career --json
./csust onlinejudge problems --limit 20 --json
./csust sunshine issues --status 受理中 --json
./csust sunshine stats --json
# 发送诉求短信验证码是远端写操作，需要显式确认
./csust sunshine send-code --phone 手机号 --yes --json

# 全站通用页面与请求；写请求需要 --yes
./csust site get --service official --path / --json
./csust site request --service official --path / --method GET --json
```

开发阶段的发现命令必须显式开启：

```bash
CSUST_EXPLORATION=1 ./csust site discover --service official --path / --depth 1 --json
CSUST_EXPLORATION=1 ./csust site scripts --service map --path / --json
```

## 认证与会话

会话文件由 adapter 管理并尽量以 `600` 权限保存。默认位置如下：

- 教务及 `web`：`~/.config/csust-cli/cookies.txt`
- `site` 和多数业务服务：`~/.config/csust-cli/sites/<host>.cookies.txt`
- VPN：`~/.config/csust-cli/vpn-cookies.txt` 和 `~/.config/csust-cli/vpn-session.json`
- `teaching` 和 `quality`：复用 VPN 的认证会话，并动态解析服务网关

可用以下方式覆盖默认配置：

- `CSUST_COOKIE_FILE`：教务/通用站点的 Cookie 文件
- `CSUST_BASE_URL`：站点适配器的基地址，适合测试或受控环境
- `CSUST_ENV_FILE`：替代默认 `.env` 文件
- `CSUST_VPN_BASE_URL`、`CSUST_VPN_COOKIE_FILE`、`CSUST_VPN_SESSION_FILE`：VPN 会话配置
- 支持 `--cookie-file` 的业务命令可使用独立会话文件

验证码不会自动依赖 Python 或 OCR。命令会保存验证码图片并返回 `captcha_required`，随后使用 `--captcha` 重试；可用 `--captcha-image` 指定图片位置。不同业务的密码环境变量也不同，命令缺少密码时会明确提示所需变量或 `--password-stdin`。

## 输出与写操作

使用 `--json` 获取机器可读结果；它可以放在命令的任意位置。成功结果和错误结果都遵循统一字段：

```json
{
  "ok": true,
  "submitted": false,
  "confirmed": true,
  "evidence": "confirmed"
}
```

- `submitted` 表示是否已经发送写请求，不等于服务端处理成功。
- `confirmed` 表示是否取得了业务成功证据；只有得到响应信号或回读验证，写操作才会报告确认成功。
- `evidence` 说明判定依据；低置信度页面会同时提供具体 `confidence_evidence`。
- 可能修改远端状态的命令需要显式 `--yes`。无法确认的写请求不会自动重试，应先查询状态再决定是否重试。
- 二进制响应使用 `--output FILE` 保存；输出文件采用临时文件加原子替换，失败时不会覆盖已有目标。
- 成功退出码为 `0`；参数、认证、网络、业务或未确认写操作退出码为 `2`。

## 开发与贡献

代码按适配边界组织：

- [`main.go`](main.go)：Go CLI 入口、全局 JSON 处理和退出码
- [`internal/adapter`](internal/adapter)：教务、VPN、网关、业务服务和全站协议适配器
- [`internal/contract`](internal/contract)：统一结果模型和置信度/写操作判定
- [`AGENTS.md`](AGENTS.md)：项目需求、架构边界和功能完成标准
- [`.github/workflows/tests.yml`](.github/workflows/tests.yml)：持续集成检查

提交修改前运行完整本地检查：

```bash
go test ./...
go vet ./...
go build ./...
go run . --help
```

测试使用本地 HTTP 测试服务和模拟响应，不需要提交学校账户数据。请在 Issue 中说明复现命令和服务范围；Pull Request 应保持 adapter 与业务命令边界清晰，并同步更新面向用户的命令说明。

维护者：[@notCorwin](https://github.com/notCorwin)。

## 获取帮助

- 首先运行 `./csust --help`，再使用 `services catalog`、`web catalog`、`vpn catalog` 或 `site catalog` 查看当前能力目录。
- 阅读 [`AGENTS.md`](AGENTS.md) 了解项目约束和验证要求。
- 报告问题或提交功能建议：[GitHub Issues](https://github.com/notCorwin/csust-cli/issues)。
- 查看自动化检查：[GitHub Actions](https://github.com/notCorwin/csust-cli/actions)。

学校页面、接口和认证流程可能变化；目录表示已映射能力，不代表每个端点都在当前网络环境中完成线上验收。涉及账号写入的操作请始终检查 `confirmed` 和 `evidence`。
