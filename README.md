# csust-cli

面向智能体的长沙理工大学服务 CLI，Go 是正式入口；教务、VPN、网络教学和质量保障能力继续由现有协议适配器承载，`site request` 由 Go 原生 HTTP 适配器直接调用服务协议。教务系统命令仍访问旧的 `xk.csust.edu.cn`，登录默认优先通过 `authserver.csust.edu.cn` 统一身份认证；`vpn` 命令对应当前 `vpn.csust.edu.cn` 的 EnUES Vue SPA。

仓库运行时仅保留 Go 实现，不再提供 Python 入口、安装脚本或双运行时回退。

开发阶段的站点探索命令需要显式设置 `CSUST_EXPLORATION=1`；正式业务命令不执行爬取。

```bash
CSUST_USERNAME=学号 CSUST_PASSWORD=密码 go run . login --auth sso --json
go run . schedule --json
go run . grades --term 2025-2026-1 --json
go run . profile --json
go run . exams --json
go run . classrooms --campus yuntang --week 1 --weekday 1 --section 1 --json
go run . selections --term 2025-2026-1 --json
go run . textbooks list --json
go run . textbooks account --json
go run . textbooks subscribe --index 1 --yes --json
go run . textbooks unsubscribe --index 1 --yes --json
go run . evaluation batches --json
go run . terms --scope schedule --json
go run . semester-start --json
go run . routes --json
go run . web get --path /jsxsd/xsxk/xklc_list --json
go run . web get --name course-selection-center --json
go run . web action --path /jsxsd/xsxk/xklc_list --ref action:... --yes --json
go run . web catalog
go run . web course-selection-center --json
go run . web course-selection-center --form 1 --data xnxq01id=2026-2027-1 --yes --json
go run . web semester-timetable --output timetable.html --json
go run . web public get --path /findmm.jsp --json
go run . web public get --path /css/images/codeFrame.png --output csust-app-qr.png --json
go run . web graduation-design --json

# 任意 csust.edu.cn 子域名：页面快照、表单/动作和通用接口；探索命令仅限开发阶段
go run . site catalog --json
CSUST_EXPLORATION=1 go run . site discover --service official --path / --depth 1 --json
go run . site get --service sunshine --path / --json
go run . site get --service legacy-host.csust.edu.cn --scheme http --path / --json
CSUST_EXPLORATION=1 go run . site scripts --service map --path / --json
go run . site form --service mail --path / --form 1 --data name=value --yes --json
go run . site action --service sunshine --path / --ref action:... --yes --json
go run . site request --service sunshine --path /api \
  --method POST --data-json @request.json --yes --json
go run . site login --service ehall --path / --auth sso --json

# VPN 门户：完整路由、控件和 bundle API 清单
go run . vpn routes --json
go run . vpn controls --json
go run . vpn catalog --json
go run . vpn login --auth cas --json
go run . vpn status --json
go run . vpn api --name users-info --json
go run . vpn api --name users-message-page --data-json '{"pageNum":1,"pageSize":20}' --json
go run . vpn api --name client-public-files-download-filepath --param filePath=/path/to/file --output download.bin --json
go run . vpn api --path /api/new-endpoint --method POST --data-json @request.json --yes --json
go run . vpn api --name users-center-uploadPicture --file file=avatar.png --yes --json

# 网络教学平台：沿用 VPN 的 CAS 统一认证会话
go run . teaching catalog --json
go run . teaching service --json
go run . teaching courses --json
go run . teaching course --course-id 61184 --json
go run . teaching get --name personal --json
go run . teaching request --name homework-stu-list \
  --param courseId=61184 --param title= --param pagingNumberPer=30 --param um=307941 --json
go run . teaching request --name homework-stu-submit-do \
  --data courseId=61184 --data um=307941 --data hwtId=105915 \
  --data answer='<html>...</html>' --yes --json
```

## 业务服务适配器

