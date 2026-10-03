// learn_detect.go —— LEARN-01 触发器的实现体。
//
// P1（判据先于实现）：本文件当前是**空实现桩**——接口、类型、语料、判据都已就位，
// 判据 LEARN-01 应当是红的。实现于下一步填入（零模型、纯本地）。
package asr

// detectLearnCandidates 目前是桩：不做任何判断，返回空候选。
// 于是 LEARN-01 的 R1（召回）必然红——这正是"先红"的证据，不是测试写错了。
func detectLearnCandidates(engine *Personalized, dict *Dictionary, events []UsageEvent) []LearnCandidate {
	return nil
}
