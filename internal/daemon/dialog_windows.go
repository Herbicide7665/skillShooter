//go:build windows

package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Win32 原生配置对话框（纯 syscall，零新依赖）：
// 内存构建 DLGTEMPLATE + WS_CHILD 控件，模态弹出；确定后写 config.json 由调用方重启生效。

var (
	comctl32    = syscall.NewLazyDLL("comctl32.dll")
	user32dlg   = syscall.NewLazyDLL("user32.dll")
	kernel32dlg = syscall.NewLazyDLL("kernel32.dll")
	getSysColor = user32dlg.NewProc("GetSysColor")
)

const (
	ctlServer   = 1001
	ctlToken    = 1002
	ctlHost     = 1003
	ctlHB       = 1004
	ctlWatch    = 1005
	ctlApply    = 1006
	ctlRoots    = 1007
	btnTest   = 1010
	btnOK     = 1 // IDOK
	btnCancel = 2 // IDCANCEL
)

const (
	_WS_CHILD       = 0x40000000
	_WS_VISIBLE     = 0x10000000
	_WS_TABSTOP     = 0x00010000
	_ES_AUTOHSCROLL = 0x0080
	_SS_LEFT        = 0x00000000
	BS_PUSHBUTTON   = 0x00000000
	// WS_EX_DLGMODALFRAME=1 | WS_EX_CLIENTEDGE=0x200
	_WSEX_EDIT      = 0x00000201
	_WSEX_BTN       = 0x00000000
	IDC_ARROW_CONST = 32512
)

type dlgTemplateExLevel struct {
	idcArrow syscall.Handle
}

type fieldDef struct {
	label      string
	defaultVal string
	ctrlID     int
	hint       string
}

var dlgFields = []fieldDef{
	{"服务端地址", "", ctlServer, "http://your-server:8790"},
	{"Token", "", ctlToken, "服务端设置页创建的 user token"},
	{"设备名", "", ctlHost, "留空=自动取主机名"},
	{"心跳间隔(分钟)", "30", ctlHB, "客户端定期上报在线状态"},
	{"监控间隔(秒)", "30", ctlWatch, "本地 skill 变更上云轮询"},
	{"应用云端合并(秒)", "300", ctlApply, "云端合并成果落回本地"},
	{"监控目录(逗号分隔)", "", ctlRoots, "留空=自动发现 .agents/.claude/.zcode"},
}

type dlgState struct {
	cfg  ClientConfig
	done bool
	saved bool
	hInst syscall.Handle
}

// ShowConfigDialog 弹出配置窗；返回 true=用户点了确定（配置已写盘）
// 在无配置首次启动与托盘「配置…」菜单两个场景调用。
func ShowConfigDialog(initial ClientConfig) bool {
	st := &dlgState{cfg: initial}
	initDlgDefaults(st)
	hInst, _, _ := kernel32dlg.NewProc("GetModuleHandleW").Call(0)
	st.hInst = syscall.Handle(hInst)

	// 预填当前配置
	if initial.Server != "" || initial.Token != "" {
		dlgFields[0].defaultVal = initial.Server
		dlgFields[1].defaultVal = initial.Token
		dlgFields[2].defaultVal = initial.Hostname
		if initial.HeartbeatSec > 0 {
			dlgFields[3].defaultVal = strconv.Itoa(initial.HeartbeatSec / 60)
		}
		if initial.Interval > 0 {
			dlgFields[4].defaultVal = strconv.Itoa(initial.Interval)
		}
		if initial.ApplyInterval > 0 {
			dlgFields[5].defaultVal = strconv.Itoa(initial.ApplyInterval)
		}
		if len(initial.Roots) > 0 {
			dlgFields[6].defaultVal = strings.Join(initial.Roots, ",")
		}
	} else {
		dlgFields[0].defaultVal = ""
		dlgFields[1].defaultVal = ""
		dlgFields[2].defaultVal = ""
	}

	tmpl := buildDialogTemplate()
	proc := syscall.NewCallback(dlgProcWrap(st))
	user32dlg.NewProc("DialogBoxIndirectParamW").Call(
		uintptr(st.hInst), uintptr(unsafe.Pointer(tmpl)), 0, proc, 0)
	return st.saved
}

func initDlgDefaults(st *dlgState) {
	if dlgFields[3].defaultVal == "" {
		dlgFields[3].defaultVal = "30"
	}
	if dlgFields[4].defaultVal == "" {
		dlgFields[4].defaultVal = "30"
	}
	if dlgFields[5].defaultVal == "" {
		dlgFields[5].defaultVal = "300"
	}
}

// ---- DLGTEMPLATE 内存构建 ----

func align4(p int) int { return (p + 3) &^ 3 }

