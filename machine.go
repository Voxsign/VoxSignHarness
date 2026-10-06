//
//  machine.go: machine-code mechanism (design v3).
//  On install, each harness generates a unique machine code (XXXX-XXXX-XXXX),
//  persisted to <log_dir>/machine.json. On serve startup, if VHS_DEVICE_SERVER
//  (the cloud relay address) is configured, it auto-registers (name + LAN base)
//  and heartbeats every few minutes to stay alive. iOS enters the machine code
//  -> cloud relay lookup -> same-LAN direct connect / cross-network relay.
//

package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"voicesign-harness/config"
)

// Ambiguous characters (0/O/1/I/L) are excluded.
const machineCodeChars = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// Adaptive heartbeat (architecture v1 §5): fast when busy, slow when idle —
// decision 5s / busy 15s / idle 2min / standby 5min.
const (
	hbDecision = 5 * time.Second
	hbBusy     = 15 * time.Second
	hbIdle     = 2 * time.Minute
	hbStandby  = 5 * time.Minute

	// standbyAfterIdle: after being idle this long (no task activity), drop to standby (cheapest heartbeat).
	standbyAfterIdle = 5 * time.Minute
)

// heartbeatStateFn reads the current heartbeat state (driven by the server task table);
// version is the state version number, and a change triggers an immediate resend
// (prevents missing millisecond-level state flickers).
type heartbeatStateFn func() (state string, pending int, version int64)

// hbSampleInterval: state sampling interval — detect transitions within 1s and resend
// immediately (event-driven + periodic dual track).
const hbSampleInterval = 1 * time.Second

func heartbeatPeriod(state string) time.Duration {
	switch state {
	case "decision":
		return hbDecision
	case "busy":
		return hbBusy
	case "standby":
		return hbStandby
	default:
		return hbIdle
	}
}

func machineCodePath(logDir string) string { return filepath.Join(logDir, "machine.json") }

// loadMachineCode reads or generates the machine code (idempotent; persisted after first generation).
func loadMachineCode(logDir string) string {
	p := machineCodePath(logDir)
	if data, err := os.ReadFile(p); err == nil {
		var m struct {
			Code string `json:"machine_code"`
		}
		if json.Unmarshal(data, &m) == nil && m.Code != "" {
			return m.Code
		}
	}
	code := newMachineCode()
	_ = os.MkdirAll(logDir, 0o755)
	_ = os.WriteFile(p, []byte(fmt.Sprintf("{\"machine_code\":%q}\n", code)), 0o644)
	return code
}

func newMachineCode() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	var sb strings.Builder
	for i := 0; i < 12; i++ {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteByte(machineCodeChars[int(b[i])%len(machineCodeChars)])
	}
	return sb.String()
}

// lanIP returns the first non-loopback IPv4 (for the registration base; falls back to 127.0.0.1).
func lanIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			ip := ipn.IP.To4()
			if ip != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}

// startDeviceRegistration is called on serve startup: if VHS_DEVICE_SERVER is set,
// register and heartbeat adaptively. stateFn supplies the task-table state
// (decision/busy/idle); heartbeat period switches with state, and a transition
// resends immediately. On a 404 (relay lost state) it re-registers. The payload
// carries state/pending/uptime/v (architecture v1 §5.2).
func startDeviceRegistration(cfg *config.Config, stateFn heartbeatStateFn) {
	server := strings.TrimRight(os.Getenv("VHS_DEVICE_SERVER"), "/")
	if server == "" {
		return
	}
	code := loadMachineCode(cfg.Global.LogDir)
	name, _ := os.Hostname()
	_, port, err := net.SplitHostPort(cfg.Server.Bind)
	if err != nil || port == "" {
		port = "8765"
	}
	base := fmt.Sprintf("http://%s:%s", lanIP(), port)
	token := cfg.Server.Token
	startedAt := time.Now()

	call := func(path string, payload string) bool {
		req, rerr := http.NewRequest(http.MethodPost, server+path, strings.NewReader(payload))
		if rerr != nil {
			return false
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, derr := (&http.Client{Timeout: 8 * time.Second}).Do(req)
		if derr != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode/100 == 2
	}

	register := func() {
		payload := fmt.Sprintf(`{"machine_code":%q,"name":%q,"base":%q}`, code, name, base)
		if call("/v1/devices/register", payload) {
			fmt.Printf("device: machine code %s registered at %s (%s)\n", code, server, base)
		} else {
			fmt.Printf("device: registration failed at %s (will retry)\n", server)
		}
	}

	heartbeat := func(state string, pending int) {
		payload := fmt.Sprintf(`{"machine_code":%q,"state":%q,"pending":%d,"uptime_s":%d,"v":"0.2.1"}`,
			code, state, pending, int(time.Since(startedAt).Seconds()))
		if !call("/v1/devices/heartbeat", payload) {
			register() // the relay may have restarted and lost state; re-registering is idempotent
		}
	}

	// effectiveState reads the task-table state; after being idle beyond standbyAfterIdle -> standby.
	var lastActivity time.Time
	effectiveState := func() (string, int, int64) {
		state, pending, ver := stateFn()
		if state != "idle" {
			lastActivity = time.Now()
		}
		if state == "idle" && !lastActivity.IsZero() &&
			time.Since(lastActivity) > standbyAfterIdle {
			return "standby", 0, ver
		}
		return state, pending, ver
	}

	go func() {
		register()
		prevState, prevVer, lastSent := "", int64(-1), time.Time{}
		for {
			state, pending, ver := effectiveState()
			now := time.Now()
			// Event-driven: state or version change -> resend immediately (don't wait for the period).
			if state != prevState || ver != prevVer {
				heartbeat(state, pending)
				prevState, prevVer, lastSent = state, ver, now
			} else if now.Sub(lastSent) >= heartbeatPeriod(state) {
				// Periodic heartbeat: pace follows the current state (fast when busy, slow when idle).
				heartbeat(state, pending)
				lastSent = now
			}
			time.Sleep(hbSampleInterval)
		}
	}()
}

// cmdMachineCode prints this machine's code (vhs machine-code).
func cmdMachineCode() {
	cfg := loadCfg()
	fmt.Println(loadMachineCode(cfg.Global.LogDir))
}
