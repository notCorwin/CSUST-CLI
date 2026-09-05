"""CLI access to the current vpn.csust.edu.cn EnUES web portal.

The portal is a Vue SPA, so its useful contract is its route/control/API
surface rather than HTML forms.  The catalog below is copied from the portal's
current route declarations and bundled request helpers.  ``vpn api`` keeps the
request body open-ended: new fields and conditional flows do not require a
new CLI release.
"""

from __future__ import annotations

import argparse
import base64
import json
import mimetypes
import os
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable
from urllib.parse import parse_qsl, quote, urlencode, urljoin, urlsplit, urlunsplit

from ..core import (
    AuthenticationFailed,
    Client,
    CredentialsRequired,
    CsustError,
    business_state,
    result_status,
    Response,
    _credentials,
    _decode_body,
    _encrypt_cas_password,
    _header_value,
    require_logged_in,
    parse_html,
    _save_cookie_refresh,
    _safe_terminal_text,
    _write_private_file,
    env_value,
)


VPN_BASE_URL = "https://vpn.csust.edu.cn"
VPN_NATIVE_URL = "http://127.0.0.1:30303"
VPN_START_PATH = "/enclient/start.html"
VPN_CONFIG_PATH = "/api/users/custom/page/login/cfg/select"
VPN_KEY_PATH = "/api/users/client/auth/generateKey"
VPN_LOGIN_PATH = "/api/users/auth/login"
VPN_INFO_PATH = "/api/users/info"
VPN_LOGOUT_PATH = "/api/users/auth/logout"
VPN_REFRESH_PATH = "/api/v1/user/refreshToken"


@dataclass(frozen=True)
class VpnApiSpec:
    name: str
    method: str
    path: str
    group: str
    mutating: bool = False
    binary: bool = False


def _api_name(path: str) -> str:
    value = path.strip().strip("/").replace("{", "-").replace("}", "")
    value = re.sub(r"^api/", "", value, flags=re.I)
    value = re.sub(r"[^A-Za-z0-9]+", "-", value).strip("-").lower()
    return value or "root"


def _api_group(path: str) -> str:
    if "/auth/" in path or "/login" in path or "/captcha" in path or "/qrCode" in path:
        return "认证与登录"
    if "/approve/" in path or "/Apply" in path or "/apply" in path:
        return "申请与审批"
    if "/message/" in path or "/banner/" in path:
        return "消息与公告"
    if "/service/" in path or "/Service" in path or "service" in path:
        return "应用与服务"
    if "/device/" in path or "/terminal/" in path or "/peripheral" in path:
        return "设备与终端"
    if "/safeSpace/" in path or "/share/" in path or "/desktop/" in path:
        return "安全空间与文件"
    if "/AVengine/" in path or "/safetyAssess/" in path or "/antivirus/" in path:
        return "安全中心"
    if "/appMarket/" in path:
        return "应用商店"
    if "/control/" in path or "/gateway/" in path or "/nac/" in path or "/local/" in path:
        return "客户端与网络"
    if "/user/" in path or "/person/" in path:
        return "用户中心"
    return "其他"


_READ_POST_LEAF = re.compile(
    r"(?:^|/)(?:get|find|list|page|count|info|detail|query|result|flow|check|supported|current|latest|baseinfoGet|get[A-Z])",
    re.I,
)


def _is_mutating(method: str, path: str) -> bool:
    if "/delete" in path.lower() or "/logout" in path.lower():
        return True
    if method in {"GET", "HEAD", "OPTIONS"}:
        return False
    leaf = path.split("?", 1)[0].rstrip("/")
    if _READ_POST_LEAF.search(leaf):
        return False
    return True


# ``method|path`` entries are deliberately kept as data.  This makes the
# catalog diffable against a freshly downloaded bundle and keeps arbitrary
# request fields out of the parser.
_API_ROWS = """
POST|/api/users/auth/login
POST|/api/users/approve/center/addDeviceApp
POST|/api/users/custom/page/login/cfg/select
POST|/api/users/basic/captcha
POST|/api/users/reset/password/find/verify/type
POST|/api/users/reset/password/send/code
POST|/api/users/reset/password/terminal/verify/code
POST|/api/users/reset/password/verify/code
POST|/api/users/auth/alertAuth
POST|/api/users/bindMobileEmail/send/code
POST|/api/users/auth/bindMobileOrEmail
POST|/api/users/bindWeChatOrEns/find/bindStatus
POST|/api/users/bindStatus/find/type
GET|/api/users/device/bindInfo/token/get
POST|/api/users/sim/send/code
POST|/api/users/flow/path
POST|/api/users/auth/logout
POST|/api/users/noPassLogin/send/code
POST|/api/users/auth/noPasswordLogin
GET|/api/users/client/auth/generateKey
POST|/api/users/findFeishuConf
GET|/api/users/auth/lark/getQrCode/{id}
GET|/api/users/weChat/getWechatQrCode
GET|/api/users/client/enapp/getCoderQr
POST|/api/users/client/enapp/getCoderQrStatus
POST|/api/users/auth/enAppQrCodeLogin
POST|/api/users/findDiTalkConf
GET|/api/users/auth/dingtalk/getQrCode/{id}
GET|/api/users/weChat/getWechatEnsQrCode
GET|/api/users/auth/weChatEns/getQrCode/{id}
POST|/api/users/v3/qrCode/generate
GET|/api/users/V3/qrCode/result/{id}
POST|/api/users/auth/qrCode
POST|/api/users/auth/oneClick
POST|/api/users/auth/oneClickAuth
POST|/api/users/auth/modifyPwd
POST|/api/users/second/send/code
POST|/api/users/auth/secondAuth
POST|/api/users/sim/roll/result
POST|/api/users/auth/simAuth
POST|/api/users/iam/SecondAuth/getSecondAuthTpye
POST|/api/users/doCBAuth
POST|/api/users/doAgainCBAuth
POST|/api/users/doAgainLocalCBAuth
POST|/api/users/doAgainSmsCBAuth
GET|/api/users/selfUnlock/type
POST|/api/user/sendCode/{type}
POST|/api/user/checkCode/{type}
POST|/api/users/approve/addApply
POST|/api/users/auth/weChat/bindAndLogin
GET|/api/users/info
POST|/api/users/auth/afterBindLogin
POST|/api/users/auth/otp/login
POST|/api/client/mfa/login/commonAuth
GET|/api/users/auth/mfa/check?anchorId=
POST|/api/users/auth/mfaSecondAuth
POST|/api/users/auth/mfaVerifyEnApp
POST|/api/users/auth/faceRecognition/login
GET|/api/client/mfa/login/homeInfo?anchorId=
POST|/api/client/mfa/login/sendVerifyCode
POST|/api/client/mfa/sendVerifyCode
POST|/api/client/mfa/commonAuth
GET|/api/client/message/secAuth?msgId=
POST|/api/client/totp/login/generateSecretQRContent
GET|/api/users/auth/certificate/{id}/getCode
POST|/api/users/auth/certificate/login
POST|/api/users/device/register/send/code
POST|/api/users/device/register/terminal/verify/code
POST|/api/users/device/register
POST|/api/users/auth/secondAuth/device
POST|/api/users/device/list/page
POST|/api/users/center/device/unbind
POST|/api/users/center/device/offline
POST|/api/users/center/uploadPicture
POST|/api/users/device/unbindUser
GET|/api/client/users/service/group?endlessType=
POST|/api/client/user/center/resetUserPassword
POST|/api/users/person/restName
POST|/api/users/unBind/message/byType
POST|/api/users/service/visit/list
GET|/api/users/service/getAllTemporaryService
GET|/api/users/service/visit/add/{id}
POST|/api/users/person/bindMobileEmail
POST|/api/users/unbind/send/code
POST|/api/users/unbind/check/code
POST|/api/client/user/addServiceGroup
POST|/api/client/user/deleteServiceGroup
POST|/api/client/user/updateServiceGroup
POST|/api/client/user/updateServiceSort
POST|/api/client/user/updateServiceGroupSort
POST|/api/users/service/visit/delete
POST|/api/client/user/diTalk/bind
POST|/api/users/person/select/enableIsOpened
GET|/api/users/message/count
GET|/api/users/message/get?messageId=
POST|/api/users/message/page
GET|/api/users/message/allRead?type=
POST|/api/users/approve/center/groupCount
POST|/api/users/approve/center/applyPage
POST|/api/users/approve/center/waitingHandle
POST|/api/users/approve/center/myHandle
POST|/api/users/approve/center/flowImage
GET|/api/users/safeSpace/getService
POST|/api/client/totp/generateSecretQRContent
POST|/api/client/share/link/page
GET|/api/client/share/link/delete/{id}
POST|/api/users/safeSpace/getShareFilePage
GET|/api//client/banner/getBannerInfo?key=
POST|/api/client/message/callback
GET|/api/client/public/files/download?filePath=
POST|/api/users/service/getAllTemporaryService?orderValue=
POST|/api/client/users/service/grouping
POST|/api/client/user/service/pageUserService?endlessType=
POST|/api/client/user/service/customGroupRemoveService
POST|/api/client/user/service/addToCustomGroup
POST|/api/users/service/getAllApplicabilityService
POST|/api/users/service/createServiceApply
POST|/api/users/service/cancleServiceApply
POST|/api/users/approve/center/addApply
POST|/api/users/approve/center/canApplyTypes
POST|/api/users/approve/center/applyFormConf
POST|/api/users/approve/center/approveUserInfos
POST|/api/users/approve/center/deal
POST|/api/users/approve/center/validBeforeApplication
POST|/api/users/approve/addApply/noToken
POST|/api/users/approve/center/applyDetail
GET|/api/users/approve/center/getFlow
POST|/api/users/peripheral/enable
GET|/api/users/terminal/csAndNetworkControl
GET|/api/users/device/labels/info
GET|/api/users/device/getByFeatureCode?featureCode=
POST|/api/users/device/bind/labels
POST|/api/users/confirmNewCommonLocation
GET|/api/users/person/getBindInfos
GET|/api/users/person/getBindQr?authConfigId=
POST|/api/users/updateCommonLocation
POST|/api/users/auth/abac/mfa
GET|/api/client/mfa/homeInfo?anchorId=
POST|/api/client/totp/sendCode
POST|/api/client/totp/resetSecret
GET|/api/users/url/open?url=

POST|/api/v1/AVengine/baseinfoGet
POST|/api/v1/AVengine/baseinfoSet
POST|/api/v1/AVengine/exportLog
POST|/api/v1/AVengine/findAVS
POST|/api/v1/AVengine/getAVList
POST|/api/v1/AVengine/getAVPath
POST|/api/v1/AVengine/getIsolator
POST|/api/v1/AVengine/getLog
POST|/api/v1/AVengine/getResult
POST|/api/v1/AVengine/isolator
POST|/api/v1/AVengine/scan
POST|/api/v1/AVengine/setIsolatorPath
POST|/api/v1/AVengine/solve
POST|/api/v1/AVengine/updateAVLib
POST|/api/v1/api/users/antivirus/trustFile/page
GET|/api/v1/api/users/device/terminal/antivirus/lib?libType=
GET|/api/v1/appMarket/getPushSoftwareData
POST|/api/v1/appMarket/getSoftCategoryList
POST|/api/v1/appMarket/getSoftPage
POST|/api/v1/appMarket/getSoftPageBeforeLogin
POST|/api/v1/appMarket/getSoftSearch
POST|/api/v1/appMarket/softUploadDistributeResult
POST|/api/v1/appMarket/softwareReport
POST|/api/v1/coms/RCICSwitch
POST|/api/v1/coms/getRCICInfo
POST|/api/v1/coms/getRCISState
POST|/api/v1/coms/getRCSInfo
POST|/api/v1/coms/patchFix
GET|/api/v1/coms/patchList
POST|/api/v1/coms/setRCISInfo
POST|/api/v1/control/autoBoot
POST|/api/v1/control/detect
GET|/api/v1/control/getLocalConfig
GET|/api/v1/control/info
POST|/api/v1/control/list
POST|/api/v1/control/notification
POST|/api/v1/control/protocol
POST|/api/v1/control/select
GET|/api/v1/desktop/GetFileCirculateList
POST|/api/v1/desktop/downloadShareFile
POST|/api/v1/desktop/enDesktopProxy
GET|/api/v1/desktop/fileCirculate?spaceName=
POST|/api/v1/desktop/fileCirculateDownload
GET|/api/v1/desktop/getAllFileSharPath
POST|/api/v1/desktop/getSharFileDownData
POST|/api/v1/desktop/newDownloadShareFile
POST|/api/v1/desktop/openShareFileDir
POST|/api/v1/desktop/redirectSvcRequest
GET|/api/v1/desktop/spaceList
GET|/api/v1/desktop/startProcess
POST|/api/v1/device/manager/getPeripheralData
GET|/api/v1/device/security/allowed
POST|/api/v1/external/runExe
POST|/api/v1/gateway/switch
POST|/api/v1/gateway/turnOn
GET|/api/v1/local/config
GET|/api/v1/local/device/info
POST|/api/v1/local/device/queryProcess
GET|/api/v1/local/getAction
GET|/api/v1/local/info
GET|/api/v1/local/language/supported
POST|/api/v1/local/language/switch
GET|/api/v1/local/manager/queryUninstallStatus?querystatus=
POST|/api/v1/local/openSoftware
GET|/api/v1/local/pollTags
POST|/api/v1/local/software/setup
POST|/api/v1/local/log/upload
GET|/api/v1/local/terminal/doProject
GET|/api/v1/local/terminal/doProject?time=0
GET|/api/v1/local/terminal/scoreRequest?mustColl=
POST|/api/v1/local/terminal/setAction
POST|/api/v1/local/writeConfig
POST|/api/v1/local/writeConfigIni
POST|/api/v1/nac/login
POST|/api/v1/nac/logout
GET|/api/v1/nac/netIsolation?type=
POST|/api/v1/nac/queryConfig
POST|/api/v1/nac/queryInfo
GET|/api/v1/nac/queryProxy
POST|/api/v1/nac/setProxy
GET|/api/v1/safetyAssess/continueTask
GET|/api/v1/safetyAssess/getDetailInfo
GET|/api/v1/safetyAssess/getOverViewInfo
GET|/api/v1/safetyAssess/pauseTask
POST|/api/v1/safetyAssess/repair
POST|/api/v1/safetyAssess/repairAll
POST|/api/v1/safetyAssess/starTask
GET|/api/v1/safetyAssess/stopTask
POST|/api/v1/sase/getEnterpriseCfg
GET|/api/v1/static/avatar/query?userName=
POST|/api/v1/static/avatar/set
GET|/api/v1/user/GetUserProtocolState
POST|/api/v1/user/detail
GET|/api/v1/user/getRedirectUrl
POST|/api/v1/user/getUserGroupedServiceList
POST|/api/v1/user/logout
POST|/api/v1/user/logout?type=1
POST|/api/v1/user/preLogin
POST|/api/v1/user/pullUES
GET|/api/v1/user/refreshToken
POST|/api/v1/user/register
POST|/api/v1/user/selectControl
POST|/api/v1/user/setUserProtocolState
POST|/api/v1/user/webSessionExpires
GET|/api/v1/version/current
POST|/api/v1/version/executeUpdate
GET|/api/v1/version/latest
POST|/api/v1/version/latestServer
"""


