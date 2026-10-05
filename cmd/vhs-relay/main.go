// vhs-relay：Mac 端常驻反向中继 agent（异网连回本机 harness）。
//
// 工作方式：主动 SSE 连接云端 GET /v1/relay/connect?machine_code=（Bearer 设备 token），
// 常驻不断开。云端把客户端请求信封（{id,method,path,headers,body}）经 SSE 推下来，
// agent 转发到本地上游 VHS_RELAY_UPSTREAM（如 http://127.0.0.1:8897），再 POST
// /v1/relay/respond 回包。Mac 无需公网端口/固定 IP。断线指数退避重连（上限 60s）。
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func main() {
	cloud := env("VHS_RELAY_CLOUD", "http://127.0.0.1:8897")
	machine := env("VHS_RELAY_MACHINE", "")
	token := env("VHS_RELAY_TOKEN", "")
	upstream := env("VHS_RELAY_UPSTREAM", "http://127.0.0.1:8897")
	if machine == "" || token == "" {
		log.Fatalf("vhs-relay: 必须设置 VHS_RELAY_MACHINE 与 VHS_RELAY_TOKEN")
	}
	log.Printf("vhs-relay 启动 cloud=%s machine=%s upstream=%s", cloud, machine, upstream)

	backoff := 1 * time.Second
	for {
		err := serve(cloud, machine, token, upstream)
		log.Printf("vhs-relay 连接断开：%v（%v 后重连）", err, backoff)
		time.Sleep(backoff)
		backoff *= 2
		if backoff > 60*time.Second {
			backoff = 60 * time.Second
		}
	}
}

func serve(cloud, machine, token, upstream string) error {
	url := fmt.Sprintf("%s/v1/relay/connect?machine_code=%s", cloud, machine)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("connect HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	log.Printf("vhs-relay 已连接 machine=%s", machine)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var env struct {
			ID      string            `json:"id"`
			Method  string            `json:"method"`
			Path    string            `json:"path"`
			Headers map[string]string `json:"headers"`
			Body    string            `json:"body"`
		}
		if err := json.Unmarshal([]byte(line[6:]), &env); err != nil {
			continue
		}
		go forward(cloud, token, upstream, env.ID, env.Method, env.Path, env.Headers, env.Body)
	}
	return scanner.Err()
}

func forward(cloud, token, upstream, id, method, path string, headers map[string]string, body string) {
	defer func() { recover() }()
	u := strings.TrimRight(upstream, "/") + path
	req, err := http.NewRequest(method, u, bytes.NewBufferString(body))
	if err != nil {
		respond(cloud, token, id, 500, nil, `{"error":"agent 构造请求失败"}`)
		return
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 35 * time.Second}
	r, err := client.Do(req)
	if err != nil {
		respond(cloud, token, id, 502, nil, `{"error":"agent 转发上游失败: `+err.Error()+`"}`)
		return
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	respond(cloud, token, id, r.StatusCode, map[string]string{"Content-Type": r.Header.Get("Content-Type")}, string(b))
}

func respond(cloud, token, id string, status int, headers map[string]string, body string) {
	payload, _ := json.Marshal(map[string]any{
		"request_id": id, "status": status, "headers": headers, "body": body,
	})
	req, _ := http.NewRequest("POST", cloud+"/v1/relay/respond", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("vhs-relay respond %s 失败: %v", id, err)
		return
	}
	r.Body.Close()
}