`services catalog` 返回已确认的业务服务、实际主机、能力类型和 `confidence_evidence`。业务命令使用语义参数；站点路径和接口细节只存在于 Go adapter 中，结果统一包含 `ok`、`submitted`、`confirmed` 和 `evidence`。

```bash
go run . services catalog --json
go run . admission-notice query --id-card 身份证号 --password-stdin --json
go run . admission-notice print --id-card 身份证号 --password-stdin --output admission.pdf --json
go run . journal search --journal transport --query 软岩 --page-size 20 --json
go run . journal article --journal highways --id 20250104 --json
go run . employment list --kind career --json
go run . employment detail --kind career --id 714397 --json
go run . onlinejudge problems --limit 20 --insecure --json
go run . onlinejudge submit --problem-id 1000 --language C++ --code @solution.cpp --yes --insecure --json
go run . party-exam scores --cookie-file exam.cookies.txt --json
go run . student-record form --json
go run . student-record upload --field photo --file photo.jpg --yes --json
go run . continuing-education status --json
go run . graduate-admissions login --username 准考证号 --password-stdin --captcha 验证码 --json
go run . archive status --system student --json
go run . archive login --system student --username 账号 --password-stdin --captcha 验证码 --json
go run . archive report --system student --report-code CODE --filter student_id=学号 --json
go run . cms-admin login --scope website --username 管理员 --password-stdin --captcha 验证码 --json
go run . security-admin login --username 用户名 --password-stdin --insecure --json
go run . virtual-lab status --json
go run . library-center status --json
```

已覆盖研究生录取通知书、交通科学与工程/公路与汽运期刊、OnlineJudge、云就业、档案预约、继续教育、党校考试、两个档案系统、虚拟实验中心、图书馆 CAS 个人中心及旧招生/邮件/后台入口。需要登录的系统使用各自 `--cookie-file`；统一认证可用 `library-center login` 或 `legacy-mail login`，档案系统登录令牌由 `archive login` 保存并由后续 `archive` 命令自动读取。OnlineJudge 的过期证书仅在显式 `--insecure` 时接受。

## VPN 映射与调用

`csust vpn routes` 固化门户当前 SPA、条件跳转和兼容/native 入口的 115 条可寻址路径及页面级 API 映射；`csust vpn controls` 固化登录方式、设备注册、协议/授权弹窗、门户 shell 菜单、工作台标签/搜索/排序、应用卡片菜单、申请资源下拉、审批/消息/安全中心/文件分享/用户中心等 77 类控件和弹窗映射；`csust vpn catalog` 固化当前前端 bundle 提取出的全部 249 个 API（含 GET/POST、动态路径、查询参数、JSON/multipart 请求、是否可能修改远端状态），并附带图片、文件、协议资源映射。

`csust vpn page --route work-bench --json` 查看单页映射。`csust vpn api --name NAME` 按目录调用；`--data-json` 支持 JSON、`@FILE` 或标准输入 `-`，`--form NAME=VALUE` 和 `--file NAME=PATH` 支持门户 multipart 表单/头像上传，`--param NAME=VALUE` 支持筛选/分页/查询参数，`--path-arg VALUE` 填充动态路径，`--output FILE` 原样保存下载/导出响应。未固化的新同源接口可直接使用 `--path`，因此不受静态目录限制；`--native` 调用 bundle 中的 `127.0.0.1:30303` EnUES 本机接口。

VPN 会话保存在 `~/.config/csust-cli/vpn-session.json`，Cookie 保存在 `~/.config/csust-cli/vpn-cookies.txt`，均为私有文件。账号优先从 `CSUST_USERNAME`、`CSUST_PASSWORD` 读取，否则读取当前目录 `.env` 的 `username`、`password`；也可用 `--password-stdin`。`vpn login` 默认按门户配置选择认证方式：当前门户隐藏本地账号登录时自动走 CAS 统一身份认证，完成回调后保存 Cookie/Token 会话；可用 `--auth cas` 强制 CAS，或用 `--auth local` 强制旧的本地 API 登录。登录后的 MFA、设备注册、审批、找回密码、二维码/第三方登录等条件流程均通过对应目录 API 原样开放。

