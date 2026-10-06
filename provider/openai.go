package provider

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// openaiClient is OpenAI compatendpoint clientuserend(   §14.2): 
// compat OpenAI / DeepSeek /  typein (model.example.com)/ Gemini compat etc
//   voice  /chat/completions   close. 
type openaiClient struct {
	name           string
	url            string // rule izeafter finish  chat/completions URL
	model          string
	apiKey         string
	responseFormat bool           // is  require json_object  form
	timeoutMs      int            //   calluse  time( heavy )
	params         map[string]any // endpoint    charseg(e.g. reasoning_effort)
	httpClient     *http.Client
}

// newOpenAIClient from config.Provider   clientuserend. 
func newOpenAIClient(p config.Provider, cfg *config.Config) *openaiClient {
	return &openaiClient{
		name:           p.Name,
		url:            normalizeEndpoint(p.Endpoint),
		model:          p.Model,
		apiKey:         p.APIKey,
		responseFormat: cfg.EffectiveResponseFormat(p),
		timeoutMs:      cfg.EffectiveTimeoutMs(p),
		params:         p.Params,
		//  timeby context   control(see Chat),      Transport    time, 
		// by and ctx  time   becomesemantic  . 
		// VHS_INSECURE_TLS=1 time ed TLS     (in /   close occur , defaultclose ). 
		httpClient: &http.Client{Transport: vhsHTTPTransport()},
	}
}


// vhsHTTPTransport   default Transport; onlycur form   VHS_INSECURE_TLS=1 time
//  ed TLS     (2026-10-04  close         LLM safety   occur ). 
func vhsHTTPTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if os.Getenv("VHS_INSECURE_TLS") == "1" {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec //  form occur , defaultclose 
	}
	return t
}

// normalizeEndpoint rule izeendpointasfinish   chat URL. 
// compat kind state: 

//   - base:              "https://host"                    -> "https://host/chat/completions"
//   -   /v1:            "https://host/v1"                 -> "https://host/v1/chat/completions"
//   - finish path:          "https://host/v1/chat/completions" -> origkindreturnback
//
// sametime    tail   (e.g. "https://host/", "https://host/v1/"). 
func normalizeEndpoint(ep string) string {
	ep = strings.TrimRight(ep, "/")
	// 2026-10-04 fix :  close /api/model/chat alreadyisfinish  chat endpoint(POST    chat.completion), 
	// normalize ifagain  /chat/completions -> /api/model/chat/chat/completions -> 404 no such endpoint
	// (    : curl   POST /api/model/chat returnback 200 chat.completion). 
	if !strings.HasSuffix(ep, "/chat/completions") && !strings.HasSuffix(ep, "/chat") {
		ep += "/chat/completions"
	}
	return ep
}

func (c *openaiClient) Name() string { return c.name }

// maxRetries is  after   heavy  num(i.e.    num = 1 + maxRetries = 3). 
var retryBackoffs = []time.Duration{300 * time.Millisecond, 600 * time.Millisecond}

// Chat send   daypatchsafety require, by §14.2 handle  / time/heavy /resolve . 
func (c *openaiClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	//   provider use cfg.EffectiveTimeoutMs   context  time( safety heavy ). 
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
		//   heavy error(4xx  serviceerror / ctx cancel /    JSON)orheavy  numuse  ->  connectreturnback. 
		if !retryable || attempt >= len(retryBackoffs) {
			return ChatResponse{}, lastErr
		}
		//    300ms / 600ms; ctx canceltime i.e. out. 
		select {
		case <-ctx.Done():
			return ChatResponse{}, ctx.Err()
		case <-time.After(retryBackoffs[attempt]):
		}
	}
}

