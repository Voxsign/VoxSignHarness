// rewrite_scope.go -- diffname ** kinduseway**  splitopen(Lead  decide, 2026-10-03). 
//
//	routeby  ("  "): **safety diffname** use( "file"/"storestore" class useword --   hasuse)
//	 basemodifywrite("modifychar"): **only allow nameclass**;  useword  **  modifywrite**
//
//  by: "file" pos sent  outnow freqrate  at  asdiffnameoutnow; 
// **modifywriteis  ,    thenismodify ; routebyonlyisreferto, refer also clarification. **
package hotcache

import "strings"

// genericRewriteBlocklist is** form  **  useword:  androuteby, **  and basemodifywrite**. 
// ⚠️    " ing " disconnect; newsendnow  useword **    **(   ). 
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

// GenericRewriteBlocklist returnback usewordlist(word ->    by), provide  and data use. 
func GenericRewriteBlocklist() map[string]string {
	out := make(map[string]string, len(genericRewriteBlocklist))
	for k, v := range genericRewriteBlocklist {
		out[k] = v
	}
	return out
}

// isGenericForRewrite  disconnect  **diffname**is beforbidstop and basemodifywrite. 
func isGenericForRewrite(alias string) bool {
	_, ok := genericRewriteBlocklist[strings.TrimSpace(alias)]
	return ok
}

// LookupForRewrite is**modifywriteuse**   :  useworddiffnameanduseuser name    ed(routeby use Lookup). 
func (c *Cache) LookupForRewrite(term string) (Result, bool) {
	term = strings.TrimSpace(term)
	if term == "" {
		return Result{NeedEscalate: true}, false
	}
	if isGenericForRewrite(term) {
		//   wordbase is useword ⇒   andmodifywrite(also     " nameto ")
		return Result{NeedEscalate: true, Status: c.Snapshot().Status}, false
	}
	if c.isBlacklisted(term) {
		// useuserpted   modify  ⇒  word again and basemodifywrite(routeby  use,  usewaysplit ). 
		return Result{NeedEscalate: true, Status: c.Snapshot().Status}, false
	}
	return c.lookupInternal(term, true)
}
