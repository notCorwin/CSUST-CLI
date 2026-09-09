package adapter

import (
	"net/url"
	"regexp"
	"strings"
)

type vpnAPISpec struct {
	method string
	path   string
}

type vpnRoute struct {
	name        string
	section     string
	path        string
	description string
	apis        []string
}

type vpnControl struct {
	name    string
	section string
	label   string
	routes  []string
	apis    []string
}

var vpnAPIRows = []vpnAPISpec{
	{"POST", "/api/users/auth/login"},
	{"POST", "/api/users/approve/center/addDeviceApp"},
	{"POST", "/api/users/custom/page/login/cfg/select"},
	{"POST", "/api/users/basic/captcha"},
	{"POST", "/api/users/reset/password/find/verify/type"},
	{"POST", "/api/users/reset/password/send/code"},
	{"POST", "/api/users/reset/password/terminal/verify/code"},
	{"POST", "/api/users/reset/password/verify/code"},
	{"POST", "/api/users/auth/alertAuth"},
	{"POST", "/api/users/bindMobileEmail/send/code"},
	{"POST", "/api/users/auth/bindMobileOrEmail"},
	{"POST", "/api/users/bindWeChatOrEns/find/bindStatus"},
	{"POST", "/api/users/bindStatus/find/type"},
	{"GET", "/api/users/device/bindInfo/token/get"},
	{"POST", "/api/users/sim/send/code"},
	{"POST", "/api/users/flow/path"},
	{"POST", "/api/users/auth/logout"},
	{"POST", "/api/users/noPassLogin/send/code"},
	{"POST", "/api/users/auth/noPasswordLogin"},
	{"GET", "/api/users/client/auth/generateKey"},
	{"POST", "/api/users/findFeishuConf"},
	{"GET", "/api/users/auth/lark/getQrCode/{id}"},
	{"GET", "/api/users/weChat/getWechatQrCode"},
	{"GET", "/api/users/client/enapp/getCoderQr"},
	{"POST", "/api/users/client/enapp/getCoderQrStatus"},
	{"POST", "/api/users/auth/enAppQrCodeLogin"},
	{"POST", "/api/users/findDiTalkConf"},
	{"GET", "/api/users/auth/dingtalk/getQrCode/{id}"},
	{"GET", "/api/users/weChat/getWechatEnsQrCode"},
	{"GET", "/api/users/auth/weChatEns/getQrCode/{id}"},
	{"POST", "/api/users/v3/qrCode/generate"},
	{"GET", "/api/users/V3/qrCode/result/{id}"},
	{"POST", "/api/users/auth/qrCode"},
	{"POST", "/api/users/auth/oneClick"},
	{"POST", "/api/users/auth/oneClickAuth"},
	{"POST", "/api/users/auth/modifyPwd"},
	{"POST", "/api/users/second/send/code"},
	{"POST", "/api/users/auth/secondAuth"},
	{"POST", "/api/users/sim/roll/result"},
	{"POST", "/api/users/auth/simAuth"},
	{"POST", "/api/users/iam/SecondAuth/getSecondAuthTpye"},
	{"POST", "/api/users/doCBAuth"},
	{"POST", "/api/users/doAgainCBAuth"},
	{"POST", "/api/users/doAgainLocalCBAuth"},
	{"POST", "/api/users/doAgainSmsCBAuth"},
	{"GET", "/api/users/selfUnlock/type"},
	{"POST", "/api/user/sendCode/{type}"},
	{"POST", "/api/user/checkCode/{type}"},
	{"POST", "/api/users/approve/addApply"},
	{"POST", "/api/users/auth/weChat/bindAndLogin"},
	{"GET", "/api/users/info"},
	{"POST", "/api/users/auth/afterBindLogin"},
	{"POST", "/api/users/auth/otp/login"},
	{"POST", "/api/client/mfa/login/commonAuth"},
	{"GET", "/api/users/auth/mfa/check?anchorId="},
	{"POST", "/api/users/auth/mfaSecondAuth"},
	{"POST", "/api/users/auth/mfaVerifyEnApp"},
	{"POST", "/api/users/auth/faceRecognition/login"},
	{"GET", "/api/client/mfa/login/homeInfo?anchorId="},
	{"POST", "/api/client/mfa/login/sendVerifyCode"},
	{"POST", "/api/client/mfa/sendVerifyCode"},
	{"POST", "/api/client/mfa/commonAuth"},
	{"GET", "/api/client/message/secAuth?msgId="},
	{"POST", "/api/client/totp/login/generateSecretQRContent"},
	{"GET", "/api/users/auth/certificate/{id}/getCode"},
	{"POST", "/api/users/auth/certificate/login"},
	{"POST", "/api/users/device/register/send/code"},
	{"POST", "/api/users/device/register/terminal/verify/code"},
	{"POST", "/api/users/device/register"},
	{"POST", "/api/users/auth/secondAuth/device"},
	{"POST", "/api/users/device/list/page"},
	{"POST", "/api/users/center/device/unbind"},
	{"POST", "/api/users/center/device/offline"},
	{"POST", "/api/users/center/uploadPicture"},
	{"POST", "/api/users/device/unbindUser"},
	{"GET", "/api/client/users/service/group?endlessType="},
	{"POST", "/api/client/user/center/resetUserPassword"},
	{"POST", "/api/users/person/restName"},
	{"POST", "/api/users/unBind/message/byType"},
	{"POST", "/api/users/service/visit/list"},
	{"GET", "/api/users/service/getAllTemporaryService"},
	{"GET", "/api/users/service/visit/add/{id}"},
	{"POST", "/api/users/person/bindMobileEmail"},
	{"POST", "/api/users/unbind/send/code"},
	{"POST", "/api/users/unbind/check/code"},
	{"POST", "/api/client/user/addServiceGroup"},
	{"POST", "/api/client/user/deleteServiceGroup"},
	{"POST", "/api/client/user/updateServiceGroup"},
	{"POST", "/api/client/user/updateServiceSort"},
	{"POST", "/api/client/user/updateServiceGroupSort"},
	{"POST", "/api/users/service/visit/delete"},
	{"POST", "/api/client/user/diTalk/bind"},
	{"POST", "/api/users/person/select/enableIsOpened"},
	{"GET", "/api/users/message/count"},
	{"GET", "/api/users/message/get?messageId="},
	{"POST", "/api/users/message/page"},
	{"GET", "/api/users/message/allRead?type="},
	{"POST", "/api/users/approve/center/groupCount"},
	{"POST", "/api/users/approve/center/applyPage"},
	{"POST", "/api/users/approve/center/waitingHandle"},
	{"POST", "/api/users/approve/center/myHandle"},
	{"POST", "/api/users/approve/center/flowImage"},
	{"GET", "/api/users/safeSpace/getService"},
	{"POST", "/api/client/totp/generateSecretQRContent"},
	{"POST", "/api/client/share/link/page"},
	{"GET", "/api/client/share/link/delete/{id}"},
	{"POST", "/api/users/safeSpace/getShareFilePage"},
	{"GET", "/api//client/banner/getBannerInfo?key="},
	{"POST", "/api/client/message/callback"},
	{"GET", "/api/client/public/files/download?filePath="},
	{"POST", "/api/users/service/getAllTemporaryService?orderValue="},
	{"POST", "/api/client/users/service/grouping"},
	{"POST", "/api/client/user/service/pageUserService?endlessType="},
	{"POST", "/api/client/user/service/customGroupRemoveService"},
	{"POST", "/api/client/user/service/addToCustomGroup"},
	{"POST", "/api/users/service/getAllApplicabilityService"},
	{"POST", "/api/users/service/createServiceApply"},
	{"POST", "/api/users/service/cancleServiceApply"},
	{"POST", "/api/users/approve/center/addApply"},
	{"POST", "/api/users/approve/center/canApplyTypes"},
	{"POST", "/api/users/approve/center/applyFormConf"},
	{"POST", "/api/users/approve/center/approveUserInfos"},
	{"POST", "/api/users/approve/center/deal"},
	{"POST", "/api/users/approve/center/validBeforeApplication"},
	{"POST", "/api/users/approve/addApply/noToken"},
	{"POST", "/api/users/approve/center/applyDetail"},
	{"GET", "/api/users/approve/center/getFlow"},
	{"POST", "/api/users/peripheral/enable"},
	{"GET", "/api/users/terminal/csAndNetworkControl"},
	{"GET", "/api/users/device/labels/info"},
	{"GET", "/api/users/device/getByFeatureCode?featureCode="},
	{"POST", "/api/users/device/bind/labels"},
	{"POST", "/api/users/confirmNewCommonLocation"},
	{"GET", "/api/users/person/getBindInfos"},
	{"GET", "/api/users/person/getBindQr?authConfigId="},
	{"POST", "/api/users/updateCommonLocation"},
	{"POST", "/api/users/auth/abac/mfa"},
	{"GET", "/api/client/mfa/homeInfo?anchorId="},
	{"POST", "/api/client/totp/sendCode"},
	{"POST", "/api/client/totp/resetSecret"},
	{"GET", "/api/users/url/open?url="},
	{"POST", "/api/v1/AVengine/baseinfoGet"},
	{"POST", "/api/v1/AVengine/baseinfoSet"},
	{"POST", "/api/v1/AVengine/exportLog"},
	{"POST", "/api/v1/AVengine/findAVS"},
	{"POST", "/api/v1/AVengine/getAVList"},
	{"POST", "/api/v1/AVengine/getAVPath"},
	{"POST", "/api/v1/AVengine/getIsolator"},
	{"POST", "/api/v1/AVengine/getLog"},
	{"POST", "/api/v1/AVengine/getResult"},
	{"POST", "/api/v1/AVengine/isolator"},
	{"POST", "/api/v1/AVengine/scan"},
	{"POST", "/api/v1/AVengine/setIsolatorPath"},
	{"POST", "/api/v1/AVengine/solve"},
	{"POST", "/api/v1/AVengine/updateAVLib"},
	{"POST", "/api/v1/api/users/antivirus/trustFile/page"},
	{"GET", "/api/v1/api/users/device/terminal/antivirus/lib?libType="},
	{"GET", "/api/v1/appMarket/getPushSoftwareData"},
	{"POST", "/api/v1/appMarket/getSoftCategoryList"},
	{"POST", "/api/v1/appMarket/getSoftPage"},
	{"POST", "/api/v1/appMarket/getSoftPageBeforeLogin"},
	{"POST", "/api/v1/appMarket/getSoftSearch"},
	{"POST", "/api/v1/appMarket/softUploadDistributeResult"},
	{"POST", "/api/v1/appMarket/softwareReport"},
	{"POST", "/api/v1/coms/RCICSwitch"},
	{"POST", "/api/v1/coms/getRCICInfo"},
	{"POST", "/api/v1/coms/getRCISState"},
	{"POST", "/api/v1/coms/getRCSInfo"},
	{"POST", "/api/v1/coms/patchFix"},
	{"GET", "/api/v1/coms/patchList"},
	{"POST", "/api/v1/coms/setRCISInfo"},
	{"POST", "/api/v1/control/autoBoot"},
	{"POST", "/api/v1/control/detect"},
	{"GET", "/api/v1/control/getLocalConfig"},
	{"GET", "/api/v1/control/info"},
	{"POST", "/api/v1/control/list"},
	{"POST", "/api/v1/control/notification"},
	{"POST", "/api/v1/control/protocol"},
	{"POST", "/api/v1/control/select"},
	{"GET", "/api/v1/desktop/GetFileCirculateList"},
	{"POST", "/api/v1/desktop/downloadShareFile"},
	{"POST", "/api/v1/desktop/enDesktopProxy"},
	{"GET", "/api/v1/desktop/fileCirculate?spaceName="},
	{"POST", "/api/v1/desktop/fileCirculateDownload"},
	{"GET", "/api/v1/desktop/getAllFileSharPath"},
	{"POST", "/api/v1/desktop/getSharFileDownData"},
	{"POST", "/api/v1/desktop/newDownloadShareFile"},
	{"POST", "/api/v1/desktop/openShareFileDir"},
	{"POST", "/api/v1/desktop/redirectSvcRequest"},
	{"GET", "/api/v1/desktop/spaceList"},
	{"GET", "/api/v1/desktop/startProcess"},
	{"POST", "/api/v1/device/manager/getPeripheralData"},
	{"GET", "/api/v1/device/security/allowed"},
	{"POST", "/api/v1/external/runExe"},
	{"POST", "/api/v1/gateway/switch"},
	{"POST", "/api/v1/gateway/turnOn"},
	{"GET", "/api/v1/local/config"},
	{"GET", "/api/v1/local/device/info"},
	{"POST", "/api/v1/local/device/queryProcess"},
	{"GET", "/api/v1/local/getAction"},
	{"GET", "/api/v1/local/info"},
	{"GET", "/api/v1/local/language/supported"},
	{"POST", "/api/v1/local/language/switch"},
	{"GET", "/api/v1/local/manager/queryUninstallStatus?querystatus="},
	{"POST", "/api/v1/local/openSoftware"},
	{"GET", "/api/v1/local/pollTags"},
	{"POST", "/api/v1/local/software/setup"},
	{"POST", "/api/v1/local/log/upload"},
	{"GET", "/api/v1/local/terminal/doProject"},
	{"GET", "/api/v1/local/terminal/doProject?time=0"},
	{"GET", "/api/v1/local/terminal/scoreRequest?mustColl="},
	{"POST", "/api/v1/local/terminal/setAction"},
	{"POST", "/api/v1/local/writeConfig"},
	{"POST", "/api/v1/local/writeConfigIni"},
	{"POST", "/api/v1/nac/login"},
	{"POST", "/api/v1/nac/logout"},
	{"GET", "/api/v1/nac/netIsolation?type="},
	{"POST", "/api/v1/nac/queryConfig"},
	{"POST", "/api/v1/nac/queryInfo"},
	{"GET", "/api/v1/nac/queryProxy"},
	{"POST", "/api/v1/nac/setProxy"},
	{"GET", "/api/v1/safetyAssess/continueTask"},
	{"GET", "/api/v1/safetyAssess/getDetailInfo"},
	{"GET", "/api/v1/safetyAssess/getOverViewInfo"},
	{"GET", "/api/v1/safetyAssess/pauseTask"},
	{"POST", "/api/v1/safetyAssess/repair"},
	{"POST", "/api/v1/safetyAssess/repairAll"},
	{"POST", "/api/v1/safetyAssess/starTask"},
	{"GET", "/api/v1/safetyAssess/stopTask"},
	{"POST", "/api/v1/sase/getEnterpriseCfg"},
	{"GET", "/api/v1/static/avatar/query?userName="},
	{"POST", "/api/v1/static/avatar/set"},
	{"GET", "/api/v1/user/GetUserProtocolState"},
	{"POST", "/api/v1/user/detail"},
	{"GET", "/api/v1/user/getRedirectUrl"},
	{"POST", "/api/v1/user/getUserGroupedServiceList"},
	{"POST", "/api/v1/user/logout"},
	{"POST", "/api/v1/user/logout?type=1"},
	{"POST", "/api/v1/user/preLogin"},
	{"POST", "/api/v1/user/pullUES"},
	{"GET", "/api/v1/user/refreshToken"},
	{"POST", "/api/v1/user/register"},
	{"POST", "/api/v1/user/selectControl"},
	{"POST", "/api/v1/user/setUserProtocolState"},
	{"POST", "/api/v1/user/webSessionExpires"},
	{"GET", "/api/v1/version/current"},
	{"POST", "/api/v1/version/executeUpdate"},
	{"GET", "/api/v1/version/latest"},
	{"POST", "/api/v1/version/latestServer"},
}