// buildRequestBody    requirebody:   charseg + endpoint   Params. 
// note :   pathall  pipe apiKey write  body(    Header). 
func buildRequestBody(c *openaiClient, req ChatRequest) ([]byte, error) {
	body := map[string]any{
		"model":    c.model,
		"messages": req.Messages,
	}
	// gpt-6-luna etcnew type connectaccept temperature=0(only allowdefaultvalue 1,    400); 
	//   ityby max_completion_tokens + response_format keep , thus class type send temperature. 
	if c.params["use_max_completion_tokens"] != true {
		body["temperature"] = 0 // harness needrequire  ity out
	}
	if req.MaxTokens > 0 {
		// gpt-6-luna etcnew type  keep max_tokens, onlyconnectaccept max_completion_tokens
		// ( typein    400 unsupported_parameter).  edendpoint Params  
		// use_max_completion_tokens:true openstart,     deepseek/openai etc  type. 
		if c.params["use_max_completion_tokens"] == true {
			body["max_completion_tokens"] = req.MaxTokens
		} else {
			body["max_tokens"] = req.MaxTokens
		}
	}
	// response_format  first : ChatRequest.ResponseFormat  formoverwrite > provider   . 
	//  form false(e.g. QUERY answer need  base)->    json_object,     refer under type outno   JSON  . 
	useJSON := c.responseFormat
	if req.ResponseFormat != nil {
		useJSON = *req.ResponseFormat
	}
	if useJSON {
		body["response_format"] = map[string]any{"type": "json_object"}
	}
	// endpoint   num  (e.g. DeepSeek   reasoning_effort:"none"),    afterbythenoverwrite. 
	for k, v := range c.params {
		body[k] = v
	}
	return json.Marshal(body)
}

// chatCompletionResponse   on become     (onlygetbase harness needneed charseg). 
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content   string          `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls"` //  emptyand content asempty = endpoint restrict function-calling
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage contract.Usage `json:"usage"`
}

// doOnce      HTTP  require, returnbackresolve close ; retryable tableshowis value heavy (429/5xx/  error). 
// error  only statuscodeand disconnectafter   body seg,      API key. 
func (c *openaiClient) doOnce(ctx context.Context, body []byte) (resp ChatResponse, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, true, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		// 2026-10-04 fix : voxsign  close  headis X-AIops-Key(read key),   Authorization Bearer
		//(    : Bearer 401 "missing or invalid read X-AIops-Key"; curl   X-AIops-Key 200). 
		if strings.Contains(c.url, "aiops.voxsign.ai") {
			req.Header.Set("X-AIops-Key", c.apiKey)
		} else {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
	}

	raw, err := c.httpClient.Do(req)
	if err != nil {
		// ctx cancel/ time  heavy ; its   error heavy . 
		if ctx.Err() != nil {
			return ChatResponse{}, false, ctx.Err()
		}
		return ChatResponse{}, true, fmt.Errorf("network request failed: %w", err)
	}
	defer raw.Body.Close()

	data, err := io.ReadAll(io.LimitReader(raw.Body, 1<<20))
	if err != nil {
		return ChatResponse{}, true, fmt.Errorf("failed to read response body: %w", err)
	}

	if raw.StatusCode < 200 || raw.StatusCode >= 300 {
		snippet := truncateResp(string(data), 300)
		// 429 / 5xx ->  heavy ; its  4xx(400/401/403…)->  i.e.  . 
		retry := raw.StatusCode == http.StatusTooManyRequests || raw.StatusCode >= 500
		return ChatResponse{}, retry, fmt.Errorf("HTTP %d: %s", raw.StatusCode, snippet)
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		// 200 but   is   JSON: heavy no  ,  connect  ( segthenattraceback   ). 
		return ChatResponse{}, false, fmt.Errorf("failed to parse response JSON: %w; body=%q", err, truncateResp(string(data), 300))
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, false, fmt.Errorf("response has no choices; body=%q", truncateResp(string(data), 300))
	}
	choice := parsed.Choices[0]
	// content asemptybutoutnow tool_calls:   endpointbe restrict  function-calling  form; 
	// base harness from send  tools  num,  at  error, need    but   returnbackempty . 
	if strings.TrimSpace(choice.Message.Content) == "" && len(choice.Message.ToolCalls) > 0 {
		return ChatResponse{}, false, fmt.Errorf(
			"endpoint returned tool_calls instead of content: likely forced function-calling mode (this harness does not send tools param)")
	}

	return ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage:        parsed.Usage,
	}, false, nil
}

// truncateResp pipe  body seg disconnectto n char ,   error  ed   trace. 
func truncateResp(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
