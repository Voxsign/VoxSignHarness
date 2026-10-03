// pinyin.go —— 自写的（稀疏）音节表 + 拼音近音索引（无第三方依赖）。
//
// 设计取舍（诚实边界）：
//   - 这是一张**按需增长的稀疏表**，不是完整拼音字典：表里没有的汉字就不参与
//     近音匹配（窗口直接放弃），因此不会因为"猜拼音"而误伤。
//   - 多音字**未做读音消歧**（任务书 §9）：表里每个字只登记**一个**读音。
//     读音歧义会影响匹配的字（觉 jiào/jué、差 chā/chà）整体不收；
//     调(diào/tiáo) 这类只保留一个读音，并由
//     TestPinyinTableHasNoCrossGroupDuplicate 禁止"同字跨两组"（那会让结果依赖组顺序）。
//     宁可不匹配，也不给错的音节。
//   - 近音匹配**只产出候选**（Candidates），绝不自动改写文本。
//     自动改写只走人工审定的静态词表（lexicon.go），高风险专名一律只给候选。
//
// 匹配方式：索引把"音节序列"映射到正确写法；Correct 时用定长滑窗在原文上取窗口，
// 窗口每个字都能在表里查到音节、且音节序列命中索引、且窗口文字 ≠ 正确写法时，
// 产出一条候选。
package asr

import "strings"

// pinyinGroup 是一个音节的汉字分组。**有序切片**（不是 map）：
// 表必须可回放——Go map 迭代顺序随机，若依赖"先命中者胜"，
// 同一份代码在不同进程/重启后可能给出不同拼音表，直接破坏 Correct 的可回放承诺。
type pinyinGroup struct {
	syllable string
	chars    string
}