var vpnRoutes = []vpnRoute{
	{"login", "公开入口", "/login", "账号登录、动态码、二维码、SSO、隐私协议、找回密码", []string{"users-auth-login", "users-client-auth-generatekey", "users-custom-page-login-cfg-select", "users-basic-captcha", "users-auth-alertauth", "users-auth-bindmobileoremail", "users-auth-dingtalk-getqrcode-id", "users-auth-enappqrcodelogin", "users-auth-facerecognition-login", "users-auth-lark-getqrcode-id", "users-auth-mfa-check-anchorid", "users-auth-nopasswordlogin", "users-auth-otp-login", "users-auth-qrcode", "users-auth-simauth", "users-auth-wechatens-getqrcode-id", "users-auth-oneclick", "users-auth-oneclickauth", "users-bindmobileemail-send-code", "users-bindstatus-find-type", "users-bindwechatorens-find-bindstatus", "users-client-enapp-getcoderqr", "users-client-enapp-getcoderqrstatus", "users-findditalkconf", "users-findfeishuconf", "users-flow-path", "users-iam-secondauth-getsecondauthtpye", "users-nopasslogin-send-code", "users-sim-roll-result", "users-sim-send-code", "users-v3-qrcode-generate", "users-v3-qrcode-result-id", "users-wechat-getwechatensqrcode", "client-mfa-login-sendverifycode"}},
	{"sign-in", "登录流程", "/login/sign-in", "登录页主组件与登录方式切换", []string{"auth-login", "users-client-auth-generatekey"}},
	{"sign-in-to-login", "登录流程", "/login/sign-in?type=toLogin", "安全基线/设备流程返回登录页", []string{"auth-login", "users-client-auth-generatekey"}},
	{"login-about", "系统菜单", "/login/about", "关于页面", []string{}},
	{"login-auth-phone-or-email", "登录流程", "/login/authPhoneOrEmail", "手机或邮箱二次认证", []string{"second-send-code", "auth-secondauth", "auth-afterbindlogin"}},
	{"login-auth-register", "登录流程", "/login/authrigister", "新设备认证注册", []string{"device-register-send-code", "device-register", "auth-afterbindlogin"}},
	{"mac-loading", "登录流程", "/login/macLoading", "客户端加载/检测", []string{"local-info", "user-prelogin"}},
	{"add-server", "登录流程", "/login/add-server", "添加服务端", []string{"user-register", "user-detail"}},
	{"company-code", "登录流程", "/login/company-code", "企业编码", []string{"user-detail"}},
	{"confirm-join", "登录流程", "/login/confirm-join", "确认加入企业", []string{"user-register"}},
	{"baseline-check", "登录流程", "/login/baseline-check", "登录前安全基线", []string{"safetyassess-getoverviewinfo", "safetyassess-getdetailinfo"}},
	{"login-sys-config", "登录流程", "/login/sys-config", "登录前系统配置", []string{"control-getlocalconfig", "control-protocol", "local-language-supported"}},
	{"network-error", "登录流程", "/login/netWorkError", "网络错误", []string{}},
	{"company-config", "登录流程", "/login/company-config", "企业配置", []string{"sase-getenterprisecfg"}},
	{"forget-password", "公开入口", "/login/forget-password", "找回/重置密码与验证码", []string{"reset-password-find-verify-type", "reset-password-send-code", "reset-password-verify-code", "reset-password-terminal-verify-code"}},
	{"device-register", "登录流程", "/login/deviceRegister", "设备注册", []string{"device-register-send-code", "device-register-terminal-verify-code", "device-register", "users-device-register", "users-device-register-send-code", "users-device-register-terminal-verify-code", "users-device-bindinfo-token-get", "users-device-labels-info"}},
	{"forget-password-success", "登录流程", "/login/forgetPasswordSuccess", "找回密码完成", []string{}},
	{"first-login-update", "登录流程", "/login/first-login-update", "首次登录修改密码", []string{"auth-modifypwd"}},
	{"first-login-update-legacy", "登录流程", "/login/firstLoginUpdatePwd", "兼容登录流程的首次改密页", []string{"auth-modifypwd"}},
	{"forcibly-auth", "登录流程", "/login/forciblyAuth", "强制二次认证", []string{"auth-secondauth", "mfa-commonauth", "auth-mfasecondauth", "client-mfa-login-sendverifycode", "users-auth-mfa-check-anchorid"}},
	{"second-auth-old", "登录流程", "/login/second-auth-old", "旧版二次认证", []string{"second-send-code", "auth-secondauth", "auth-nopasswordlogin"}},
	{"second-auth-old-other", "登录流程", "/login/second-auth-old-other", "旧版二次认证其他方式", []string{"second-send-code", "auth-secondauth", "auth-nopasswordlogin"}},
	{"new-device", "登录流程", "/login/newdevice", "新设备注册提示页", []string{"device-register-send-code", "device-register"}},
	{"code-register", "登录流程", "/login/codeRigister", "验证码注册页", []string{"device-register-send-code", "device-register"}},
	{"register-fail", "登录流程", "/login/rigisterFail", "设备注册失败页", []string{"device-register"}},
	{"second-auth", "登录流程", "/login/second-auth", "二次认证", []string{"second-send-code", "auth-secondauth", "auth-mfasecondauth", "users-docbauth", "users-doagaincbauth", "users-doagainlocalcbauth", "users-doagainsmscbauth"}},
	{"second-auth-detail", "登录流程", "/login/second-auth-detail", "二次认证详情", []string{"auth-secondauth-device", "client-message-secauth-msgid"}},
	{"device-unbind-auth-detail", "登录流程", "/login/device-unbind-auth-detail", "设备解绑的手机/邮箱/SIM 认证详情", []string{"second-send-code", "auth-secondauth", "auth-afterbindlogin"}},
	{"bind-info", "登录流程", "/login/bind-info", "绑定信息", []string{"person-getbindinfos", "person-bindmobileemail"}},
	{"device-unbind", "登录流程", "/login/device-unbind", "设备解绑", []string{"center-device-unbind", "unbind-send-code", "unbind-check-code", "users-center-device-unbind", "users-device-unbinduser", "users-unbind-message-bytype", "users-device-getbyfeaturecode-featurecode"}},
	{"device-unbind-auth", "登录流程", "/login/device-unbind-auth", "设备解绑认证", []string{"auth-secondauth-device", "users-device-bindinfo-token-get", "users-second-send-code"}},
	{"enhanced-auth", "登录流程", "/login/enhancedAuth", "增强认证/MFA", []string{"auth-abac-mfa", "mfa-commonauth", "mfa-sendverifycode"}},
	{"enhanced-auth-type", "登录流程", "/login/enhancedAuthType", "增强认证方式", []string{"auth-mfasecondauth", "auth-mfaverifyenapp", "client-totp-login-generatesecretqrcontent"}},
	{"bind-otp", "登录流程", "/login/bindOtp", "绑定 OTP", []string{"client-totp-generatesecretqrcontent", "client-totp-sendcode", "client-totp-resetsecret"}},
	{"self-unlock", "登录流程", "/login/selfUnlock", "自助解锁", []string{"selfunlock-type", "user-sendcode-type", "user-checkcode-type"}},
	{"unlock-success", "登录流程", "/login/unlockSuccess", "解锁完成", []string{}},
	{"account-unlock-apply", "登录流程", "/login/accountUnlockApply", "账号解锁申请", []string{"approve-addapply"}},
	{"third-account-bind", "登录流程", "/login/thirdaccountBind", "第三方账号绑定", []string{"auth-certificate-id-getcode", "auth-certificate-login"}},
	{"wechat-login-loading", "登录流程", "/login/wxLoginLoading", "微信登录等待", []string{"wechat-getwechatqrcode", "auth-wechat-bindandlogin"}},
	{"sso-loading", "登录流程", "/login/ssoLoading", "SSO 等待", []string{"users-info"}},
	{"baseline-to-login", "登录流程", "/login/baselineToLogin", "基线完成回登录", []string{"user-getusergroupedservicelist"}},
	{"new-equipment-registration", "登录流程", "/login/new-equipment-registration", "新设备注册", []string{"device-register", "device-register-send-code"}},
	{"register-success", "登录流程", "/login/rigisterSuccess", "设备注册完成", []string{}},
	{"device-code-apply", "登录流程", "/login/device-code-apply", "设备验证码申请", []string{"device-register-send-code"}},
	{"device-second-auth", "登录流程", "/login/device-second-auth", "设备二次认证", []string{"auth-secondauth-device"}},
	{"auth-register-phone", "登录流程", "/login/authrigister_phone", "认证手机号注册", []string{"user-sendcode-type", "user-checkcode-type", "auth-afterbindlogin", "users-bindmobileemail-send-code", "users-auth-bindmobileoremail"}},
	{"device-regist-success", "登录流程", "/login/device-regist-success", "设备登记完成", []string{}},
	{"device-approval", "登录流程", "/login/device-approval", "设备审批", []string{"approve-addapply", "approve-center-deal", "users-approve-center-adddeviceapp"}},
	{"mfa-login-loading", "登录流程", "/login/mfaLoginLoading", "MFA 登录等待", []string{"mfa-login-commonauth", "auth-mfasecondauth"}},
	{"visitor-request", "公开入口", "/login/visitorRequest", "访客申请", []string{"user-sendcode-type", "user-checkcode-type", "user-register"}},
	{"login-device-unbind-legacy", "登录流程", "/login/deviceUnBind", "旧版设备解绑流程", []string{"center-device-unbind", "unbind-send-code", "unbind-check-code"}},
	{"login-system-conf", "系统菜单", "/login/systemConf", "系统配置菜单入口", []string{"control-getlocalconfig", "control-protocol", "local-language-supported", "v1-user-getredirecturl"}},
	{"log-export", "系统菜单", "/prePage/logExport", "客户端日志导出入口", []string{}},
	{"log-report", "系统菜单", "/prePage/logReport", "客户端日志上报页面", []string{"local-log-upload"}},
	{"prepage-company-code", "客户端流程", "/prePage/companycode", "客户端服务端选择/企业编码页", []string{"user-detail", "user-register"}},
	{"prepage-mac-privacy", "客户端流程", "/prePage/macPrivacy", "Mac 客户端隐私与扩展授权页", []string{"user-getuserprotocolstate", "user-setuserprotocolstate"}},
	{"prepage-old-login", "客户端流程", "/prePage/oldLogin", "旧版本登录页", []string{"version-latestserver", "user-prelogin"}},
	{"prepage-serve-conf", "客户端流程", "/prePage/serveConf", "客户端服务端配置页", []string{"version-latestserver", "user-detail"}},
	{"home", "工作台", "/home", "登录后门户壳、侧栏和全局状态", []string{"users-info", "users-message-count", "client-users-service-group-endlesstype", "users-auth-logout", "v1-user-logout", "v1-user-logout-type-1", "v1-user-pullues", "v1-user-refreshtoken", "v1-user-websessionexpires"}},
	{"errorguide", "工作台", "/home/errorguide", "错误/诊断指引", []string{"local-terminal-scorerequest-mustcoll", "local-terminal-doproject"}},
	{"overview", "工作台", "/home/overview", "门户概览、横幅、公告", []string{"client-banner-getbannerinfo-key", "users-message-page"}},
	{"personal-center", "用户中心", "/home/personal-center", "账号安全、绑定信息、活动设备", []string{"users-info", "person-getbindinfos", "device-list-page", "client-user-center-resetuserpassword", "client-user-ditalk-bind", "users-auth-bindmobileoremail", "users-bindmobileemail-send-code", "users-bindstatus-find-type", "users-bindwechatorens-find-bindstatus", "users-center-device-offline", "users-center-device-unbind", "users-center-uploadpicture", "users-device-bind-labels", "users-device-bindinfo-token-get", "users-device-getbyfeaturecode-featurecode", "users-device-labels-info", "users-device-unbinduser", "users-person-getbindqr-authconfigid", "users-person-restname", "users-person-select-enableisopened", "v1-static-avatar-set"}},
	{"work-bench", "工作台", "/home/work-bench", "最近访问、全部应用、应用分组、搜索、排序", []string{"client-users-service-group-endlesstype", "client-users-service-grouping", "client-user-service-pageuserservice-endlesstype", "users-service-visit-list", "users-service-visit-add-id", "users-service-visit-delete", "client-user-addservicegroup", "client-user-deleteservicegroup", "client-user-updateservicegroup", "client-user-updateservicesort", "client-user-updateservicegroupsort", "client-user-service-addtocustomgroup", "client-user-service-customgroupremoveservice", "users-url-open-url", "users-confirmnewcommonlocation", "users-updatecommonlocation", "v1-local-polltags"}},
	{"work-bench-all", "工作台", "/home/work-bench-all", "兼容版本全部应用工作台", []string{"client-users-service-group-endlesstype", "users-service-visit-list"}},
	{"exclusion-zone", "工作台", "/home/work-bench/exclusion-zone", "隔离区工作台", []string{"users-info", "users-url-open-url", "message-secauth-msgid"}},
	{"approve-center", "申请与审批", "/home/approve-center", "我的申请、待办、已办、流程图、详情", []string{"approve-center-groupcount", "approve-center-applypage", "approve-center-waitinghandle", "approve-center-myhandle", "approve-center-flowimage", "approve-center-getflow", "approve-center-applydetail", "approve-center-deal"}},
	{"approve-initiated", "申请与审批", "/home/approve-center?tabName=Initiated", "审批中心我的申请标签状态", []string{"approve-center-applypage", "approve-center-applydetail"}},
	{"approve-pending", "申请与审批", "/home/approve-center?tabName=Pending", "审批中心待办标签状态", []string{"approve-center-waitinghandle", "approve-center-deal"}},
	{"apply-app", "申请与审批", "/home/apply-app", "应用申请", []string{"approve-center-canapplytypes", "approve-center-applyformconf", "approve-center-approveuserinfos", "approve-center-addapply", "approve-center-validbeforeapplication", "users-approve-center-adddeviceapp"}},
	{"temporary-account", "申请与审批", "/home/temporaryAccount", "临时账号申请", []string{"service-getalltemporaryservice", "service-getalltemporaryservice-ordervalue", "service-createserviceapply"}},
	{"account-apply", "申请与审批", "/home/accountApply", "账号申请", []string{"service-getallapplicabilityservice", "service-createserviceapply", "approve-center-addapply"}},
	{"cancel-account-apply", "申请与审批", "/home/cancelAccountApply", "账号注销申请", []string{"service-cancleServiceApply", "approve-center-addapply"}},
	{"usb-apply", "申请与审批", "/home/usbApply", "USB 存储设备申请", []string{"users-peripheral-enable", "approve-center-addapply", "approve-addapply-notoken"}},
	{"app-store", "应用商店", "/home/app-store", "软件商店、分类、搜索、安装/更新/卸载", []string{"appmarket-getsoftcategorylist", "appmarket-getsoftpage", "appmarket-getsoftsearch", "appmarket-getpushsoftwaredata", "appmarket-getsoftpagebeforelogin", "appmarket-softuploaddistributeresult", "appmarket-softwareReport", "local-software-setup", "external-runexe", "v1-local-opensoftware", "v1-local-manager-queryuninstallstatus-querystatus"}},
	{"app-market-legacy", "应用商店", "/home/appMarket", "兼容版本应用市场入口", []string{"appmarket-getsoftcategorylist", "appmarket-getsoftpage", "appmarket-getsoftsearch", "appmarket-getpushsoftwaredata", "appmarket-getsoftpagebeforelogin"}},
	{"app-market-detail", "应用商店", "/home/appMarketDetail", "应用详情与分发结果", []string{"appmarket-getsoftpage", "appmarket-softwareReport", "appmarket-softuploaddistributeresult"}},
	{"secure-space", "安全中心", "/home/secure-space", "安全空间/我的空间", []string{"safespace-getservice", "safespace-getsharefilepage", "desktop-spacelist", "desktop-getallfilesharpath", "desktop-filecirculate-spacename"}},
	{"secure-center", "安全中心", "/home/secure-center", "安全中心总览与风险等级", []string{"safetyassess-getoverviewinfo", "safetyassess-getdetailinfo", "safetyassess-startask", "v1-device-security-allowed"}},
	{"safety-detail", "安全中心", "/home/secure-center/safety-detail", "安全检测详情", []string{"safetyassess-getdetailinfo", "safetyassess-repair", "safetyassess-repairall"}},
	{"safety-detail-scan", "安全中心", "/home/secure-center/safety-detail?scanType=", "按扫描类型查看安全详情", []string{"safetyassess-getdetailinfo", "safetyassess-repair"}},
	{"safety-detail-repair-all", "安全中心", "/home/secure-center/safety-detail?isRepair=all&scanType=", "安全中心全部修复结果状态", []string{"safetyassess-repairall", "safetyassess-getdetailinfo"}},
	{"safety-baseline", "安全中心", "/home/secure-center/safety-baseline", "安全基线", []string{"safetyassess-getoverviewinfo", "safetyassess-pausetask", "safetyassess-continuetask", "safetyassess-stoptask"}},
	{"quarantine", "安全中心", "/home/secure-center/quarantine", "病毒隔离区", []string{"avengine-getisolator", "avengine-isolator", "avengine-solve"}},
	{"trust-zone", "安全中心", "/home/secure-center/trust-zone", "信任区", []string{"antivirus-trustfile-page"}},
	{"virus-logs", "安全中心", "/home/secure-center/virus-logs", "病毒扫描日志", []string{"avengine-getlog", "avengine-exportlog"}},
	{"quick-scan", "安全中心", "/home/secure-center/QuickScan", "快速扫描", []string{"avengine-scan", "avengine-getresult", "avengine-getavlist"}},
	{"custom-scan", "安全中心", "/home/secure-center/CustomScan", "自定义扫描", []string{"avengine-scan", "avengine-getresult"}},
	{"linux-quick-scan", "安全中心", "/home/secure-center/LinuxQuickScan", "Linux 快速扫描", []string{"avengine-scan", "avengine-getresult"}},
	{"safe-result", "安全中心", "/home/secure-center/SafeResult", "安全扫描结果", []string{"avengine-getresult", "safetyassess-getdetailinfo"}},
	{"risk-result", "安全中心", "/home/secure-center/RiskResult", "风险结果", []string{"safetyassess-getdetailinfo", "safetyassess-repair"}},
	{"virus-scan-setting", "安全中心", "/home/secure-center/VirusScanSetting", "病毒扫描设置", []string{"avengine-baseinfoget", "avengine-baseinfoset", "avengine-setisolatorpath", "avengine-updateavlib", "v1-avengine-findavs", "v1-avengine-getavpath", "v1-api-users-device-terminal-antivirus-lib-libtype"}},
	{"message-center", "消息与公告", "/home/message-center", "系统、审批、登录、安全消息", []string{"users-message-count", "users-message-page", "users-message-get-messageid", "users-message-allread-type", "client-message-callback"}},
	{"home-sys-config", "客户端与网络", "/home/sys-config", "系统配置、通知、自启动、协议、代理", []string{"control-getlocalconfig", "control-autoboot", "control-notification", "control-protocol", "nac-queryproxy", "nac-setproxy", "local-language-switch", "v1-control-detect", "v1-local-config", "v1-local-writeconfig", "v1-local-writeconfigini", "v1-gateway-switch", "v1-gateway-turnon", "v1-nac-login", "v1-nac-logout", "v1-nac-netisolation-type", "v1-nac-queryconfig", "v1-nac-queryinfo"}},
	{"notice-center", "消息与公告", "/home/notice-center", "公告中心", []string{"client-banner-getbannerinfo-key", "users-message-page"}},
	{"tool-box", "客户端与网络", "/home/tool-box", "工具箱", []string{"local-info", "control-info", "local-terminal-doproject", "desktop-startprocess", "v1-local-config", "v1-local-getaction", "v1-local-device-queryprocess", "v1-local-terminal-doproject-time-0", "v1-local-writeconfig", "v1-local-writeconfigini"}},
	{"terminal-info", "客户端与网络", "/home/terminal-info", "终端信息与控制", []string{"local-device-info", "control-info", "control-list", "control-select", "device-manager-getperipheraldata", "users-terminal-csandnetworkcontrol", "v1-api-users-device-terminal-antivirus-lib-libtype", "v1-device-manager-getperipheraldata", "v1-device-security-allowed", "v1-local-device-queryprocess", "v1-local-terminal-doproject-time-0", "v1-local-terminal-setaction", "v1-user-selectcontrol"}},
	{"long-range-control", "客户端与网络", "/home/long-range-control", "远程控制", []string{"coms-getrcsinfo", "coms-getrcicinfo", "coms-getrcisstate", "coms-rcicswitch", "coms-setrcisinfo", "coms-patchlist", "coms-patchfix"}},
	{"long-range-control-menu", "客户端与网络", "/home/long-range-Control", "远程控制菜单入口（保留门户大小写路径）", []string{"coms-getrcsinfo", "coms-getrcicinfo", "coms-getrcisstate", "coms-rcicswitch", "coms-setrcisinfo", "coms-patchlist", "coms-patchfix"}},
	{"file-share", "安全空间与文件", "/home/flie-share", "文件分享、接收、流转、下载", []string{"share-link-page", "share-link-delete-id", "desktop-getfilecirculatelist", "desktop-downloadsharefile", "desktop-newdownloadsharefile", "desktop-filecirculatedownload", "desktop-getsharfiledowndata", "desktop-opensharefiledir", "client-public-files-download-filepath", "v1-desktop-endesktopproxy", "v1-desktop-filecirculate-spacename", "v1-desktop-redirectsvcrequest"}},
	{"file-share-received", "安全空间与文件", "/home/flie-share?name=myReceive", "文件分享接收列表状态", []string{"safespace-getsharefilepage", "desktop-downloadsharefile", "desktop-filecirculatedownload"}},
	{"file-link", "安全空间与文件", "/home/file-link", "分享链接", []string{"share-link-page", "share-link-delete-id"}},
	{"enhanced-auth-login", "登录流程", "/home/enhancedAuthLogin", "登录后增强认证", []string{"mfa-commonauth", "auth-mfasecondauth", "auth-mfaverifyenapp"}},
	{"enhanced-auth-login-type", "登录流程", "/home/enhancedAuthLoginType", "登录后认证类型", []string{"mfa-login-commonauth", "mfa-login-homeinfo-anchorid"}},
	{"home-bind-otp", "用户中心", "/home/bindOtp", "登录后 OTP 绑定", []string{"client-totp-generatesecretqrcontent", "client-totp-sendcode", "client-totp-resetsecret"}},
	{"mfa-loading", "登录流程", "/home/mfaLoading", "MFA 加载", []string{"mfa-homeinfo-anchorid", "mfa-commonauth"}},
	{"service", "工作台", "/home/service", "认证后服务落地页", []string{"users-info", "users-url-open-url"}},
	{"save-center", "用户中心", "/home/saveCenter", "客户端保存中心入口", []string{"users-info", "static-avatar-query-username"}},
	{"workbench-legacy", "工作台", "/home/workbench", "兼容版本工作台入口", []string{"client-users-service-group-endlesstype", "users-service-visit-list"}},
	{"workbench-file-apply", "申请与审批", "/home/workbench/file_apply", "文件申请审批兼容页", []string{"approve-center-applypage", "approve-center-waitinghandle", "approve-center-myhandle", "approve-center-deal"}},
	{"sys-config-system", "客户端与网络", "/home/sys-config?type=system", "系统配置页系统状态", []string{"control-getlocalconfig", "control-autoboot", "control-notification", "control-protocol", "nac-queryproxy", "nac-setproxy"}},
	{"sys-config-companies", "客户端与网络", "/home/sys-config?type=companies", "系统配置页企业状态", []string{"sase-getenterprisecfg", "control-getlocalconfig"}},
	{"sys-config-update", "客户端与网络", "/home/sys-config?type=update", "系统配置页更新状态", []string{"version-current", "version-latest", "version-latestserver", "version-executeupdate"}},
	{"error", "系统入口", "/error", "错误页", []string{}},
	{"mac-privacy", "系统入口", "/macPrivacy", "客户端隐私页", []string{"user-getuserprotocolstate", "user-setuserprotocolstate"}},
	{"root", "系统入口", "/", "门户根入口", []string{"users-custom-page-login-cfg-select"}},
}

