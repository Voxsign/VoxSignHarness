//
//  machine.go：机器码机制（设计稿 v3）——
//  每台 Harness 装机时生成唯一机器码（XXXX-XXXX-XXXX，持久化 <log_dir>/machine.json）；
//  serve 启动时若配置 VHS_DEVICE_SERVER（云道地址），自动注册（name+LAN base）并
//  每 2 分钟心跳保活。iOS 输机器码 → 云道 lookup → 同网直连 / 异网转发。
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

// 去掉易混淆字符（0/O/1/I/L）。
const machineCodeChars = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// 自适应心跳（架构 v1 §5）：忙快闲慢——decision 5s / busy 15s / idle 2min / standby 5min。
const (
	hbDecision = 5 * time.Second
	hbBusy     = 15 * time.Second
	hbIdle     = 2 * time.Minute
	hbStandby  = 5 * time.Minute

	// standbyAfterIdle：idle 持续这么久（无任何任务活动）降为 standby（最省心跳）。
	standbyAfterIdle = 5 * time.Minute
)

// heartbeatStateFn 读取当前心跳状态（由 server 任务表驱动）；version 是状态版本号，
// 变化即补发（防毫秒级状态闪烁漏报）。
type heartbeatStateFn func() (state string, pending int, version int64)

// hbSampleInterval 状态采样间隔：1s 内发现状态切换并立即补发（事件驱动 + 周期双轨）。
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

// loadMachineCode 读取或生成机器码（幂等，首次生成后持久化）。
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

// lanIP 取首个非回环 IPv4（注册 base 用；取不到退 127.0.0.1）。
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

// startDeviceRegistration serve 启动时调用：VHS_DEVICE_SERVER 非空 → 注册 + 自适应心跳。
// stateFn 提供任务表状态（decision/busy/idle），心跳按状态切换周期，切换瞬间立即补发；
// 心跳 404（云道数据丢失）则重新注册。payload 携带 state/pending/uptime/v（架构 v1 §5.2）。
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
			fmt.Printf("device: 机器码 %s 已注册 %s（%s）\n", code, server, base)
		} else {
			fmt.Printf("device: 注册失败 %s（将重试）\n", server)
		}
	}

	heartbeat := func(state string, pending int) {
		payload := fmt.Sprintf(`{"machine_code":%q,"state":%q,"pending":%d,"uptime_s":%d,"v":"0.2.1"}`,
			code, state, pending, int(time.Since(startedAt).Seconds()))
		if !call("/v1/devices/heartbeat", payload) {
			register() // 云道可能重启丢数据，重注册幂等
		}
	}

	// effectiveState 读任务表状态；idle 持续超过 standbyAfterIdle → 降 standby。
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
			// 事件驱动：状态或版本变化 → 立即补发（不等周期）。
			if state != prevState || ver != prevVer {
				heartbeat(state, pending)
				prevState, prevVer, lastSent = state, ver, now
			} else if now.Sub(lastSent) >= heartbeatPeriod(state) {
				// 周期心跳：按当前状态频率（忙快闲慢）。
				heartbeat(state, pending)
				lastSent = now
			}
			time.Sleep(hbSampleInterval)
		}
	}()
}

// cmdMachineCode 打印本机机器码（vhs machine-code）。
func cmdMachineCode() {
	cfg := loadCfg()
	fmt.Println(loadMachineCode(cfg.Global.LogDir))
}
