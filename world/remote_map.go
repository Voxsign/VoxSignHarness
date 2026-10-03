// remote_map.go —— **P1 错误实现（仅用于让 EXT-11 先红）**。
//
// 这是一个"朴素映射"：远端声称什么，只要名字看起来像本地工具就给授权。
// 它正是 EXT-11 要抓的那种实现；下一 commit 会改成"恒不授权"的正确版本。
package world

var naiveLocalTools = map[string]bool{
	"run": true, "git": true, "file": true, "search": true, "test": true, "verify": true,
}

// MapRemoteCapability 把远端声称的能力映射为本机授权（当前实现是**错的**）。
func MapRemoteCapability(remote string) (string, bool) {
	if naiveLocalTools[remote] {
		return remote, true
	}
	return "", false
}
