// robust.go — out calluse  ity   (   v1 §7): 
//   1. split  time(fast/mid/long)
//   2.  disconnect(linkcontinue   N   ->  open 30s ->  open  )
//   3. refernum  heavy (only etccalluse, +  )
//   4. hasboundaryandsend bulkhead(out     signal , fullthenfast   ,       )
//
//     : docs/  -  and  ity-v1.md §7
package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"runtime/debug"
	"time"
)

// split  time(   v1 §7.1    ). 
const (
	tFast = 5 * time.Second   // fastcalluse: JWKS etc
	tMid  = 30 * time.Second  // incalluse: ASR    sendetc
	tLong = 120 * time.Second //  calluse:  type  / endprocess
)

// ----------  disconnect(§7.2) ----------

type cbState int

const (
	cbClosed cbState = iota
	cbOpen
	cbHalfOpen
)

const (
	cbThreshold = 8                    // linkcontinue   value(   send  time   , 8  only disconnect)
	cbCooldown  = 30 * time.Second     //  openafter but  
	cbHalfMax   = 1                     //  openperiodonly   1     require
)

// circuitBreaker   exampleprotect  out   .  andsendsafesafetybycalluse keep or  goroutine  use. 
type circuitBreaker struct {
	state    cbState
	failures int
	openedAt time.Time
	probes   int
}

// allow returnbackis   base calluse;  disconnect openand but edthenfast   . 
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

// report on   calluseclose : become   (backto closed),     ( open disconnect). 
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

// ----------   heavy (§7.2) ----------

// retryBackoff refernum   +   : 1s -> 2s -> 4s(onlimit 3  heavy ,   4    ). 
func retryBackoff(attempt int) time.Duration {
	if attempt > 3 {
		attempt = 3
	}
	base := time.Duration(1<<uint(attempt-1)) * time.Second
	jitter := time.Duration(rand.Int63n(int64(base) / 2))
	return base + jitter
}

// ---------- hasboundaryandsend bulkhead(§7.3) ----------

// bulkhead   out    hasboundarysignal : fullthenfast   (503 semantic),            . 
type bulkhead struct {
	sem chan struct{}
}

func newBulkhead(max int) *bulkhead {
	if max < 1 {
		max = 1
	}
	return &bulkhead{sem: make(chan struct{}, max)}
}

// acquire    get    ; wait ed acquireTimeout fast   . 
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

// ----------   out callusein  ----------

// robustClient split  time  HTTP client(   v1 §7.1). 
func robustClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// robustJSON     acceptpreventprotect out  JSON calluse: 
// bulkhead hasboundaryandsend ->  disconnect   ->( etc)  heavy  -> split  time. 
// idempotent=true time   2xx/  erroronheavy (onlimit 3  ); false timeonly    +  disconnect  . 
// returnback after   HTTP statuscodeand  body;   errorreturnback err. 
func robustJSON(ctx context.Context, bh *bulkhead, cb *circuitBreaker,
	method, url string, body []byte, timeout time.Duration, idempotent bool) (int, []byte, error) {
	return robustJSONHdr(ctx, bh, cb, method, url, body, timeout, idempotent, nil)
}

// robustJSONHdr same robustJSON,  out keep define requirehead(e.g. Authorization). 
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
		success := err == nil && status >= 200 && status < 500 // 5xx/  error as  
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
		// 5xx or  errorand etc ->   heavy . 
		if status >= 500 && status < 600 {
			// serveserviceend   5xx:   afterheavy . 
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

// safeGo start   accept panic protect after  goroutine(P0-2). 
//
//   :  before serviceafter  goroutine   `go func(){...}()`, pipeline.Run in   panic  
//  connect crash    server(   require  bug   safety  linetask). safeGo pipe  task 
// panic         goroutine  :  day + , calluse onPanic  recvtail(tgt task canceled, 
// writetrace error), processcontinuecontinuestore . 
//
// onPanic  as nil; onPanic   alsobe recover   ,   becauserecvtail      . 
//
// note : calluse if  goroutine bodyin `defer mu.Unlock()`,   defer   base num recover ofbefore
// done unwind, thus onPanic   timecalluse keephas    already  -- safesafetyheavynew  . 
func safeGo(name string, fn func(), onPanic func(recovered any)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[safeGo] %s: panic recovered: %v\n%s", name, r, debug.Stack())
				if onPanic != nil {
					func() {
						defer func() { _ = recover() }() // onPanic     again 
						onPanic(r)
					}()
				}
			}
		}()
		fn()
	}()
}
