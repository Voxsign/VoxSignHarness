// trie.go --  write   form   (no   dependency). 
//
// useway:      outorig in has in word . curbeforewordtable  , butclose      --
//   use rune as   trie(before  ),   nodept    word (samebefore   timesafetykeep ), 
//   becomebase O(n ·   word   ), andwordtable  numnoclose. 
//
// aftercontinueifwordtableon ,    filein become Aho–Corasick(  fail refer ), connect  change. 
package asr

// trieNode is  before nodept. 
type trieNode struct {
	next    map[rune]*trieNode
	entries []int // by nodeptclosetail word undertgt(index into compiled.entries)
}

// trie isby rune as  before  . 
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

// matchAt returnbackfrom i raise  on  hasword undertgt, by      . 
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

// isASCIIAlnum useat  word  word boundary  : AR    in AAAR     . 
func isASCIIAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
