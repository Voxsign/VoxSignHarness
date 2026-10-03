// Command vhs-asr 是 ASR 个性化识别层的**独立服务入口**（需求 §6 / 验收 #1）。
//
// 它只做三件事：读配置 → 装配（Engine + Dictionary + Tracer + Pipeline）→ 监听。
// 不含业务判断，也不执行业务动作（红线 #1）。
//
// 配置优先级：环境变量 > JSON 配置文件（VHS_ASR_CONFIG）> 默认值。
// 配置是 JSON（需求明令不用 YAML）。
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"voicesign-harness/asr"
	"voicesign-harness/hotcache"
	"voicesign-harness/recog"
)

type fileConfig struct {
	Addr       string `json:"addr"`
	DataDir    string `json:"data_dir"`
	Dictionary string `json:"dictionary"`
	Traces     string `json:"traces"`
}

func main() {
	cfg := fileConfig{}
	if p := os.Getenv("VHS_ASR_CONFIG"); p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			log.Fatalf("读取配置 %s 失败: %v", p, err)
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Fatalf("配置 %s 不是合法 JSON: %v", p, err)
		}
	}

	addr := envOr("VHS_ASR_ADDR", cfg.Addr)
	if addr == "" {
		addr = "127.0.0.1:8787"
	}
	dataDir := envOr("VHS_ASR_DATA", cfg.DataDir)
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dataDir = filepath.Join(home, ".voicesign", "asr")
	}
	dictPath := envOr("VHS_ASR_DICT", cfg.Dictionary)
	if dictPath == "" {
		dictPath = filepath.Join(dataDir, "custom-dictionary.json")
	}
	tracePath := envOr("VHS_ASR_TRACES", cfg.Traces)
	if tracePath == "" {
		tracePath = filepath.Join(dataDir, "traces-asr.jsonl")
	}

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Fatalf("创建数据目录 %s 失败: %v", dataDir, err)
	}
	dict, err := asr.NewDictionary(dictPath)
	if err != nil {
		log.Fatalf("加载词典失败: %v", err)
	}
	tracer, err := asr.NewTracer(tracePath)
	if err != nil {
		log.Fatalf("打开轨迹失败: %v", err)
	}
	defer func() { _ = tracer.Close() }()

	pipe := asr.NewPipeline(asr.NewEngine(), dict, tracer)
	// 服务侧组装缓存（K9）：L2 = /api/services（只读），K7 定期刷新。
	servicesURL := envOr("VHS_SERVICES_URL", "https://aiops.peterzou.com/api/services")
	hot := hotcache.New(filepath.Join(dataDir, "services-cache.json"), time.Hour,
		hotcache.HTTPFetcher(servicesURL, os.Getenv("AIOPS_KEY"), 10*time.Second))
	// 启动即刷一次并**记日志**（可观测：别名条数/来源/状态）——真跑时要能看到它。
	snap := hot.Refresh(context.Background())
	log.Printf("L2 缓存：别名 %d 条 status=%s source=%s", len(snap.Aliases), snap.Status, snap.Source)
	stop := hot.StartRefresh(context.Background(), time.Hour)
	defer stop()
	pipe.Hot = &recog.Rewriter{Engine: asr.NewEngine(), Hot: hot}
	srv := &http.Server{Addr: addr, Handler: asr.NewServer(pipe).Handler()}
	log.Printf("vhs-asr 监听 %s（数据目录 %s，契约 v1）", addr, dataDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("服务退出: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
