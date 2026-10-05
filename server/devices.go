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
	RegisteredAt time.Time `json:"registered_at"`
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
func (s *Server) deviceToken(next http.HandlerFunc) http.HandlerFunc {
	return s.cors(func(w http.ResponseWriter, r *http.Request) {
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

// POST /v1/devices/heartbeat {machine_code} → 更新在线状态；未注册机器码 → 404 提示先注册。
func (s *Server) handleDevicesHeartbeat(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "机器码未注册"})
		return
	}
	rec.Online = true
	rec.LastSeen = time.Now()
	s.devices.save()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "online": true})
}

// POST /v1/devices/lookup {machine_code} → {name, base, online, token}。
// 免鉴权：机器码即凭证。未注册/离线仍返回（iOS 探测直连后决定同网直连或云道转发）。
func (s *Server) handleDevicesLookup(w http.ResponseWriter, r *http.Request) {
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
		"name": rec.Name, "base": rec.Base, "online": rec.Online, "token": rec.Token})
}
