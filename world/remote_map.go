// remote_map.go —— 远端"声称"与"本机授权"的边界（A4）。
//
// 契约：网关读到的任何内容（主机、服务、端口、network、甚至名为 deploy/run/git 的服务）
// 都只是**声明**，不构成本机能力或授权。本机能力只来自 `tools` 契约注册表，
// 权限校验永远在消费方（需求 4.7）。
//
// 因此本函数**恒返回不授权**。EXT-11 给它一条**可失败路径**：
// 一个"远端名字像本地工具就授权"的朴素实现会立刻让该判据变红
// （本文件的前一 version 就是那个朴素实现，用于证明判据可红）。
package world

// MapRemoteCapability 把远端声称的能力映射为本机授权。
// 恒返回 ("", false)：读 200 ≠ 写权限（A4）。
func MapRemoteCapability(string) (string, bool) { return "", false }
