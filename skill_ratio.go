package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"voicesign-harness/skill"
)

// cmdSkillRatio 打印**真实**的技能层自动化率。
//
// 为什么要有它（2026-10-03）：
//
//	「自动化率 4/31 = 12.9%」这个数字在仓库里**没有出处**、且**无法由代码产生**
//	（`AutomatedRatio` 零生产调用；唯一调用在测试里喂硬编码 knowhow）。
//	⇒ 一次性探测不算能力。本子命令把它变成**可复现**：同一条命令、同一份数据、同一个数。
//
// 口径（来自 `skill/judgement.go`，**不是我定的**）：
//
//	自动化率 = `Manual == false` 的判据条数 / 判据总条数
//	判据来自 knowhow 的 **judging / cautions / basis / style** 四个字段
//	（`steps` 不进判据 —— 按 VHS-SKILL-001 §3：steps → 规划模板）
//	当前实现里**唯一**的自动化条件是：`basis` 条目含「已查证」⇒ `checkVerifiedOverHearsay`
//
// 退出码：0 = 算出来了；1 = 算不出来（缺 key / 拉取失败 / 解不出 knowhow）——**不当通过**。
func cmdSkillRatio(args []string) {
	base := os.Getenv("AIOPS_GATEWAY")
	if strings.TrimSpace(base) == "" {
		base = "https://aiops.peterzou.com"
	}
	key := os.Getenv("AIOPS_KEY")
	if strings.TrimSpace(key) == "" {
		fmt.Fprintln(os.Stderr, "vhs skill-ratio: ❌ 缺 AIOPS_KEY ⇒ **无法判定**（不是 0%，是算不出来）。")
		fmt.Fprintln(os.Stderr, "  ⇒ 请设置 AIOPS_KEY（技能系统 Bearer token）后重跑。")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	f := &skill.Fetcher{BaseURL: base, APIKey: key}
	list, err := f.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vhs skill-ratio: ❌ 拉取技能清单失败（**装置/网络问题，不是产品结论**）: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("技能来源: %s · 技能数 %d\n", base, len(list))
	fmt.Printf("%-34s %6s %5s %5s %5s %5s | %6s %5s\n",
		"技能", "steps", "judg", "caut", "basis", "style", "判据", "自动")

	totC, totA, totJ := 0, 0, 0
	failed := 0
	for _, s := range list {
		raw, err := f.Get(ctx, s.ID)
		if err != nil {
			// ⚠️ **不静默**：每个失败都计数并打印
			fmt.Printf("%-34s  ⚠️ Get 失败: %v\n", s.ID, err)
			failed++
			continue
		}
		kh, err := skill.KnowhowFromRaw(raw)
		if err != nil {
			fmt.Printf("%-34s  ⚠️ knowhow 解不出: %v\n", s.ID, err)
			failed++
			continue
		}
		cs := skill.CriteriaFromKnowhow(s.ID, s.Version, kh)
		a, t := skill.AutomatedRatio(cs)
		if t == 0 && len(kh.Steps) == 0 {
			continue // 空技能（如探测占位），不计入
		}
		totC += t
		totA += a
		totJ += len(kh.Judging)
		fmt.Printf("%-34s %6d %5d %5d %5d %5d | %6d %5d\n",
			s.ID, len(kh.Steps), len(kh.Judging), len(kh.Cautions), len(kh.Basis), len(kh.Style), t, a)
	}

	fmt.Println(strings.Repeat("-", 78))
	fmt.Printf("判据合计 %d · 自动 %d\n", totC, totA)
	fmt.Printf("judging 合计 %d\n", totJ)
	if totC > 0 {
		fmt.Printf("**自动化率 = %d/%d = %.1f%%**（口径：Manual=false / 全部判据）\n",
			totA, totC, float64(totA)*100/float64(totC))
	}
	if totJ > 0 {
		fmt.Printf("（参考）judging 口径 = %d/%d = %.1f%% —— 当前实现下 judging **不可能**非人工\n",
			totA, totJ, float64(totA)*100/float64(totJ))
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "⚠️ 有 %d 个技能拉取/解析失败 ⇒ 上述数字**不是全量**，不可当全局值。\n", failed)
		os.Exit(1)
	}
}
