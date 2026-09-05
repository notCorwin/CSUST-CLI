# csust-cli

面向智能体的长沙理工大学教务系统与 VPN 门户 CLI。教务系统命令继续使用旧的 `xk.csust.edu.cn`；`vpn` 命令对应当前 `vpn.csust.edu.cn` 的 EnUES Vue SPA。

```bash
CSUST_USERNAME=学号 CSUST_PASSWORD=密码 python3 csust.py login --json
python3 csust.py schedule --json
python3 csust.py grades --term 2025-2026-1 --json
python3 csust.py profile --json
python3 csust.py exams --json
python3 csust.py classrooms --campus yuntang --week 1 --weekday 1 --section 1 --json
python3 csust.py selections --term 2025-2026-1 --json
python3 csust.py textbooks list --json
python3 csust.py textbooks account --json
python3 csust.py textbooks subscribe --index 1 --yes --json
python3 csust.py textbooks unsubscribe --index 1 --yes --json
python3 csust.py evaluation batches --json
python3 csust.py terms --scope schedule --json
python3 csust.py semester-start --json
python3 csust.py routes --json
python3 csust.py web get --path /jsxsd/xsxk/xklc_list --json
python3 csust.py web action --path /jsxsd/xsxk/xklc_list --index 1 --yes --json
python3 csust.py web catalog
python3 csust.py web course-selection-center --json
python3 csust.py web course-selection-center --form 1 --data xnxq01id=2026-2027-1 --yes --json
python3 csust.py web semester-timetable --output timetable.html --json
python3 csust.py web public get --path /findmm.jsp --json
python3 csust.py web public get --path /css/images/codeFrame.png --output csust-app-qr.png --json
python3 csust.py web graduation-design --json

# VPN 门户：完整路由、控件和 bundle API 清单
python3 csust.py vpn routes --json
python3 csust.py vpn controls --json
python3 csust.py vpn catalog --json
python3 csust.py vpn login --auth cas --json
python3 csust.py vpn status --json
python3 csust.py vpn api --name users-info --json
python3 csust.py vpn api --name users-message-page --data-json '{"pageNum":1,"pageSize":20}' --json
python3 csust.py vpn api --name client-public-files-download-filepath --param filePath=/path/to/file --output download.bin --json
python3 csust.py vpn api --path /api/new-endpoint --method POST --data-json @request.json --yes --json
python3 csust.py vpn api --name users-center-uploadPicture --file file=avatar.png --yes --json

# 网络教学平台：沿用 VPN 的 CAS 统一认证会话
python3 csust.py teaching catalog --json
python3 csust.py teaching service --json
python3 csust.py teaching courses --json
python3 csust.py teaching course --course-id 61184 --json
python3 csust.py teaching get --name personal --json
python3 csust.py teaching request --name homework-stu-list \
  --param courseId=61184 --param title= --param pagingNumberPer=30 --param um=307941 --json
python3 csust.py teaching request --name homework-stu-submit-do \
  --data courseId=61184 --data um=307941 --data hwtId=105915 \
  --data answer='<html>...</html>' --yes --json
```

## VPN 映射与调用

`csust vpn routes` 固化门户当前 SPA、条件跳转和兼容/native 入口的 115 条可寻址路径及页面级 API 映射；`csust vpn controls` 固化登录方式、设备注册、协议/授权弹窗、门户 shell 菜单、工作台标签/搜索/排序、应用卡片菜单、申请资源下拉、审批/消息/安全中心/文件分享/用户中心等 77 类控件和弹窗映射；`csust vpn catalog` 固化当前前端 bundle 提取出的全部 249 个 API（含 GET/POST、动态路径、查询参数、JSON/multipart 请求、是否可能修改远端状态），并附带图片、文件、协议资源映射。

`csust vpn page --route work-bench --json` 查看单页映射。`csust vpn api --name NAME` 按目录调用；`--data-json` 支持 JSON、`@FILE` 或标准输入 `-`，`--form NAME=VALUE` 和 `--file NAME=PATH` 支持门户 multipart 表单/头像上传，`--param NAME=VALUE` 支持筛选/分页/查询参数，`--path-arg VALUE` 填充动态路径，`--output FILE` 原样保存下载/导出响应。未固化的新同源接口可直接使用 `--path`，因此不受静态目录限制；`--native` 调用 bundle 中的 `127.0.0.1:30303` EnUES 本机接口。

VPN 会话保存在 `~/.config/csust-cli/vpn-session.json`，Cookie 保存在 `~/.config/csust-cli/vpn-cookies.txt`，均为私有文件。账号优先从 `CSUST_USERNAME`、`CSUST_PASSWORD` 读取，否则读取当前目录 `.env` 的 `username`、`password`；也可用 `--password-stdin`。`vpn login` 默认按门户配置选择认证方式：当前门户隐藏本地账号登录时自动走 CAS 统一身份认证，完成回调后保存 Cookie/Token 会话；可用 `--auth cas` 强制 CAS，或用 `--auth local` 强制旧的本地 API 登录。登录后的 MFA、设备注册、审批、找回密码、二维码/第三方登录等条件流程均通过对应目录 API 原样开放。

