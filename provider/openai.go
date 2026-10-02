package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// openaiClient 是 OpenAI 兼容端点的客户端（架构 §14.2）：
// 兼容 OpenAI / DeepSeek / 模型中心（model.peterzou.com）/ Gemini 兼容层等
// 任何声明了 /chat/completions 的网关。
type openaiClient struct {
	name           string
	url            string // 规范化后的完整 chat/completions URL
	model          string
	apiKey         string
	responseFormat bool           // 是否请求 json_object 模式
	timeoutMs      int            // 单次调用总超时（含重试）
	params         map[string]any // 端点专属透传字段（如 reasoning_effort）
	httpClient     *http.Client
}

// newOpenAIClient 从 config.Provider 构建客户端。
func newOpenAIClient(p config.Provider, cfg *config.Config) *openaiClient {
	return &openaiClient{
		name:           p.Name,
		url:            normalizeEndpoint(p.Endpoint),
		model:          p.Model,
		apiKey:         p.APIKey,
		responseFormat: cfg.EffectiveResponseFormat(p),
		timeoutMs:      cfg.EffectiveTimeoutMs(p),
		params:         p.Params,
		// 超时由 context 统一控制（见 Chat），这里不设 Transport 级硬超时，
		// 以免与 ctx 超时叠加造成语义不清。
		httpClient: &http.Client{},
	}
}

// normalizeEndpoint 规范化端点为完整的 chat/completions URL。
// 兼容三种形态：
//   - base：             "https://host"                    → "https://host/chat/completions"
//   - 带 /v1：           "https://host/v1"                 → "https://host/v1/chat/completions"
//   - 完整路径：         "https://host/v1/chat/completions" → 原样返回
//
// 同时去掉任意尾部斜杠（如 "https://host/"、"https://host/v1/"）。
func normalizeEndpoint(ep string) string {
	ep = strings.TrimRight(ep, "/")
	if !strings.HasSuffix(ep, "/chat/completions") {
		ep += "/chat/completions"
	}
	return ep
}

func (c *openaiClient) Name() string { return c.name }

// maxRetries 是失败后的最大重试次数（即总尝试次数 = 1 + maxRetries = 3）。
var retryBackoffs = []time.Duration{300 * time.Millisecond, 600 * time.Millisecond}

// Chat 发一次聊天补全请求，按 §14.2 处理鉴权/超时/重试/解析。
func (c *openaiClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	// 每 provider 用 cfg.EffectiveTimeoutMs 的 context 超时（含全部重试）。
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.timeoutMs)*time.Millisecond)
	defer cancel()

	body, err := buildRequestBody(c, req)
	if err != nil {
		return ChatResponse{}, err
	}

	var lastErr error
	for attempt := 0; ; attempt++ {
		resp, retryable, err := c.doOnce(ctx, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		// 不可重试错误（4xx 业务错误 / ctx 取消 / 畸形 JSON）或重试次数用尽 → 直接返回。
		if !retryable || attempt >= len(retryBackoffs) {
			return ChatResponse{}, lastErr
		}
		// 退避 300ms / 600ms；ctx 取消时立即退出。
		select {
		case <-ctx.Done():
			return ChatResponse{}, ctx.Err()
		case <-time.After(retryBackoffs[attempt]):
		}
	}
}

// buildRequestBody 组装请求体：固定字段 + 端点透传 Params。
// 注意：任何路径都不得把 apiKey 写进 body（鉴权走 Header）。
func buildRequestBody(c *openaiClient, req ChatRequest) ([]byte, error) {
	body := map[string]any{
		"model":    c.model,
		"messages": req.Messages,
	}
	// gpt-6-luna 等新模型不接受 temperature=0（只允许默认值 1，实测 400）；
	// 确定性由 max_completion_tokens + response_format 保证，故这类模型不发 temperature。
	if c.params["use_max_completion_tokens"] != true {
		body["temperature"] = 0 // harness 要求确定性输出
	}
	if req.MaxTokens > 0 {
		// gpt-6-luna 等新模型不支持 max_tokens，只接受 max_completion_tokens
		// （模型中心实测 400 unsupported_parameter）。通过端点 Params 的
		// use_max_completion_tokens:true 开启，不影响 deepseek/openai 等旧模型。
		if c.params["use_max_completion_tokens"] == true {
			body["max_completion_tokens"] = req.MaxTokens
		} else {
			body["max_tokens"] = req.MaxTokens
		}
	}
	if c.responseFormat {
		body["response_format"] = map[string]any{"type": "json_object"}
	}
	// 端点专属参数透传（如 DeepSeek 的 reasoning_effort:"none"），放在最后以便覆盖。
	for k, v := range c.params {
		body[k] = v
	}
	return json.Marshal(body)
}

// chatCompletionResponse 镜像上游成功响应信封（只取本 harness 需要的字段）。
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content   string          `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls"` // 非空且 content 为空 = 端点强制 function-calling
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage contract.Usage `json:"usage"`
}

// doOnce 执行一次 HTTP 请求，返回解析结果；retryable 表示是否值得重试（429/5xx/网络错误）。
// 错误信息只含状态码与截断后的响应体片段，绝不包含 API key。
func (c *openaiClient) doOnce(ctx context.Context, body []byte) (resp ChatResponse, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, true, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	raw, err := c.httpClient.Do(req)
	if err != nil {
		// ctx 取消/超时不可重试；其余网络错误可重试。
		if ctx.Err() != nil {
			return ChatResponse{}, false, ctx.Err()
		}
		return ChatResponse{}, true, fmt.Errorf("网络请求失败: %w", err)
	}
	defer raw.Body.Close()

	data, err := io.ReadAll(io.LimitReader(raw.Body, 1<<20))
	if err != nil {
		return ChatResponse{}, true, fmt.Errorf("读取响应体失败: %w", err)
	}

	if raw.StatusCode < 200 || raw.StatusCode >= 300 {
		snippet := truncateResp(string(data), 300)
		// 429 / 5xx → 可重试；其他 4xx（400/401/403…）→ 立即失败。
		retry := raw.StatusCode == http.StatusTooManyRequests || raw.StatusCode >= 500
		return ChatResponse{}, retry, fmt.Errorf("HTTP %d: %s", raw.StatusCode, snippet)
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		// 200 但响应不是合法 JSON：重试无意义，直接报错（片段便于轨迹回放定位）。
		return ChatResponse{}, false, fmt.Errorf("解析响应 JSON 失败: %w; body=%q", err, truncateResp(string(data), 300))
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, false, fmt.Errorf("响应无 choices; body=%q", truncateResp(string(data), 300))
	}
	choice := parsed.Choices[0]
	// content 为空但出现 tool_calls：说明端点被强制进了 function-calling 模式；
	// 本 harness 从不发送 tools 参数，属于配置错误，需明确报错而非静默返回空串。
	if strings.TrimSpace(choice.Message.Content) == "" && len(choice.Message.ToolCalls) > 0 {
		return ChatResponse{}, false, fmt.Errorf(
			"端点返回了 tool_calls 而非 content：疑似被强制 function-calling 模式（本 harness 不发送 tools 参数）")
	}

	return ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage:        parsed.Usage,
	}, false, nil
}

// truncateResp 把响应体片段截断到 n 字符，避免错误信息过长污染轨迹。
func truncateResp(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
