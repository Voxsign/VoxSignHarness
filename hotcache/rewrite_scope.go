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
	"文件": "generic word (observed: '先改这个文件再提交' rewritten to file-store)",
	"文档": "generic word (same family)",
	"存储": "generic word",
	"数据": "generic word",
	"服务": "generic word",
	"系统": "generic word",
	"任务": "generic word",
	"内容": "generic word",
	"状态": "generic word",
	"配置": "generic word",
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