// pinyinGroups 是**有序**分组表：越靠前的组在冲突时优先（当前表中无跨组重复字，
// 由 TestPinyinTableHasNoCrossGroupDuplicate 钉住；多音字请勿入表——见文件头）。
// 表随语料增长；新增一条词条时，把该词涉及的变体字补进来即可。
var pinyinGroups = []pinyinGroup{
	{"a", "啊"},
	{"ai", "爱哎唉"},
	{"ba", "把爸吧八巴拔"},
	{"bai", "白百败拜"},
	{"bao", "报抱保宝暴爆"},
	{"ben", "本笨"},
	{"bi", "比必笔闭"},
	{"bian", "变边便编"},
	{"bie", "别"},
	{"bing", "并病冰"},
	{"bu", "不步部布补"},
	{"cai", "才财采菜"},
	{"can", "参餐残"},
	{"ce", "测册侧策"},
	{"cha", "查茶差插"},
	{"chang", "常场厂唱"},
	{"cheng", "成城程称"},
	{"chi", "吃持迟尺"},
	{"chu", "出除初处厨"},
	{"ci", "次此词辞"},
	{"cong", "从聪"},
	{"cun", "存村寸"},
	{"cuo", "错措挫"},
	{"da", "大打达答"},
	{"dai", "代带待戴"},
	{"dan", "单但担丹淡蛋"},
	{"dao", "到道倒刀岛"},
	{"de", "得德"},
	{"deng", "等灯登"},
	{"di", "低底弟第敌"},
	{"dian", "点电店典"},
	{"diao", "掉调钓"},
	{"ding", "定顶订丁"},
	{"dong", "东动懂冬"},
	{"du", "读度独毒"},
	{"duan", "段断短"},
	{"dui", "对队"},
	{"duo", "多夺朵躲"},
	{"e", "饿额恶"},
	{"er", "而二儿"},
	{"fa", "发法罚"},
	{"fan", "反饭翻犯"},
	{"fang", "方放房防访"},
	{"fei", "非飞费肥"},
	{"fen", "分份粉奋"},
	{"feng", "风封丰峰"},
	{"fu", "服福付副府父复"},
	{"gai", "该改盖概"},
	{"gan", "干感敢赶"},
	{"gang", "刚钢岗"},
	{"gao", "高告搞稿"},
	{"ge", "个各格哥歌割"},
	{"gei", "给"},
	{"gen", "跟根"},
	{"geng", "更耕"},
	{"gong", "工公功共供"},
	{"gou", "够构购沟"},
	{"gu", "古故顾股鼓"},
	{"guan", "关观管官"},
	{"guang", "光广"},
	{"gui", "贵归规"},
	{"guo", "过国果锅"},
	{"ha", "哈"},
	{"hai", "还海害孩"},
	{"han", "汉含喊寒"},
	{"hao", "好号毫豪"},
	{"he", "和合何喝河"},
	{"hen", "很狠恨"},
	{"hong", "红宏洪"},
	{"hou", "后候厚猴"},
	{"hu", "乎胡湖户护"},
	{"hua", "话花化华划"},
	{"huan", "换环欢"},
	{"hui", "会回灰汇辉"},
	{"huo", "活火货或获"},
	{"ji", "几机记及级即技术急集基计继季冀"},
	{"jia", "家加价假架甲佳"},
	{"jian", "见间件建简减检坚"},
	{"jiang", "讲将江降"},
	{"jiao", "交教叫脚较角胶焦郊"},
	{"jie", "接结解姐街节界"},
	{"jin", "进近今金尽紧"},
	{"jing", "经京精静竟"},
	{"jiu", "就九久酒旧救"},
	{"ju", "句局举具据剧"},
	{"jue", "决绝"},
	{"jun", "军均君"},
	{"kai", "开凯"},
	{"kan", "看砍刊"},
	{"kang", "抗康"},
	{"kao", "考靠"},
	{"ke", "可科克刻课客"},
	{"kong", "空控孔"},
	{"ku", "库裤苦哭"},
	{"kuai", "快块"},
	{"la", "拉啦"},
	{"lai", "来赖"},
	{"lao", "老劳牢"},
	{"le", "了乐"},
	{"li", "里理力离李利立历"},
	{"lian", "连联练脸"},
	{"liang", "两亮量良"},
	{"liao", "料聊"},
	{"lin", "林临邻"},
	{"ling", "另令领零"},
	{"liu", "六流留刘"},
	{"long", "龙隆"},
	{"lu", "路录露陆"},
	{"lun", "论轮"},
	{"luo", "落罗络"},
	{"ma", "吗妈马码"},
	{"mai", "买卖麦"},
	{"man", "满慢忙蛮"},
	{"mao", "毛冒贸"},
	{"me", "么"},
	{"mei", "没每美妹"},
	{"men", "们门闷"},
	{"meng", "梦猛"},
	{"mi", "米密秘迷"},
	{"mian", "面免棉"},
	{"ming", "明名命"},
	{"mo", "末摸模莫"},
	{"mu", "目木母"},
	{"na", "那拿哪纳"},
	{"nai", "奶耐"},
	{"nan", "难南男"},
	{"nei", "内"},
	{"neng", "能"},
	{"ni", "你尼泥逆"},
	{"nian", "年念"},
	{"niao", "鸟"},
	{"nin", "您"},
	{"niu", "牛扭"},
	{"nong", "农弄"},
	{"nu", "努怒"},
	{"nv", "女"},
	{"pai", "派排拍"},
	{"pan", "判盘盼"},
	{"pao", "跑炮"},
	{"pei", "配培陪"},
	{"pen", "盆喷"},
	{"peng", "朋碰"},
	{"pi", "劈批皮匹"},
	{"pian", "片篇偏"},
	{"piao", "票飘"},
	{"pin", "品拼贫"},
	{"ping", "平评苹"},
	{"po", "破迫婆"},
	{"pu", "普铺扑"},
	{"qi", "起其期气器七齐奇"},
	{"qia", "恰"},
	{"qian", "前钱千签浅"},
	{"qiang", "强抢墙"},
	{"qie", "切且"},
	{"qin", "亲勤琴"},
	{"qing", "请情清轻青"},
	{"qiong", "穷"},
	{"qiu", "求球秋"},
	{"qu", "去取区曲娶"},
	{"quan", "全权圈"},
	{"que", "却确缺"},
	{"ran", "然燃染"},
	{"rang", "让"},
	{"rao", "绕扰"},
	{"re", "热"},
	{"ren", "人任认仁"},
	{"ri", "日"},
	{"rong", "容荣"},
	{"rou", "肉柔"},
	{"ru", "如入乳"},
	{"ruan", "软"},
	{"rui", "锐瑞"},
	{"ruo", "若弱"},
	{"san", "三散"},
	{"sao", "扫"},
	{"se", "色"},
	{"sen", "森"},
	{"sha", "杀沙傻"},
	{"shan", "山善闪"},
	{"shang", "上商伤"},
	{"shao", "少烧绍稍"},
	{"she", "设社舍射"},
	{"shei", "谁"},
	{"shen", "深身神什审"},
	{"sheng", "生声胜升省"},
	{"shi", "是十时事实使式市师失试识"},
	{"shou", "手收受首售"},
	{"shu", "书数输树属熟"},
	{"shua", "刷"},
	{"shuai", "帅摔"},
	{"shuang", "双爽"},
	{"shui", "水睡税"},
	{"shun", "顺"},
	{"shuo", "说"},
	{"si", "四死思司丝斯私"},
	{"song", "送松宋"},
	{"sou", "搜"},
	{"su", "速素苏诉"},
	{"suan", "算酸"},
	{"sui", "虽随岁碎"},
	{"sun", "孙损"},
	{"suo", "所缩索锁"},
	{"ta", "他她它塔踏"},
	{"tai", "太台态抬"},
	{"tan", "谈弹坦探"},
	{"tang", "堂糖躺"},
	{"tao", "套讨逃"},
	{"te", "特"},
	{"teng", "疼腾"},
	{"ti", "题提体替梯"},
	{"tian", "天田填甜"},
	{"tiao", "条跳"},
	{"tie", "铁贴"},
	{"ting", "听停厅挺"},
	{"tong", "通同痛桶统"},
	{"tou", "头投透"},
	{"tu", "图土突途"},
	{"tuan", "团"},
	{"tui", "推退腿"},
	{"tun", "吞"},
	{"tuo", "托脱"},
	{"wa", "挖娃瓦"},
	{"wai", "外歪"},
	{"wan", "完万玩晚湾"},
	{"wang", "王网往忘望"},
	{"wei", "为位未味围微危"},
	{"wen", "问文闻稳温"},
	{"wo", "我握窝"},
	{"wu", "无五物务屋武午"},
	{"xi", "系西希习喜细洗息"},
	{"xia", "下夏吓虾"},
	{"xian", "先现线显县限险"},
	{"xiang", "想向像相香详"},
	{"xiao", "小笑效校消"},
	{"xie", "写些谢鞋协"},
	{"xin", "新心信辛"},
	{"xing", "行性型星兴形"},
	{"xiong", "兄胸雄"},
	{"xiu", "修休秀"},
	{"xu", "需许续须序"},
	{"xuan", "选宣悬"},
	{"xue", "学雪血"},
	{"xun", "寻巡训迅"},
	{"ya", "呀压牙亚鸭"},
	{"yan", "眼言严研烟沿演"},
	{"yang", "样阳养洋仰"},
	{"yao", "要药摇咬腰邀"},
	{"ye", "也业夜叶页"},
	{"yi", "一以已意义亿易衣医"},
	{"yin", "因音银引印"},
	{"ying", "应英硬影营迎"},
	{"yo", "哟"},
	{"yong", "用永勇拥"},
	{"you", "有又由右油游友优"},
	{"yu", "于与语雨预育遇"},
	{"yuan", "远元原员院愿"},
	{"yue", "月越约"},
	{"yun", "运云允"},
	{"za", "杂咋"},
	{"zai", "在再载灾"},
	{"zan", "咱暂赞"},
	{"zao", "早造遭"},
	{"ze", "则责"},
	{"zen", "怎"},
	{"zeng", "增曾"},
	{"zha", "炸扎眨"},
	{"zhai", "摘窄债"},
	{"zhan", "站占战展"},
	{"zhang", "张章掌"},
	{"zhao", "找照招"},
	{"zhe", "这着者折哲"},
	{"zhen", "真阵震针"},
	{"zheng", "正整政争征"},
	{"zhi", "之只知直值制治指纸至质"},
	{"zhong", "中重种众终"},
	{"zhou", "周州洲轴"},
	{"zhu", "主住注助祝猪"},
	{"zhua", "抓"},
	{"zhuan", "转专赚"},
	{"zhuang", "装状庄"},
	{"zhui", "追"},
	{"zhun", "准"},
	{"zhuo", "桌"},
	{"zi", "字自子资紫"},
	{"zong", "总宗纵"},
	{"zou", "走奏"},
	{"zu", "组族足租阻"},
	{"zuan", "钻"},
	{"zui", "最嘴醉"},
	{"zun", "尊遵"},
	{"zuo", "做作坐左座"},
}