def _build_api_catalog() -> tuple[VpnApiSpec, ...]:
    rows: list[VpnApiSpec] = []
    seen: set[str] = set()
    for raw in _API_ROWS.splitlines():
        raw = raw.strip()
        if not raw:
            continue
        method, path = raw.split("|", 1)
        method, path = method.strip().upper(), path.strip()
        name = _api_name(path)
        if name in seen:
            raise RuntimeError(f"duplicate VPN API catalog name: {name}")
        seen.add(name)
        binary = path.endswith("/download") or "/files/download" in path
        rows.append(VpnApiSpec(name, method, path, _api_group(path), _is_mutating(method, path), binary))
    return tuple(rows)


VPN_API_CATALOG = _build_api_catalog()
VPN_API_BY_NAME = {item.name: item for item in VPN_API_CATALOG}
VPN_API_BY_PATH = {item.path: item for item in VPN_API_CATALOG}


def _api_aliases(spec: VpnApiSpec) -> set[str]:
    names = {spec.name}
    for prefix in ("users-", "client-", "v1-", "v1-api-", "v1-api-users-"):
        if spec.name.startswith(prefix):
            names.add(spec.name[len(prefix):])
    return names


for _spec in VPN_API_CATALOG:
    for _alias in _api_aliases(_spec):
        VPN_API_BY_NAME.setdefault(_alias, _spec)


VPN_RESOURCE_CATALOG = (
    {"name": "files", "web_path": "/enclient/files/{path}", "native_path": "/api/v1/files/{path}", "kind": "image/download"},
    {"name": "pics", "web_path": "/enclient/files/{path}", "native_path": "/api/v1/pics/{path}", "kind": "image"},
    {"name": "service-agreement", "web_path": "/enclient/serviceAgreement.html", "native_path": "", "kind": "login agreement"},
    {"name": "user-terms", "web_path": "/enclient/userTerms.html", "native_path": "", "kind": "login terms"},
)


