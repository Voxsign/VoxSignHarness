//go:build vhsg1

// no_guess_criteria_test.go -- **    **(Lead  decide: first   , again   in). 
//
//  useity : if    and     **splitnumdiff <  value** ⇒ **    **(    in,   ). 
// and VHS-ZHIJI-001 §2.1"  clarification,   ", JEV   ambiguous issame   restrict. 
//  valuetgt UNVALIDATED. 
package hotcache

import (
	"path/filepath"
	"testing"
	"time"
)

// ①  body data: ` ops` ** allow**again to `aiops-portal`. 
func TestNoGuessOhOpsNeverMismatches(t *testing.T) {
	c := g1Cache(t)
	res, ok := c.Lookup("哎ops")
	if ok && res.Canonical == "aiops-portal" {
		t.Fatalf("[错配门槛] 仍然错配到 aiops-portal（route=%s）—— 错配比未命中更危险", res.Route)
	}
	//  allow: pos  in aiops; or     (NeedEscalate)
	if ok && res.Canonical != "aiops" {
		t.Errorf("[错配门槛] 命中了一个既非期望也非不匹配的结果: %+v", res)
	}
	if !ok && !res.NeedEscalate {
		t.Errorf("[错配门槛] 未命中时必须给升级信号: %+v", res)
	}
}

// ②  useity :     splitnumconnect  ⇒     . 
func TestNoGuessTieRefusesToMatch(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	//     and in**   same**   : abc and abd( in abx,     1)
	c.PutAlias("abc", "canon-abc", "remote")
	c.PutAlias("abd", "canon-abd", "remote")
	res, ok := c.Lookup("abx")
	if ok {
		t.Errorf("[错配门槛] 两个等距候选却给了答案（猜）: %+v", res)
	}
	if !res.NeedEscalate {
		t.Errorf("[错配门槛] 拒绝匹配时必须给升级信号: %+v", res)
	}
}

// ③   ize: unique    pos  in( then  pipe  also  ). 
func TestNoGuessUniqueNeighborStillMatches(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	c.PutAlias("voice-sign", "voice-sign", "remote")
	res, ok := c.Lookup("voice-signn")
	if !ok || res.Canonical != "voice-sign" {
		t.Fatalf("[错配门槛] 唯一近邻被误杀（门槛过严）: %+v ok=%v", res, ok)
	}
}

// G1-②   rule ize + prevent edhead. 
func TestG1MixedNormalizationAndGuards(t *testing.T) {
	c := g1Cache(t)
	// posexample:  ops -> aiops(    )
	res, ok := c.Lookup("哎ops")
	if !ok || res.Canonical != "aiops" {
		t.Errorf("[G1-②] 混排未归一到 aiops: %+v ok=%v", res, ok)
	}
	// revexample:       be  (voice-signn        in voice-sign)
	if r, ok := c.Lookup("voice-signn"); !ok || r.Canonical != "voice-sign" {
		t.Errorf("[G1-② 防过头] 纯拉丁串被误伤: %+v ok=%v", r, ok)
	}
	// revexample: ` ops` also  heavynew  to aiops-portal
	if r, ok := c.Lookup("哎ops"); ok && r.Canonical == "aiops-portal" {
		t.Errorf("[G1-② 防回退] 又错配到 aiops-portal: %+v", r)
	}
}

//  use data(Lead  decide): **    ize, if  close to    canonical ⇒     **
// (exampleout: itsin   canonical thenis"rule word  " ⇒   first). 
//  by: "  "dayoccur to ,  by"  after  unique"is    izerule   ,  is  word fixpatch. 
func TestNormalizationMustBeUniqueOrRefuse(t *testing.T) {
	// ①  resolve ⇒ reject:    same canonical   tosame  ( ops / aiops same )
	c := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	c.PutAlias("爱ops", "canon-A", "remote")  //    -> aiops
	c.PutAlias("aiops", "canon-B", "remote") //    -> aiops(  )
	res, ok := c.Lookup("哎ops")              //    -> aiops
	if ok && res.Canonical != "canon-B" && res.Canonical != "" {
		//  allow: rule word  (canon-B) first;  then  reject
		t.Errorf("[归一唯一性] 多解却给了非自身答案（猜）: %+v", res)
	}
	if !ok && !res.NeedEscalate {
		t.Errorf("[归一唯一性] 拒绝匹配时必须给升级信号: %+v", res)
	}

	// ② exampleout: rule word   in ⇒  first( is"reject")
	c2 := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	c2.PutAlias("aiops", "aiops", "remote")
	c2.PutAlias("爱ops", "aiops-portal", "remote")
	r2, ok2 := c2.Lookup("哎ops")
	if !ok2 || r2.Canonical != "aiops" {
		t.Errorf("[归一唯一性] 规范词自身优先未生效: %+v ok=%v", r2, ok2)
	}

	// ③ no   ⇒ pos  in(prevent"  reject" ed keep )
	c3 := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	c3.PutAlias("哎ops", "aiops", "remote")
	r3, ok3 := c3.Lookup("哎ops")
	if !ok3 || r3.Canonical != "aiops" {
		t.Errorf("[归一唯一性] 唯一解被误拒（过度保守）: %+v ok=%v", r3, ok3)
	}
}

// G1-③      diff +  classrevexample. 
func TestG1LatinToleranceAndGuards(t *testing.T) {
	c := g1Cache(t)
	// revexample :  diff  after ` ops` **  **heavynew  ( base alreadydisconnectlang,   again   )
	if r, ok := c.Lookup("哎ops"); ok && r.Canonical == "aiops-portal" {
		t.Errorf("[G1-③ 防回退] 又错配到 aiops-portal: %+v", r)
	}
	// revexample : **         be  **
	if r, ok := c.Lookup("zzzqqqxxyy"); ok {
		t.Errorf("[G1-③ 防治过头] 不相干的拉丁串被匹配: %+v", r)
	}
}

// diffname usewaysplit (Lead  decide):  useword** routeby,   modifywrite**. 
func TestRewriteScopeGenericWordsNotRewritten(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	c.PutAlias("文件", "file-store", "remote")     //  useword(observed   modify )
	c.PutAlias("爱ops", "aiops-portal", "remote") //  namechange 

	// ①  useword: routeby  in, **modifywrite   in**
	if _, ok := c.Lookup("文件"); !ok {
		t.Errorf("[改写域] 路由应仍能用通用词")
	}
	if r, ok := c.LookupForRewrite("文件"); ok {
		t.Errorf("[改写域] 通用词被允许改写（会改坏正常句子）: %+v", r)
	}
	// ②  namechange : modifywrite    in(prevent edhead)
	if r, ok := c.LookupForRewrite("爱ops"); !ok || r.Canonical != "aiops-portal" {
		t.Errorf("[改写域] 专名变形被误杀: %+v ok=%v", r, ok)
	}
	// ③  usewordlist   form  
	if _, ok := GenericRewriteBlocklist()["文件"]; !ok {
		t.Errorf("[改写域] 通用词清单未显式登记「文件」")
	}
}
