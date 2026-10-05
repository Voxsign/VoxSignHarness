// robust.go — 外部调用健壮性四件套（架构 v1 §7）：
//   1. 分级超时（fast/mid/long）
//   2. 熔断（连续失败 N 次 → 打开 30s → 半开探活）
//   3. 指数退避重试（仅幂等调用，+抖动）
//   4. 有界并发 bulkhead（外部通道独立信号量，满则快速失败，不阻塞主循环）
//
// 设计文档：docs/架构-协议与健壮性-v1.md §7
package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

// 分级超时（架构 v1 §7.1 传输层）。
const (
	tFast = 5 * time.Second   // 快调用：JWKS 等
	tMid  = 30 * time.Second  // 中调用：ASR 平台转发等
	tLong = 120 * time.Second // 长调用：模型推理/远端进程
)

// ---------- 熔断（§7.2） ----------

type cbState int

const (
	cbClosed cbState = iota
	cbOpen
	cbHalfOpen
)

const (
	cbThreshold = 8                    // 连续失败阈值（平台偶发抖动时不误伤，8 次才熔断）
	cbCooldown  = 30 * time.Second     // 打开后冷却窗口
	cbHalfMax   = 1                     // 半开期只放行 1 个探活请求
)

// circuitBreaker 单实例保护一条外部通道。非并发安全由调用方持锁或单 goroutine 使用。
type circuitBreaker struct {
	state    cbState
	failures int
	openedAt time.Time
	probes   int
}

// allow 返回是否放行本次调用；熔断打开且冷却未过则快速失败。
func (cb *circuitBreaker) allow() bool {
	switch cb.state {
	case cbClosed:
		return true
	case cbOpen:
		if time.Since(cb.openedAt) >= cbCooldown {
			cb.state = cbHalfOpen
			cb.probes = 0
			return true
		}
		return false
	case cbHalfOpen:
		if cb.probes >= cbHalfMax {
			return false
		}
		cb.probes++
		return true
	}
	return true
}

// report 上报一次调用结果：成功复位（回到 closed），失败累计（打开熔断）。
func (cb *circuitBreaker) report(success bool) {
	switch cb.state {
	case cbClosed:
		if success {
			cb.failures = 0
		} else {
			cb.failures++
			if cb.failures >= cbThreshold {
				cb.state = cbOpen
				cb.openedAt = time.Now()
			}
		}
	case cbHalfOpen:
		if success {
			cb.state = cbClosed
			cb.failures = 0
		} else {
			cb.state = cbOpen
			cb.openedAt = time.Now()
		}
	}
}

// ---------- 退避重试（§7.2） ----------

// retryBackoff 指数退避 + 抖动：1s → 2s → 4s（上限 3 次重试，共 4 次尝试）。
func retryBackoff(attempt int) time.Duration {
	if attempt > 3 {
		attempt = 3
	}
	base := time.Duration(1<<uint(attempt-1)) * time.Second
	jitter := time.Duration(rand.Int63n(int64(base) / 2))
	return base + jitter
}

// ---------- 有界并发 bulkhead（§7.3） ----------

// bulkhead 独立外部通道的有界信号量：满则快速失败（503 语义），绝不排队堆积阻塞主循环。
type bulkhead struct {
	sem chan struct{}
}

func newBulkhead(max int) *bulkhead {
	if max < 1 {
		max = 1
	}
	return &bulkhead{sem: make(chan struct{}, max)}
}

// acquire 尝试获取一个槽位；等待超过 acquireTimeout 快速失败。
func (b *bulkhead) acquire(ctx context.Context, acquireTimeout time.Duration) error {
	select {
	case b.sem <- struct{}{}:
		return nil
	case <-time.After(acquireTimeout):
		return fmt.Errorf("外部通道繁忙（bulkhead 满），快速失败")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *bulkhead) release() { <-b.sem }

// ---------- 统一外部调用入口 ----------

// robustClient 分级超时的 HTTP client（架构 v1 §7.1）。
func robustClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// robustJSON 执行一次受防护的外部 JSON 调用：
// bulkhead 有界并发 → 熔断判定 →（幂等）退避重试 → 分级超时。
// idempotent=true 时在非 2xx/网络错误上重试（上限 3 次）；false 时只做单次 + 熔断累计。
// 返回最后一次 HTTP 状态码与响应体；网络错误返回 err。
func robustJSON(ctx context.Context, bh *bulkhead, cb *circuitBreaker,
	method, url string, body []byte, timeout time.Duration, idempotent bool) (int, []byte, error) {
	return robustJSONHdr(ctx, bh, cb, method, url, body, timeout, idempotent, nil)
}

// robustJSONHdr 同 robustJSON，额外支持自定义请求头（如 Authorization）。
func robustJSONHdr(ctx context.Context, bh *bulkhead, cb *circuitBreaker,
	method, url string, body []byte, timeout time.Duration, idempotent bool,
	hdr map[string]string) (int, []byte, error) {

	if !cb.allow() {
		return 0, nil, fmt.Errorf("熔断打开（channel open），快速失败")
	}
	if err := bh.acquire(ctx, 2*time.Second); err != nil {
		return 0, nil, err
	}
	defer bh.release()

	attempt := 1
	for {
		status, respBody, err := doOnceHdr(ctx, method, url, body, timeout, hdr)
		success := err == nil && status >= 200 && status < 500 // 5xx/网络错误视为失败
		if success {
			cb.report(true)
			return status, respBody, nil
		}
		cb.report(false)

		if !idempotent || attempt >= 4 {
			if err != nil {
				return 0, nil, err
			}
			return status, respBody, nil
		}
		// 5xx 或网络错误且幂等 → 退避重试。
		if status >= 500 && status < 600 {
			// 服务端明确 5xx：退避后重试。
			select {
			case <-time.After(retryBackoff(attempt)):
			case <-ctx.Done():
				return status, respBody, ctx.Err()
			}
			attempt++
			continue
		}
		if err != nil {
			select {
			case <-time.After(retryBackoff(attempt)):
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			}
			attempt++
			continue
		}
		return status, respBody, nil
	}
}

func doOnce(ctx context.Context, method, url string, body []byte, timeout time.Duration) (int, []byte, error) {
	return doOnceHdr(ctx, method, url, body, timeout, nil)
}

func doOnceHdr(ctx context.Context, method, url string, body []byte, timeout time.Duration, hdr map[string]string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	cli := robustClient(timeout)
	resp, err := cli.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, data, nil
}