_ROUTE_ROWS = (
    ("login", "公开入口", "/login", "账号登录、动态码、二维码、SSO、隐私协议、找回密码", ("users-auth-login", "users-client-auth-generatekey", "users-custom-page-login-cfg-select", "users-basic-captcha", "users-auth-alertauth", "users-auth-bindmobileoremail", "users-auth-dingtalk-getqrcode-id", "users-auth-enappqrcodelogin", "users-auth-facerecognition-login", "users-auth-lark-getqrcode-id", "users-auth-mfa-check-anchorid", "users-auth-nopasswordlogin", "users-auth-otp-login", "users-auth-qrcode", "users-auth-simauth", "users-auth-wechatens-getqrcode-id", "users-auth-oneclick", "users-auth-oneclickauth", "users-bindmobileemail-send-code", "users-bindstatus-find-type", "users-bindwechatorens-find-bindstatus", "users-client-enapp-getcoderqr", "users-client-enapp-getcoderqrstatus", "users-findditalkconf", "users-findfeishuconf", "users-flow-path", "users-iam-secondauth-getsecondauthtpye", "users-nopasslogin-send-code", "users-sim-roll-result", "users-sim-send-code", "users-v3-qrcode-generate", "users-v3-qrcode-result-id", "users-wechat-getwechatensqrcode", "client-mfa-login-sendverifycode")),
    ("sign-in", "登录流程", "/login/sign-in", "登录页主组件与登录方式切换", ("auth-login", "users-client-auth-generatekey")),
    ("sign-in-to-login", "登录流程", "/login/sign-in?type=toLogin", "安全基线/设备流程返回登录页", ("auth-login", "users-client-auth-generatekey")),
    ("login-about", "系统菜单", "/login/about", "关于页面", ()),
    ("login-auth-phone-or-email", "登录流程", "/login/authPhoneOrEmail", "手机或邮箱二次认证", ("second-send-code", "auth-secondauth", "auth-afterbindlogin")),
    ("login-auth-register", "登录流程", "/login/authrigister", "新设备认证注册", ("device-register-send-code", "device-register", "auth-afterbindlogin")),
    ("mac-loading", "登录流程", "/login/macLoading", "客户端加载/检测", ("local-info", "user-prelogin")),
    ("add-server", "登录流程", "/login/add-server", "添加服务端", ("user-register", "user-detail")),
    ("company-code", "登录流程", "/login/company-code", "企业编码", ("user-detail",)),
    ("confirm-join", "登录流程", "/login/confirm-join", "确认加入企业", ("user-register",)),
    ("baseline-check", "登录流程", "/login/baseline-check", "登录前安全基线", ("safetyassess-getoverviewinfo", "safetyassess-getdetailinfo")),
    ("login-sys-config", "登录流程", "/login/sys-config", "登录前系统配置", ("control-getlocalconfig", "control-protocol", "local-language-supported")),
    ("network-error", "登录流程", "/login/netWorkError", "网络错误", ()),
    ("company-config", "登录流程", "/login/company-config", "企业配置", ("sase-getenterprisecfg",)),
    ("forget-password", "公开入口", "/login/forget-password", "找回/重置密码与验证码", ("reset-password-find-verify-type", "reset-password-send-code", "reset-password-verify-code", "reset-password-terminal-verify-code")),
    ("device-register", "登录流程", "/login/deviceRegister", "设备注册", ("device-register-send-code", "device-register-terminal-verify-code", "device-register", "users-device-register", "users-device-register-send-code", "users-device-register-terminal-verify-code", "users-device-bindinfo-token-get", "users-device-labels-info")),
    ("forget-password-success", "登录流程", "/login/forgetPasswordSuccess", "找回密码完成", ()),
    ("first-login-update", "登录流程", "/login/first-login-update", "首次登录修改密码", ("auth-modifypwd",)),
    ("first-login-update-legacy", "登录流程", "/login/firstLoginUpdatePwd", "兼容登录流程的首次改密页", ("auth-modifypwd",)),
    ("forcibly-auth", "登录流程", "/login/forciblyAuth", "强制二次认证", ("auth-secondauth", "mfa-commonauth", "auth-mfasecondauth", "client-mfa-login-sendverifycode", "users-auth-mfa-check-anchorid")),
    ("second-auth-old", "登录流程", "/login/second-auth-old", "旧版二次认证", ("second-send-code", "auth-secondauth", "auth-nopasswordlogin")),
    ("second-auth-old-other", "登录流程", "/login/second-auth-old-other", "旧版二次认证其他方式", ("second-send-code", "auth-secondauth", "auth-nopasswordlogin")),
    ("new-device", "登录流程", "/login/newdevice", "新设备注册提示页", ("device-register-send-code", "device-register")),
    ("code-register", "登录流程", "/login/codeRigister", "验证码注册页", ("device-register-send-code", "device-register")),
    ("register-fail", "登录流程", "/login/rigisterFail", "设备注册失败页", ("device-register",)),
    ("second-auth", "登录流程", "/login/second-auth", "二次认证", ("second-send-code", "auth-secondauth", "auth-mfasecondauth", "users-docbauth", "users-doagaincbauth", "users-doagainlocalcbauth", "users-doagainsmscbauth")),
    ("second-auth-detail", "登录流程", "/login/second-auth-detail", "二次认证详情", ("auth-secondauth-device", "client-message-secauth-msgid")),
    ("device-unbind-auth-detail", "登录流程", "/login/device-unbind-auth-detail", "设备解绑的手机/邮箱/SIM 认证详情", ("second-send-code", "auth-secondauth", "auth-afterbindlogin")),
    ("bind-info", "登录流程", "/login/bind-info", "绑定信息", ("person-getbindinfos", "person-bindmobileemail")),
    ("device-unbind", "登录流程", "/login/device-unbind", "设备解绑", ("center-device-unbind", "unbind-send-code", "unbind-check-code", "users-center-device-unbind", "users-device-unbinduser", "users-unbind-message-bytype", "users-device-getbyfeaturecode-featurecode")),
    ("device-unbind-auth", "登录流程", "/login/device-unbind-auth", "设备解绑认证", ("auth-secondauth-device", "users-device-bindinfo-token-get", "users-second-send-code")),
    ("enhanced-auth", "登录流程", "/login/enhancedAuth", "增强认证/MFA", ("auth-abac-mfa", "mfa-commonauth", "mfa-sendverifycode")),
    ("enhanced-auth-type", "登录流程", "/login/enhancedAuthType", "增强认证方式", ("auth-mfasecondauth", "auth-mfaverifyenapp", "client-totp-login-generatesecretqrcontent")),
    ("bind-otp", "登录流程", "/login/bindOtp", "绑定 OTP", ("client-totp-generatesecretqrcontent", "client-totp-sendcode", "client-totp-resetsecret")),
    ("self-unlock", "登录流程", "/login/selfUnlock", "自助解锁", ("selfunlock-type", "user-sendcode-type", "user-checkcode-type")),
    ("unlock-success", "登录流程", "/login/unlockSuccess", "解锁完成", ()),
    ("account-unlock-apply", "登录流程", "/login/accountUnlockApply", "账号解锁申请", ("approve-addapply",)),
    ("third-account-bind", "登录流程", "/login/thirdaccountBind", "第三方账号绑定", ("auth-certificate-id-getcode", "auth-certificate-login")),
    ("wechat-login-loading", "登录流程", "/login/wxLoginLoading", "微信登录等待", ("wechat-getwechatqrcode", "auth-wechat-bindandlogin")),
    ("sso-loading", "登录流程", "/login/ssoLoading", "SSO 等待", ("users-info",)),
    ("baseline-to-login", "登录流程", "/login/baselineToLogin", "基线完成回登录", ("user-getusergroupedservicelist",)),
    ("new-equipment-registration", "登录流程", "/login/new-equipment-registration", "新设备注册", ("device-register", "device-register-send-code")),
    ("register-success", "登录流程", "/login/rigisterSuccess", "设备注册完成", ()),
    ("device-code-apply", "登录流程", "/login/device-code-apply", "设备验证码申请", ("device-register-send-code",)),
    ("device-second-auth", "登录流程", "/login/device-second-auth", "设备二次认证", ("auth-secondauth-device",)),
    ("auth-register-phone", "登录流程", "/login/authrigister_phone", "认证手机号注册", ("user-sendcode-type", "user-checkcode-type", "auth-afterbindlogin", "users-bindmobileemail-send-code", "users-auth-bindmobileoremail")),
    ("device-regist-success", "登录流程", "/login/device-regist-success", "设备登记完成", ()),
    ("device-approval", "登录流程", "/login/device-approval", "设备审批", ("approve-addapply", "approve-center-deal", "users-approve-center-adddeviceapp")),
    ("mfa-login-loading", "登录流程", "/login/mfaLoginLoading", "MFA 登录等待", ("mfa-login-commonauth", "auth-mfasecondauth")),
    ("visitor-request", "公开入口", "/login/visitorRequest", "访客申请", ("user-sendcode-type", "user-checkcode-type", "user-register")),
    ("login-device-unbind-legacy", "登录流程", "/login/deviceUnBind", "旧版设备解绑流程", ("center-device-unbind", "unbind-send-code", "unbind-check-code")),
    ("login-system-conf", "系统菜单", "/login/systemConf", "系统配置菜单入口", ("control-getlocalconfig", "control-protocol", "local-language-supported", "v1-user-getredirecturl")),
    ("log-export", "系统菜单", "/prePage/logExport", "客户端日志导出入口", ()),
    ("log-report", "系统菜单", "/prePage/logReport", "客户端日志上报页面", ("local-log-upload",)),
    ("prepage-company-code", "客户端流程", "/prePage/companycode", "客户端服务端选择/企业编码页", ("user-detail", "user-register")),
    ("prepage-mac-privacy", "客户端流程", "/prePage/macPrivacy", "Mac 客户端隐私与扩展授权页", ("user-getuserprotocolstate", "user-setuserprotocolstate")),
    ("prepage-old-login", "客户端流程", "/prePage/oldLogin", "旧版本登录页", ("version-latestserver", "user-prelogin")),
    ("prepage-serve-conf", "客户端流程", "/prePage/serveConf", "客户端服务端配置页", ("version-latestserver", "user-detail")),
    ("home", "工作台", "/home", "登录后门户壳、侧栏和全局状态", ("users-info", "users-message-count", "client-users-service-group-endlesstype", "users-auth-logout", "v1-user-logout", "v1-user-logout-type-1", "v1-user-pullues", "v1-user-refreshtoken", "v1-user-websessionexpires")),
    ("errorguide", "工作台", "/home/errorguide", "错误/诊断指引", ("local-terminal-scorerequest-mustcoll", "local-terminal-doproject")),
    ("overview", "工作台", "/home/overview", "门户概览、横幅、公告", ("client-banner-getbannerinfo-key", "users-message-page")),
    ("personal-center", "用户中心", "/home/personal-center", "账号安全、绑定信息、活动设备", ("users-info", "person-getbindinfos", "device-list-page", "client-user-center-resetuserpassword", "client-user-ditalk-bind", "users-auth-bindmobileoremail", "users-bindmobileemail-send-code", "users-bindstatus-find-type", "users-bindwechatorens-find-bindstatus", "users-center-device-offline", "users-center-device-unbind", "users-center-uploadpicture", "users-device-bind-labels", "users-device-bindinfo-token-get", "users-device-getbyfeaturecode-featurecode", "users-device-labels-info", "users-device-unbinduser", "users-person-getbindqr-authconfigid", "users-person-restname", "users-person-select-enableisopened", "v1-static-avatar-set")),
    ("work-bench", "工作台", "/home/work-bench", "最近访问、全部应用、应用分组、搜索、排序", ("client-users-service-group-endlesstype", "client-users-service-grouping", "client-user-service-pageuserservice-endlesstype", "users-service-visit-list", "users-service-visit-add-id", "users-service-visit-delete", "client-user-addservicegroup", "client-user-deleteservicegroup", "client-user-updateservicegroup", "client-user-updateservicesort", "client-user-updateservicegroupsort", "client-user-service-addtocustomgroup", "client-user-service-customgroupremoveservice", "users-url-open-url", "users-confirmnewcommonlocation", "users-updatecommonlocation", "v1-local-polltags")),
    ("work-bench-all", "工作台", "/home/work-bench-all", "兼容版本全部应用工作台", ("client-users-service-group-endlesstype", "users-service-visit-list")),
    ("exclusion-zone", "工作台", "/home/work-bench/exclusion-zone", "隔离区工作台", ("users-info", "users-url-open-url", "message-secauth-msgid")),
    ("approve-center", "申请与审批", "/home/approve-center", "我的申请、待办、已办、流程图、详情", ("approve-center-groupcount", "approve-center-applypage", "approve-center-waitinghandle", "approve-center-myhandle", "approve-center-flowimage", "approve-center-getflow", "approve-center-applydetail", "approve-center-deal")),
    ("approve-initiated", "申请与审批", "/home/approve-center?tabName=Initiated", "审批中心我的申请标签状态", ("approve-center-applypage", "approve-center-applydetail")),
    ("approve-pending", "申请与审批", "/home/approve-center?tabName=Pending", "审批中心待办标签状态", ("approve-center-waitinghandle", "approve-center-deal")),
    ("apply-app", "申请与审批", "/home/apply-app", "应用申请", ("approve-center-canapplytypes", "approve-center-applyformconf", "approve-center-approveuserinfos", "approve-center-addapply", "approve-center-validbeforeapplication", "users-approve-center-adddeviceapp")),
    ("temporary-account", "申请与审批", "/home/temporaryAccount", "临时账号申请", ("service-getalltemporaryservice", "service-getalltemporaryservice-ordervalue", "service-createserviceapply")),
    ("account-apply", "申请与审批", "/home/accountApply", "账号申请", ("service-getallapplicabilityservice", "service-createserviceapply", "approve-center-addapply")),
    ("cancel-account-apply", "申请与审批", "/home/cancelAccountApply", "账号注销申请", ("service-cancleServiceApply", "approve-center-addapply")),
    ("usb-apply", "申请与审批", "/home/usbApply", "USB 存储设备申请", ("users-peripheral-enable", "approve-center-addapply", "approve-addapply-notoken")),
    ("app-store", "应用商店", "/home/app-store", "软件商店、分类、搜索、安装/更新/卸载", ("appmarket-getsoftcategorylist", "appmarket-getsoftpage", "appmarket-getsoftsearch", "appmarket-getpushsoftwaredata", "appmarket-getsoftpagebeforelogin", "appmarket-softuploaddistributeresult", "appmarket-softwareReport", "local-software-setup", "external-runexe", "v1-local-opensoftware", "v1-local-manager-queryuninstallstatus-querystatus")),
    ("app-market-legacy", "应用商店", "/home/appMarket", "兼容版本应用市场入口", ("appmarket-getsoftcategorylist", "appmarket-getsoftpage", "appmarket-getsoftsearch", "appmarket-getpushsoftwaredata", "appmarket-getsoftpagebeforelogin")),
    ("app-market-detail", "应用商店", "/home/appMarketDetail", "应用详情与分发结果", ("appmarket-getsoftpage", "appmarket-softwareReport", "appmarket-softuploaddistributeresult")),
    ("secure-space", "安全中心", "/home/secure-space", "安全空间/我的空间", ("safespace-getservice", "safespace-getsharefilepage", "desktop-spacelist", "desktop-getallfilesharpath", "desktop-filecirculate-spacename")),
    ("secure-center", "安全中心", "/home/secure-center", "安全中心总览与风险等级", ("safetyassess-getoverviewinfo", "safetyassess-getdetailinfo", "safetyassess-startask", "v1-device-security-allowed")),
    ("safety-detail", "安全中心", "/home/secure-center/safety-detail", "安全检测详情", ("safetyassess-getdetailinfo", "safetyassess-repair", "safetyassess-repairall")),
    ("safety-detail-scan", "安全中心", "/home/secure-center/safety-detail?scanType=", "按扫描类型查看安全详情", ("safetyassess-getdetailinfo", "safetyassess-repair")),
    ("safety-detail-repair-all", "安全中心", "/home/secure-center/safety-detail?isRepair=all&scanType=", "安全中心全部修复结果状态", ("safetyassess-repairall", "safetyassess-getdetailinfo")),
    ("safety-baseline", "安全中心", "/home/secure-center/safety-baseline", "安全基线", ("safetyassess-getoverviewinfo", "safetyassess-pausetask", "safetyassess-continuetask", "safetyassess-stoptask")),
    ("quarantine", "安全中心", "/home/secure-center/quarantine", "病毒隔离区", ("avengine-getisolator", "avengine-isolator", "avengine-solve")),
    ("trust-zone", "安全中心", "/home/secure-center/trust-zone", "信任区", ("antivirus-trustfile-page",)),
    ("virus-logs", "安全中心", "/home/secure-center/virus-logs", "病毒扫描日志", ("avengine-getlog", "avengine-exportlog")),
    ("quick-scan", "安全中心", "/home/secure-center/QuickScan", "快速扫描", ("avengine-scan", "avengine-getresult", "avengine-getavlist")),
    ("custom-scan", "安全中心", "/home/secure-center/CustomScan", "自定义扫描", ("avengine-scan", "avengine-getresult")),
    ("linux-quick-scan", "安全中心", "/home/secure-center/LinuxQuickScan", "Linux 快速扫描", ("avengine-scan", "avengine-getresult")),
    ("safe-result", "安全中心", "/home/secure-center/SafeResult", "安全扫描结果", ("avengine-getresult", "safetyassess-getdetailinfo")),
    ("risk-result", "安全中心", "/home/secure-center/RiskResult", "风险结果", ("safetyassess-getdetailinfo", "safetyassess-repair")),
    ("virus-scan-setting", "安全中心", "/home/secure-center/VirusScanSetting", "病毒扫描设置", ("avengine-baseinfoget", "avengine-baseinfoset", "avengine-setisolatorpath", "avengine-updateavlib", "v1-avengine-findavs", "v1-avengine-getavpath", "v1-api-users-device-terminal-antivirus-lib-libtype")),
    ("message-center", "消息与公告", "/home/message-center", "系统、审批、登录、安全消息", ("users-message-count", "users-message-page", "users-message-get-messageid", "users-message-allread-type", "client-message-callback")),
    ("home-sys-config", "客户端与网络", "/home/sys-config", "系统配置、通知、自启动、协议、代理", ("control-getlocalconfig", "control-autoboot", "control-notification", "control-protocol", "nac-queryproxy", "nac-setproxy", "local-language-switch", "v1-control-detect", "v1-local-config", "v1-local-writeconfig", "v1-local-writeconfigini", "v1-gateway-switch", "v1-gateway-turnon", "v1-nac-login", "v1-nac-logout", "v1-nac-netisolation-type", "v1-nac-queryconfig", "v1-nac-queryinfo")),
    ("notice-center", "消息与公告", "/home/notice-center", "公告中心", ("client-banner-getbannerinfo-key", "users-message-page")),
    ("tool-box", "客户端与网络", "/home/tool-box", "工具箱", ("local-info", "control-info", "local-terminal-doproject", "desktop-startprocess", "v1-local-config", "v1-local-getaction", "v1-local-device-queryprocess", "v1-local-terminal-doproject-time-0", "v1-local-writeconfig", "v1-local-writeconfigini")),
    ("terminal-info", "客户端与网络", "/home/terminal-info", "终端信息与控制", ("local-device-info", "control-info", "control-list", "control-select", "device-manager-getperipheraldata", "users-terminal-csandnetworkcontrol", "v1-api-users-device-terminal-antivirus-lib-libtype", "v1-device-manager-getperipheraldata", "v1-device-security-allowed", "v1-local-device-queryprocess", "v1-local-terminal-doproject-time-0", "v1-local-terminal-setaction", "v1-user-selectcontrol")),
    ("long-range-control", "客户端与网络", "/home/long-range-control", "远程控制", ("coms-getrcsinfo", "coms-getrcicinfo", "coms-getrcisstate", "coms-rcicswitch", "coms-setrcisinfo", "coms-patchlist", "coms-patchfix")),
    ("long-range-control-menu", "客户端与网络", "/home/long-range-Control", "远程控制菜单入口（保留门户大小写路径）", ("coms-getrcsinfo", "coms-getrcicinfo", "coms-getrcisstate", "coms-rcicswitch", "coms-setrcisinfo", "coms-patchlist", "coms-patchfix")),
    ("file-share", "安全空间与文件", "/home/flie-share", "文件分享、接收、流转、下载", ("share-link-page", "share-link-delete-id", "desktop-getfilecirculatelist", "desktop-downloadsharefile", "desktop-newdownloadsharefile", "desktop-filecirculatedownload", "desktop-getsharfiledowndata", "desktop-opensharefiledir", "client-public-files-download-filepath", "v1-desktop-endesktopproxy", "v1-desktop-filecirculate-spacename", "v1-desktop-redirectsvcrequest")),
    ("file-share-received", "安全空间与文件", "/home/flie-share?name=myReceive", "文件分享接收列表状态", ("safespace-getsharefilepage", "desktop-downloadsharefile", "desktop-filecirculatedownload")),
    ("file-link", "安全空间与文件", "/home/file-link", "分享链接", ("share-link-page", "share-link-delete-id")),
    ("enhanced-auth-login", "登录流程", "/home/enhancedAuthLogin", "登录后增强认证", ("mfa-commonauth", "auth-mfasecondauth", "auth-mfaverifyenapp")),
    ("enhanced-auth-login-type", "登录流程", "/home/enhancedAuthLoginType", "登录后认证类型", ("mfa-login-commonauth", "mfa-login-homeinfo-anchorid")),
    ("home-bind-otp", "用户中心", "/home/bindOtp", "登录后 OTP 绑定", ("client-totp-generatesecretqrcontent", "client-totp-sendcode", "client-totp-resetsecret")),
    ("mfa-loading", "登录流程", "/home/mfaLoading", "MFA 加载", ("mfa-homeinfo-anchorid", "mfa-commonauth")),
    ("service", "工作台", "/home/service", "认证后服务落地页", ("users-info", "users-url-open-url")),
    ("save-center", "用户中心", "/home/saveCenter", "客户端保存中心入口", ("users-info", "static-avatar-query-username")),
    ("workbench-legacy", "工作台", "/home/workbench", "兼容版本工作台入口", ("client-users-service-group-endlesstype", "users-service-visit-list")),
    ("workbench-file-apply", "申请与审批", "/home/workbench/file_apply", "文件申请审批兼容页", ("approve-center-applypage", "approve-center-waitinghandle", "approve-center-myhandle", "approve-center-deal")),
    ("sys-config-system", "客户端与网络", "/home/sys-config?type=system", "系统配置页系统状态", ("control-getlocalconfig", "control-autoboot", "control-notification", "control-protocol", "nac-queryproxy", "nac-setproxy")),
    ("sys-config-companies", "客户端与网络", "/home/sys-config?type=companies", "系统配置页企业状态", ("sase-getenterprisecfg", "control-getlocalconfig")),
    ("sys-config-update", "客户端与网络", "/home/sys-config?type=update", "系统配置页更新状态", ("version-current", "version-latest", "version-latestserver", "version-executeupdate")),
    ("error", "系统入口", "/error", "错误页", ()),
    ("mac-privacy", "系统入口", "/macPrivacy", "客户端隐私页", ("user-getuserprotocolstate", "user-setuserprotocolstate")),
    ("root", "系统入口", "/", "门户根入口", ("users-custom-page-login-cfg-select",)),
)