var vpnControls = []vpnControl{
	{"login.account", "登录页", "账号密码登录", []string{"login", "sign-in"}, []string{"users-client-auth-generatekey", "auth-login", "users-custom-page-login-cfg-select", "users-basic-captcha"}},
	{"login.dynamic-code", "登录页", "动态码登录", []string{"login", "sign-in"}, []string{"noPassLogin-send-code", "auth-nopasswordlogin", "users-second-send-code"}},
	{"login.cas", "登录页", "CAS 统一身份认证", []string{"login", "sign-in"}, []string{}},
	{"login.sso", "登录页", "其他单点登录", []string{"login", "sign-in"}, []string{"auth-qrCode", "auth-oneclick", "users-auth-oneclickauth"}},
	{"login.qr-code", "登录页", "二维码登录", []string{"login", "sign-in"}, []string{"v3-qrcode-generate", "v3-qrcode-result-id", "auth-qrcode", "users-client-enapp-getcoderqr", "users-client-enapp-getcoderqrstatus", "users-auth-enappqrcodelogin", "users-v3-qrcode-generate", "users-v3-qrcode-result-id"}},
	{"login.wechat", "登录页", "微信/企业微信登录", []string{"login", "wechat-login-loading"}, []string{"wechat-getwechatqrcode", "wechat-getwechatensqrcode", "auth-wechat-bindandlogin", "users-auth-wechatens-getqrcode-id"}},
	{"login.dingtalk", "登录页", "钉钉登录", []string{"login"}, []string{"auth-dingtalk-getqrcode-id", "users-findditalkconf"}},
	{"login.feishu", "登录页", "飞书登录", []string{"login"}, []string{"findfeishuconf", "auth-lark-getqrcode-id", "users-findfeishuconf"}},
	{"login.otp", "登录页", "OTP 登录", []string{"login", "bind-otp"}, []string{"auth-otp-login", "client-totp-login-generatesecretqrcontent", "client-totp-generatesecretqrcontent", "client-totp-sendcode", "client-totp-resetsecret"}},
	{"login.third-account", "登录页", "第三方账号登录", []string{"login", "third-account-bind"}, []string{"auth-certificate-id-getcode", "auth-certificate-login"}},
	{"login.face", "登录页", "人脸识别登录", []string{"login"}, []string{"auth-facerecognition-login", "users-auth-facerecognition-login"}},
	{"login.sim", "登录页", "SIM 登录", []string{"login"}, []string{"sim-send-code", "auth-simauth", "sim-roll-result", "users-sim-send-code", "users-sim-roll-result", "users-auth-simauth"}},
	{"login.ukey", "登录页", "UKey 登录", []string{"login"}, []string{"docbauth", "doagaincbauth", "users-docbauth", "users-doagaincbauth", "users-doagainlocalcbauth", "users-doagainsmscbauth"}},
	{"login.privacy", "登录页", "隐私协议勾选/服务协议/使用条款", []string{"login", "mac-privacy"}, []string{"user-getuserprotocolstate", "user-setuserprotocolstate"}},
	{"login.forgot", "登录页", "忘记密码", []string{"login", "forget-password"}, []string{"reset-password-find-verify-type", "reset-password-send-code", "reset-password-verify-code", "users-reset-password-terminal-verify-code"}},
	{"login.language", "登录页", "中文/English/Thai", []string{"login", "home-sys-config"}, []string{"local-language-supported", "local-language-switch"}},
	{"login.visitor", "登录页", "访客申请入口", []string{"login", "visitor-request"}, []string{"user-sendcode-type", "user-checkcode-type", "user-register", "users-selfunlock-type"}},
	{"login.remember", "登录页", "记住登录/一周免登录", []string{"login"}, []string{}},
	{"login.protocol-dialog", "登录页", "隐私协议/服务协议确认弹窗", []string{"login", "mac-privacy"}, []string{"user-getuserprotocolstate", "user-setuserprotocolstate", "users-custom-page-login-cfg-select"}},
	{"login.authorization-dialog", "登录页", "第三方授权确认/取消", []string{"login", "third-account-bind"}, []string{"users-auth-alertauth", "users-auth-afterbindlogin", "users-auth-bindmobileoremail", "users-flow-path", "users-iam-secondauth-getsecondauthtpye", "users-auth-secondauth", "users-auth-secondauth-device", "users-auth-abac-mfa", "users-auth-mfa-check-anchorid", "users-auth-mfasecondauth", "users-auth-mfaverifyenapp", "client-mfa-commonauth", "client-mfa-homeinfo-anchorid", "client-mfa-login-commonauth", "client-mfa-login-homeinfo-anchorid", "client-mfa-login-sendverifycode", "client-mfa-sendverifycode"}},
	{"login.device-registration", "登录流程", "新设备注册/设备审批/验证码", []string{"device-register", "new-device", "code-register", "new-equipment-registration", "auth-register-phone", "device-approval"}, []string{"users-device-register", "users-device-register-send-code", "users-device-register-terminal-verify-code", "users-peripheral-enable"}},
	{"shell.sidebar", "门户壳", "工作台/消息侧栏", []string{"home", "work-bench", "message-center"}, []string{"users-message-count"}},
	{"shell.system-config", "系统菜单", "系统配置菜单项", []string{"home-sys-config", "login-system-conf"}, []string{"control-getlocalconfig", "control-protocol", "local-language-switch", "v1-control-detect", "v1-control-info", "v1-control-list", "v1-control-select", "v1-local-config", "v1-local-writeconfig", "v1-local-writeconfigini", "v1-user-getredirecturl", "v1-user-getusergroupedservicelist", "v1-user-prelogin", "v1-user-pullues", "v1-user-refreshtoken", "v1-user-websessionexpires", "v1-version-current", "v1-version-executeupdate", "v1-version-latest", "v1-version-latestserver"}},
	{"shell.company-config", "系统菜单", "企业配置菜单项", []string{"tool-box", "company-config"}, []string{"sase-getenterprisecfg"}},
	{"shell.log-report", "系统菜单", "日志上报抽屉", []string{"tool-box", "log-report"}, []string{"local-log-upload"}},
	{"shell.log-export", "系统菜单", "日志导出菜单项", []string{"tool-box", "log-export"}, []string{}},
	{"shell.about", "系统菜单", "关于菜单项", []string{"tool-box", "login-about"}, []string{}},
	{"shell.change-account", "系统菜单", "切换账号/注销", []string{"tool-box", "home"}, []string{"auth-logout", "user-logout", "v1-user-logout-type-1"}},
	{"shell.exit", "系统菜单", "退出客户端", []string{"tool-box", "home"}, []string{"user-logout"}},
	{"shell.network-proxy", "工具箱", "网络代理抽屉", []string{"tool-box", "home-sys-config"}, []string{"nac-queryproxy", "nac-setproxy"}},
	{"shell.restart-confirm", "系统配置", "语言切换重启确认弹窗", []string{"home-sys-config", "login-system-conf"}, []string{"local-language-switch", "user-logout"}},
	{"workbench.recent", "工作台", "最近访问标签", []string{"work-bench"}, []string{"users-service-visit-list", "users-service-visit-delete"}},
	{"workbench.all", "工作台", "全部应用标签", []string{"work-bench", "work-bench-all"}, []string{"client-user-service-pageuserservice-endlesstype"}},
	{"workbench.cas", "工作台", "CAS 认证组标签", []string{"work-bench"}, []string{"client-users-service-group-endlesstype"}},
	{"workbench.finance", "工作台", "经管应用标签", []string{"work-bench"}, []string{"client-users-service-group-endlesstype"}},
	{"workbench.search", "工作台", "应用搜索", []string{"work-bench"}, []string{"client-user-service-pageuserservice-endlesstype"}},
	{"workbench.sort", "工作台", "自定义排序", []string{"work-bench"}, []string{"client-user-updateservicesort", "client-user-updateservicegroupsort"}},
	{"workbench.app-menu", "工作台", "应用卡片下拉菜单", []string{"work-bench"}, []string{"client-user-service-addtocustomgroup", "client-user-service-customgroupremoveservice", "client-user-deleteservicegroup", "client-users-service-grouping", "users-service-visit-add-id", "users-service-visit-delete"}},
	{"workbench.new-group", "工作台", "新建应用分组弹窗", []string{"work-bench"}, []string{"client-user-addservicegroup"}},
	{"workbench.manage-group", "工作台", "管理分组应用弹窗", []string{"work-bench"}, []string{"client-user-updateservicegroup", "client-user-service-addtocustomgroup", "client-user-service-customgroupremoveservice"}},
	{"workbench.tag", "工作台", "设置标签弹窗", []string{"work-bench"}, []string{"client-user-updateservicegroup"}},
	{"workbench.frequent", "工作台", "设置常用位置", []string{"work-bench"}, []string{"users-confirmnewcommonlocation", "users-updatecommonlocation"}},
	{"workbench.uninstall", "工作台", "卸载应用确认", []string{"work-bench"}, []string{"local-openSoftware", "local-manager-queryuninstallstatus-querystatus"}},
	{"workbench.apply-resource", "工作台", "申请资源下拉菜单", []string{"work-bench"}, []string{"approve-center-canapplytypes", "service-getalltemporaryservice", "users-service-getalltemporaryservice-ordervalue", "users-service-getallapplicabilityservice", "users-service-createserviceapply", "users-service-cancleserviceapply"}},
	{"workbench.approval-center", "工作台", "审批中心入口", []string{"work-bench", "approve-center"}, []string{"approve-center-groupcount"}},
	{"workbench.announcement", "工作台", "公告卡片与更多", []string{"work-bench", "notice-center"}, []string{"client-banner-getbannerinfo-key", "users-message-page"}},
	{"workbench.app-launch", "工作台", "应用卡片打开/跳转", []string{"work-bench", "work-bench-all"}, []string{"users-url-open-url", "v1-user-getredirecturl", "v1-user-getusergroupedservicelist"}},
	{"message.filters", "消息中心", "消息类型筛选", []string{"message-center"}, []string{"users-message-page", "users-message-count"}},
	{"message.detail", "消息中心", "消息详情", []string{"message-center"}, []string{"users-message-get-messageid", "client-message-secauth-msgid", "client-message-callback"}},
	{"message.read-all", "消息中心", "全部已读", []string{"message-center"}, []string{"users-message-allread-type"}},
	{"approval.tabs", "审批中心", "申请/待办/已办标签", []string{"approve-center"}, []string{"approve-center-applypage", "approve-center-waitinghandle", "approve-center-myhandle", "users-approve-center-getflow"}},
	{"approval.detail", "审批中心", "申请详情/流程图", []string{"approve-center"}, []string{"approve-center-applydetail", "approve-center-flowimage", "users-approve-center-getflow"}},
	{"approval.deal", "审批中心", "审批处理", []string{"approve-center"}, []string{"approve-center-deal"}},
	{"approval.form", "申请页面", "动态申请表单", []string{"apply-app", "account-apply", "usb-apply"}, []string{"approve-center-applyformconf", "approve-center-addapply", "users-approve-addapply", "users-approve-addapply-notoken", "users-approve-center-adddeviceapp", "users-approve-center-approveuserinfos", "users-approve-center-validbeforeapplication"}},
	{"app-market.search", "应用商店", "搜索/分类/分页", []string{"app-store", "app-market-detail"}, []string{"appmarket-getsoftsearch", "appmarket-getsoftcategorylist", "appmarket-getsoftpage", "v1-appmarket-getpushsoftwaredata", "v1-appmarket-getsoftpagebeforelogin"}},
	{"app-market.install", "应用商店", "下载/安装/更新/取消", []string{"app-store", "app-market-detail"}, []string{"local-software-setup", "appmarket-softuploaddistributeresult", "v1-external-runexe"}},
	{"app-market.uninstall", "应用商店", "卸载", []string{"app-store"}, []string{"local-openSoftware", "local-manager-queryuninstallstatus-querystatus"}},
	{"app-market.report", "应用商店", "分发结果/软件上报", []string{"app-store"}, []string{"appmarket-softwareReport"}},
	{"security.scan", "安全中心", "快速/完整/自定义扫描", []string{"secure-center", "quick-scan", "custom-scan", "linux-quick-scan"}, []string{"avengine-scan", "avengine-getresult", "v1-avengine-getavlist", "v1-avengine-findavs", "v1-safetyassess-getoverviewinfo", "v1-safetyassess-getdetailinfo", "v1-safetyassess-startask"}},
	{"security.scan-control", "安全中心", "暂停/继续/停止扫描", []string{"safety-baseline"}, []string{"safetyassess-pausetask", "safetyassess-continuetask", "safetyassess-stoptask"}},
	{"security.repair", "安全中心", "单项/全部修复", []string{"safety-detail", "risk-result"}, []string{"safetyassess-repair", "safetyassess-repairall", "avengine-solve"}},
	{"security.quarantine", "安全中心", "隔离/恢复/删除", []string{"quarantine"}, []string{"avengine-getisolator", "avengine-isolator", "avengine-solve"}},
	{"security.trust", "安全中心", "信任区", []string{"trust-zone"}, []string{"antivirus-trustfile-page"}},
	{"security.logs", "安全中心", "扫描日志导出", []string{"virus-logs"}, []string{"avengine-getlog", "avengine-exportlog"}},
	{"security.settings", "安全中心", "病毒库/隔离目录/引擎设置", []string{"virus-scan-setting"}, []string{"avengine-baseinfoget", "avengine-baseinfoset", "avengine-setisolatorpath", "avengine-updateavlib", "v1-avengine-findavs", "v1-avengine-getavpath", "v1-api-users-device-terminal-antivirus-lib-libtype"}},
	{"space.service", "安全空间", "空间开通/我的空间", []string{"secure-space"}, []string{"safespace-getservice", "desktop-spacelist"}},
	{"space.share", "安全空间", "文件分享/链接删除/下载", []string{"secure-space", "file-share", "file-link"}, []string{"share-link-page", "share-link-delete-id", "safespace-getsharefilepage", "desktop-downloadsharefile", "client-public-files-download-filepath", "v1-desktop-endesktopproxy", "v1-desktop-filecirculate-spacename", "v1-desktop-filecirculatedownload", "v1-desktop-getallfilesharpath", "v1-desktop-getfilecirculatelist", "v1-desktop-getsharfiledowndata", "v1-desktop-newdownloadsharefile", "v1-desktop-opensharefiledir", "v1-desktop-redirectsvcrequest", "v1-desktop-startprocess"}},
	{"terminal.info", "终端", "终端信息/外设", []string{"terminal-info"}, []string{"local-device-info", "device-manager-getperipheraldata", "device-security-allowed", "v1-local-device-queryprocess"}},
	{"terminal.control", "终端", "协议/通知/自启动/代理", []string{"home-sys-config", "terminal-info"}, []string{"control-protocol", "control-notification", "control-autoboot", "nac-queryproxy", "nac-setproxy", "v1-control-detect", "v1-control-info", "v1-control-list", "v1-control-select", "v1-gateway-switch", "v1-gateway-turnon", "v1-local-config", "v1-local-getaction", "v1-local-info", "v1-local-polltags", "v1-local-terminal-doproject", "v1-local-terminal-doproject-time-0", "v1-local-terminal-scorerequest-mustcoll", "v1-local-terminal-setaction", "v1-local-writeconfig", "v1-local-writeconfigini", "v1-nac-login", "v1-nac-logout", "v1-nac-netisolation-type", "v1-nac-queryconfig", "v1-nac-queryinfo", "v1-user-selectcontrol"}},
	{"remote-control", "客户端", "远程控制/补丁", []string{"long-range-control"}, []string{"coms-getrcsinfo", "coms-getrcicinfo", "coms-patchlist", "coms-patchfix", "v1-coms-getrcisstate", "v1-coms-rcicswitch", "v1-coms-setrcisinfo"}},
	{"user.account", "用户中心", "账号基本信息/重置密码", []string{"personal-center"}, []string{"users-info", "user-detail", "auth-modifypwd", "client-user-center-resetuserpassword", "users-person-restname", "users-person-select-enableisopened", "v1-user-detail", "v1-user-getusergroupedservicelist", "v1-user-getredirecturl"}},
	{"user.bindings", "用户中心", "手机邮箱/微信/企业应用绑定", []string{"personal-center", "bind-info"}, []string{"person-getbindinfos", "person-getbindqr-authconfigid", "person-bindmobileemail", "bindwechatorens-find-bindstatus", "users-auth-bindmobileoremail", "users-bindmobileemail-send-code", "users-bindstatus-find-type", "users-bindwechatorens-find-bindstatus", "client-user-ditalk-bind"}},
	{"user.devices", "用户中心", "活动设备/下线/解绑", []string{"personal-center", "device-unbind"}, []string{"device-list-page", "center-device-offline", "center-device-unbind", "device-unbinduser", "users-device-bind-labels", "users-device-bindinfo-token-get", "users-device-getbyfeaturecode-featurecode", "users-device-labels-info", "users-center-device-offline", "users-center-device-unbind", "users-device-unbinduser", "users-terminal-csandnetworkcontrol", "users-unbind-check-code", "users-unbind-message-bytype", "users-unbind-send-code"}},
	{"user.avatar", "用户中心", "头像查询/上传", []string{"personal-center"}, []string{"static-avatar-query-username", "center-uploadpicture", "v1-static-avatar-set"}},
	{"user.common-location", "用户中心", "常用位置", []string{"personal-center", "work-bench"}, []string{"users-confirmnewcommonlocation", "users-updatecommonlocation"}},
	{"protocol.state", "协议", "协议状态读取/设置", []string{"mac-privacy", "home-sys-config"}, []string{"user-getuserprotocolstate", "user-setuserprotocolstate"}},
	{"native.gateway", "客户端", "网关/NAC/本地代理", []string{"home-sys-config", "terminal-info"}, []string{"gateway-switch", "gateway-turnon", "nac-login", "nac-logout", "nac-netisolation-type", "nac-queryconfig", "nac-queryinfo"}},
}

