// registry.go —— 按配置构建通道客户端；key 只从环境变量读（L1/L2/L5）。
//
// 为什么没有直接复用 provider/：其 normalizeEndpoint 会把端点规范成
// `<base>/chat/completions`，而 AIOps 实测端点是 `/api/model/chat`
// （ASR-EXT-006 §1）。按 A5「不猜路径」，我们**用实测路径原样发起**，
// 协议仍是标准 OpenAI 兼容（chat.completion / choices[].message.content / usage）。
// 这一点已作为 C 类信号上报（复用点不成立，不是不愿意复用）。
package modelcenter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Response 是一次模型调用的最小信封（与 OpenAI 兼容格式对齐）。
type Response struct {
	Channel          Channel `json:"channel"`
	ModelID          string  `json:"model_id"` // L5：每次调用必须可归因
	Content          string  `json:"content"`
	FinishReason     string  `json:"finish_reason"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
}

// WriteToken 是"允许写回持久知识"的能力凭证（L2）。
// 未导出字段 ⇒ 外部无法伪造；只有 learn 通道且 enabled 时才会签发。
type WriteToken struct{ channel Channel }

// Channel 返回该令牌对应的通道（审计用）。
func (t WriteToken) Channel() Channel { return t.channel }

type chatClient struct {
	endpoint string // 完整端点（实测路径）
	key      string // 只存在于内存，绝不落盘/打印
	model    string
	timeout  time.Duration
	hc       *http.Client
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *chatClient) chat(ctx context.Context, prompt string) (Response, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: []message{{Role: "user", Content: prompt}},
		Stream:   false,
	})
	if err != nil {
		return Response{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key) // 绝不记录该字符串
	resp, err := c.hc.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("模型调用失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode != http.StatusOK {
		// 不回显响应体全文（可能含敏感信息），只给状态码与前 200 字节的**去除换行**片段。
		snippet := strings.ReplaceAll(string(data), "\n", " ")
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return Response{}, fmt.Errorf("模型调用 HTTP %d: %s", resp.StatusCode, snippet)
	}
	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return Response{}, fmt.Errorf("响应不是 OpenAI 兼容 JSON: %w", err)
	}
	out := Response{ModelID: cr.Model, FinishReason: "", TotalTokens: cr.Usage.TotalTokens,
		PromptTokens: cr.Usage.PromptTokens, CompletionTokens: cr.Usage.CompletionTokens}
	if out.ModelID == "" {
		out.ModelID = c.model
	}
	if len(cr.Choices) > 0 {
		out.Content = cr.Choices[0].Message.Content
		out.FinishReason = cr.Choices[0].FinishReason
	}
	return out, nil
}

// Registry 是"通道 → 客户端"的只读注册表。
type Registry struct {
	cfg     Config
	clients map[Channel]*chatClient
	tokens  map[Channel]WriteToken
}

// NewRegistry 校验配置并只为 **enabled** 的通道构建客户端（fail-closed）。
// key 从 `<APIKeyEnv>` 环境变量读；缺失则报错，绝不内置默认 key。
func NewRegistry(cfg Config) (*Registry, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(os.Getenv(cfg.Gateway.APIKeyEnv))
	if key == "" {
		return nil, fmt.Errorf("缺少 %s：请通过环境变量或 .env 提供（本包不读取、不内置任何密钥）", cfg.Gateway.APIKeyEnv)
	}
	endpoint := strings.TrimRight(cfg.Gateway.BaseURL, "/") + cfg.Gateway.ChatPath
	r := &Registry{cfg: cfg, clients: map[Channel]*chatClient{}, tokens: map[Channel]WriteToken{}}
	for _, ch := range AllChannels() {
		cc := cfg.Channels[string(ch)]
		if !cc.Enabled {
			continue // 未启用的通道不建立任何客户端（也就无法被误调）
		}
		timeout := time.Duration(cc.TimeoutMS) * time.Millisecond
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		r.clients[ch] = &chatClient{endpoint: endpoint, key: key, model: cc.ModelID, timeout: timeout, hc: &http.Client{}}
	}
	r.tokens[ChannelLearn] = WriteToken{channel: ChannelLearn}
	return r, nil
}

// Enabled 报告某通道是否可用。
func (r *Registry) Enabled(ch Channel) bool {
	_, ok := r.clients[ch]
	return ok
}

// Invoke 通过指定通道发起一次调用。未启用的通道一律拒绝（不静默降级到别的通道）。
func (r *Registry) Invoke(ctx context.Context, ch Channel, prompt string) (Response, error) {
	client, ok := r.clients[ch]
	if !ok {
		return Response{}, fmt.Errorf("通道 %q 未启用（enabled:false）—— 不自动改用其它通道", ch)
	}
	resp, err := client.chat(ctx, prompt)
	if err != nil {
		return Response{}, err
	}
	resp.Channel = ch
	// L5：模型标识以配置为准（若上游回显不同，仍记录**配置值**以便归因）。
	resp.ModelID = r.cfg.Channels[string(ch)].ModelID
	return resp, nil
}

// WriteBackToken 只在 learn 通道 enabled 时签发（L2：唯一写回通道）。
func (r *Registry) WriteBackToken(ch Channel) (WriteToken, bool) {
	if ch != ChannelLearn || !r.Enabled(ChannelLearn) {
		return WriteToken{}, false
	}
	t, ok := r.tokens[ChannelLearn]
	return t, ok
}