VPN_ROUTE_CATALOG = tuple(
    {
        "name": name,
        "section": section,
        "path": path,
        "aliases": [path.removeprefix(prefix) for prefix in ("/login/", "/home/") if path.startswith(prefix)],
        "description": description,
        "apis": list(apis),
    }
    for name, section, path, description, apis in _ROUTE_ROWS
)


_CONTROL_ROWS = (
    ("login.account", "登录页", "账号密码登录", ("login", "sign-in"), ("users-client-auth-generatekey", "auth-login", "users-custom-page-login-cfg-select", "users-basic-captcha")),
    ("login.dynamic-code", "登录页", "动态码登录", ("login", "sign-in"), ("noPassLogin-send-code", "auth-nopasswordlogin", "users-second-send-code")),
    ("login.cas", "登录页", "CAS 统一身份认证", ("login", "sign-in"), ()),
    ("login.sso", "登录页", "其他单点登录", ("login", "sign-in"), ("auth-qrCode", "auth-oneclick", "users-auth-oneclickauth")),
    ("login.qr-code", "登录页", "二维码登录", ("login", "sign-in"), ("v3-qrcode-generate", "v3-qrcode-result-id", "auth-qrcode", "users-client-enapp-getcoderqr", "users-client-enapp-getcoderqrstatus", "users-auth-enappqrcodelogin", "users-v3-qrcode-generate", "users-v3-qrcode-result-id")),
    ("login.wechat", "登录页", "微信/企业微信登录", ("login", "wechat-login-loading"), ("wechat-getwechatqrcode", "wechat-getwechatensqrcode", "auth-wechat-bindandlogin", "users-auth-wechatens-getqrcode-id")),
    ("login.dingtalk", "登录页", "钉钉登录", ("login",), ("auth-dingtalk-getqrcode-id", "users-findditalkconf")),
    ("login.feishu", "登录页", "飞书登录", ("login",), ("findfeishuconf", "auth-lark-getqrcode-id", "users-findfeishuconf")),
    ("login.otp", "登录页", "OTP 登录", ("login", "bind-otp"), ("auth-otp-login", "client-totp-login-generatesecretqrcontent", "client-totp-generatesecretqrcontent", "client-totp-sendcode", "client-totp-resetsecret")),
    ("login.third-account", "登录页", "第三方账号登录", ("login", "third-account-bind"), ("auth-certificate-id-getcode", "auth-certificate-login")),
    ("login.face", "登录页", "人脸识别登录", ("login",), ("auth-facerecognition-login", "users-auth-facerecognition-login")),
    ("login.sim", "登录页", "SIM 登录", ("login",), ("sim-send-code", "auth-simauth", "sim-roll-result", "users-sim-send-code", "users-sim-roll-result", "users-auth-simauth")),
    ("login.ukey", "登录页", "UKey 登录", ("login",), ("docbauth", "doagaincbauth", "users-docbauth", "users-doagaincbauth", "users-doagainlocalcbauth", "users-doagainsmscbauth")),
    ("login.privacy", "登录页", "隐私协议勾选/服务协议/使用条款", ("login", "mac-privacy"), ("user-getuserprotocolstate", "user-setuserprotocolstate")),
    ("login.forgot", "登录页", "忘记密码", ("login", "forget-password"), ("reset-password-find-verify-type", "reset-password-send-code", "reset-password-verify-code", "users-reset-password-terminal-verify-code")),
    ("login.language", "登录页", "中文/English/Thai", ("login", "home-sys-config"), ("local-language-supported", "local-language-switch")),
    ("login.visitor", "登录页", "访客申请入口", ("login", "visitor-request"), ("user-sendcode-type", "user-checkcode-type", "user-register", "users-selfunlock-type")),
    ("login.remember", "登录页", "记住登录/一周免登录", ("login",), ()),
    ("login.protocol-dialog", "登录页", "隐私协议/服务协议确认弹窗", ("login", "mac-privacy"), ("user-getuserprotocolstate", "user-setuserprotocolstate", "users-custom-page-login-cfg-select")),
    ("login.authorization-dialog", "登录页", "第三方授权确认/取消", ("login", "third-account-bind"), ("users-auth-alertauth", "users-auth-afterbindlogin", "users-auth-bindmobileoremail", "users-flow-path", "users-iam-secondauth-getsecondauthtpye", "users-auth-secondauth", "users-auth-secondauth-device", "users-auth-abac-mfa", "users-auth-mfa-check-anchorid", "users-auth-mfasecondauth", "users-auth-mfaverifyenapp", "client-mfa-commonauth", "client-mfa-homeinfo-anchorid", "client-mfa-login-commonauth", "client-mfa-login-homeinfo-anchorid", "client-mfa-login-sendverifycode", "client-mfa-sendverifycode")),
    ("login.device-registration", "登录流程", "新设备注册/设备审批/验证码", ("device-register", "new-device", "code-register", "new-equipment-registration", "auth-register-phone", "device-approval"), ("users-device-register", "users-device-register-send-code", "users-device-register-terminal-verify-code", "users-peripheral-enable")),
    ("shell.sidebar", "门户壳", "工作台/消息侧栏", ("home", "work-bench", "message-center"), ("users-message-count",)),
    ("shell.system-config", "系统菜单", "系统配置菜单项", ("home-sys-config", "login-system-conf"), ("control-getlocalconfig", "control-protocol", "local-language-switch", "v1-control-detect", "v1-control-info", "v1-control-list", "v1-control-select", "v1-local-config", "v1-local-writeconfig", "v1-local-writeconfigini", "v1-user-getredirecturl", "v1-user-getusergroupedservicelist", "v1-user-prelogin", "v1-user-pullues", "v1-user-refreshtoken", "v1-user-websessionexpires", "v1-version-current", "v1-version-executeupdate", "v1-version-latest", "v1-version-latestserver")),
    ("shell.company-config", "系统菜单", "企业配置菜单项", ("tool-box", "company-config"), ("sase-getenterprisecfg",)),
    ("shell.log-report", "系统菜单", "日志上报抽屉", ("tool-box", "log-report"), ("local-log-upload",)),
    ("shell.log-export", "系统菜单", "日志导出菜单项", ("tool-box", "log-export"), ()),
    ("shell.about", "系统菜单", "关于菜单项", ("tool-box", "login-about"), ()),
    ("shell.change-account", "系统菜单", "切换账号/注销", ("tool-box", "home"), ("auth-logout", "user-logout", "v1-user-logout-type-1")),
    ("shell.exit", "系统菜单", "退出客户端", ("tool-box", "home"), ("user-logout",)),
    ("shell.network-proxy", "工具箱", "网络代理抽屉", ("tool-box", "home-sys-config"), ("nac-queryproxy", "nac-setproxy")),
    ("shell.restart-confirm", "系统配置", "语言切换重启确认弹窗", ("home-sys-config", "login-system-conf"), ("local-language-switch", "user-logout")),
    ("workbench.recent", "工作台", "最近访问标签", ("work-bench",), ("users-service-visit-list", "users-service-visit-delete")),
    ("workbench.all", "工作台", "全部应用标签", ("work-bench", "work-bench-all"), ("client-user-service-pageuserservice-endlesstype",)),
    ("workbench.cas", "工作台", "CAS 认证组标签", ("work-bench",), ("client-users-service-group-endlesstype",)),
    ("workbench.finance", "工作台", "经管应用标签", ("work-bench",), ("client-users-service-group-endlesstype",)),
    ("workbench.search", "工作台", "应用搜索", ("work-bench",), ("client-user-service-pageuserservice-endlesstype",)),
    ("workbench.sort", "工作台", "自定义排序", ("work-bench",), ("client-user-updateservicesort", "client-user-updateservicegroupsort")),
    ("workbench.app-menu", "工作台", "应用卡片下拉菜单", ("work-bench",), ("client-user-service-addtocustomgroup", "client-user-service-customgroupremoveservice", "client-user-deleteservicegroup", "client-users-service-grouping", "users-service-visit-add-id", "users-service-visit-delete")),
    ("workbench.new-group", "工作台", "新建应用分组弹窗", ("work-bench",), ("client-user-addservicegroup",)),
    ("workbench.manage-group", "工作台", "管理分组应用弹窗", ("work-bench",), ("client-user-updateservicegroup", "client-user-service-addtocustomgroup", "client-user-service-customgroupremoveservice")),
    ("workbench.tag", "工作台", "设置标签弹窗", ("work-bench",), ("client-user-updateservicegroup",)),
    ("workbench.frequent", "工作台", "设置常用位置", ("work-bench",), ("users-confirmnewcommonlocation", "users-updatecommonlocation")),
    ("workbench.uninstall", "工作台", "卸载应用确认", ("work-bench",), ("local-openSoftware", "local-manager-queryuninstallstatus-querystatus")),
    ("workbench.apply-resource", "工作台", "申请资源下拉菜单", ("work-bench",), ("approve-center-canapplytypes", "service-getalltemporaryservice", "users-service-getalltemporaryservice-ordervalue", "users-service-getallapplicabilityservice", "users-service-createserviceapply", "users-service-cancleserviceapply")),
    ("workbench.approval-center", "工作台", "审批中心入口", ("work-bench", "approve-center"), ("approve-center-groupcount",)),
    ("workbench.announcement", "工作台", "公告卡片与更多", ("work-bench", "notice-center"), ("client-banner-getbannerinfo-key", "users-message-page")),
    ("workbench.app-launch", "工作台", "应用卡片打开/跳转", ("work-bench", "work-bench-all"), ("users-url-open-url", "v1-user-getredirecturl", "v1-user-getusergroupedservicelist")),
    ("message.filters", "消息中心", "消息类型筛选", ("message-center",), ("users-message-page", "users-message-count")),
    ("message.detail", "消息中心", "消息详情", ("message-center",), ("users-message-get-messageid", "client-message-secauth-msgid", "client-message-callback")),
    ("message.read-all", "消息中心", "全部已读", ("message-center",), ("users-message-allread-type",)),
    ("approval.tabs", "审批中心", "申请/待办/已办标签", ("approve-center",), ("approve-center-applypage", "approve-center-waitinghandle", "approve-center-myhandle", "users-approve-center-getflow")),
    ("approval.detail", "审批中心", "申请详情/流程图", ("approve-center",), ("approve-center-applydetail", "approve-center-flowimage", "users-approve-center-getflow")),
    ("approval.deal", "审批中心", "审批处理", ("approve-center",), ("approve-center-deal",)),
    ("approval.form", "申请页面", "动态申请表单", ("apply-app", "account-apply", "usb-apply"), ("approve-center-applyformconf", "approve-center-addapply", "users-approve-addapply", "users-approve-addapply-notoken", "users-approve-center-adddeviceapp", "users-approve-center-approveuserinfos", "users-approve-center-validbeforeapplication")),
    ("app-market.search", "应用商店", "搜索/分类/分页", ("app-store", "app-market-detail"), ("appmarket-getsoftsearch", "appmarket-getsoftcategorylist", "appmarket-getsoftpage", "v1-appmarket-getpushsoftwaredata", "v1-appmarket-getsoftpagebeforelogin")),
    ("app-market.install", "应用商店", "下载/安装/更新/取消", ("app-store", "app-market-detail"), ("local-software-setup", "appmarket-softuploaddistributeresult", "v1-external-runexe")),
    ("app-market.uninstall", "应用商店", "卸载", ("app-store",), ("local-openSoftware", "local-manager-queryuninstallstatus-querystatus")),
    ("app-market.report", "应用商店", "分发结果/软件上报", ("app-store",), ("appmarket-softwareReport",)),
    ("security.scan", "安全中心", "快速/完整/自定义扫描", ("secure-center", "quick-scan", "custom-scan", "linux-quick-scan"), ("avengine-scan", "avengine-getresult", "v1-avengine-getavlist", "v1-avengine-findavs", "v1-safetyassess-getoverviewinfo", "v1-safetyassess-getdetailinfo", "v1-safetyassess-startask")),
    ("security.scan-control", "安全中心", "暂停/继续/停止扫描", ("safety-baseline",), ("safetyassess-pausetask", "safetyassess-continuetask", "safetyassess-stoptask")),
    ("security.repair", "安全中心", "单项/全部修复", ("safety-detail", "risk-result"), ("safetyassess-repair", "safetyassess-repairall", "avengine-solve")),
    ("security.quarantine", "安全中心", "隔离/恢复/删除", ("quarantine",), ("avengine-getisolator", "avengine-isolator", "avengine-solve")),
    ("security.trust", "安全中心", "信任区", ("trust-zone",), ("antivirus-trustfile-page",)),
    ("security.logs", "安全中心", "扫描日志导出", ("virus-logs",), ("avengine-getlog", "avengine-exportlog")),
    ("security.settings", "安全中心", "病毒库/隔离目录/引擎设置", ("virus-scan-setting",), ("avengine-baseinfoget", "avengine-baseinfoset", "avengine-setisolatorpath", "avengine-updateavlib", "v1-avengine-findavs", "v1-avengine-getavpath", "v1-api-users-device-terminal-antivirus-lib-libtype")),
    ("space.service", "安全空间", "空间开通/我的空间", ("secure-space",), ("safespace-getservice", "desktop-spacelist")),
    ("space.share", "安全空间", "文件分享/链接删除/下载", ("secure-space", "file-share", "file-link"), ("share-link-page", "share-link-delete-id", "safespace-getsharefilepage", "desktop-downloadsharefile", "client-public-files-download-filepath", "v1-desktop-endesktopproxy", "v1-desktop-filecirculate-spacename", "v1-desktop-filecirculatedownload", "v1-desktop-getallfilesharpath", "v1-desktop-getfilecirculatelist", "v1-desktop-getsharfiledowndata", "v1-desktop-newdownloadsharefile", "v1-desktop-opensharefiledir", "v1-desktop-redirectsvcrequest", "v1-desktop-startprocess")),
    ("terminal.info", "终端", "终端信息/外设", ("terminal-info",), ("local-device-info", "device-manager-getperipheraldata", "device-security-allowed", "v1-local-device-queryprocess")),
    ("terminal.control", "终端", "协议/通知/自启动/代理", ("home-sys-config", "terminal-info"), ("control-protocol", "control-notification", "control-autoboot", "nac-queryproxy", "nac-setproxy", "v1-control-detect", "v1-control-info", "v1-control-list", "v1-control-select", "v1-gateway-switch", "v1-gateway-turnon", "v1-local-config", "v1-local-getaction", "v1-local-info", "v1-local-polltags", "v1-local-terminal-doproject", "v1-local-terminal-doproject-time-0", "v1-local-terminal-scorerequest-mustcoll", "v1-local-terminal-setaction", "v1-local-writeconfig", "v1-local-writeconfigini", "v1-nac-login", "v1-nac-logout", "v1-nac-netisolation-type", "v1-nac-queryconfig", "v1-nac-queryinfo", "v1-user-selectcontrol")),
    ("remote-control", "客户端", "远程控制/补丁", ("long-range-control",), ("coms-getrcsinfo", "coms-getrcicinfo", "coms-patchlist", "coms-patchfix", "v1-coms-getrcisstate", "v1-coms-rcicswitch", "v1-coms-setrcisinfo")),
    ("user.account", "用户中心", "账号基本信息/重置密码", ("personal-center",), ("users-info", "user-detail", "auth-modifypwd", "client-user-center-resetuserpassword", "users-person-restname", "users-person-select-enableisopened", "v1-user-detail", "v1-user-getusergroupedservicelist", "v1-user-getredirecturl")),
    ("user.bindings", "用户中心", "手机邮箱/微信/企业应用绑定", ("personal-center", "bind-info"), ("person-getbindinfos", "person-getbindqr-authconfigid", "person-bindmobileemail", "bindwechatorens-find-bindstatus", "users-auth-bindmobileoremail", "users-bindmobileemail-send-code", "users-bindstatus-find-type", "users-bindwechatorens-find-bindstatus", "client-user-ditalk-bind")),
    ("user.devices", "用户中心", "活动设备/下线/解绑", ("personal-center", "device-unbind"), ("device-list-page", "center-device-offline", "center-device-unbind", "device-unbinduser", "users-device-bind-labels", "users-device-bindinfo-token-get", "users-device-getbyfeaturecode-featurecode", "users-device-labels-info", "users-center-device-offline", "users-center-device-unbind", "users-device-unbinduser", "users-terminal-csandnetworkcontrol", "users-unbind-check-code", "users-unbind-message-bytype", "users-unbind-send-code")),
    ("user.avatar", "用户中心", "头像查询/上传", ("personal-center",), ("static-avatar-query-username", "center-uploadpicture", "v1-static-avatar-set")),
    ("user.common-location", "用户中心", "常用位置", ("personal-center", "work-bench"), ("users-confirmnewcommonlocation", "users-updatecommonlocation")),
    ("protocol.state", "协议", "协议状态读取/设置", ("mac-privacy", "home-sys-config"), ("user-getuserprotocolstate", "user-setuserprotocolstate")),
    ("native.gateway", "客户端", "网关/NAC/本地代理", ("home-sys-config", "terminal-info"), ("gateway-switch", "gateway-turnon", "nac-login", "nac-logout", "nac-netisolation-type", "nac-queryconfig", "nac-queryinfo")),
)


