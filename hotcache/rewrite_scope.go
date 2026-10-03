// rewrite_scope.go —— 别名的**两种用途**必须分开（Lead 裁决，2026-10-03）。
//
//	路由匹配（"找谁"）：**全部别名**可用（含"文件"/"存储"这类通用词 —— 这里有用）
//	文本改写（"改字"）：**只允许专名类**；通用词一律**不得改写**
//
// 理由：「文件」在正常句子里出现的频率远高于它作为别名出现；
// **改写是替换，替换错了就是改坏；路由只是指向，指错了还能回问。**
package hotcache

import "strings"

// genericRewriteBlocklist 是**显式登记**的通用词：参与路由、**不参与文本改写**。
// ⚠️ 不得靠"看着像"判断；新发现的通用词请**登记在此**（附来源）。
var genericRewriteBlocklist = map[string]string{
	"文件": "通用词（observed：'先改这个文件再提交' 被改写成 file-store）",
	"文档": "通用词（同上族）",
	"存储": "通用词",
	"数据": "通用词",
	"服务": "通用词",
	"系统": "通用词",
	"任务": "通用词",
	"内容": "通用词",
	"状态": "通用词",
	"配置": "通用词",
}

// GenericRewriteBlocklist 返回通用词清单（词 → 登记理由），供审计与判据使用。
func GenericRewriteBlocklist() map[string]string {
	out := make(map[string]string, len(genericRewriteBlocklist))
	for k, v := range genericRewriteBlocklist {
		out[k] = v
	}
	return out
}

// isGenericForRewrite 判断某个**别名**是否被禁止参与文本改写。
func isGenericForRewrite(alias string) bool {
	_, ok := genericRewriteBlocklist[strings.TrimSpace(alias)]
	return ok
}

// LookupForRewrite 是**改写用**的查询：通用词别名与用户黑名单一律跳过（路由请用 Lookup）。
func (c *Cache) LookupForRewrite(term string) (Result, bool) {
	term = strings.TrimSpace(term)
	if term == "" {
		return Result{NeedEscalate: true}, false
	}
	if isGenericForRewrite(term) {
		// 这个词本身是通用词 ⇒ 不参与改写（也不去猜它的"专名对应"）
		return Result{NeedEscalate: true, Status: c.Snapshot().Status}, false
	}
	if c.isBlacklisted(term) {
		// 用户点过〔这个改错了〕⇒ 该词不再参与文本改写（路由仍可用，两用途分离）。
		return Result{NeedEscalate: true, Status: c.Snapshot().Status}, false
	}
	return c.lookupInternal(term, true)
}
