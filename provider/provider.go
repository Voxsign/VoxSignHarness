// Package provider is typeconnectin (   §6 / §14.2): 
// toon (agent)      Provider connect , toin   OpenAI compat endpointclientuserendandin  mock. 
// this packageonlydependency contract / config    ,  out dependency(onlytgtapprove ). 
//
//    form: newaddendpoint = config providers table   ; newadd kind =   NewRegistry in nowbaseconnect andnote . 
package provider

import (
	"context"
	"fmt"

	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// Provider is     typeendpoint(   §6.3).  nowneedkeep : 
//   -   today /error  in   API key; 
//   - ctx canceltimereturn immediately ctx.Err(). 
type Provider interface {
	// Name returnbacknote name(and config providers tablein  name   ). 
	Name() string
	// Chat send   OpenAI compat daypatchsafety require. MaxTokens as 0 time send  charseg. 
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// ChatRequest is   typecalluse in . 
type ChatRequest struct {
	Messages  []contract.Message // finish     (system/user/assistant)
	MaxTokens int                // 0 =    requirebodyin   max_tokens
	// ResponseFormat overwrite provider   response_format   : 
	//   true  = base calluse restrict json_object; false = base calluse   response_format; 
	//   nil   = use provider   (EffectiveResponseFormat). 
	// useway: QUERY  howeverlanglanganswer need  base, prompt alreadyforbid JSON,   close  json_object, 
	//  then type   refer under outno   JSON  (e.g. {"x":0})->     (M7    2026-10-03). 
	ResponseFormat *bool
}

// ChatResponse is typecalluse out (andon   resolve      ). 
type ChatResponse struct {
	Content      string         // choices[0].message.content( typeorigstart base,   is ActionPlan JSON)
	FinishReason string         // choices[0].finish_reason
	Usage        contract.Usage // token use 
}

// Registry is provider name ->  example  note table. by config   ity  ,   periodread-only. 
type Registry struct {
	providers map[string]Provider
	mockSet   map[string]bool // mock name  , provide IsMock   
	order     []string        // note   , keep  Names()   ity out
}

// NewRegistry      config invoice   provider: 
// kind=mock -> in  line mock; kind=openai -> OpenAI compatclientuserend; 
//    kind     prevent   (pos case under config.validate alreadyblock). 
func NewRegistry(cfg *config.Config) (*Registry, error) {
	r := &Registry{
		providers: map[string]Provider{},
		mockSet:   map[string]bool{},
	}
	for _, p := range cfg.Providers {
		var prov Provider
		switch p.Kind {
		case config.MockKind:
			prov = &mockProvider{name: p.Name}
			r.mockSet[p.Name] = true
		case config.OpenAIKind:
			prov = newOpenAIClient(p, cfg)
		default:
			//   prevent : config.validate alreadyreject   kind,    botpreventstopbe ed. 
			return nil, fmt.Errorf("provider %q: unsupported kind %q (only %s/%s)",
				p.Name, p.Kind, config.OpenAIKind, config.MockKind)
		}
		r.providers[p.Name] = prov
		r.order = append(r.order, p.Name)
	}
	return r, nil
}

// Get bynameget provider;  note  namecharreturnback error. 
func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider not registered: %q", name)
	}
	return p, nil
}

// Names bynote   returnbacksafety  provider name(  ity, thenattrace/call  out). 
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// IsMock     provider is asin  line mock( send   require, no key i.e.     ). 
func (r *Registry) IsMock(name string) bool {
	return r.mockSet[name]
}

// mockProvider in  lineendpoint:  send     require, returnback      ActionPlan JSON, 
// providebasely  opensend/ showand     use. unique    case is ctx becancel. 
type mockProvider struct {
	name string
}

// mockContent is mock   returnback in (   ActionPlan,   connectbe contract.ParseActionPlan resolve ). 
const mockContent = `{"actions":[{"tool":"get_time"}],"final":"（本地闭环测试）已获取系统时间"}`

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	select {
	case <-ctx.Done():
		return ChatResponse{}, ctx.Err()
	default:
	}
	return ChatResponse{
		Content:      mockContent,
		FinishReason: "stop",
		Usage:        contract.Usage{}, // basely mock, use safety 0
	}, nil
}