VPN_CONTROL_CATALOG = tuple(
    {"name": name, "section": section, "label": label, "routes": list(routes), "apis": list(apis)}
    for name, section, label, routes, apis in _CONTROL_ROWS
)


def _read_json_file(path: Path, *, code: str, label: str) -> dict[str, object]:
    try:
        if path.is_symlink() or (path.exists() and not path.is_file()):
            raise OSError(f"{path} 不是普通文件或是符号链接")
        value = json.loads(path.read_text(encoding="utf-8")) if path.exists() else {}
    except (OSError, UnicodeError, ValueError) as exc:
        raise CsustError(f"无法读取{label} {path}: {exc}", code=code) from exc
    if not isinstance(value, dict):
        raise CsustError(f"{label} {path} 格式无效", code=code)
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass
    return value


def _parse_pairs(values: Iterable[str], option: str) -> list[tuple[str, str]]:
    result = []
    for value in values:
        if "=" not in value:
            raise CsustError(f"{option} 必须是 NAME=VALUE", code="invalid_argument")
        name, item = value.split("=", 1)
        if not name:
            raise CsustError(f"{option} 的名称不能为空", code="invalid_argument")
        result.append((name, item))
    return result


def _parse_files(values: Iterable[str]) -> list[tuple[str, object]]:
    parts: list[tuple[str, object]] = []
    for name, value in _parse_pairs(values, "--file"):
        path = Path(value).expanduser()
        try:
            if path.is_symlink() or not path.is_file():
                raise OSError("必须是普通文件且不能是符号链接")
            content = path.read_bytes()
        except (OSError, UnicodeError) as exc:
            raise CsustError(f"无法读取上传文件 {path}: {exc}", code="invalid_argument") from exc
        content_type = mimetypes.guess_type(path.name)[0] or "application/octet-stream"
        parts.append((name, (path.name, content, content_type)))
    return parts


