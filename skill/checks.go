package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// checks.go -- **  ize   **(`Criterion.Check`    body). 
//
// ⚠️ as  needhas  file(2026-10-03): 
//   `criteriaOnly`  give datatgt `Check` nameand `Manual=false`("     "), 
//   but**   base **origfirst store (`ExecutedCheck` needneedcalluse   `passed`   ). 
//   ⇒ i.e.:  databe**voicecalled**     , but** has     **. 
//   ⇒ basefilepatchon  body -- **"   ize"   is"has       "**. 
//
//  line( use `evidence.go`): 
//   ·    **        **(file /  forbidclose ), ** allowdefault true**
//   ·    ⇒ returnback error, ** returnback" ed"**("  out " != " ed")

// CheckResult is     close . 
type CheckResult struct {
	Passed bool
	Detail string // ** class read  data**(   ,     )
}

// CheckRunner is      signature. 
//
// root is  root(   need  file); extra provideneedneed out in    use(e.g. forbidclose ). 
type CheckRunner func(root string, extra map[string]string) (CheckResult, error)

// checkRegistry   safety   ize   . 
//
// ⚠️ newadd     **sametime**    , and  `TestAllChecksAreRegistered`  beoverwrite --
//  then `criteriaOnly` tgtout   `Check` name **no   **( posisbasefileneedfix  ). 
var checkRegistry = map[string]CheckRunner{
	"checkADRExists":           checkADRExists,
	"checkWithinBoundary":      checkWithinBoundary,
	"checkVerifiedOverHearsay": checkVerifiedOverHearsay,
}

// RunCheck      name   . 
//
// ⚠️ namechar    ⇒ **returnback error**(** returnback ed**)--   now     allow  cur . 
func RunCheck(name, root string, extra map[string]string) (CheckResult, error) {
	r, ok := checkRegistry[name]
	if !ok {
		return CheckResult{}, fmt.Errorf("skill: checker %q **not implemented** (%d registered) -> must not count as pass", name, len(checkRegistry))
	}
	return r(root, extra)
}

// IsCheckImplemented      check nameis has**    body**. 
// provide `vhs skill-ratio`  split"tgt   ize"and"  has   ". 
func IsCheckImplemented(name string) bool {
	_, ok := checkRegistry[name]
	return ok
}

// --------------------     now --------------------

// checkADRExists  "decide has ADR": ** file**(target ④ ptname: ""decide hasADR" file"). 
//
//  data:    store  ADR(  decide   )file. 
//  diff path(** formwriteout ,   " ing then "**): 
//
//	· path  `adr` obj seg(  split  write),   name .md
//	· orfilename e.g. `ADR-<numchar>` / `NNNN-<tgt >.md` and  docs/ under
//
// ⚠️ 2026-10-03   : base  **curbefore has** ADR file ⇒ base      `false`. 
//
//	⇒  ** is    **, is**produce   ADR   ** --   izepipe"human   has"changebecome"     ". 
func checkADRExists(root string, _ map[string]string) (CheckResult, error) {
	if strings.TrimSpace(root) == "" {
		return CheckResult{}, fmt.Errorf("checkADRExists: repo root path required (**cannot judge, not a pass**)")
	}
	var found []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //  edread  obj ( because limit    )
		}
		name := d.Name()
		if d.IsDir() {
			//  ed    audioobj 
			switch name {
			case ".git", "node_modules", "vendor", ".calib-evidence":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		low := strings.ToLower(rel)
		//  path ①: path has adr obj seg
		if strings.Contains(low, "/adr/") || strings.HasPrefix(low, "adr/") {
			found = append(found, rel)
			return nil
		}
		//  path ②: filename e.g. ADR-123 / 0001-xxx.md and  docs/ under
		base := strings.ToUpper(name)
		if strings.Contains(low, "docs/") &&
			(strings.HasPrefix(base, "ADR-") || isNNNNDashed(name)) {
			found = append(found, rel)
		}
		return nil
	})
	if err != nil {
		return CheckResult{}, fmt.Errorf("checkADRExists: failed to walk repo: %w", err)
	}
	if len(found) == 0 {
		return CheckResult{
			Passed: false,
			Detail: "no ADR files in repo (rule: path contains adr/ segment, or ADR-* / NNNN-*.md under docs/)",
		}, nil
	}
	return CheckResult{
		Passed: true,
		Detail: fmt.Sprintf("found %d ADR files, e.g. %s", len(found), strings.Join(head(found, 3), " · ")),
	}, nil
}

// checkWithinBoundary  " end out-of-scope": **usedomain forbid**(target ④ ptname). 
//
// domain forbid  decideby `pipeline` produceoccur(`verdict.Allowed` / `BOUNDARY_VIOLATION`). 
// base   ** heavynew now forbid**, read-onlycalluse      forbidclose (`extra["gate_allowed"]`). 
//
// ⚠️    forbidclose  ⇒ **returnback error**("   forbid" != " out-of-scope"). 
func checkWithinBoundary(_ string, extra map[string]string) (CheckResult, error) {
	v, ok := extra["gate_allowed"]
	if !ok {
		return CheckResult{}, fmt.Errorf(
			"checkWithinBoundary: no domain-guard result provided (extra[gate_allowed]) -> **cannot judge, not a pass**; " +
				"the guard decision is in pipeline (verdict.Allowed / BOUNDARY_VIOLATION)")
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "allowed":
		reason := extra["gate_reason"]
		return CheckResult{Passed: true, Detail: "domain-guard verdict = allowed (verdict.Allowed)" + suffix(reason)}, nil
	case "false", "0", "no", "denied":
		reason := extra["gate_reason"]
		return CheckResult{Passed: false, Detail: "domain-guard verdict = **BOUNDARY_VIOLATION**" + suffix(reason)}, nil
	default:
		return CheckResult{}, fmt.Errorf("checkWithinBoundary: unparseable guard result %q -> **cannot judge, not a pass**", v)
	}
}

// checkVerifiedOverHearsay  "already   firstat  called"( hasrule, patchon  body). 
//
// already   check produceoccur  data >  headcalled. `extra["evidence_kind"]` getvalue: verified | hearsay. 
func checkVerifiedOverHearsay(_ string, extra map[string]string) (CheckResult, error) {
	k, ok := extra["evidence_kind"]
	if !ok {
		return CheckResult{}, fmt.Errorf("checkVerifiedOverHearsay: no evidence kind provided (extra[evidence_kind]) -> **cannot judge, not a pass**")
	}
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "verified":
		return CheckResult{Passed: true, Detail: "evidence comes from an **executed check** (ExecutedCheck)"}, nil
	case "hearsay":
		return CheckResult{Passed: false, Detail: "evidence is **hearsay only** -> insufficient for this criterion"}, nil
	default:
		return CheckResult{}, fmt.Errorf("checkVerifiedOverHearsay: unparseable evidence kind %q -> **cannot judge, not a pass**", k)
	}
}

// --------------------     --------------------

func isNNNNDashed(name string) bool {
	//  e.g. 0001-some-title.md
	i := strings.IndexByte(name, '-')
	if i != 4 {
		return false
	}
	for _, c := range name[:4] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func head(xs []string, n int) []string {
	if len(xs) <= n {
		return xs
	}
	return xs[:n]
}

func suffix(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "（" + s + "）"
}
