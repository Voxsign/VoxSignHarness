//
// relay.go：机器码异网转发基建（端口按服务分、不按机器分）。
//
// 云端只暴露 443（内部 8898），不新增 per-machine 端口。每台 Mac 主动建立一条到
// /v1/relay/connect 的 SSE 长连接；云端把「机器码 → 活动连接」放内存表（O(1)）。
// 客户端请求 https://voxsign.ai/relay/<machine_code>/<path> 时，云端按机器码查到连接，
// 把 {id,method,path,query,headers,body} 经 SSE 推给 Mac agent，agent 转发本地上游
// （127.0.0.1:8897），再 POST /v1/relay/respond 回包。v1 为 request/response（非流式）。
//
// 鉴权：connect/respond/forward 均要求 Bearer == 设备注册表中该 machine_code 的 token。
// 多租户：请求方 JWT 租户（若有）必须 == 设备 tenant，否则 403。
package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// relayResp agent 回报的上游响应。
type relayResp struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body"`
}

// relayConn 一条活动的 Mac 反向连接。
type relayConn struct {
	machine string
	tenant  string
	send    chan []byte // 出站 SSE 事件（JSON 信封），由 connect handler 消费
}

type relayHub struct {
	mu       sync.Mutex
	conns    map[string]*relayConn      // machine_code → 活动连接（O(1)）
	pendings map[string]chan *relayResp // request_id → 等待者
}

func newRelayHub() *relayHub {
	return &relayHub{conns: map[string]*relayConn{}, pendings: map[string]chan *relayResp{}}
}

// authorizeDevice 校验 Bearer/X-Token == 注册表中某机器码的 token，返回该记录。
func (s *Server) authorizeDevice(tok string) *DeviceRecord {
	if s.devices == nil || tok == "" {
		return nil
	}
	for _, d := range s.devices.devices {
		if d != nil && d.Token == tok {
			return d
		}
	}
	return nil
}

// bearerToken 取 Authorization: Bearer / X-Token。
func bearerToken(r *http.Request) string {
	t := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if t == "" {
		t = r.Header.Get("X-Token")
	}
	return strings.TrimSpace(t)
}

// GET /v1/relay/connect?machine_code= —— Mac agent 常驻 SSE。
// 鉴权：token 必须 == 注册表该 machine_code 的 token。
func (s *Server) handleRelayConnect(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("machine_code"))
	tok := bearerToken(r)
	rec := s.devices.findByToken(tok)
	if code == "" || rec == nil || rec.MachineCode != code {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "relay 鉴权失败：token 与机器码不匹配"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "不支持流式"})
		return
	}
	conn := &relayConn{machine: code, tenant: rec.Tenant, send: make(chan []byte, 16)}
	s.relay.mu.Lock()
	if old, exists := s.relay.conns[code]; exists {
		close(old.send) // 同码重连：替掉旧连接
	}
	s.relay.conns[code] = conn
	s.relay.mu.Unlock()
	log.Printf("[relay] agent 连接 machine=%s tenant=%s", code, rec.Tenant)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// 退出时清理连接
	defer func() {
		s.relay.mu.Lock()
		if cur, ok := s.relay.conns[code]; ok && cur == conn {
			delete(s.relay.conns, code)
		}
		s.relay.mu.Unlock()
		log.Printf("[relay] agent 断开 machine=%s", code)
	}()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case env, open := <-conn.send:
			if !open {
				return
			}
			if _, err := w.Write(append([]byte("data: "), env...)); err != nil {
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// POST /v1/relay/respond —— agent 回报：{request_id,status,headers,body}。
func (s *Server) handleRelayRespond(w http.ResponseWriter, r *http.Request) {
	tok := bearerToken(r)
	if rec := s.devices.findByToken(tok); rec == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "relay respond 鉴权失败"})
		return
	}
	var in struct {
		RequestID string `json:"request_id"`
		Status    int    `json:"status"`
		Headers   map[string]string `json:"headers"`
		Body      string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body 解析失败"})
		return
	}
	s.relay.mu.Lock()
	ch, ok := s.relay.pendings[in.RequestID]
	if ok {
		delete(s.relay.pendings, in.RequestID)
	}
	s.relay.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "request_id 无等待者（超时或已清理）"})
		return
	}
	st := in.Status
	if st == 0 {
		st = http.StatusOK
	}
	ch <- &relayResp{Status: st, Headers: in.Headers, Body: in.Body}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleRelayForward 客户端转发入口：/v1/relay/{code}/{path...}。
func (s *Server) handleRelayForward(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/relay/")
	parts := strings.SplitN(rest, "/", 2)
	code := parts[0]
	sub := "/"
	if len(parts) == 2 {
		sub = "/" + parts[1]
	}
	tok := bearerToken(r)
	rec := s.devices.findByToken(tok)
	if rec == nil || rec.MachineCode != code {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token 与机器码不匹配"})
		return
	}
	// 多租户：请求方带 JWT 租户时，必须与设备归属租户一致。
	if caller := ctxTenant(r.Context()); caller != "" && rec.Tenant != "" && caller != rec.Tenant {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "跨租户访问被拒绝"})
		return
	}
	s.relay.mu.Lock()
	conn, ok := s.relay.conns[code]
	s.relay.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "机器未在线（无活动反向连接）"})
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 15<<20))
	rid := "relay-" + time.Now().Format("150405.000000000") + "-" + code
	wait := make(chan *relayResp, 1)
	s.relay.mu.Lock()
	s.relay.pendings[rid] = wait
	s.relay.mu.Unlock()
	defer func() {
		s.relay.mu.Lock()
		delete(s.relay.pendings, rid)
		s.relay.mu.Unlock()
	}()

	env, _ := json.Marshal(map[string]any{
		"id":     rid,
		"method": r.Method,
		"path":   sub + r.URL.RawQuery,
		"headers": map[string]string{"Authorization": r.Header.Get("Authorization")},
		"body":   string(body),
	})
	select {
	case conn.send <- env:
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "agent 忙或缓冲满"})
		return
	}
	select {
	case resp := <-wait:
		for k, v := range resp.Headers {
			if k == "Content-Length" {
				continue
			}
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.Status)
		io.Copy(w, bytes.NewBufferString(resp.Body))
	case <-time.After(30 * time.Second):
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "agent 30s 未响应"})
	case <-r.Context().Done():
	}
}