## 网络教学平台映射

`csust teaching catalog` 固化 Chrome 实际可见的网络教学平台页面、课程菜单、弹窗、表单动作、资源中心和作业 SPA/API 清单。`teaching request` 支持同源 HTML 页面、传统表单、JSON/multipart 请求、文件上传、下载/导出和分页/筛选参数；`get` 输出页面的链接、表单、控件、选项、表格和脚本端点。平台网关前缀每次从 VPN 的“网络教学平台”服务动态解析，避免把当前会话的服务令牌写进配置。写请求仍需显式 `--yes`。

## 教学质量保障系统映射

`csust quality` 通过 VPN 的 CAS 统一认证会话进入“教学一体化”网关，再登录旧版教学质量保障门户；不会把网关前缀或服务令牌固化到配置。`quality catalog` 映射网页端当前可见的 69 个学生页面，`quality routes` 读取登录后的实时菜单，`quality get` 输出页面中的表单、控件、表格、动作和脚本端点。未固化的网页路径或接口可直接用 `quality request --path` 调用，因此页面清单不会限制实际能力覆盖。

```bash
go run . quality login --vpn-captcha-info '{"captcha":"CODE"}' --json
go run . quality status --json
go run . quality catalog --json
go run . quality routes --json
go run . quality graduation-design --json
go run . quality public get --path /findmm.jsp --json
go run . quality get --name student-evaluation --json
go run . quality form --path /jsxsd/... --form 1 --data NAME=VALUE --yes --json
go run . quality action --path /jsxsd/... --ref action:... --data NAME=VALUE --yes --json
go run . quality evaluation batches --json
go run . quality evaluation courses --path PATH --json
go run . quality evaluation form --path PATH --json
go run . quality request --path /jsxsd/... --method POST \
  --data NAME=VALUE --yes --json
```

条件显示的“毕业设计”入口通过 `quality graduation-design` 解析并校验外部 SSO；`quality form` 和 `quality action` 直接复用网页端表单/动作解析，会自动带上隐藏字段并解析页面脚本状态，动作可用稳定 `--ref` 或旧的 `--index --fingerprint`。评价的 `courses`、`form`、`save`、`submit` 会直接复用网页结构解析。保存和提交会校验隐藏字段、题目及选项，并在响应无法确认成功时返回未验证。所有通用写请求均需显式 `--yes`。

## Web 映射

`csust web catalog` 和 `ROUTE_CATALOG` 是同一份可检查清单；每一行的 `command` 就是对应 CLI 命令 `csust web <command>`，默认 GET 页面。该命令还支持：

- `--name NAME`：按登录后主页的实时菜单文本或稳定命令名发现页面；与 `--path` 互斥。
- `--param NAME=VALUE`：初始查询参数。
- `--form N [--button M] --data NAME=VALUE`：按页面表单和按钮提交，自动带上未修改的隐藏字段。
- `--action N --data NAME=VALUE`：按结构化页面输出中的动作序号执行链接、弹窗、表单或页面 JavaScript 中可解析的 URL 动作。
- `web action --ref REF`：按页面动作的语义引用重新定位动作，不依赖页面动作顺序；`--index` 仍可用，但必须配合 `--fingerprint`。
- `--output FILE`：原样保存打印、导出和下载响应；`web request` 另外支持 GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS。

`web get` 返回版本化页面快照，包含 `kind`、`capabilities`、`shape_fingerprint`、表格 `headers/data_rows`、控件 `label` 和动作 `ref`。HTML 页面优先使用直接 HTTP(S) 解析；只有脚本空壳而没有静态表单/链接/表格时标记为 `dynamic`，同时保留脚本和可发现接口，未知接口仍可通过 `web request` 显式调用。

