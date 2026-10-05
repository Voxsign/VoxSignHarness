//
//  devices.go：云道设备注册表——自建 Harness 机器码机制（设计稿 v3 第 2、4 节）。
//
//  机器（自建 Harness）装机时生成唯一机器码；启动后向云道注册（name+base）并
//  每 2 分钟心跳保活。iOS 输入机器码 → POST /v1/devices/lookup → 拿到机器身份与
//  访问 token：同网直连 base，异网走云道转发（按需，relay 基建后续）。
//
//  鉴权：register/heartbeat 需 Bearer VHS_TOKEN（云端 VHS_TOKEN）；lookup 免鉴权
//  （机器码本身即凭证，iOS 自建模式下无会话 JWT）。
//

package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DeviceRecord 一台已注册机器的目录条目。
type DeviceRecord struct {
	MachineCode  string    `json:"machine_code"`
	Name         string    `json:"name"`
	Base         string    `json:"base"`
	Token        string    `json:"token"` // 云道为该机器下发的访问凭证
	Online       bool      `json:"online"`
	LastSeen     time.Time `json:"last_seen"`
	State        string    `json:"state,omitempty"`  // 架构 v1 §5：idle|busy|decision|standby
	Pending      int       `json:"pending,omitempty"` // 活动任务数（decision/busy 时）
	RegisteredAt time.Time `json:"registered_at"`
}

// heartbeatPeriod 状态 → 心跳周期（架构 v1 §5.1 频率映射）。
func heartbeatPeriod(state string) time.Duration {
	switch state {
	case "decision":
		return 5 * time.Second
	case "busy":
		return 15 * time.Second
	case "standby":
		return 5 * time.Minute
	default: // idle
		return 2 * time.Minute
	}
}

// deviceOnline 窗口化在线判定（架构 v1 §5.3）：now − last_seen < 3×当前周期
// 才算在线——慢心跳（idle/standby）不会误判离线。
func deviceOnline(rec *DeviceRecord) bool {
	if rec == nil || rec.LastSeen.IsZero() {
		return false
	}
	return time.Since(rec.LastSeen) < 3*heartbeatPeriod(rec.State)
}

// deviceRegistry 设备注册表（<log_dir>/devices/devices.json 持久化）。
type deviceRegistry struct {
	mu      sync.Mutex
	path    string
	devices map[string]*DeviceRecord // machine_code → record
}

func newDeviceRegistry(logDir string) *deviceRegistry {
	dir := filepath.Join(logDir, "devices")
	_ = os.MkdirAll(dir, 0o755)
	r := &deviceRegistry{path: filepath.Join(dir, "devices.json"), devices: map[string]*DeviceRecord{}}
	if data, err := os.ReadFile(r.path); err == nil {
		var list []*DeviceRecord
		if json.Unmarshal(data, &list) == nil {
			for _, d := range list {
				if d != nil && d.MachineCode != "" {
					r.devices[d.MachineCode] = d
				}
			}
		}
	}
	return r
}

func (r *deviceRegistry) save() {
	list := make([]*DeviceRecord, 0, len(r.devices))
	for _, d := range r.devices {
		list = append(list, d)
	}
	if data, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(r.path, data, 0o644)
	}
}

func newDeviceToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// deviceToken 设备接口鉴权：Bearer/X-Token == 服务端 VHS_TOKEN。
// （云道模式下 register/heartbeat 由 Harness 端持有 VHS_TOKEN 调用，不要求会话 JWT。）
//
// 防护顺序（2026-10-05 收尾修复，docs/测试结果报告-云端服务器端功能-20261005.md 问题 2/3）：
// 先判 s.devices == nil（本地模式，server.New 仅云端模式创建注册表）→ 501。
// 设备功能按设计仅云端可用；本地模式的问题是"功能未启用"而非"配置错误"，
// 故先于 token 检查判定，且绝不触碰 s.devices（否则 devices.go 注册处理处 nil deref panic）。
// 再判 s.cfg.Server.Token == ""（云端模式漏配 VHS_TOKEN）→ 503。
// 属服务端配置错误——客户端无从提供有效凭证，故不用 401（401 暗示"带正确 token 再来"，
// 此处根本没有 token 可带）；硬保护堵住"两端凭证都为空即放行 register"的生产风险。
// 最后正常比较：凭证不匹配 → 401。
func (s *Server) deviceToken(next http.HandlerFunc) http.HandlerFunc {
	return s.cors(func(w http.ResponseWriter, r *http.Request) {
		if s.devices == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "设备注册表仅云端模式可用（本地模式不支持设备注册）"})
			return
		}
		if s.cfg.Server.Token == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "服务端未配置 VHS_TOKEN，设备接口鉴权不可用（服务端配置错误）"})
			return
		}
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" {
			tok = r.Header.Get("X-Token")
		}
		if tok != s.cfg.Server.Token {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token 无效"})
			return
		}
		next(w, r)
	})
}

// POST /v1/devices/register {machine_code, name, base} → 登记/更新 + {ok, machine_code, token}。
// 幂等：同一机器码重复注册仅刷新 name/base/在线状态；首次注册签发访问 token。
func (s *Server) handleDevicesRegister(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MachineCode string `json:"machine_code"`
		Name        string `json:"name"`
		Base        string `json:"base"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body 解析失败"})
		return
	}
	in.MachineCode = strings.TrimSpace(in.MachineCode)
	if in.MachineCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "machine_code 不能为空"})
		return
	}
	s.devices.mu.Lock()
	rec, ok := s.devices.devices[in.MachineCode]
	if !ok {
		rec = &DeviceRecord{MachineCode: in.MachineCode, Token: newDeviceToken(), RegisteredAt: time.Now()}
		s.devices.devices[in.MachineCode] = rec
	}
	rec.Name = in.Name
	rec.Base = in.Base
	rec.Online = true
	rec.LastSeen = time.Now()
	s.devices.save()
	s.devices.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "machine_code": rec.MachineCode, "token": rec.Token})
}

// POST /v1/devices/heartbeat {machine_code, state, pending} → 更新在线状态 + 状态；
// 未注册机器码 → 404 提示先注册。online 判定窗口化（架构 v1 §5.3）。
func (s *Server) handleDevicesHeartbeat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MachineCode string `json:"machine_code"`
		State       string `json:"state"`
		Pending     int    `json:"pending"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body 解析失败"})
		return
	}
	s.devices.mu.Lock()
	defer s.devices.mu.Unlock()
	rec, ok := s.devices.devices[strings.TrimSpace(in.MachineCode)]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "机器码未注册"})
		return
	}
	rec.Online = true
	rec.LastSeen = time.Now()
	if in.State != "" {
		rec.State = in.State
	}
	rec.Pending = in.Pending
	s.devices.save()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "online": true, "state": rec.State})
}

// POST /v1/devices/lookup {machine_code} → {name, base, online, state, pending, token}。
// 免鉴权：机器码即凭证。未注册/离线仍返回（iOS 探测直连后决定同网直连或云道转发）。
// 注意：本路由走 public（不经 deviceToken 中间件），本地模式 s.devices 为 nil，
// 入口必须自行防护，否则 nil deref panic（与 register/heartbeat 同根因）。
func (s *Server) handleDevicesLookup(w http.ResponseWriter, r *http.Request) {
	if s.devices == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "设备注册表仅云端模式可用（本地模式不支持设备查询）"})
		return
	}
	var in struct {
		MachineCode string `json:"machine_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body 解析失败"})
		return
	}
	s.devices.mu.Lock()
	defer s.devices.mu.Unlock()
	rec, ok := s.devices.devices[strings.TrimSpace(in.MachineCode)]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "机器码无效或未注册"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": rec.Name, "base": rec.Base,
		"online": deviceOnline(rec), "state": rec.State, "pending": rec.Pending,
		"token": rec.Token})
}