## 网络教学平台映射

`csust teaching catalog` 固化 Chrome 实际可见的网络教学平台页面、课程菜单、弹窗、表单动作、资源中心和作业 SPA/API 清单。`teaching request` 支持同源 HTML 页面、传统表单、JSON/multipart 请求、文件上传、下载/导出和分页/筛选参数；`get` 输出页面的链接、表单、控件、选项、表格和脚本端点。平台网关前缀每次从 VPN 的“网络教学平台”服务动态解析，避免把当前会话的服务令牌写进配置。写请求仍需显式 `--yes`。

## 教学质量保障系统映射

`csust quality` 通过 VPN 的 CAS 统一认证会话进入“教学一体化”网关，再登录旧版教学质量保障门户；不会把网关前缀或服务令牌固化到配置。`quality catalog` 映射网页端当前可见的 69 个学生页面，`quality routes` 读取登录后的实时菜单，`quality get` 输出页面中的表单、控件、表格、动作和脚本端点。未固化的网页路径或接口可直接用 `quality request --path` 调用，因此页面清单不会限制实际能力覆盖。

```bash
python3 csust.py quality login --vpn-captcha-info '{"captcha":"CODE"}' --json
python3 csust.py quality status --json
python3 csust.py quality catalog --json
python3 csust.py quality routes --json
python3 csust.py quality graduation-design --json
python3 csust.py quality public get --path /findmm.jsp --json
python3 csust.py quality get --name student-evaluation --json
python3 csust.py quality form --path /jsxsd/... --form 1 --data NAME=VALUE --yes --json
python3 csust.py quality action --path /jsxsd/... --index 1 --data NAME=VALUE --yes --json
python3 csust.py quality evaluation batches --json
python3 csust.py quality evaluation courses --path PATH --json
python3 csust.py quality evaluation form --path PATH --json
python3 csust.py quality request --path /jsxsd/... --method POST \
  --data NAME=VALUE --yes --json
```

条件显示的“毕业设计”入口通过 `quality graduation-design` 解析并校验外部 SSO；`quality form` 和 `quality action` 直接复用网页端表单/动作解析，会自动带上隐藏字段并解析页面脚本状态。评价的 `courses`、`form`、`save`、`submit` 会直接复用网页结构解析。保存和提交会校验隐藏字段、题目及选项，并在响应无法确认成功时返回未验证。所有通用写请求均需显式 `--yes`。

## Web 映射

`csust web catalog` 和 `ROUTE_CATALOG` 是同一份可检查清单；每一行的 `command` 就是对应 CLI 命令 `csust web <command>`，默认 GET 页面。该命令还支持：

- `--param NAME=VALUE`：初始查询参数。
- `--form N [--button M] --data NAME=VALUE`：按页面表单和按钮提交，自动带上未修改的隐藏字段。
- `--action N --data NAME=VALUE`：按结构化页面输出中的动作序号执行链接、弹窗、表单或页面 JavaScript 中可解析的 URL 动作。
- `--output FILE`：原样保存打印、导出和下载响应；`web request` 另外支持 GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS。

当前清单按网站二级菜单归组：教学评价 1；我的申请 3；我的考试 5；成绩管理 3；培养方案 5；我的课表 8；选课管理 9；教材管理 2；辅修管理 1；实验教学 2；第二课堂学分 2；学科竞赛 1；创新创业 5；公告留言 3；个人信息 2；在线问答 1；教学周历 1；学籍管理 8；我的成绩 4；毕业管理 3，共 69 条。`routes --json` 还返回登录、忘记密码、验证码、APP 登录页切换和条件显示的毕业设计外部 SSO 入口。

账号密码优先从 `CSUST_USERNAME`、`CSUST_PASSWORD` 读取；未设置时读取当前目录 `.env` 中的 `username`、`password`，不会交互询问或写入密码。`.env` 应保持 `600` 权限。验证码由 `ddddocr` 在本机识别；失败会自动重试 3 次，不会等待人工输入。可用 `--captcha CODE` 做测试覆盖。

会话 Cookie 保存在 `~/.config/csust-cli/cookies.txt`，权限为 `600`；验证码图片保存在同目录，权限为 `600`。可用 `CSUST_BASE_URL` 和 `CSUST_COOKIE_FILE` 覆盖站点与会话文件路径。现站点使用 HTTP 时每个进程的首次请求会向 stderr 发出安全警告。

没有有效 Cookie 时，所有登录后查询和页面命令都会在凭据变量存在时自动登录。`routes` 会返回登录后主页清单；`web get` 或任一 69 个页面命令会输出结构化链接、表单、控件、选项、表格、动作和脚本函数。登录页/找回密码使用 `web public get|action|post`，条件显示的毕业设计使用 `web graduation-design`。

教材、评价和通用 POST/网页动作等可能修改账号数据的操作始终要求 `--yes`。通用写请求会依据响应中的成功/失败信号确认结果；无法确认时返回未验证，不会自动重试。教材选订/退订还要求单条精确目标，并在提交后重新查询验证。

安装为 `csust` 命令：

```bash
python3 -m pip install -e .
csust schedule --json
```

运行检查：

```bash
python3 -m unittest -q
```