当前清单按网站二级菜单归组：教学评价 1；我的申请 3；我的考试 5；成绩管理 3；培养方案 5；我的课表 8；选课管理 9；教材管理 2；辅修管理 1；实验教学 2；第二课堂学分 2；学科竞赛 1；创新创业 5；公告留言 3；个人信息 2；在线问答 1；教学周历 1；学籍管理 8；我的成绩 4；毕业管理 3，共 69 条。`routes --json` 还返回登录、忘记密码、验证码、APP 登录页切换和条件显示的毕业设计外部 SSO 入口。

## 全站通用映射

`csust site` 面向 `csust.edu.cn` 根域名和全部子域名开放，不把静态入口清单当作能力边界。`site catalog` 是已观察到的服务起点；开发阶段设置 `CSUST_EXPLORATION=1` 后，`site discover` 才从实时页面抓取链接、表单、动作、脚本端点并报告新出现的官方子域名，`site scripts` 才读取同源 SPA 脚本并提取常见 API/页面端点。`site get` 返回与 `web get` 相同的结构化页面快照，`site form`、`site action` 复用网页结构解析；`site request` 通过 Go 原生 HTTP 适配器支持 GET/POST/PUT/PATCH/DELETE、JSON、表单、multipart 和文件下载，并对写请求返回明确的确认或未验证结果。原始 HTML 响应使用 `http-response-v1` 合约并附带低置信度依据；需要页面结构时使用 `site get`。每个子域名使用独立 Cookie 文件；`site login` 使用统一身份认证建立该页面的 SSO 会话。

已观察服务使用目录内的传输方案；新服务默认 HTTPS，旧的 HTTP 子域名可显式加 `--scheme http`。默认只执行当前子域名动作；若网页表单明确把登录/提交目标放到外部 HTTP(S) 服务（例如邮箱门户），可在 `site form`/`site action` 加 `--allow-external`，仍会拒绝脚本、邮件协议、目录跳转和 HTTPS 降级。页面明确跳转到其他官方子域名时，使用对应服务名或官方主机名，再配合服务内 `--path` 调用。

账号密码优先从 `CSUST_USERNAME`、`CSUST_PASSWORD` 读取；未设置时读取当前目录 `.env` 中的 `username`、`password`，不会交互询问或写入密码。`.env` 应保持 `600` 权限；权限过宽时默认拒绝读取，如需兼容旧环境可显式设置 `CSUST_ALLOW_INSECURE_ENV=1`。`login` 的 `--auth auto` 在标准 `xk.csust.edu.cn` 地址优先走统一身份认证，网络层失败才回退旧的教务登录；可用 `--auth sso` 强制统一认证，或 `--auth local` 强制旧登录。统一认证需要验证码时，Go 适配器会保存验证码图片并返回 `captcha_required`，需人工提供 `--captcha CODE` 后重试；运行时不依赖 Python/OCR。

会话 Cookie 保存在 `~/.config/csust-cli/cookies.txt`，权限为 `600`；验证码图片保存在同目录，权限为 `600`。可用 `CSUST_BASE_URL` 和 `CSUST_COOKIE_FILE` 覆盖站点与会话文件路径。现站点使用 HTTP 时每个进程的首次请求会向 stderr 发出安全警告。

没有有效 Cookie 时，所有登录后查询和页面命令都会在凭据变量存在时自动登录。`routes` 会返回登录后主页清单；`web get` 或任一 69 个页面命令会输出结构化链接、表单、控件、选项、表格、动作和脚本函数。登录页/找回密码使用 `web public get|action|post`，条件显示的毕业设计使用 `web graduation-design`。

教材、评价和通用 POST/网页动作等可能修改账号数据的操作始终要求 `--yes`。通用写请求会依据响应中的成功/失败信号确认结果；无法确认时返回未验证，不会自动重试。教材选订/退订还要求单条精确目标，并在提交后重新查询验证。

