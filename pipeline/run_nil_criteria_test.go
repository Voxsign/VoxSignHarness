// run_nil_criteria_test.go —— Run 入口必填检查（先红→绿）：**不许崩，必须报错**。
package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"voicesign-harness/space"
)

// runNoPanic 在**独立 goroutine** 里跑 Run，并断言"根本没崩"（而非"我兜住了"）。
//
// 理由：Run 可能在**后台 goroutine** 崩 ⇒ 主 goroutine 的 recover 抓不到。
func runNoPanic(t *testing.T, o *Options, text string) (err error) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("[Run] PANIC（必须返回 error）: %v", r)
			}
		}()
		_, err = Run(context.Background(), o, text)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("[Run] 超时（可能挂在后台 goroutine）")
	}
	return err
}

// ① nil Options ⇒ 返回 error，不得 panic
func TestRunNilOptionsReturnsError(t *testing.T) {
	err := runNoPanic(t, nil, "查看当前目录")
	if err == nil {
		t.Fatal("[Run①] nil Options 未返回 error")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("[Run①] 错误信息应指明 nil: %v", err)
	}
}

// ② 空 Options ⇒ 返回 error，不得 panic
func TestRunEmptyOptionsReturnsError(t *testing.T) {
	err := runNoPanic(t, &Options{}, "查看当前目录")
	if err == nil {
		t.Fatal("[Run②] 空 Options 未返回 error（此前是 panic）")
	}
}

// ③ 缺 Spaces ⇒ 返回 error 且指明域门禁缺失
func TestRunMissingSpacesReturnsError(t *testing.T) {
	err := runNoPanic(t, &Options{Providers: nil}, "查看当前目录")
	if err == nil {
		t.Fatal("[Run③] 缺 Spaces 未返回 error")
	}
	if !strings.Contains(err.Error(), "Spaces") {
		t.Errorf("[Run③] 应指明 Spaces: %v", err)
	}
}

// ⑤ 反例：**不许因为加了检查就把一切拒掉** —— 有 Spaces 时不得返回配置类错误
func TestRunWithSpacesIsNotBlanketRejected(t *testing.T) {
	reg, err0 := space.Load(t.TempDir())
	if err0 != nil {
		t.Skipf("space.Load 不可用（环境相关）：%v", err0)
	}
	err := runNoPanic(t, &Options{Spaces: reg}, "")
	if err != nil && (strings.Contains(err.Error(), "Options 为 nil") || strings.Contains(err.Error(), "Spaces 未配置")) {
		t.Errorf("[Run⑤] 完整的 Spaces 却被配置检查拒掉: %v", err)
	}
}