def _json_argument(value: str | None) -> object:
    if value is None:
        return {}
    if value == "-":
        value = sys.stdin.read()
    elif value.startswith("@"):
        try:
            value = Path(value[1:]).read_text(encoding="utf-8")
        except (OSError, UnicodeError) as exc:
            raise CsustError(f"无法读取 JSON 参数文件：{exc}", code="invalid_argument") from exc
    try:
        parsed = json.loads(value)
    except (TypeError, ValueError) as exc:
        raise CsustError("--data-json 必须是有效 JSON，或使用 @FILE/−", code="invalid_argument") from exc
    if not isinstance(parsed, (dict, list)) and parsed is not None:
        raise CsustError("--data-json 顶层必须是对象、数组或 null", code="invalid_argument")
    return parsed


def _redact(value: object) -> object:
    sensitive = re.compile(r"pass|password|passwd|token|secret|cookie|session|captcha|sign|ticket|nonce|csrf", re.I)
    if isinstance(value, dict):
        return {str(key): ("<redacted>" if sensitive.search(str(key)) else _redact(item)) for key, item in value.items()}
    if isinstance(value, list):
        return [_redact(item) for item in value]
    return value


class VpnClient(Client):
    """The same safe urllib client, with the EnUES JSON headers and token."""

    def __init__(
        self,
        base_url: str | None = None,
        cookie_file: Path | None = None,
        session_file: Path | None = None,
        *,
        native: bool = False,
        load_cookies: bool = True,
    ) -> None:
        self.native = native
        configured = base_url or env_value("CSUST_VPN_NATIVE_URL" if native else "CSUST_VPN_BASE_URL", VPN_NATIVE_URL if native else VPN_BASE_URL)
        cookie = cookie_file or Path(env_value("CSUST_VPN_COOKIE_FILE", str(Path.home() / ".config" / "csust-cli" / "vpn-cookies.txt"))).expanduser()
        self.session_file = Path(session_file or env_value("CSUST_VPN_SESSION_FILE", str(Path.home() / ".config" / "csust-cli" / "vpn-session.json"))).expanduser()
        super().__init__(configured, cookie, load_cookies=load_cookies)
        self.session = _read_json_file(self.session_file, code="vpn_session_read_failed", label="VPN 会话文件") if self.session_file.exists() else {}
        self.pending_token = ""

    def _api_path(self, path: str) -> str:
        if not isinstance(path, str) or not path.strip():
            raise CsustError("VPN API 路径不能为空", code="invalid_path")
        path = path.strip()
        if path.lower().startswith(("http://", "https://")):
            target = self.url(path)
            if urlsplit(target).netloc.lower() != urlsplit(self.base_url).netloc.lower():
                raise CsustError("VPN API 只允许访问当前站点", code="invalid_path")
            return target
        if self.native:
            if path.startswith("/enclient/"):
                path = path[len("/enclient"):]
            return path if path.startswith("/api/") else "/api/v1/" + path.lstrip("/")
        if path.startswith("/enclient/"):
            return path
        return "/enclient/" + path.lstrip("/")

    def _headers(self) -> dict[str, str]:
        language = env_value("CSUST_VPN_LANGUAGE", "zh")
        accept_language = {
            "en": "en,zh-CN;q=0.9,zh;q=0.8",
            "th": "th-TH,en;q=0.8;q=0.9,en-GB;q=0.7;q=0.8,en-US;q=0.6;q=0.7",
        }.get(language, "zh-CN,zh;q=0.9,en;q=0.8")
        use_mode = "1" if env_value("CSUST_VPN_REGISTERED", "false").lower() == "true" else "0"
        cookie_values = ["BROWSER_LOGIN=1", f"UseMode={use_mode}"]
        cookie_values.extend(
            f"{cookie.name}={cookie.value}"
            for cookie in self.cookies
            if cookie.name not in {"BROWSER_LOGIN", "UseMode"}
        )
        headers = {
            "Accept": "application/json,text/plain,*/*",
            "Content-Type": "application/json",
            "Accept-Language": accept_language,
            "ajax-Flow": "ajaxFlow",
            "UseMode": use_mode,
            "Cookie": "; ".join(cookie_values),
            "userProtocolState": "1",
            "Referer": self.url(VPN_START_PATH),
            "Origin": self.base_url,
        }
        feature_code = env_value("CSUST_VPN_FEATURE_CODE")
        device_info = env_value("CSUST_VPN_DEVICE_INFO")
        if feature_code:
            headers["featureCode"] = feature_code
        if device_info:
            headers["Device-Info"] = device_info
        token = self.session.get("token") or self.pending_token
        if isinstance(token, str) and token:
            headers["Authorization"] = "Bearer " + token
        return headers

    def save_session(self) -> None:
        _write_private_file(
            self.session_file,
            (json.dumps(self.session, ensure_ascii=False, indent=2) + "\n").encode("utf-8"),
            code="vpn_session_write_failed",
            label="VPN 会话文件",
        )

    def clear_session(self) -> None:
        self.session = {}
        self.save_session()

    def _set_tokens(self, data: object) -> None:
        if not isinstance(data, dict):
            return
        token = data.get("token") or data.get("entoken")
        refresh = data.get("refreshToken")
        if isinstance(token, str) and token:
            self.session["token"] = token
        if isinstance(refresh, str) and refresh:
            self.session["refreshToken"] = refresh
        for key in ("account", "username", "name", "userId", "id"):
            if key in data and isinstance(data[key], (str, int)):
                self.session[key] = data[key]

    def refresh(self) -> bool:
        if not self.session.get("refreshToken"):
            return False
        response = self.request(self._api_path(VPN_REFRESH_PATH), method="GET", headers=self._headers())
        _save_cookie_refresh(self, response)
        value = _response_json(response)
        if not isinstance(value, dict) or str(value.get("code")) != "200":
            return False
        self._set_tokens(value.get("data"))
        self.save_session()
        return bool(self.session.get("token"))

    def request_api(
        self,
        path: str,
        *,
        method: str,
        data: object | None = None,
        multipart: list[tuple[str, object]] | None = None,
        params: Iterable[tuple[str, str]] = (),
        path_args: Iterable[str] = (),
        binary: bool = False,
        retry_refresh: bool = True,
    ) -> tuple[Response, object | None]:
        path = _resolve_path(path, params, path_args)
        target = self._api_path(path)
        request_options: dict[str, object] = {
            "method": method,
            "headers": self._headers(),
            "binary": binary,
            "with_metadata": True,
        }
        if method not in {"GET", "HEAD", "OPTIONS"}:
            if multipart is not None:
                request_options["multipart"] = multipart
            else:
                request_options["json_body"] = data
        response = self.request(target, **request_options)
        assert isinstance(response, Response)
        _save_cookie_refresh(self, response)
        value = _response_json(response)
        if retry_refresh and not self.native and path.rstrip("/") != VPN_REFRESH_PATH.rstrip("/") and _response_code(value) == "3010":
            if self.refresh():
                return self.request_api(path, method=method, data=data, multipart=multipart, params=(), path_args=(), binary=binary, retry_refresh=False)
        if isinstance(value, dict):
            previous = dict(self.session)
            self._set_tokens(value.get("data"))
            if self.session != previous:
                self.save_session()
        return response, value

    def call_spec(
        self,
        spec: VpnApiSpec,
        *,
        data: object | None,
        multipart: list[tuple[str, object]] | None = None,
        params: Iterable[tuple[str, str]],
        path_args: Iterable[str],
        output: str | None = None,
        require_confirmation: bool = True,
        yes: bool = False,
    ) -> dict[str, object]:
        if require_confirmation and spec.mutating and not yes:
            raise CsustError(f"{spec.name} 可能改变远端状态，请加 --yes", code="confirmation_required")
        response, value = self.request_api(
            spec.path,
            method=spec.method,
            data=data,
            multipart=multipart,
            params=params,
            path_args=path_args,
            binary=bool(output) or spec.binary,
        )
        request_info = {"name": spec.name, "method": spec.method, "path": spec.path, "native": self.native}
        from .web import _feedback

        payload = _redact(value if value is not None else _feedback(response))
        if business_state(payload, success_codes=("200",)) is False:
            result_status(payload, mutating=spec.mutating, details={"request": request_info}, success_codes=("200",))
        if value is None:
            require_logged_in(response)
        if output:
            body = response.body if isinstance(response.body, bytes) else str(response.body).encode("utf-8")
            _write_private_file(Path(output).expanduser(), body, code="vpn_output_write_failed", label="VPN 响应")
            saved = {"request": request_info, "status": response.status, "downloaded": True, "output": str(Path(output).expanduser()), "bytes": len(body)}
            return {**saved, **result_status(payload, mutating=spec.mutating, details=saved, success_codes=("200",))}
        return {**result_status(payload, mutating=spec.mutating, details={"request": request_info}, success_codes=("200",)), "request": request_info, "status": response.status, "response": payload}


def _response_json(response: Response) -> object | None:
    body = _decode_body(response.body, response.headers)
    try:
        return json.loads(body)
    except (TypeError, ValueError) as exc:
        if "json" in _header_value(response.headers, "Content-Type").lower():
            raise CsustError("VPN 返回了无效 JSON", code="parse_error") from exc
        return None


def _response_code(value: object) -> str | None:
    return str(value.get("code")) if isinstance(value, dict) and "code" in value else None


def _resolve_path(path: str, params: Iterable[tuple[str, str]], path_args: Iterable[str]) -> str:
    args = list(path_args)
    placeholders = re.findall(r"\{[^}]+\}", path)
    if placeholders:
        if len(args) < len(placeholders):
            raise CsustError(f"路径需要 {len(placeholders)} 个 --path-arg", code="invalid_argument")
        for placeholder, value in zip(placeholders, args):
            path = path.replace(placeholder, quote(value, safe=""), 1)
        args = args[len(placeholders):]
    split = urlsplit(path)
    if args:
        split = split._replace(path=split.path.rstrip("/") + "/" + "/".join(quote(value, safe="") for value in args))
    supplied = list(params)
    names = {key for key, _ in supplied}
    pairs = [(key, value) for key, value in parse_qsl(split.query, keep_blank_values=True) if key not in names]
    return urlunsplit((split.scheme, split.netloc, split.path, urlencode(pairs + supplied, doseq=True), split.fragment))