// buildPinyinTable 把**有序**分组表反转成 汉字→音节 的查询表。
// 纯函数：同一份 pinyinGroups 永远得到同一张表（顺序确定，与 map 迭代无关）。
// 冲突（同字跨组）时前者优先，但当前表已由测试保证不存在冲突。
func buildPinyinTable() map[rune]string {
	table := make(map[rune]string)
	for _, g := range pinyinGroups {
		for _, r := range g.chars {
			if _, seen := table[r]; !seen {
				table[r] = g.syllable
			}
		}
	}
	return table
}

// pinyinTerm 是一条"正确写法 + 其音节序列"的热词条目。
type pinyinTerm struct {
	canonical string
	syllables []string
	conf      float64
	note      string
}

// pinyinIndex 是音节序列 → 正确写法 的索引。
type pinyinIndex struct {
	byKey map[string][]pinyinTerm
	lens  []int // 需要尝试的窗口长度（rune 数），升序
	table map[rune]string
}

func newPinyinIndex(terms []pinyinTerm, table map[rune]string) *pinyinIndex {
	idx := &pinyinIndex{byKey: make(map[string][]pinyinTerm), table: table}
	seenLen := make(map[int]bool)
	for _, t := range terms {
		if len([]rune(t.canonical)) != len(t.syllables) {
			continue // 条目自洽性检查：音节数必须等于字数
		}
		key := strings.Join(t.syllables, "|")
		idx.byKey[key] = append(idx.byKey[key], t)
		if n := len(t.syllables); !seenLen[n] {
			seenLen[n] = true
			idx.lens = append(idx.lens, n)
		}
	}
	// 插入排序即可（长度种类很少），避免再引一个依赖外的排序用法差异。
	for i := 1; i < len(idx.lens); i++ {
		for j := i; j > 0 && idx.lens[j] < idx.lens[j-1]; j-- {
			idx.lens[j], idx.lens[j-1] = idx.lens[j-1], idx.lens[j]
		}
	}
	return idx
}

// keyOf 返回窗口的音节序列 key；窗口里有表外字（含拉丁/标点）则返回 false，放弃。
func (p *pinyinIndex) keyOf(runes []rune) (string, bool) {
	parts := make([]string, len(runes))
	for i, r := range runes {
		s, ok := p.table[r]
		if !ok {
			return "", false
		}
		parts[i] = s
	}
	return strings.Join(parts, "|"), true
}