var vpnAPINamePattern = regexp.MustCompile(`[^A-Za-z0-9]+`)

func vpnAPIName(path string) string {
	value := strings.Trim(path, "/")
	value = strings.TrimPrefix(value, "api/")
	value = vpnAPINamePattern.ReplaceAllString(value, "-")
	return strings.ToLower(strings.Trim(value, "-"))
}

func vpnAPICatalog() []map[string]any {
	result := make([]map[string]any, 0, len(vpnAPIRows))
	for _, row := range vpnAPIRows {
		result = append(result, map[string]any{
			"name": vpnAPIName(row.path), "method": row.method, "path": row.path,
			"group": vpnAPIGroup(row.path), "mutating": vpnAPIMutating(row.method, row.path),
		})
	}
	return result
}

func vpnAPIGroup(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "/auth/"), strings.Contains(lower, "/login"), strings.Contains(lower, "/captcha"), strings.Contains(lower, "/qrcode"):
		return "认证与登录"
	case strings.Contains(lower, "/approve/"), strings.Contains(lower, "/apply"):
		return "申请与审批"
	case strings.Contains(lower, "/message/"), strings.Contains(lower, "/banner/"):
		return "消息与公告"
	case strings.Contains(lower, "/service/"):
		return "应用与服务"
	case strings.Contains(lower, "/device/"), strings.Contains(lower, "/terminal/"), strings.Contains(lower, "/peripheral"):
		return "设备与终端"
	case strings.Contains(lower, "/safespace/"), strings.Contains(lower, "/share/"), strings.Contains(lower, "/desktop/"):
		return "安全空间与文件"
	case strings.Contains(lower, "/avengine/"), strings.Contains(lower, "/safetyassess/"), strings.Contains(lower, "/antivirus/"):
		return "安全中心"
	case strings.Contains(lower, "/appmarket/"):
		return "应用商店"
	case strings.Contains(lower, "/control/"), strings.Contains(lower, "/gateway/"), strings.Contains(lower, "/nac/"), strings.Contains(lower, "/local/"):
		return "客户端与网络"
	case strings.Contains(lower, "/user/"), strings.Contains(lower, "/person/"):
		return "用户中心"
	default:
		return "其他"
	}
}

