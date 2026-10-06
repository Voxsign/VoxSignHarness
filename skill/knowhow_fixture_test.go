// knowhow_fixture_test.go -- **  serveservice knowhow**(   get, 2026-10-03)occurbecome  fixture. 
//
//   : GET https://aiops.peterzou.com/api/skill/skills/{id}(Header X-AIops-Key). 
// **to **:  numand Lead  value    (30/11/9/6 = 56);     humanadd . 
package skill

func realKnowhow() map[string]Knowhow {
	return map[string]Knowhow{
		// ai-native-architecture-design 1.0.1:    30  
		"ai-native-architecture-design": {
			Steps: []string{
				"澄清约束：目标/边界/涉众/规模/技术栈/成本合规；关键信息不足先提≤3个必要问题，不臆造假设",
				"确定质量属性：从功能外提取NFR并排序（可用性/性能/扩展/安全/可维护/成本/可观测），显式列出冲突",
				"生成候选方案：至少2-3个，覆盖保守基线到AI原生两端，不把唯一解当默认",
				"权衡分析：决策矩阵按质量属性加权打分，每个关键决策给正反论证与放弃理由",
				"记录决策：每个重大决策一条ADR（背景/备选/决策/后果/状态）",
				"产出与验证：按交付物规范输出，用可能失败的方式自查（图示一致/数字可追溯/风险已登记）",
			},
			Judging: []string{
				"输入与约束是否全部读入并反映在文档中",
				"每个重大决策是否有ADR与备选方案记录",
				"图示与文字一致、C4分层正确、一张图一个问题",
				"数字/容量/成本可追溯，或明确标注为估算及口径",
				"涉及AI层时覆盖模型/Agent/记忆/治理四类增量",
				"风险登记册列明每项风险的缓解措施",
				"质量属性冲突显式列出并给出取舍结论",
			},
			Cautions: []string{
				"不臆造假设：关键信息不足先提问；低风险假设显式标注并说明影响",
				"不把唯一解当默认：必须给出候选方案与权衡分析",
				"图示与文字必须一致：C4分层正确，一张图只回答一个问题",
				"数字必须可追溯：容量/成本标注来源或明确为估算及口径",
				"AI层设计必覆盖：模型、Agent、记忆、治理四类增量缺一不可",
				"风险必须登记：每项风险列缓解措施，质量属性冲突给取舍结论",
				"范式优先：从新范式出发，旧经验逐条核验可迁移性，不因老经验而默认正确",
			},
			Basis: []string{
				"经典架构能力（C4/ADR/质量属性/权衡分析）是地基，AI时代不推翻",
				"AI时代四层增量：模型与推理、Agent/工作流、记忆与状态、治理与保障",
				"范式优先原则：先问新范式内核，再逐条核验旧经验可迁移性（paradigm-playbook S1-S6）",
				"底层标准化、上层差异化；演进不推翻、灰度替换、双轨运行",
				"知识是活资产：随证据更新，ADR可废弃，不把决策当永久真理",
			},
			Style: []string{
				"默认交付：架构设计文档(HLD) + C4图示 + ADR决策记录集；用户指定形态时以用户为准",
				"HLD固定12节：背景/约束与假设/质量属性/系统上下文/容器组件/关键流程/数据存储/AI层/安全合规/演进路线/风险登记册/决策索引",
				"图示优先Mermaid/ECharts/SVG等可渲染可验证格式",
				"ADR一条一决策、一页以内，模板见references/frameworks.md",
				"落地用脚手架：python scripts/scaffold_arch_repo.py <repo> [--path <dir>]",
			},
		},
		// arch-guardian 0.1.0:    11  
		"arch-guardian": {
			Steps: []string{
				"澄清目标与边界",
				"提取质量属性并排序",
				"给出候选方案",
				"权衡后记录决策",
			},
			Judging: []string{
				"约束未越界",
				"决策有 ADR",
			},
			Cautions: []string{
				"不臆造假设",
				"关键歧义一次问清",
			},
			Basis: []string{
				"已查证优先于一方称",
			},
			Style: []string{
				"结论先行",
				"证据支撑",
			},
		},
		// deep-research 0.3.0:    9  
		"deep-research": {
			Steps: []string{
				"澄清任务目标与边界",
				"收集并核验证据",
				"综合输出结构化结论",
			},
			Judging: []string{
				"结论与证据一致性",
			},
			Cautions: []string{
				"不得虚构事实",
				"区分已查证与推测",
			},
			Basis: []string{
				"可追溯来源优先",
			},
			Style: []string{
				"结论先行",
				"证据支撑",
			},
		},
		// arch-review 0.1.0:    6  
		"arch-review": {
			Steps: []string{
				"澄清约束",
				"定质量属性",
			},
			Judging: []string{
				"质量属性有排序",
			},
			Cautions: []string{
				"不臆造假设",
			},
			Basis: []string{
				"证据分级",
			},
			Style: []string{
				"先想明白再做",
			},
		},
	}
}
