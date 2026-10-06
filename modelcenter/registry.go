// registry.go -- by      clientuserend; key onlyfrom  change read(L1/L2/L5). 
//
// as   has connect use provider/: its normalizeEndpoint  pipeendpointrule become
// `<base>/chat/completions`, but AIOps   endpointis `/api/model/chat`
// (ASR-EXT-006 §1). by A5"  path",   **use  pathorigkindsendraise**, 
//    istgtapprove OpenAI compat(chat.completion / choices[].message.content / usage). 
//   ptalready as C classsignalon ( usept become ,  is    use). 
package modelcenter

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Response is   typecalluse     (and OpenAI compat formto ). 
type Response struct {
	// RequestedModel/ActualModel/Substituted useat**e.g.     occur   type**. 
	RequestedModel   string
	ActualModel      string
	Substituted      bool
	Channel          Channel `json:"channel"`
	ModelID          string  `json:"model_id"` // L5:   calluse   attribution
	Content          string  `json:"content"`
	FinishReason     string  `json:"finish_reason"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
}

// WriteToken is" allowwritebackkeep   "     (L2). 
//   outcharseg ⇒ out no   ; onlyhas learn   and enabled timeonly  send. 
type WriteToken struct{ channel Channel }

// Channel returnback tokento    (  use). 
func (t WriteToken) Channel() Channel { return t.channel }

type chatClient struct {
	endpoint string // finish endpoint(  path)
	key      string // onlystore atinstore,     /  
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
	req.Header.Set("Authorization", "Bearer "+c.key) //      char  
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
		//  back   bodysafety (       ), onlygivestatuscodeandbefore 200 charnode **    ** seg. 
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
	// ⚠️ ** type    **(Lead 2026-10-03   :  require deepseek-reasoner,    model=deepseek-flash). 
	//   ifonly "  require  type", model_id thenis   ⇒ ASR-MODEL-01  attribution  . 
	substituted := cr.Model != "" && !strings.EqualFold(cr.Model, c.model)
	if substituted {
		log.Printf("模型替换：请求 %s，响应 %s（model_substituted=true）", c.model, cr.Model)
	}
	out := Response{ModelID: cr.Model, RequestedModel: c.model, ActualModel: cr.Model,
		Substituted: substituted, FinishReason: "", TotalTokens: cr.Usage.TotalTokens,
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

// Registry is"   -> clientuserend" read-onlynote table. 
type Registry struct {
	cfg     Config
	clients map[Channel]*chatClient
	tokens  map[Channel]WriteToken
}

// NewRegistry verify  andonlyas **enabled**      clientuserend(fail-closed). 
// key from `<APIKeyEnv>`   change read;   then  ,   in default key. 
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
			continue //  startuse        clientuserend(alsothenno be call)
		}
		timeout := time.Duration(cc.TimeoutMS) * time.Millisecond
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		// ⚠️     **ResolveModel**(model_id  first,  thenget tier)--
		// read-only cc.ModelID    tier-only   sendout **model:""** ⇒ on  502(base   rootbecause). 
		model, err := r.cfg.ResolveModel(ch)
		if err != nil {
			return nil, fmt.Errorf("通道 %q 模型解析失败（fail-closed）: %w", ch, err)
		}
		r.clients[ch] = &chatClient{endpoint: endpoint, key: key, model: model, timeout: timeout, hc: &http.Client{Transport: modelcenterTransport()}}
	}
	r.tokens[ChannelLearn] = WriteToken{channel: ChannelLearn}
	return r, nil
}

// Enabled      is  use. 
func (r *Registry) Enabled(ch Channel) bool {
	_, ok := r.clients[ch]
	return ok
}

// Invoke  edrefer   sendraise  calluse.  startuse     reject(     todiff   ). 
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
	// L5:  typetgt by  asapprove(ifon back  same,    **  value**bythenattribution). 
	// ⚠️ **  use  valueoverwrite   type**( then model_id   is"  require ",  is"  occur  "). 
	//      model timeonlyback   value; hasthen  by**  authoritative**asapprove. 
	if resp.ActualModel == "" {
		resp.ActualModel = r.cfg.Channels[string(ch)].ModelID
	}
	if resp.RequestedModel == "" {
		resp.RequestedModel = resp.ActualModel
	}
	resp.ModelID = resp.ActualModel
	if resp.RequestedModel != "" && resp.ActualModel != "" &&
		!strings.EqualFold(resp.RequestedModel, resp.ActualModel) {
		resp.Substituted = true
	}
	return resp, nil
}

// WriteBackToken only  learn    enabled time send(L2: uniquewriteback  ). 
func (r *Registry) WriteBackToken(ch Channel) (WriteToken, bool) {
	if ch != ChannelLearn || !r.Enabled(ChannelLearn) {
		return WriteToken{}, false
	}
	t, ok := r.tokens[ChannelLearn]
	return t, ok
}

// modelcenterTransport   default Transport; onlycur form   VHS_INSECURE_TLS=1 time
//  ed TLS     (2026-10-04  close         LLM safety   occur ). 
func modelcenterTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if os.Getenv("VHS_INSECURE_TLS") == "1" {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec //  form occur , defaultclose 
	}
	return t
}
