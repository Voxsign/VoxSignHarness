// basefileis builder  back protect,   at stubsmith   data ;  data see e2e/arch_test.go. 
//
//  scenario:      A1(decide  #7   in )--"note     , use     " be  UNKNOWN, 
// rootbecauseispipe"note   intent" nowbecometo   lang wordtable  (     /note   /newadd  …). 
// fix pipe data becomeclose ity  (note  word +  ize word +   nameword, see taskintent.go
// registerToolRequest), basefile   fix  **   to**, preventstopback : 
//
//  1. posto: same note intent  same    all  REGISTER_TOOL(useuser  then  wordtable); 
//  2. revto:  "note /  /  "charkindbutsemantic isnote  sent ,   be   --
//      itsis"  undernote table"   is QUERY, "pipe  by modifybecomein "   is EDIT. 
//
// basefileonlyoverwritebase fix  pos/revexample,    finish  databody ( is stubsmith    ). 
package input

import (
	"testing"

	"voicesign-harness/contract"
)

// TestRegisterToolStructuralHits   : note  word    nameword  same  all  REGISTER_TOOL. 
func TestRegisterToolStructuralHits(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []string{
		// task  recv sent. 
		"加一个工具把 md 转 pdf",
		"给我加个工具，能把 markdown 转成 pdf",
		"注册一个命令，用来压缩图片",
		// same close    its   (   dataisclose but is  wordtable). 
		"新增一个技能",
		"添加个插件",
		"创建一个脚本",
		"新建一个命令",
		"加个新工具",
	}
	for _, text := range cases {
		if got := c.ClassifyTask(text); got.Intent != contract.IntentRegisterTool {
			t.Errorf("注册意图未命中: %q 被判 %s（期望 REGISTER_TOOL）", text, got.Intent)
		}
	}
}

// TestRegisterToolNoFalsePositiveAmongLookalikes   revto: 
// sent  outnow"note /  /  " etcatnote intent, pos  EDIT/QUERY/TEST   be  . 
func TestRegisterToolNoFalsePositiveAmongLookalikes(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)

	//   keepkeep  classdiff close revexample( data  A1   recv 2  connectdependency   ). 
	exact := []struct{ text, want string }{
		{"查一下注册表", contract.IntentQuery},      // "note "isnameword   split,  is word
		{"查一下已注册的工具", contract.IntentQuery},   //  wordandnamewordoftime ing" " = describe,  isnote 
		{"把提交按钮改成中文", contract.IntentEdit},    //  "  "but is COMMIT/REGISTER_TOOL
		{"把命令改成中文", contract.IntentEdit},      //  "  "but  is EDIT
		{"把那个工具的说明改成中文", contract.IntentEdit}, //  "  "but  is EDIT
		{"跑一下工具链的测试", contract.IntentTest},    //  "  chain"but  is TEST
		{"删掉那个工具", contract.IntentEdit},       //  "  "but  isdelete  
	}
	for _, tc := range exact {
		if got := c.ClassifyTask(tc.text); got.Intent != tc.want {
			t.Errorf("误判: %q 被判 %s（期望 %s）", tc.text, got.Intent, tc.want)
		}
	}

	// onlyneedrequire" is REGISTER_TOOL"    sent(classdiffbyits  has datadecide , basefile   disconnectlang). 
	notRegister := []string{
		"添加一个注释",    // "note " is  nameword( wordtablealso   in)
		"增加一个测试",    //   is TEST
		"加个说明文档",    //   is  ,  is note   
		"给这个报告加个图表", //   isin 
		"参加一个工具培训",  // " "onlyis"  "   split
	}
	for _, text := range notRegister {
		if got := c.ClassifyTask(text); got.Intent == contract.IntentRegisterTool {
			t.Errorf("误判: %q 被判 REGISTER_TOOL，但它不是注册指令", text)
		}
	}
}