func vpnAPIMutating(method, path string) bool {
	method = strings.ToUpper(method)
	if method == "GET" || method == "HEAD" || method == "OPTIONS" {
		lower := strings.ToLower(path)
		return strings.Contains(lower, "/delete/") || strings.Contains(lower, "/logout")
	}
	lower := strings.ToLower(path)
	for _, token := range []string{"get", "find", "list", "page", "count", "info", "detail", "query", "result", "flow", "check", "supported", "current", "latest", "baseinfo", "download", "applydetail", "flowimage", "waitinghandle", "myhandle"} {
		if strings.Contains(lower, "/"+token) || strings.HasSuffix(lower, token) {
			return false
		}
	}
	return true
}

func vpnSpec(name string) (vpnAPISpec, bool) {
	needle := strings.ToLower(strings.TrimSpace(name))
	if canonical, ok := map[string]string{
		"antivirus-trustfile-page":    "v1-api-users-antivirus-trustfile-page",
		"message-secauth-msgid":       "client-message-secauth-msgid",
		"mfa-commonauth":              "client-mfa-commonauth",
		"mfa-homeinfo-anchorid":       "client-mfa-homeinfo-anchorid",
		"mfa-login-commonauth":        "client-mfa-login-commonauth",
		"mfa-login-homeinfo-anchorid": "client-mfa-login-homeinfo-anchorid",
		"mfa-sendverifycode":          "client-mfa-sendverifycode",
		"share-link-delete-id":        "client-share-link-delete-id",
		"share-link-page":             "client-share-link-page",
	}[needle]; ok {
		needle = canonical
	}
	for _, row := range vpnAPIRows {
		if vpnAPIName(row.path) == needle || strings.TrimPrefix(vpnAPIName(row.path), "users-") == needle || strings.TrimPrefix(vpnAPIName(row.path), "v1-") == needle {
			return row, true
		}
	}
	return vpnAPISpec{}, false
}

