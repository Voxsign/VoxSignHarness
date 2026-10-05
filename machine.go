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

// startDeviceRegistration serve 启动时调用：VHS_DEVICE_SERVER 非空 → 注册 + 心跳。
// 注册（幂等）→ 每 2 分钟心跳；心跳 404（云道数据丢失）则重新注册。
func startDeviceRegistration(cfg *config.Config) {
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

	go func() {
		register()
		tick := time.NewTicker(2 * time.Minute)
		defer tick.Stop()
		for range tick.C {
			payload := fmt.Sprintf(`{"machine_code":%q}`, code)
			if !call("/v1/devices/heartbeat", payload) {
				register() // 云道可能重启丢数据，重注册幂等
			}
		}
	}()
}

// cmdMachineCode 打印本机机器码（vhs machine-code）。
func cmdMachineCode() {
	cfg := loadCfg()
	fmt.Println(loadMachineCode(cfg.Global.LogDir))
}