构建 Go 入口：

```bash
go build -o csust .
./csust schedule --json
```

运行检查：

```bash
go test ./...
go vet ./...
```

## 结果判定与兼容变化

命令成功退出 `0`；参数、认证、网络、业务失败及写入结果未确认退出 `2`。
`--json` 可放在顶层命令前或最终子命令后，标准输出为一份 JSON；诊断信息写入 stderr。
原有命令、别名和目录保持可用，但调用方不能再依赖“HTTP 200 就退出 0”。

| 字段或错误码 | 含义 |
| --- | --- |
| `ok: true` | 本次任务成功；写操作必须取得业务成功证据 |
| `submitted` | 是否已发送写请求，不代表服务端已完成处理 |
| `confirmed` | 写操作是否取得成功反馈；只读操作表示请求通过结果检查 |
| `mutation_rejected` | 服务端明确拒绝写操作 |
| `mutation_unverified` | 已发送写请求，但没有足够证据确认；先查询状态，再决定是否重试 |
| `business_rejected` | 只读接口明确返回业务错误 |
| `parse_error` | 响应 JSON 无效，或页面动作表达式无法可靠解析 |

错误继续使用 `{"error":"说明","code":"错误码","details":{...}}`。
业务判定错误的 `details` 包含 `submitted`、`confirmed` 和 `evidence`（`rejected` 或 `unknown`）；有请求摘要时一并返回。
发生网络错误时不能据此断定写入未执行，CLI 不会因超时或断连自动重发写请求。
VPN 仅在已知的认证拒绝码 `3010` 下尝试一次令牌刷新及重发；教学服务发现最多恢复认证一次。

下载的文件保存与业务成功分别判定。未知写响应仍原样保存，但退出 `2`，并在错误 `details` 中给出 `downloaded`、`output` 和 `bytes`。
登录后下载遇到登录页，或下载遇到明确错误响应时，不会替换目标文件。通过 POST 返回文件本身不证明业务写入成功；目录明确标注的只读 POST 按只读请求处理。

教学与质量保障的 `--file` 可以与重复的 `--data` 字段同时使用，空字段也会保留；`--data-json` 与 `--data/--file` 互斥。
`--name` 与 `--path` 互斥。无法解析的脚本动作仍在页面结构中展示，并附 `parse_error`；可以用通用 `request` 命令显式提供路径和参数。
HTML 成功判定只使用直接执行的反馈语句或简短独立确认文本，不将历史表格、链接文字或未执行函数中的“成功”当作本次操作结果。

## 安装与验证

CLI 运行时只依赖 Go 和内置协议适配器：

```bash
go build -o csust .
go test ./...
go vet ./...
go run . --help
```

GitHub Actions 在每次 push 和 pull request 时执行 Go 测试、静态检查、构建与命令入口检查。
测试使用本地 HTTP 服务与模拟响应，不会提交学校账户数据。

验证记录（2026-09-09）：`go test ./...` 与 `go vet ./...` 通过；覆盖统一结果模型、业务 JSON 解包、敏感字段脱敏、脚本数据解析、页面快照、动作引用、二进制下载、会话刷新、URL 校验、通用 JSON 请求、外部页面动作目标和 CLI 退出码。已对期刊检索、就业信息、录取通知书接口、OnlineJudge 题目、党校会话、档案系统入口、继续教育、虚拟实验中心、图书馆 CAS 入口及指定旧系统进行协议级实测；写操作仅在获得远端成功状态或回读证据时报告 `confirmed: true`。
线上尝试了教务登录、课表、成绩、教材列表、VPN 登录与状态、教学课程及质量保障状态和菜单；本机代理下旧教务站返回 HTTP 502，VPN 连接失败，直连探测也未成功。因此本轮没有取得教务/VPN 的线上业务验收通过记录，也未执行线上业务写操作。
目录中的页面/API 数量表示已映射范围，不等同于每个端点已通过线上验证。