func vpnFindRoute(value string) (vpnRoute, bool, bool) {
	needle := strings.ToLower(strings.TrimSpace(value))
	var found vpnRoute
	count := 0
	for _, route := range vpnRoutes {
		if strings.ToLower(route.name) == needle || strings.ToLower(route.path) == needle {
			found, count = route, count+1
			continue
		}
		for _, alias := range []string{strings.TrimPrefix(route.path, "/login/"), strings.TrimPrefix(route.path, "/home/")} {
			if alias != route.path && strings.ToLower(alias) == needle {
				found, count = route, count+1
			}
		}
	}
	return found, count > 0, count > 1
}

func vpnAPIPath(base *url.URL, path string, native bool) (*url.URL, *siteError) {
	if strings.TrimSpace(path) == "" {
		return nil, &siteError{Code: "invalid_path", Message: "VPN API 路径不能为空"}
	}
	path = strings.TrimSpace(path)
	parsed, err := url.Parse(path)
	if err != nil || parsed.User != nil {
		return nil, &siteError{Code: "invalid_path", Message: "VPN API 路径格式无效"}
	}
	if parsed.IsAbs() || parsed.Host != "" {
		if !strings.EqualFold(parsed.Hostname(), base.Hostname()) {
			return nil, &siteError{Code: "invalid_path", Message: "VPN API 必须保持当前 VPN 主机"}
		}
	} else {
		requestPath := parsed.Path
		for i := 0; i < 2; i++ {
			requestPath, _ = url.PathUnescape(requestPath)
		}
		if strings.Contains(requestPath, "\\") {
			return nil, &siteError{Code: "invalid_path", Message: "VPN API 路径无效"}
		}
		for _, part := range strings.Split(requestPath, "/") {
			if part == "." || part == ".." {
				return nil, &siteError{Code: "invalid_path", Message: "VPN API 路径不能包含目录跳转"}
			}
		}
		if native {
			if strings.HasPrefix(parsed.Path, "/enclient/") {
				parsed.Path = strings.TrimPrefix(parsed.Path, "/enclient")
			}
			if !strings.HasPrefix(parsed.Path, "/api/") {
				parsed.Path = "/api/v1/" + strings.TrimPrefix(parsed.Path, "/")
			}
		} else if !strings.HasPrefix(parsed.Path, "/enclient/") {
			parsed.Path = "/enclient/" + strings.TrimPrefix(parsed.Path, "/")
		}
		parsed.Scheme, parsed.Host = base.Scheme, base.Host
	}
	return parsed, nil
}