def _spec_by_name(name: str) -> VpnApiSpec:
    key = name.strip().lower()
    spec = VPN_API_BY_NAME.get(key)
    if spec is None:
        raise CsustError(f"未知 VPN API：{name}；先运行 csust vpn catalog", code="unknown_api")
    return spec


def _client(client: Client | None, *, native: bool = False) -> VpnClient:
    if isinstance(client, VpnClient) and client.native == native:
        return client
    return VpnClient(native=native)


def _encrypt_password(password: str, login_key: str) -> str:
    if len(login_key.encode("utf-8")) != 16:
        raise CsustError("VPN 登录密钥不是 16 字节，无法复现门户 AES 登录协议", code="vpn_protocol_error")
    try:
        from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
        from cryptography.hazmat.primitives.padding import PKCS7
    except ImportError as exc:
        raise CsustError("VPN 登录需要 cryptography 依赖", code="dependency_missing", details={"package": "cryptography"}) from exc
    padder = PKCS7(128).padder()
    padded = padder.update(password.encode("utf-8")) + padder.finalize()
    cipher = Cipher(algorithms.AES(login_key.encode("utf-8")), modes.CBC(login_key[::-1].encode("utf-8")))
    encryptor = cipher.encryptor()
    return base64.b64encode(encryptor.update(padded) + encryptor.finalize()).decode("ascii")


def _cas_enabled(config: object) -> bool:
    if not isinstance(config, dict) or not isinstance(config.get("data"), dict):
        return False
    data = config["data"]
    if str(data.get("defaultAuthType", "")).upper() in {"CAS", "SSO_CAS"}:
        return True
    login_types = data.get("ssoLoginTypes")
    return any(
        isinstance(item, dict)
        and str(item.get("loginType", "")).upper() == "SSO_CAS"
        and not item.get("hidden")
        for item in (login_types if isinstance(login_types, list) else [])
    )


def _cas_form(response: Response, account: str, password: str, captcha_info: str | None) -> tuple[str, list[tuple[str, str]]]:
    document = parse_html(_decode_body(response.body, response.headers))
    form = document.first("form", element_id="pwdFromId")
    if form is None:
        raise AuthenticationFailed("统一认证登录页缺少账号密码表单", details={"url": response.url})
    fields: list[tuple[str, str]] = []
    for element in form.find_all("input"):
        name = element.attr("name").strip()
        input_type = element.attr("type", "text").lower()
        if not name or element.is_disabled() or input_type in {"button", "file", "reset", "submit"}:
            continue
        if input_type in {"checkbox", "radio"} and "checked" not in element.attrs:
            continue
        if name not in {"username", "password", "passwordText"}:
            fields.append((name, element.attr("value")))
    salt_input = form.first("input", element_id="pwdEncryptSalt")
    salt = salt_input.attr("value") if salt_input is not None else ""
    fields.extend((("username", account), ("password", _encrypt_cas_password(password, salt))))
    if captcha_info:
        parsed = _json_argument(captcha_info)
        if not isinstance(parsed, dict):
            raise CsustError("--captcha-info 必须是 JSON 对象", code="invalid_argument")
        captcha = parsed.get("captcha") or parsed.get("code") or parsed.get("value")
        if captcha:
            fields.append(("captcha", str(captcha)))
    action = urljoin(response.url, form.attr("action") or response.url)
    service = next((value for key, value in parse_qsl(urlsplit(response.url).query, keep_blank_values=True) if key == "service"), "")
    action_parts = urlsplit(action)
    action_query = parse_qsl(action_parts.query, keep_blank_values=True)
    if service and not any(key == "service" for key, _value in action_query):
        action_query.append(("service", service))
        action = urlunsplit(action_parts._replace(query=urlencode(action_query, doseq=True)))
    return action, fields


def _cas_login(vpn: VpnClient, account: str, password: str, config: object, captcha_info: str | None) -> dict[str, object]:
    data = config.get("data") if isinstance(config, dict) else None
    configured = data.get("casLoginUrl") if isinstance(data, dict) else None
    cas_url = vpn.url(str(configured or "/enclient/api/users/admin/custom/page/login/sso/cas"))
    target_parts = urlsplit(cas_url)
    base_parts = urlsplit(vpn.base_url)
    if (target_parts.scheme, target_parts.hostname) != (base_parts.scheme, base_parts.hostname):
        raise CsustError("统一认证地址不是当前 VPN 站点", code="invalid_path")
    cas_headers = vpn._headers()
    cas_headers.pop("Cookie", None)
    first = vpn.request(cas_url, method="GET", headers={**cas_headers, "Accept": "text/html,application/xhtml+xml"}, with_metadata=True)
    assert isinstance(first, Response)
    _save_cookie_refresh(vpn, first)
    action, fields = _cas_form(first, account, password, captcha_info)
    headers = dict(cas_headers)
    headers.update(
        {
            "Accept": "text/html,application/xhtml+xml",
            "Content-Type": "application/x-www-form-urlencoded",
            "Referer": first.url,
            "Origin": vpn.base_url,
        }
    )
    response = vpn.request(action, method="POST", data=fields, headers=headers, with_metadata=True)
    assert isinstance(response, Response)
    _save_cookie_refresh(vpn, response)
    value = _response_json(response)
    if isinstance(value, dict):
        code = _response_code(value)
        if code not in {None, "200"}:
            raise AuthenticationFailed(str(value.get("message") or value.get("messages") or "统一认证登录失败"), details={"code": code})
        vpn._set_tokens(value.get("data"))
    document = parse_html(_decode_body(response.body, response.headers))
    if document.first("form", element_id="pwdFromId") is not None or "/authserver/login" in urlsplit(response.url).path:
        error = document.first("span", element_id="showErrorTip")
        message = error.text() if error is not None else "账号、密码错误或需要验证码"
        raise AuthenticationFailed(f"统一认证登录失败：{_safe_terminal_text(message)[:160]}")
    info_spec = _spec_by_name(_api_name(VPN_INFO_PATH))
    _info_response, info = vpn.request_api(info_spec.path, method=info_spec.method, retry_refresh=False)
    if not isinstance(info, dict) or _response_code(info) not in {None, "200"}:
        raise AuthenticationFailed("统一认证回调成功，但 VPN 会话未建立", details={"response": _redact(info)})
    vpn.session["account"] = account
    vpn.session["auth"] = "cas"
    vpn.save_session()
    return {"ok": True, "username": account, "auth": "cas", "session_file": str(vpn.session_file), "user": _redact(info.get("data"))}


def _login_password(args: argparse.Namespace) -> str:
    if getattr(args, "password_stdin", False):
        return sys.stdin.readline().rstrip("\r\n")
    return _credentials(getattr(args, "username", None))[1]


def login(args: argparse.Namespace, client: Client | None = None) -> dict[str, object]:
    vpn = _client(client)
    account = (getattr(args, "username", None) or env_value("CSUST_USERNAME", env_value("username"))).strip()
    password = _login_password(args)
    if not account or not password:
        raise CredentialsRequired("缺少 VPN 登录账号或密码（可用 CSUST_USERNAME/CSUST_PASSWORD、.env 或 --password-stdin）")
    vpn.session = {}
    vpn.pending_token = ""
    vpn.clear_cookies()
    config_spec = _spec_by_name(_api_name(VPN_CONFIG_PATH))
    _response, config = vpn.request_api(config_spec.path, method=config_spec.method, data={}, retry_refresh=False)
    if isinstance(config, dict) and _response_code(config) not in {None, "200"}:
        raise AuthenticationFailed(str(config.get("messages") or config.get("message") or "VPN 登录配置获取失败"), details={"code": config.get("code")})
    auth = getattr(args, "auth", "auto")
    if auth not in {"auto", "cas", "local"}:
        raise CsustError("--auth 必须是 auto、cas 或 local", code="invalid_argument")
    if auth == "cas" or (auth == "auto" and _cas_enabled(config)):
        return _cas_login(vpn, account, password, config, getattr(args, "captcha_info", None))
    _response, key_value = vpn.request_api(VPN_KEY_PATH, method="GET", retry_refresh=False)
    if not isinstance(key_value, dict) or _response_code(key_value) != "200":
        raise AuthenticationFailed("VPN 登录密钥获取失败", details={"response": _redact(key_value)})
    token_seed = key_value.get("data", {}).get("enToken") if isinstance(key_value.get("data"), dict) else None
    if not isinstance(token_seed, str):
        raise AuthenticationFailed("VPN 登录密钥响应格式无效")
    pieces = token_seed.split("-")
    login_key = pieces[2] if len(pieces) >= 3 else pieces[-1]
    vpn.pending_token = token_seed
    payload: dict[str, object] = {"account": account, "passwd": _encrypt_password(password, login_key)}
    captcha_info = getattr(args, "captcha_info", None)
    if captcha_info:
        parsed_captcha = _json_argument(captcha_info)
        if not isinstance(parsed_captcha, dict):
            raise CsustError("--captcha-info 必须是 JSON 对象", code="invalid_argument")
        payload["captchaInfo"] = parsed_captcha
    spec = _spec_by_name(_api_name(VPN_LOGIN_PATH))
    _response, result = vpn.request_api(spec.path, method=spec.method, data=payload, retry_refresh=False)
    if not isinstance(result, dict):
        raise AuthenticationFailed("VPN 登录返回格式无效")
    code = _response_code(result)
    if code != "200":
        return {"ok": False, "username": account, "code": code, "message": result.get("messages") or result.get("message"), "next": "second-auth" if code in {"2050", "2051", "2060", "2080", "4010", "4020", "4030", "4040"} else None, "response": _redact(result.get("data"))}
    vpn._set_tokens(result.get("data"))
    vpn.session["account"] = account
    vpn.save_session()
    info_spec = _spec_by_name(_api_name(VPN_INFO_PATH))
    _response, info = vpn.request_api(info_spec.path, method=info_spec.method, retry_refresh=False)
    return {"ok": True, "username": account, "session_file": str(vpn.session_file), "user": _redact(info.get("data") if isinstance(info, dict) else info)}


def logout(args: argparse.Namespace, client: Client | None = None) -> dict[str, object]:
    vpn = _client(client, native=bool(getattr(args, "native", False)))
    try:
        spec = _spec_by_name(_api_name(VPN_LOGOUT_PATH))
        result = vpn.call_spec(spec, data={}, params=(), path_args=(), require_confirmation=False, yes=True)
    finally:
        vpn.clear_cookies()
        vpn.save()
        vpn.clear_session()
    return {"ok": True, "logged_out": True, "session_file": str(vpn.session_file), "remote": result if "result" in locals() else None}


def _has_session_cookie(vpn: VpnClient) -> bool:
    return any(cookie.name.lower() in {"access_token", "jsessionid", "session", "sessionid", "castgc"} for cookie in vpn.cookies)


def run_status(args: argparse.Namespace, client: Client | None = None) -> dict[str, object]:
    vpn = _client(client, native=bool(getattr(args, "native", False)))
    if not vpn.session.get("token") and not _has_session_cookie(vpn):
        return {"ok": True, "logged_in": False, "native": vpn.native, "session_file": str(vpn.session_file)}
    spec = _spec_by_name(_api_name(VPN_INFO_PATH))
    result = vpn.call_spec(spec, data=None, params=(), path_args=(), require_confirmation=False, yes=True)
    return {"ok": bool(result.get("ok")), "logged_in": bool(result.get("ok")), "native": vpn.native, "session_file": str(vpn.session_file), "user": result.get("response")}


def run_routes(_args: argparse.Namespace, _client: Client | None = None) -> dict[str, object]:
    return {"source": "https://vpn.csust.edu.cn/enclient/start.html", "routes": list(VPN_ROUTE_CATALOG), "route_count": len(VPN_ROUTE_CATALOG), "api_count": len(VPN_API_CATALOG)}


