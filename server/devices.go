//
//  devices.go:     note table--   Harness   code restrict(    v3   2, 4 node). 
//
//    (   Harness)  timeoccurbecomeunique  code; start afterto  note (name+base)and
//    2 splitclock  keep . iOS  in  code -> POST /v1/devices/lookup ->  to    and
//     token: same  link base, diff     send(byneed, relay base aftercontinue). 
//
//    : register/heartbeat need Bearer VHS_TOKEN( end VHS_TOKEN); lookup    
//  (  codebase i.e.  , iOS    formunderno   JWT). 
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

// DeviceRecord   alreadynote    obj  obj. 
type DeviceRecord struct {
	MachineCode  string    `json:"machine_code"`
	Name         string    `json:"name"`
	Base         string    `json:"base"`
	Token        string    `json:"token"` //   as   undersend     
	Online       bool      `json:"online"`
	LastSeen     time.Time `json:"last_seen"`
	State        string    `json:"state,omitempty"`  //    v1 §5: idle|busy|decision|standby
	Pending      int       `json:"pending,omitempty"` //   tasknum(decision/busy time)
	Tenant       string    `json:"tenant,omitempty"`  //    user(  user  ; baselydefault "default")
	RegisteredAt time.Time `json:"registered_at"`
}

// heartbeatPeriod status ->    period(   v1 §5.1 freqrate  ). 
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

// deviceOnline   ize line  (   v1 §5.3): now − last_seen < 3×curbefore period
// only  line--slow  (idle/standby)     line. 
func deviceOnline(rec *DeviceRecord) bool {
	if rec == nil || rec.LastSeen.IsZero() {
		return false
	}
	return time.Since(rec.LastSeen) < 3*heartbeatPeriod(rec.State)
}

// deviceRegistry   note table(<log_dir>/devices/devices.json keep ize). 
type deviceRegistry struct {
	mu      sync.Mutex
	path    string
	devices map[string]*DeviceRecord // machine_code -> record
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

// findByToken by   token rev   (    middle use).   inreturnback nil. 
// O(n):   num as  num(  num), noneed  . keep bycalluse decide (      , read-onlysafesafety). 
func (r *deviceRegistry) findByToken(tok string) *DeviceRecord {
	if tok == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.devices {
		if d != nil && d.Token == tok {
			return d
		}
	}
	return nil
}

// deviceToken   connect   : Bearer/X-Token == serveserviceend VHS_TOKEN. 
// (   formunder register/heartbeat by Harness endkeephas VHS_TOKEN calluse,  needrequire   JWT. )
//
// preventprotect  (2026-10-05 recvtailfix , docs/  close   - endserveservice end  -20261005.md    2/3): 
// first  s.devices == nil(basely form, server.New only end form  note table)-> 501. 
//     by  only end use; basely form   is"   startuse"but "  error", 
// thusfirstat token     , and  trigger  s.devices( then devices.go note handleplace nil deref panic). 
// again  s.cfg.Server.Token == ""( end form   VHS_TOKEN)-> 503. 
//  serveserviceend  error--clientuserendnofrom providehas   , thus use 401(401  show" pos  token again ", 
//  placerootbase has token   );  protect  " end  allasemptyi.e.   register" occurproducerisk. 
//  afterpos   :       -> 401. 
func (s *Server) deviceToken(next http.HandlerFunc) http.HandlerFunc {
	return s.cors(func(w http.ResponseWriter, r *http.Request) {
		if s.devices == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "device registry available only in cloud mode (local mode does not support device registration)"})
			return
		}
		if s.cfg.Server.Token == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "VHS_TOKEN not configured on server; device endpoint auth unavailable (server misconfiguration)"})
			return
		}
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" {
			tok = r.Header.Get("X-Token")
		}
		if tok != s.cfg.Server.Token {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		next(w, r)
	})
}

// POST /v1/devices/register {machine_code, name, base} ->   /changenew + {ok, machine_code, token}. 
//  etc: same   codeheavy note only new name/base/ linestatus; first note  send   token. 
func (s *Server) handleDevicesRegister(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MachineCode string `json:"machine_code"`
		Name        string `json:"name"`
		Base        string `json:"base"`
		Token       string `json:"token"` //   :  formrefer    token(  /Mac  sidewritesame  token);      send
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse request body"})
		return
	}
	in.MachineCode = strings.TrimSpace(in.MachineCode)
	if in.MachineCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "machine_code must not be empty"})
		return
	}
	in.Token = strings.TrimSpace(in.Token)
	s.devices.mu.Lock()
	rec, ok := s.devices.devices[in.MachineCode]
	if !ok {
		rec = &DeviceRecord{MachineCode: in.MachineCode, RegisteredAt: time.Now()}
		s.devices.devices[in.MachineCode] = rec
	}
	//  form token  firstoverwrite; first note and  formgiveonly   send. 
	if in.Token != "" {
		rec.Token = in.Token
	} else if rec.Token == "" {
		rec.Token = newDeviceToken()
	}
	rec.Name = in.Name
	rec.Base = in.Base
	rec.Online = true
	rec.LastSeen = time.Now()
	//  user  :  end  JWT note timebyonunder write;  thendefault "default"(basely/  user E2E). 
	if t := ctxTenant(r.Context()); t != "" {
		rec.Tenant = t
	} else if rec.Tenant == "" {
		rec.Tenant = "default"
	}
	s.devices.save()
	s.devices.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "machine_code": rec.MachineCode, "token": rec.Token})
}

// POST /v1/devices/heartbeat {machine_code, state, pending} -> changenew linestatus + status; 
//  note   code -> 404  showfirstnote . online     ize(   v1 §5.3). 
func (s *Server) handleDevicesHeartbeat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MachineCode string `json:"machine_code"`
		State       string `json:"state"`
		Pending     int    `json:"pending"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse request body"})
		return
	}
	s.devices.mu.Lock()
	defer s.devices.mu.Unlock()
	rec, ok := s.devices.devices[strings.TrimSpace(in.MachineCode)]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "machine code not registered"})
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

// POST /v1/devices/lookup {machine_code} -> {name, base, online, state, pending, token}. 
//    :   codei.e.  .  note / line returnback(iOS    linkafterdecide same  linkor   send). 
// note : baserouteby  public(   deviceToken middle ), basely form s.devices as nil, 
// in     preventprotect,  then nil deref panic(and register/heartbeat samerootbecause). 
func (s *Server) handleDevicesLookup(w http.ResponseWriter, r *http.Request) {
	if s.devices == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "device registry available only in cloud mode (local mode does not support device lookup)"})
		return
	}
	var in struct {
		MachineCode string `json:"machine_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse request body"})
		return
	}
	s.devices.mu.Lock()
	defer s.devices.mu.Unlock()
	rec, ok := s.devices.devices[strings.TrimSpace(in.MachineCode)]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "machine code invalid or not registered"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": rec.Name, "base": rec.Base,
		"online": deviceOnline(rec), "state": rec.State, "pending": rec.Pending,
		"token": rec.Token})
}
