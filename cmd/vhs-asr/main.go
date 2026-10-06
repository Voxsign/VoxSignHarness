// Command vhs-asr is the standalone service entrypoint for the ASR personalization
// recognition layer (spec §6 / acceptance #1).
//
// It does only three things: read config -> assemble (Engine + Dictionary + Tracer +
// Pipeline) -> listen. It contains no business decisions and performs no business
// actions (red line #1).
//
// Config precedence: environment variables > JSON config file (VHS_ASR_CONFIG) > defaults.
// Config is JSON (the spec explicitly forbids YAML).
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
	"voicesign-harness/modelcenter"
	"voicesign-harness/plan"
	"voicesign-harness/recog"
)

type fileConfig struct {
	Addr       string `json:"addr"`
	DataDir    string `json:"data_dir"`
	Dictionary string `json:"dictionary"`
	Traces     string `json:"traces"`
}

// mustModel resolves the channel model id (returns "?" on error; logging only).
func mustModel(cfg modelcenter.Config, ch modelcenter.Channel) string {
	if m, err := cfg.ResolveModel(ch); err == nil {
		return m
	}
	return "?"
}

func main() {
	cfg := fileConfig{}
	if p := os.Getenv("VHS_ASR_CONFIG"); p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			log.Fatalf("failed to read config %s: %v", p, err)
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Fatalf("config %s is not valid JSON: %v", p, err)
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
		log.Fatalf("failed to create data dir %s: %v", dataDir, err)
	}
	dict, err := asr.NewDictionary(dictPath)
	if err != nil {
		log.Fatalf("failed to load dictionary: %v", err)
	}
	tracer, err := asr.NewTracer(tracePath)
	if err != nil {
		log.Fatalf("failed to open trace: %v", err)
	}
	defer func() { _ = tracer.Close() }()

	pipe := asr.NewPipeline(asr.NewEngine(), dict, tracer)
	// Assemble the service-side cache (K9): L2 = /api/services (read-only), refreshed periodically by K7.
	servicesURL := envOr("VHS_SERVICES_URL", "https://aiops.voxsign.ai/api/services")
	hot := hotcache.New(filepath.Join(dataDir, "services-cache.json"), time.Hour,
		hotcache.HTTPFetcher(servicesURL, os.Getenv("AIOPS_KEY"), 10*time.Second))
	// The "this was corrected wrong" blacklist (§5.1 item 5 / A10): loaded at startup, persisted on change.
	hot.SetBlacklistPath(filepath.Join(dataDir, "blacklist.json"))
	if err := hot.LoadBlacklist(); err != nil {
		log.Printf("blacklist load failed (non-fatal; continuing with empty blacklist): %v", err)
	}
	// Refresh once at startup and log it (observability: alias count / source / status).
	snap := hot.Refresh(context.Background())
	log.Printf("L2 cache: aliases=%d status=%s source=%s", len(snap.Aliases), snap.Status, snap.Source)
	stop := hot.StartRefresh(context.Background(), time.Hour)
	defer stop()
	pipe.Hot = &recog.Rewriter{Engine: asr.NewEngine(), Hot: hot}
	srvObj := asr.NewServer(pipe)
	// Wire up the real L2: read model-center config -> plan channel -> strong model
	// (adapted to plan.PlanModel). On missing key / bad config, log explicitly and
	// leave L2 disabled (never pretend it is enabled silently).
	mcPath := envOr("VHS_MODEL_CENTER", filepath.Join(".", "config", "model-center.json"))
	if mcfg, err := modelcenter.Load(mcPath); err == nil {
		if reg, err := modelcenter.NewRegistry(mcfg); err == nil {
			srvObj.Models = &mcfg
			srvObj.PlanModel = plan.ChannelPlanModel{Registry: reg, Channel: modelcenter.ChannelPlan}
			log.Printf("L2 wired: plan channel -> %s", mustModel(mcfg, modelcenter.ChannelPlan))
		} else {
			log.Printf("L2 not wired (model center unavailable): %v", err)
		}
	} else {
		log.Printf("L2 not wired (failed to read model-center config): %v", err)
	}
	srvObj.DataDir = dataDir // profile attribution source (SCOPE-PROFILE-01); explicit none:no_profile when absent
	// Teach-word / clear-taught / rewrite blacklist: all wired to the real cache.
	srvObj.Teach = hot.Teach
	srvObj.ClearTaught = hot.ClearTaught
	srvObj.Blacklist = hot.Blacklist
	srv := &http.Server{Addr: addr, Handler: srvObj.Handler()}
	log.Printf("vhs-asr listening on %s (data dir %s, contract v1); Line-A default endpoint=http://127.0.0.1:8787", addr, dataDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
