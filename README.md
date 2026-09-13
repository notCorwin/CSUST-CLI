# CSUST CLI

[![Tests](https://github.com/notCorwin/csust-cli/actions/workflows/tests.yml/badge.svg)](https://github.com/notCorwin/csust-cli/actions/workflows/tests.yml)

面向智能体的长沙理工大学服务 CLI。它把分散的教务、认证、VPN、服务大厅、图书馆、招生和其他校内系统整理成可组合的业务命令，并为脚本和智能体提供统一的 JSON 结果。

项目直接使用学校服务的 HTTP、CAS、JSON 和传统表单协议。它不是无头浏览器，也不是爬虫客户端：浏览器只用于开发阶段发现能力，正式运行不启动浏览器。

这份 README 只保留安装、入口和稳定约定。完整文档索引见 [`docs/README.md`](docs/README.md)；调用流程、认证续办、结果判定和重复提交边界见 [`docs/OPERATING_MODEL.md`](docs/OPERATING_MODEL.md)，设计取舍和维护流程见 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)。

## 能做什么

项目覆盖的重点是可验证的业务能力，而不是网页数量。当前能力大致包括：

- 教务和学生事务：课表、成绩、考试、选课、培养方案、学籍、申请、公告和消息。
- 认证与校内入口：统一认证、VPN、eHall、融合服务大厅及其网关业务。
- 校园服务：图书馆、校园网、校园地图、电子证明、实验预约、档案和财务查询。
- 教学、科研与就业：网络教学、教学质量、网络课程、继续教育、OnlineJudge、科研和招聘。
- 公开内容：官网、期刊、招生信息、培训资讯及其他已验证的公共目录。

服务列表会随上游页面、网络和权限变化。请用 `services catalog` 查看当前探测结果；目录中的不可达或未取得业务协议的入口会保留状态，但不会被当作已支持的业务能力。

## 快速开始

### 环境要求

- Go 1.27 或更高版本
- 能访问目标学校服务的网络环境；部分服务只能在校内网或相应 VPN 环境中访问
- 对需要登录的服务拥有对应账号、角色和验证码/二次认证条件

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

### 第一次调用

先看当前版本的命令入口和服务证据：

```bash
./csust --help
./csust services catalog --json
```

登录后从只读查询开始：

```bash
export CSUST_USERNAME='学号'
printf '%s\n' '密码' | ./csust login --auth sso --password-stdin --json
./csust schedule --json
./csust grades --term 2025-2026-1 --json
```

命令参数以业务概念为中心；服务端字段、隐藏令牌和内部路径不需要由调用者提供。具体参数以 `./csust <命令> --help` 为准，不以旧文档或网页按钮名称为准。

### 使用 `.env`

也可以在当前目录使用未提交的 `.env`：

```dotenv
username=学号
password=密码
```

`.env` 默认要求权限为 `600`，应保持在 `.gitignore` 中。只有确实需要兼容旧环境时才设置 `CSUST_ALLOW_INSECURE_ENV=1`；密码更推荐通过标准输入提供。

## 认证与本地状态

不同业务系统的账号、角色、Cookie 和 Token 不保证相互兼容。登录成功不代表其他服务也已登录；网络教学和教学质量等业务还可能依赖已建立的 VPN 会话。

验证码、短信、邮件、密保和 TFA 可能把一次登录拆成多步。命令返回 `captcha_required` 或 `pending` 时，按返回信息补充下一步，不要把待续状态当作失败或最终成功。CLI 不自动把 OCR 猜测当作认证结果。

会话和配置默认保存在 `~/.config/csust-cli/` 下；`CSUST_COOKIE_FILE`、`CSUST_ENV_FILE`、`CSUST_BASE_URL` 以及 `CSUST_VPN_*` 可用于受控环境、测试或独立会话。专用业务的其他凭据变量请以该命令的帮助为准。会话文件等同于凭据，不要提交、分享或放入问题报告。

## 输出与写操作

使用 `--json` 获取机器可读结果。常用公共字段如下：

```json
{
  "ok": true,
  "submitted": false,
  "confirmed": true,
  "evidence": "confirmed"
}
```

- `ok` 表示本次 CLI 流程是否完成；待续流程还要查看 `pending` 和 `next`。
- `submitted` 表示是否已经发出可能改变远端状态的请求，不表示写入成功。
- `confirmed` 表示是否取得服务成功信号或回读证据；写操作不要只看 `ok`。
- `evidence` 说明判定依据。出现 `mutation_unverified` 时先回读状态，不要自动重试。
- 会修改远端数据的业务操作需要显式 `--yes`；文件响应使用 `--output FILE`。

成功退出码为 `0`，参数、认证、网络、业务或未确认写操作通常以 `2` 退出。对于报名、预约、缴费、删除、撤回、审批和密码修改等操作，请按 [`docs/OPERATING_MODEL.md`](docs/OPERATING_MODEL.md) 的状态规则处理。

## 开发与贡献

项目的长期约定是：公共命令表达业务语义，协议差异留在 adapter；能通过协议完成的能力不退回浏览器自动化；写操作必须有明确、可验证的成功判定。新增能力前请先阅读：

- [`docs/README.md`](docs/README.md)：文档索引和信息源边界
- [`docs/OPERATING_MODEL.md`](docs/OPERATING_MODEL.md)：调用者和智能体的使用边界
- [`docs/DOMAIN_MODEL.md`](docs/DOMAIN_MODEL.md)：业务对象和统一字段边界
- [`docs/SERVICE_MAP.md`](docs/SERVICE_MAP.md)：按业务意图选择服务
- [`docs/RESULT_CONTRACT.md`](docs/RESULT_CONTRACT.md)：JSON 结果和错误处理
- [`docs/AUTHENTICATION.md`](docs/AUTHENTICATION.md)：认证、会话和人工续办
- [`docs/DISCOVERY.md`](docs/DISCOVERY.md)：服务发现和证据管理
- [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md)：贡献与验收
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)：设计原因、适配边界和服务变化排查顺序
- [`AGENTS.md`](AGENTS.md)：项目需求、探索优先级和完成标准

提交修改前运行：

```bash
go test ./...
go vet ./...
go build ./...
go run . --help
git diff --check
```

测试使用本地模拟服务，不需要真实账号或学校在线状态。问题反馈请附上脱敏后的命令、服务名、网络/登录前提、JSON 中的 `code` 与 `evidence`，不要附带密码、Token、Cookie、验证码原图或会话文件。

维护者：[@notCorwin](https://github.com/notCorwin)。

## 获取帮助

- 运行 `./csust --help` 查看当前版本的命令语法。
- 运行 `./csust services catalog --json` 查看当前服务入口及探测证据。
- 从 [`docs/README.md`](docs/README.md) 选择适合当前任务的专题文档。
- 阅读 [`docs/OPERATING_MODEL.md`](docs/OPERATING_MODEL.md) 处理认证、待续状态和写操作。
- 在 [GitHub Issues](https://github.com/notCorwin/csust-cli/issues) 报告问题或提出功能建议。
- 在 [GitHub Actions](https://github.com/notCorwin/csust-cli/actions) 查看自动化检查。
