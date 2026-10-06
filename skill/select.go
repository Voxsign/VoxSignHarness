// Package skill --    :  name   and"beed   form  "(SK-4  path). 
//
//  path(Lead 2026-10-03): ** name  + state != deprecated**; 
// **beed     form  **( allow    ); **  serveservicesidenumdata**. 
package skill

// Skill is   numdata(   /api/skill/skills). 
type Skill struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	State   string `json:"state"` // active | deprecated | ...
	Kind    string `json:"kind"`  // examplee.g. research ⇒   end
}

// Filtered     beed     **anditsorigbecause**( form,  allow  ). 
type Filtered struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Selection is  close . 
type Selection struct {
	Included []Skill    `json:"included"`
	Filtered []Filtered `json:"filtered"`
}

// DefaultWhitelist is** codein  ** inize name (SK-10:    ,  bycalluse    ). 
//
//  data Lead 2026-10-03  inize disconnect(state != deprecated  by StateClass pipeclose). 
var DefaultWhitelist = map[string]bool{
	"ai-native-architecture-design": true,
	"arch-guardian":                 true,
	"arch-review":                   true,
	"deep-research":                 true,
}

// Select by name   :    name  ⇒ not_in_whitelist; state=deprecated ⇒ deprecated. 
// **  **: len(Included)+len(Filtered) == len(all)( has    be    ). 
func Select(all []Skill, allow map[string]bool) Selection {
	var sel Selection
	for _, s := range all {
		switch StateClass(s.State) {
		case "offline":
			sel.Filtered = append(sel.Filtered, Filtered{ID: s.ID, Reason: "offline:" + s.State})
		case "unknown":
			// SK-9: **     state ⇒   in +   **(to "    ⇒ Unknown,   ")
			sel.Filtered = append(sel.Filtered, Filtered{ID: s.ID, Reason: "unknown_state:" + s.State})
		case "online":
			if !allow[s.ID] {
				sel.Filtered = append(sel.Filtered, Filtered{ID: s.ID, Reason: "not_in_whitelist"})
				continue
			}
			sel.Included = append(sel.Included, s)
		}
	}
	return sel
}
