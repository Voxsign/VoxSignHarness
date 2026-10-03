// trie.go —— 自写的多模式匹配器（无第三方依赖）。
//
// 用途：一次扫描找出原文中所有命中的词条。当前词表很小，但结构必须能长大——
// 这里用 rune 为边的 trie（前缀树），每个节点可挂多条词条（同前缀多条时全保留），
// 匹配成本 O(n · 最长词条长度)，与词表总条数无关。
//
// 后续若词表上千，可在此文件内换成 Aho–Corasick（加 fail 指针），接口不变。
package asr

// trieNode 是一个前缀节点。
type trieNode struct {
	next    map[rune]*trieNode
	entries []int // 以该节点结尾的词条下标（index into compiled.entries）
}

// trie 是以 rune 为边的前缀树。
type trie struct {
	root *trieNode
}

func newTrie(entries []entry) *trie {
	t := &trie{root: &trieNode{}}
	for i, e := range entries {
		if e.from == "" {
			continue
		}
		n := t.root
		for _, r := range e.from {
			if n.next == nil {
				n.next = make(map[rune]*trieNode)
			}
			child := n.next[r]
			if child == nil {
				child = &trieNode{}
				n.next[r] = child
			}
			n = child
		}
		n.entries = append(n.entries, i)
	}
	return t
}

// matchAt 返回从 i 起匹配上的所有词条下标，按匹配长度升序。
func (t *trie) matchAt(runes []rune, i int) []int {
	var out []int
	n := t.root
	for j := i; j < len(runes); j++ {
		n = n.next[runes[j]]
		if n == nil {
			break
		}
		out = append(out, n.entries...)
	}
	return out
}

// isASCIIAlnum 用于拉丁词条的词边界判定：AR 不应命中 AAAR 里的子串。
func isASCIIAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
