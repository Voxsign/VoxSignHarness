package selfheal

// IsReadOnlyTool      is [read-only]--onlyhasread-only    only allow       heavy . 
// writeclass/ reversible  (git commit / deploy / note append / filewrite)  [    heavy ], 
//     ask/stop/fallback,  disconnectclose only attribution, by decide . 
//
//   origthen[keep ]:      by" read-only"handle(     heavy , also  heavy  reversible  ). 
func IsReadOnlyTool(tool string, args map[string]any) bool {
	switch tool {
	case "search", "get_time", "verify", "read", "file_list", "file-list", "list_dir":
		return true
	case "file":
		// file   readwritesame : only read/exists  read-only; write/append iswrite,    heavy . 
		act, _ := args["action"].(string)
		return act == "" || act == "read" || act == "exists"
	default:
		// git / run / test / deploy / note / pipeline …     heavy . 
		return false
	}
}