def run_controls(_args: argparse.Namespace, _client: Client | None = None) -> dict[str, object]:
    return {"controls": list(VPN_CONTROL_CATALOG), "control_count": len(VPN_CONTROL_CATALOG), "api_count": len(VPN_API_CATALOG)}


def run_catalog(args: argparse.Namespace, _client: Client | None = None) -> dict[str, object]:
    items = list(VPN_API_CATALOG)
    group = getattr(args, "group", None)
    if group:
        items = [item for item in items if item.group == group]
    return {"source": "https://vpn.csust.edu.cn/enclient/start.html", "catalog": [item.__dict__ for item in items], "api_count": len(items), "resources": list(VPN_RESOURCE_CATALOG)}


def _route_key(value: str) -> str:
    try:
        split = urlsplit(value)
    except ValueError:
        return value.casefold()
    return urlunsplit(("", "", split.path.rstrip("/") or "/", split.query, split.fragment)).casefold()


def run_page(args: argparse.Namespace, _client: Client | None = None) -> dict[str, object]:
    wanted = str(args.route).strip()
    wanted_key = _route_key(wanted)
    matches = [
        item
        for item in VPN_ROUTE_CATALOG
        if item["name"].casefold() == wanted.casefold()
        or _route_key(str(item["path"])) == wanted_key
        or wanted.casefold() in {str(alias).casefold() for alias in item["aliases"]}
    ]
    if len(matches) > 1:
        raise CsustError(f"VPN 路由别名有歧义：{wanted}；请使用完整路径或路由名", code="ambiguous_route")
    if not matches:
        raise CsustError(f"未知 VPN 路由：{wanted}；先运行 csust vpn routes", code="unknown_route")
    match = matches[0]
    route_names = {match["name"]}
    base_path = urlsplit(str(match["path"])).path.lower().rstrip("/") or "/"
    route_names.update(
        item["name"]
        for item in VPN_ROUTE_CATALOG
        if urlsplit(str(item["path"])).path.lower().rstrip("/") == base_path
    )
    return {"route": match, "controls": [item for item in VPN_CONTROL_CATALOG if route_names.intersection(item["routes"])]}


def run_api(args: argparse.Namespace, client: Client | None = None) -> dict[str, object]:
    vpn = _client(client, native=bool(getattr(args, "native", False)))
    if args.name and args.path:
        raise CsustError("--name 与 --path 不能同时使用", code="invalid_argument")
    if args.name:
        spec = _spec_by_name(args.name)
        path = spec.path
        method = spec.method
    elif args.path:
        path = args.path
        method = (args.method or "GET").upper()
        spec = VpnApiSpec(_api_name(path), method, path, _api_group(path), _is_mutating(method, path), False)
    else:
        raise CsustError("--name 与 --path 至少指定一个", code="invalid_argument")
    if method not in {"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"}:
        raise CsustError("不支持的 HTTP 方法", code="invalid_argument")
    multipart = [(name, value) for name, value in _parse_pairs(getattr(args, "form", ()), "--form")]
    multipart.extend(_parse_files(getattr(args, "file", ())))
    if multipart and args.data_json is not None:
        raise CsustError("--data-json 不能与 --form/--file 同时使用", code="invalid_argument")
    if method in {"GET", "HEAD", "OPTIONS"}:
        if args.data_json is not None or multipart:
            raise CsustError("GET/HEAD/OPTIONS 只能使用 --param", code="invalid_argument")
        data = None
        multipart = None
    else:
        data = None if multipart else _json_argument(args.data_json)
    return vpn.call_spec(spec, data=data, multipart=multipart, params=_parse_pairs(args.param, "--param"), path_args=args.path_arg, output=args.output, yes=args.yes)


def run_resource(args: argparse.Namespace, client: Client | None = None) -> dict[str, object]:
    resource = next((item for item in VPN_RESOURCE_CATALOG if item["name"] == args.name), None)
    if resource is None:
        raise CsustError(f"未知 VPN 资源：{args.name}", code="unknown_resource")
    if not args.path:
        return {"resource": resource}
    if not args.output:
        raise CsustError("下载资源时必须提供 --output", code="invalid_argument")
    vpn = _client(client, native=bool(getattr(args, "native", False)))
    template = resource["native_path"] if vpn.native else resource["web_path"]
    if not template:
        raise CsustError(f"资源 {resource['name']} 不支持 native 下载", code="unsupported")
    path = str(template).replace("{path}", args.path.lstrip("/"))
    response = vpn.request(vpn._api_path(path), method="GET", headers=vpn._headers(), binary=True, with_metadata=True)
    assert isinstance(response, Response)
    _save_cookie_refresh(vpn, response)
    require_logged_in(response)
    result_status(_response_json(response), mutating=False, success_codes=("200",))
    body = response.body if isinstance(response.body, bytes) else str(response.body).encode("utf-8")
    output = Path(args.output).expanduser()
    _write_private_file(output, body, code="vpn_output_write_failed", label="VPN 资源")
    return {"ok": True, "resource": resource["name"], "output": str(output), "bytes": len(body)}


def run_sso_url(args: argparse.Namespace, client: Client | None = None) -> dict[str, object]:
    vpn = _client(client)
    response, config = vpn.request_api(VPN_CONFIG_PATH, method="POST", data={}, retry_refresh=False)
    configured = config.get("data", {}).get("casLoginUrl") if isinstance(config, dict) and isinstance(config.get("data"), dict) else None
    configured = configured or "/enclient/api/users/admin/custom/page/login/sso/cas"
    return {"ok": True, "url": vpn.url(str(configured)), "config": _redact(config)}


def _add_json_flag(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--json", action="store_true", default=argparse.SUPPRESS, help="输出 JSON")


def register(subparsers: argparse._SubParsersAction) -> None:
    vpn = subparsers.add_parser("vpn", help="调用 vpn.csust.edu.cn EnUES 门户的完整 SPA/API 能力")
    children = vpn.add_subparsers(dest="vpn_command", required=True)

    login_parser = children.add_parser("login", help="登录 VPN 门户并保存会话")
    login_parser.add_argument("--username", help="账号；密码从 CSUST_PASSWORD/.env 读取")
    login_parser.add_argument("--password-stdin", action="store_true", help="从标准输入读取一行密码")
    login_parser.add_argument("--auth", choices=("auto", "cas", "local"), default="auto", help="认证方式；auto 按门户配置优先使用 CAS")
    login_parser.add_argument("--captcha-info", help="当门户启用图形/滑块验证码时传入 JSON 对象")
    _add_json_flag(login_parser)
    login_parser.set_defaults(feature_runner=login, feature_renderer=render)

    logout_parser = children.add_parser("logout", help="退出 VPN 门户并清除本机会话")
    logout_parser.add_argument("--native", action="store_true", help="调用本机 EnUES 服务")
    _add_json_flag(logout_parser)
    logout_parser.set_defaults(feature_runner=logout, feature_renderer=render)

    status_parser = children.add_parser("status", help="检查 VPN 会话和当前用户")
    status_parser.add_argument("--native", action="store_true", help="检查本机 EnUES 服务")
    _add_json_flag(status_parser)
    status_parser.set_defaults(feature_runner=run_status, feature_renderer=render)

    routes = children.add_parser("routes", help="列出全部 SPA 页面路由和页面级 API 映射")
    _add_json_flag(routes)
    routes.set_defaults(feature_runner=run_routes, feature_renderer=render)

    controls = children.add_parser("controls", help="列出菜单、标签、弹窗、下拉项和条件控件映射")
    _add_json_flag(controls)
    controls.set_defaults(feature_runner=run_controls, feature_renderer=render)

    catalog = children.add_parser("catalog", help="列出从当前门户 bundle 提取的全部 API")
    catalog.add_argument("--group", help="按分组过滤")
    _add_json_flag(catalog)
    catalog.set_defaults(feature_runner=run_catalog, feature_renderer=render)

    page = children.add_parser("page", help="查看一个 SPA 页面及其全部控件/API 映射")
    page.add_argument("--route", required=True, help="路由名或 /home/... 路径")
    _add_json_flag(page)
    page.set_defaults(feature_runner=run_page, feature_renderer=render)

    api = children.add_parser("api", help="按目录名称或同源路径发送 JSON API 请求")
    api.add_argument("--name", help="catalog 中的 API 名称")
    api.add_argument("--path", help="同源 API 路径；用于 bundle 未列出的新接口")
    api.add_argument("--method", help="使用 --path 时指定 HTTP 方法，默认 GET")
    api.add_argument("--data-json", "--data", dest="data_json", help="JSON 请求体；可用 @FILE 或 -")
    api.add_argument("--form", action="append", default=[], help="multipart 文本字段 NAME=VALUE，可重复")
    api.add_argument("--file", action="append", default=[], help="multipart 文件字段 NAME=PATH，可重复")
    api.add_argument("--param", action="append", default=[], help="查询参数 NAME=VALUE，可重复")
    api.add_argument("--path-arg", action="append", default=[], help="填充路径模板参数，可重复")
    api.add_argument("--output", help="原样保存二进制/导出响应")
    api.add_argument("--yes", action="store_true", help="确认可能改变远端状态的请求")
    api.add_argument("--native", action="store_true", help="调用 127.0.0.1:30303 本机 EnUES 服务")
    _add_json_flag(api)
    api.set_defaults(feature_runner=run_api, feature_renderer=render)

    resource = children.add_parser("resource", help="列出或下载门户图片、文件和协议资源")
    resource.add_argument("--name", required=True, choices=[item["name"] for item in VPN_RESOURCE_CATALOG])
    resource.add_argument("--path", help="资源路径；不填则只返回映射")
    resource.add_argument("--output", help="保存文件路径")
    resource.add_argument("--native", action="store_true", help="调用本机资源路径")
    _add_json_flag(resource)
    resource.set_defaults(feature_runner=run_resource, feature_renderer=render)

    sso = children.add_parser("sso-url", help="读取当前配置并返回 CAS 单点登录入口")
    _add_json_flag(sso)
    sso.set_defaults(feature_runner=run_sso_url, feature_renderer=render)


def render(data: dict[str, object]) -> None:
    if "catalog" in data:
        for item in data.get("catalog", []):
            if isinstance(item, dict):
                print("\t".join(_safe_terminal_text(item.get(key)) for key in ("name", "group", "method", "path", "mutating")))
        return
    if "routes" in data:
        for item in data.get("routes", []):
            if isinstance(item, dict):
                print("\t".join(_safe_terminal_text(item.get(key)) for key in ("name", "section", "path", "description")))
        return
    if "controls" in data:
        for item in data.get("controls", []):
            if isinstance(item, dict):
                print("\t".join(_safe_terminal_text(item.get(key)) for key in ("name", "section", "label")))
        return
    if data.get("downloaded"):
        print(f"已保存：{_safe_terminal_text(data.get('output'))}（{_safe_terminal_text(data.get('bytes'))} bytes）")
        return
    if data.get("logged_in") is False:
        print("VPN 未登录")
        return
    if data.get("logged_out"):
        print("VPN 已退出登录")
        return
    if data.get("ok") and data.get("username"):
        print(f"VPN 登录成功：{_safe_terminal_text(data.get('username'))}")
        print(f"会话已保存：{_safe_terminal_text(data.get('session_file'))}")
        return
    if "url" in data:
        print(_safe_terminal_text(data.get("url")))
        return
    if "resource" in data and len(data) == 1:
        print(json.dumps(data, ensure_ascii=False))
        return
    print(json.dumps(data, ensure_ascii=False))


if __name__ == "__main__":
    # ponytail: the catalog is the single executable surface; use the CLI for
    # integration checks rather than adding a second demo harness.
    raise SystemExit("use csust vpn ...")
