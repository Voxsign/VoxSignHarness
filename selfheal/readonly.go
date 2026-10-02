package selfheal

// IsReadOnlyTool 判定工具族是否【只读】——只有只读族的失败才允许自愈执行器自动重放。
// 写类/不可逆工具（git commit / deploy / note append / 文件写）失败【绝不自动重放】，
// 动作转 ask/stop/fallback，诊断结论只进归因，由人决定。
//
// 判定原则【保守】：判定不了一律按"非只读"处理（宁可不自动重试，也不误重放不可逆动作）。
func IsReadOnlyTool(tool string, args map[string]any) bool {
	switch tool {
	case "search", "get_time", "verify", "read", "file_list", "file-list", "list_dir":
		return true
	case "file":
		// file 工具读写同源：仅 read/exists 算只读；write/append 是写，不自动重放。
		act, _ := args["action"].(string)
		return act == "" || act == "read" || act == "exists"
	default:
		// git / run / test / deploy / note / pipeline …一律不自动重放。
		return false
	}
}
