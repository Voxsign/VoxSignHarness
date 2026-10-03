package skill

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// SK-KH-1 · 判据先红：`Knowhow` 的 JSON 形状必须与**技能系统实际返回**的一致。
//
// ⚠️ 2026-10-03 真跑发现（见 Issue #4）：
//
//	技能服务 `GET /api/skill/skills/{id}` 的响应顶层键是
//	  manifest · scripts · ok · id · version · state · personalized · caller
//	而 `Knowhow` 期望的五个字段（steps/cautions/style/judging/basis）**在 `manifest` 里面**。
//	⇒ `json.Unmarshal(raw, &kh)` 得到**全空** ⇒ `CriteriaFromKnowhow` ⇒ **0 条判据**
//	⇒ `AutomatedRatio` ⇒ **(0,0)** ⇒ 任何"自动化率"都算不出来。
//
// 本判据**真连技能系统**（需 AIOPS_KEY）；无 key ⇒ 显式 SKIP 并说明（不当通过）。
// 判据先红：当前实现下它应 **FAIL** —— 而 FAIL 的信息会指出错在哪。
func TestSKKH1KnowhowShapeMatchesRemote(t *testing.T) {
	key := os.Getenv("AIOPS_KEY")
	if key == "" {
		t.Skip("[SK-KH-1] 无 AIOPS_KEY ⇒ 本参考系无法判定（**不算通过也不算失败**）；" +
			"请提供 key 后重跑。")
	}
	f := &Fetcher{BaseURL: "https://aiops.peterzou.com", APIKey: key}
	ctx := context.Background()

	list, err := f.List(ctx)
	if err != nil {
		t.Fatalf("[SK-KH-1] List 失败（**这是装置/网络问题，不是形状问题**）: %v", err)
	}
	if len(list) == 0 {
		t.Fatalf("[SK-KH-1] 技能清单为空")
	}

	// 抽查前 3 个技能，逐个核形状
	checked := 0
	for _, s := range list {
		if checked >= 3 {
			break
		}
		raw, err := f.Get(ctx, s.ID)
		if err != nil {
			t.Errorf("[SK-KH-1] Get(%s) 失败: %v", s.ID, err)
			continue
		}
		var top map[string]json.RawMessage
		if err := json.Unmarshal(raw, &top); err != nil {
			t.Errorf("[SK-KH-1] %s 顶层不是 JSON 对象: %v", s.ID, err)
			continue
		}

		// ① 接线路径：KnowhowFromRaw（2026-10-03 接线，修复"knowhow 在 manifest.knowhow 两层"）
		//    Fetcher.Get 是否直接返回 knowhow 属接口设计，仍待裁；测试先走接线函数保证判据可验。
		khDirect, derr := KnowhowFromRaw(raw)
		if derr != nil {
			t.Logf("[SK-KH-1] %s：KnowhowFromRaw 未解出（%v）——见 ② 对照", s.ID, derr)
		}
		directN := len(khDirect.Steps) + len(khDirect.Cautions) + len(khDirect.Style) +
			len(khDirect.Judging) + len(khDirect.Basis)

		// ② 剥**两层**：manifest → knowhow（⚠️ 2026-10-03 真跑确定：knowhow 在 manifest.knowhow）
		//    我最初猜"剥一层 manifest 就够" —— **判据当时 FAIL 否证了它**（剥一层仍 0 条）。
		var khInner Knowhow
		innerN := 0
		if m, ok := top["manifest"]; ok {
			var mm map[string]json.RawMessage
			if json.Unmarshal(m, &mm) == nil {
				if k, ok2 := mm["knowhow"]; ok2 {
					_ = json.Unmarshal(k, &khInner)
					innerN = len(khInner.Steps) + len(khInner.Cautions) + len(khInner.Style) +
						len(khInner.Judging) + len(khInner.Basis)
				}
			}
		}

		checked++
		t.Logf("[SK-KH-1] %s：直解得 %d 条 · 剥 manifest 后得 %d 条", s.ID, directN, innerN)

		if directN == 0 && innerN == 0 {
			t.Errorf("[SK-KH-1] %s：**两种解都得到 0 条** ⇒ Knowhow 形状与远端不匹配（且不是「少一层壳」能修的；正确路径是 manifest.knowhow）", s.ID)
		}
		if directN == 0 && innerN > 0 {
			t.Errorf("[SK-KH-1] %s：**直解 0 条、剥 manifest 后 %d 条** ⇒ "+
				"确认缺口 = Knowhow 期望顶层而 knowhow 在 manifest 里。"+
				"⇒ 判据要求：`Fetcher.Get` 或 `Knowhow` 必须让**直解可得**（二者选一，属接口设计，待裁）", s.ID, innerN)
		}
	}
	if checked == 0 {
		t.Fatalf("[SK-KH-1] 一个技能都没核成（Get 全失败）⇒ 无结论")
	}
}
