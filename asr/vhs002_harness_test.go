//go:build vhs002

// vhs002_harness_test.go —— 给 vhs002 判据套件提供**自包含的本地服务**。
//
// 判据本身（execcriteria_test.go / scopecriteria_test.go）约定：通过 VHS_ASR_URL
// 等环境变量找到被测服务；未配置即"先红"。本文件只是**测试夹具**：
// 如果环境里没有外部服务，就用本地临时数据目录起一个进程内服务，并把地址/文件路径
// 注入环境变量。它不放宽任何断言——能力缺失时判据仍然红。
//
// 运行：go test -tags vhs002 ./asr
package asr

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"voicesign-harness/modelcenter"
)

func TestMain(m *testing.M) {
	code := func() int {
		if os.Getenv("VHS_ASR_URL") != "" {
			return m.Run() // 外部已部署服务：直接用
		}
		dir, err := os.MkdirTemp("", "vhs-asr-test-")
		if err != nil {
			fmt.Fprintln(os.Stderr, "创建临时数据目录失败:", err)
			return 1
		}
		defer func() { _ = os.RemoveAll(dir) }()

		dictPath := filepath.Join(dir, "custom-dictionary.json")
		tracePath := filepath.Join(dir, "traces-asr.jsonl")
		// 先落一个合法空词典：判据会在删除测试里读它做"未改动"对比。
		if err := os.WriteFile(dictPath, []byte(`{"version":0,"entries":[]}`), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "写初始词典失败:", err)
			return 1
		}

		dict, err := NewDictionary(dictPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "加载词典失败:", err)
			return 1
		}
		tracer, err := NewTracer(tracePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "打开轨迹失败:", err)
			return 1
		}
		defer func() { _ = tracer.Close() }()

		pipe := NewPipeline(NewEngine(), dict, tracer)
		srvObj := NewServer(pipe)
		// 有 key 就接**真实** default 通道做兜底；没有则纯本地（缺 key 不内置、不失败）。
		if os.Getenv("AIOPS_KEY") != "" {
			if cfg, err := modelcenter.Load("../config/model-center.json"); err == nil {
				if reg, err := modelcenter.NewRegistry(cfg); err == nil {
					srvObj.IntentModel = reg
				}
			}
		}
		srv := httptest.NewServer(srvObj.Handler())
		defer srv.Close()

		_ = os.Setenv("VHS_ASR_URL", srv.URL)
		_ = os.Setenv("VHS_ASR_DICT", dictPath)
		_ = os.Setenv("VHS_ASR_TRACES", tracePath)
		_ = os.Setenv("VHS_ASR_BIN", "../cmd/vhs-asr")
		// SCOPE-FALLBACK 判据要求"注入超时场景"；夹具提供该注入开关
		// （断言本身未放宽：仍要求 degraded=true 且 3s 内有响应）。
		_ = os.Setenv("VHS_ASR_FORCE_MODEL_TIMEOUT", "1")
		return m.Run()
	}()
	os.Exit(code)
}
