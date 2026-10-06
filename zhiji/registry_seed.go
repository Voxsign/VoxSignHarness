// registry_seed.go --    ·    per-model   note table kind numdata(M1 A5). 
//
// pipe model-hub/config/models.json  name    6   typeby ModelProfile    Register, 
//  as  routeby( keep  /    ) initstart in. 
//
// ⚠️ heavyneedvoice : basefilein has  numchar(   / LostInMiddle / InstructionFollow etc)
//    are **POC kind   value**,    open  and type     disconnect, ** is  baseapprove**. 
//      eval / bench   after  by  numdatawrite-back approve--  pipe    numcurauthoritativenumchar use. 
//
//     origthen(   §6.4:  keep  /    ): 
//   - conservative(      ,   risk   ):   type / basely type /    type /
//        chain type(reasoner needprotectmiddle  )  keep . 
//   - aggressive(    +   backpatch): onlygivecurbefore      type. 
package zhiji

import "errors"

// errNilRegistry  inemptynote tablesent . 
var errNilRegistry = errors.New("zhiji: SeedDefaultModels 的 Registry 不能为空")

// SeedDefaultModels pipe name  6   typenote  note table(alreadystore thenoverwritechangenew). 
//      Register   i.e. returnback,  continuecontinueafterface  type. 
func SeedDefaultModels(r *Registry) error {
	if r == nil {
		return errNilRegistry
	}

	profiles := []ModelProfile{
		// 1) gpt-4o-mini -- OpenAI     type, then fast,  form  but   inetc. 
		//     keep :  becomebase  type    ,   pipe    . 
		{
			ID:                "gpt-4o-mini",
			NominalWindow:     128000,
			EffectiveWindow:   128000, // POC   :  etcattgtcalled,    
			LostInMiddle:      0.35,   // POC   : toinseg    inetc  
			FormatPref:        "json",
			InstructionFollow: 0.78, // POC   : refer     
			PriceClass:        "cheap",
			Strengths:         []string{"cost-effective", "low-latency", "json-formatting"},
			Density:           DensityConservative,
		},
		// 2) gpt-4o -- curbefore   use  typeof , refer    ,  dependency  backpatch. 
		//       : unique aggressive,     after  vector_index backpatch node. 
		{
			ID:                "gpt-4o",
			NominalWindow:     128000,
			EffectiveWindow:   128000, // POC   :  etcattgtcalled,    
			LostInMiddle:      0.40,    // POC   
			FormatPref:        "json",
			InstructionFollow: 0.90, // POC   : refer    
			PriceClass:        "premium",
			Strengths:         []string{"strong-reasoning", "instruction-following", "vision"},
			Density:           DensityAggressive,
		},
		// 3) deepseek-chat -- DeepSeek  useto   type, ity   , in  ,    mid. 
		//    mid  keep handle:    e.g.  ,    risk  , firstkeep   eval. 
		{
			ID:                "deepseek-chat",
			NominalWindow:     128000,
			EffectiveWindow:   128000, // POC   :  etcattgtcalled,    
			LostInMiddle:      0.45,   // POC   : inseg    at  
			FormatPref:        "json",
			InstructionFollow: 0.75, // POC   
			PriceClass:        "mid",
			Strengths:         []string{"cost-effective", "chinese-strong"},
			Density:           DensityConservative,
		},
		// 4) deepseek-reasoner -- R1      chain   type. 
		//    close :   CoT middle   base thenis"needprotect   ",       disconnect  chain, 
		//    thusgive conservative protect,         . 
		{
			ID:                "deepseek-reasoner",
			NominalWindow:     128000,
			EffectiveWindow:   128000, // POC   :    chain   use     
			LostInMiddle:      0.50,   // POC   :  onunder underinseg     
			FormatPref:        "markdown",
			InstructionFollow: 0.80, // POC   
			PriceClass:        "mid",
			Strengths:         []string{"long-cot-reasoning", "deep-thinking"},
			Density:           DensityConservative,
		},
		// 5) local-qwen = qwen2.5:7b -- basely 7B   type,   /   but  haslimit. 
		//     keep : basely  type      ,   alsoby 32k tgtcalled. 
		{
			ID:                "local-qwen",
			NominalWindow:     32768,
			EffectiveWindow:   32768, // POC   : qwen2.5:7b tgtcalled 32k,    
			LostInMiddle:      0.55,  // POC   :   typeinseg  change  
			FormatPref:        "markdown",
			InstructionFollow: 0.60, // POC   :   typerefer     
			PriceClass:        "cheap",
			Strengths:         []string{"local-private", "low-latency", "no-egress"},
			Density:           DensityConservative,
		},
		// 6) jev-latest -- typesafe-jev close ize disconnectendpoint,   OpenAI  state. 
		//         ->   keep (  risk); NominalWindow give    safesafety  value, 
		//      toendpoint  onunder  numafter approve. 
		{
			ID:                "jev-latest",
			NominalWindow:     8192,  // POC   : endpoint    , givekeep  value,   approve
			EffectiveWindow:   8192,  // POC   : same NominalWindow
			LostInMiddle:      0.50,  // POC   :   firstbyinetc
			FormatPref:        "json",
			InstructionFollow: 0.70, // POC   : close izeendpoint out end 
			PriceClass:        "cheap",
			Strengths:         []string{"typesafe-structured-judgment", "deterministic-output"},
			Density:           DensityConservative,
		},
	}

	for _, p := range profiles {
		if err := r.Register(p); err != nil {
			return err
		}
	}
	return nil
}