func buildDialogTemplate() *byte {
	// DLGTEMPLATEW: style/xdword/cdit/xdword/y/x/cx/cy + menu/class/title + fonts
	buf := make([]byte, 0, 2048)
	w := func(v uint16) { buf = append(buf, byte(v), byte(v>>8)) }
	dw := func(v uint32) {
		buf = append(buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	strz := func(s string) {
		r, _ := syscall.UTF16FromString(s)
		for _, c := range r {
			w(c)
		}
	}

	const (
		WS_POPUP    = 0x80000000
		WS_CAPTION  = 0x00C00000
		WS_SYSMENU  = 0x00080000
		DS_MODAL    = 0x80 // DS_MODALFRAME
		DS_SETFONT  = 0x40
		DS_CENTER   = 0x800
	)
	dw(uint32(WS_POPUP | WS_CAPTION | WS_SYSMENU | DS_MODAL | DS_SETFONT | DS_CENTER)) // style
	dw(0)                                                                               // exStyle
	// cdit: 7 字段×(label+edit) + 2 按钮 + 1 测试连接按钮 + 状态行 = 20
	dw(20)
	w(0) // x y cx cy（居中，坐标系统像素比）
	w(0)
	w(300)
	w(210)
	w(0)     // menu: none
	w(0)     // window class: predefined dialog
	strz("SkillShooter 客户端配置")
	w(9)     // font size
	strz("Segoe UI") // font name

	// ---- DLGITEMTEMPLATE 列表 ----
	// 布局行高（dlg 单位约 0.35rem）：
	type rect struct{ x, y, cx, cy uint16 }
	item := func(style, exStyle uint32, r rect, id uint16, class string, title string, extra uint16) {
		p := len(buf)
		_ = p
		dw(style | _WS_CHILD | _WS_VISIBLE)
		dw(exStyle)
		w(r.x); w(r.y); w(r.cx); w(r.cy)
		w(id)
		// class atom 预定义：0x0080=Button 0x0081=Edit 0x0082=Static
		switch class {
		case "BUTTON":
			w(0x0080)
		case "EDIT":
			w(0x0081)
		default:
			w(0x0082)
		}
		strz(title)
		// creation data count = 0
		w(0)
	}

	y := uint16(7)
	for i, f := range dlgFields {
		item(_SS_LEFT, 0, rect{8, y, 92, 8}, 0xFFFF, "STATIC", f.label, 0)
		item(_WS_TABSTOP|_ES_AUTOHSCROLL, _WSEX_EDIT, rect{104, y, 190, 12}, uint16(f.ctrlID), "EDIT", f.defaultVal, 0)
		if i == 6 {
			// 监控目录行给宽一点 + 换行留高
			y += 16
		} else {
			y += 15
		}
		if i == 5 {
			y += 4 // 给 dirs 一点空隙
		}
	}
	// 按钮行
	item(_WS_TABSTOP|BS_PUSHBUTTON, _WSEX_BTN, rect{104, y, 60, 14}, btnTest, "BUTTON", "测试连接", 0)
	item(_WS_TABSTOP|BS_PUSHBUTTON, _WSEX_BTN, rect{172, y, 60, 14}, btnOK, "BUTTON", "确定并重启", 0)
	item(_WS_TABSTOP|BS_PUSHBUTTON, _WSEX_BTN, rect{238, y, 56, 14}, btnCancel, "BUTTON", "取消", 0)
	item(_SS_LEFT, 0, rect{8, uint16(y+18), 288, 8}, 0xFFFE, "STATIC", "保存到 ~/.skillshooter/config.json，确定后客户端自动重启生效", 0)

	// 4 字节对齐
	if len(buf)%4 != 0 {
		buf = append(buf, make([]byte, 4-len(buf)%4)...)
	}
	return &buf[0]
}

// ---- dlgProc ----

func getDlgItemText(hDlg syscall.Handle, id int) string {
	lenProc := user32dlg.NewProc("GetWindowTextLengthW")
	hItem, _, _ := user32dlg.NewProc("GetDlgItem").Call(uintptr(hDlg), uintptr(id))
	if hItem == 0 {
		return ""
	}
	n, _, _ := lenProc.Call(hItem)
	if n == 0 {
		return ""
	}
	b := make([]uint16, n+1)
	user32dlg.NewProc("GetWindowTextW").Call(hItem, uintptr(unsafe.Pointer(&b[0])), n+1)
	return strings.TrimSpace(syscall.UTF16ToString(b))
}

func setDlgItemText(hDlg syscall.Handle, id int, text string) {
	p, _ := syscall.UTF16PtrFromString(text)
	user32dlg.NewProc("SetDlgItemTextW").Call(uintptr(hDlg), uintptr(id), uintptr(unsafe.Pointer(p)))
}

func setDlgItemInt(hDlg syscall.Handle, id int, v int) {
	user32dlg.NewProc("SetDlgItemInt").Call(uintptr(hDlg), uintptr(id), uintptr(v), 1)
}

func msgBox(text, title string) {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(title)
	user32dlg.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x40)
}

func (st *dlgState) collect(hDlg syscall.Handle) (ClientConfig, bool) {
	cfg := ClientConfig{}
	cfg.Server = getDlgItemText(hDlg, ctlServer)
	cfg.Token = getDlgItemText(hDlg, ctlToken)
	cfg.Hostname = getDlgItemText(hDlg, ctlHost)
	if cfg.Server == "" || cfg.Token == "" {
		return cfg, false
	}
	if v, err := strconv.Atoi(getDlgItemText(hDlg, ctlHB)); err == nil && v > 0 {
		cfg.HeartbeatSec = v * 60
	}
	if v, err := strconv.Atoi(getDlgItemText(hDlg, ctlWatch)); err == nil && v > 0 {
		cfg.Interval = v
	}
	if v, err := strconv.Atoi(getDlgItemText(hDlg, ctlApply)); err == nil && v > 0 {
		cfg.ApplyInterval = v
	}
	cfg.JobPollSec = 5
	for _, r := range strings.Split(getDlgItemText(hDlg, ctlRoots), ",") {
		if r = strings.TrimSpace(r); r != "" {
			cfg.Roots = append(cfg.Roots, r)
		}
	}
	return cfg, true
}

// testConnect 用窗内填的 server+token 调 /api/whoami（不落盘）
func testConnect(server, token string) string {
	if !strings.HasPrefix(server, "http") {
		server = "http://" + server
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", strings.TrimRight(server, "/")+"/api/whoami", nil)
	if err != nil {
		return "地址无效: " + err.Error()
	}
	req.Header.Set("X-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		return "连接失败: " + err.Error()
	}
	defer resp.Body.Close()
	var out struct {
		OK    bool   `json:"ok"`
		Role  string `json:"role"`
		Machine string `json:"machine"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "响应解析失败: " + err.Error()
	}
	if !out.OK {
		return "token 无效或已吊销"
	}
	bound := out.Machine
	if bound == "" {
		bound = "未绑定设备（本 token 为 admin 或尚未认领）"
	}
	return fmt.Sprintf("连接正常 · 角色: %s · 绑定设备: %s", out.Role, bound)
}

const (
	WM_INITDIALOG = 0x0110
	WM_COMMAND     = 0x0111
	WM_CLOSE       = 0x0010
)

const (
	WMCOMMAND_IDMASK = 0xFFFF
	BNCLICKED        = 0
)

// 状态行 ID
const statLineID = 0xFFFE

var dlgProcMu sync.Mutex

func dlgProcWrap(st *dlgState) func(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	return func(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
		switch msg {
		case WM_INITDIALOG:
			return 1
		case WM_COMMAND:
			cmd := wParam >> 16 & 0xFFFF
			id := wParam & 0xFFFF
			if cmd != 0 { // BN_CLICKED == 0
				return 0
			}
			_ = BNCLICKED
			switch id {
			case btnTest:
				srv := getDlgItemText(hwnd, ctlServer)
				tok := getDlgItemText(hwnd, ctlToken)
				if srv == "" || tok == "" {
					setDlgItemText(hwnd, statLineID, "请先填写服务端地址与 Token")
					return 1
				}
				setDlgItemText(hwnd, statLineID, "测试中…")
				res := testConnect(srv, tok)
				setDlgItemText(hwnd, statLineID, res)
				return 1
			case btnOK:
				cfg, ok := st.collect(hwnd)
				if !ok {
					setDlgItemText(hwnd, statLineID, "服务端地址与 Token 必填")
					return 1
				}
				// 保留用户既有 no_* 开关（collect 未覆盖的运行开关）
				if old, okOld := LoadClientConfig(); okOld {
					cfg.NoWatch, cfg.NoApply, cfg.NoJobs, cfg.NoHeartbeat = old.NoWatch, old.NoApply, old.NoJobs, old.NoHeartbeat
					cfg.JobPollSec = old.JobPollSec
				}
				if err := SaveClientConfig(cfg); err != nil {
					setDlgItemText(hwnd, statLineID, "保存失败: "+err.Error())
					return 1
				}
				st.saved = true
				user32dlg.NewProc("EndDialog").Call(uintptr(hwnd), 1)
				return 1
			case btnCancel:
				user32dlg.NewProc("EndDialog").Call(uintptr(hwnd), 0)
				return 1
			}
		case WM_CLOSE:
			user32dlg.NewProc("EndDialog").Call(uintptr(hwnd), 0)
			return 1
		}
		return 0
	}
}

// ShowConfigDialog 的跨平台入口包装（非 Windows 在 tray_stub 用 CLI 提示实现）
var _ = getSysColor
