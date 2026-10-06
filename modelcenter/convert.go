// convert.go --  clientuserendand provider/  **    **(prevent    ). 
//
// this packageand provider/ is**  clientuserend**,  boundarye.g.under: 
//
//	provider/         harness  chain occurproducepath: heavy , response_format, params   , 
//	                  endpointrule ize(<base>/chat/completions). ** needasdiff line modify . **
//	modelcenter/          (default/diagnose/learn)   andcalluse: C1–C5 fail-closed, 
//	                  L2 writebacktoken  , **by  path**(/api/model/chat)origkindsendraise. 
//	                    inheavy / form, keepkeep  ,    . 
//
//  er**   same**(OpenAI compat: choices[].message.content / finish_reason / usage). 
// basefile provide to  ,  "    "changebecome**       data**: 
//     sidemodify  semantic, round-trip     change . 
package modelcenter

import (
	"voicesign-harness/contract"
	"voicesign-harness/provider"
)

// ToProviderResponse pipethis package   become provider    classtype. 
// Channel / ModelID is**   ** ity, provider    hasto charseg,   time  (see  ). 
func ToProviderResponse(r Response) provider.ChatResponse {
	return provider.ChatResponse{
		Content:      r.Content,
		FinishReason: r.FinishReason,
		Usage: contract.Usage{
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			TotalTokens:      r.TotalTokens,
		},
	}
}

// FromProviderResponse pipe provider    becomethis package  classtype. 
// Channel  empty, ModelID bycalluse by  backfill(L5: by  asapprove). 
func FromProviderResponse(r provider.ChatResponse) Response {
	return Response{
		Content:          r.Content,
		FinishReason:     r.FinishReason,
		PromptTokens:     r.Usage.PromptTokens,
		CompletionTokens: r.Usage.CompletionTokens,
		TotalTokens:      r.Usage.TotalTokens,
	}
}
