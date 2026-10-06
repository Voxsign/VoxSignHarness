package input

import (
	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// Correcter iscorrection  connect . memory.Dictionary dayhowever nowbaseconnect ; 
//    use stub notein,    input  revtodependency memory(keepkeepmodule boundary). 
type Correcter interface {
	Correct(text string) (string, []contract.Correction)
}

// Result ismanageline  handle artifact,       middleclose bythentraceback   (   §5.1). 
type Result struct {
	Raw         string                `json:"raw"`         // ①ASR orig , no  keep 
	Cleaned     string                `json:"cleaned"`     // ②cleanafter
	Corrected   string                `json:"corrected"`   // ③word correctionafter
	Corrections []contract.Correction `json:"corrections"` // ③correction  
	Intent      contract.Intent       `json:"intent"`      // ④intent JSON(CorrectedText already fill; low-confidencetime Ask  empty)
}

// Pipeline  raise clean->correction->intent     manageline. 
type Pipeline struct {
	Cleaner       *Cleaner
	Correcter     Correcter
	Classifier    *Classifier
	LowConfAction string // ask | model; M1 only now ask semantic
}

// NewPipeline from    manageline: 
//   - LowConfAction get cfg.Input.LowConfAction(ask|model). 
//     note : M1 only now ask semantic(low-confidence connectclarification,    ); 
//     model   orchestrate aftercontinue   (by agent  fast typemodifywrite),  placeonly     . 
func NewPipeline(cfg *config.Config, correcter Correcter) *Pipeline {
	c := &config.Config{}
	if cfg != nil {
		c = cfg
	}
	return &Pipeline{
		Cleaner:       NewCleaner(c.Input.Fillers),
		Correcter:     correcter,
		Classifier:    NewClassifier(c.Input.IntentConf),
		LowConfAction: c.Input.LowConfAction,
	}
}

// Process  finish manageline: ①raw origkindkeep  ②clean ③correct ④classify(corrected). 
func (p *Pipeline) Process(raw string) (Result, error) {
	res := Result{Raw: raw}

	// ② clean
	res.Cleaned = p.Cleaner.Clean(raw)

	// ③ correction(Correcter  empty: noword time ed)
	res.Corrected = res.Cleaned
	if p.Correcter != nil {
		corrected, corr := p.Correcter.Correct(res.Cleaned)
		res.Corrected = corrected
		res.Corrections = corr
	}

	// ④ intentclassify(baseatcorrectionafter base)
	res.Intent = p.Classifier.Classify(res.Corrected)
	res.Intent.Corrections = res.Corrections

	return res, nil
}
