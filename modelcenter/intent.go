// intent.go —— 把 default 通道接成 asr.IntentModel（结构化满足其接口，无需反向依赖）。
package modelcenter

import (
	"context"
	"encoding/json"
	"strings"
)

// ClassifyIntent 通过本通道做一次意图兜底判断（仅用于低置信场景）。
func (r *Registry) ClassifyIntent(ctx context.Context, text string) (string, float64, error) {
	prompt := "只输出 JSON，不要解释：{\"intent\":\"<类别>\",\"confidence\":0到1}。\n" +
		"类别只能取：EDIT/DEBUG/QUERY/TEST/COMMIT/DEPLOY/NOTE/ASK/ORCHESTRATE。\n" +
		"用户输入：" + text
	resp, err := r.Invoke(ctx, ChannelDefault, prompt)
	if err != nil {
		return "", 0, err
	}
	s := strings.TrimSpace(resp.Content)
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 {
		s = s[:j+1]
	}
	var out struct {
		Intent     string  `json:"intent"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return "", 0, err
	}
	return out.Intent, out.Confidence, nil
}
