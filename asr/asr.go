// Package asr is voice-sign harness   **ASR  ityize diff **. 
//
//   (owner 2026-10-03     after  ): 
//   - voice-sign harness is** body**; 
//   - this packageis harness  **in   **,  code at harness,      ; 
//   - dependency to** to**: harness -> asr, this package**  import** harness   its  . 
//
// curbeforestage: **P1( datafirstat now)**. basefileonlydefine**connect andclasstype**, 
//  asbyaftercontinuestage now;  see harness_test.go(now  curis  ). 
//
//     : docs/vhs-asr-  -v2.md
package asr

import "time"

// Engine is ASR  ityize diff uniquein . 
//
//   needpt: **Correct isnostatus  num** --   ,  andsend,  back ,   process. 
//  ityizestatusby Observe changenew, by Lexicon    . **statusand  split . **
type Engine interface {
	// Correct  pos   ASR origstart base. no  use. 
	Correct(req CorrectRequest) CorrectResult
	// Observe back   rev (useuserconnectaccept/modifyback), useat line  .  has  use. 
	Observe(fb Feedback) error
	// Lexicon  outcurbefore ityizestatus, provide  . 
	Lexicon(domain string) Lexicon
}

// CorrectRequest is   pos require. 
type CorrectRequest struct {
	Raw     string   // ASR origstart out. **   bot**,   becauseas"need  "but  
	Context []string //  periodonunder (same segto  before sent),       
	Domain  string   // curbeforedomain,    word first 
}

// CorrectResult is   posclose . 
type CorrectResult struct {
	Text        string        //  posafter base. **only  pos correction, tgtpt   in**(see Punctuated)
	Corrections []Correction  // to Text    placechange, all  data( asempty = Text  place modify)
	Candidates  []Candidate   //    connect time   ( asemptytableshow"   /  out ")
	Latency     time.Duration // base  pos time

	// Punctuated is**tgtpt  after**  base(needrequire 4.3 /  data C1 v2). 
	//
	//    (C1 v2  safesafetyside): 
	//   - **tgtpt   in Text** -- Text only pos correction, Punctuated only tgtpt  ; 
	//   -  er  tgtptafter  ** char etc**; keep class objalso and Raw  tgtptafter char etc; 
	//   -     tgtpt  time, Punctuated == Text. 
	Punctuated string

	// PunctuationCorrections istgtpt       (Kind  as "punctuation"). 
	//
	// as  and Corrections splitopen: C4  safesafety changeformis"Text  modifythen  has Correction   "
	// (prevent empty  ). tgtpt   modify Text, if   Corrections   connect  C4. 
	//
	//   is** in**semantic: Start == End ==  inpt  Text in charnodeundertgt, 
	// From == "", To ==  in tgtpt; Confidence and Evidence    empty. 
	PunctuationCorrections []Correction
}

// Correction is place body pos -- **   back toorig  time**. 
type Correction struct {
	Start      int     //   Raw in raisestart**charnode**undertgt
	End        int     // closeend**charnode**undertgt(   open)
	From       string  // orig  seg
	To         string  //  posafter seg
	Kind       string  // filler | homophone | hotword | dictionary | truncation | punctuation
	Confidence float64 // 0..1
	Evidence   string  // as     ( in  rule/  word ), provideattribution
}

// Candidate is     pos. 
type Candidate struct {
	Text       string
	Confidence float64
	Reason     string
}

// Feedback is  rev , useat line  . 
type Feedback struct {
	Raw       string
	Corrected string
	Accepted  bool   // useuseris connectaccept
	Source    string // user_edit | implicit | reviewer
}

// Lexicon is ityizestatus    fast . 
type Lexicon struct {
	Domain   string
	Hotwords []Hotword
	Version  string
}

// Hotword is   word/  word . 
type Hotword struct {
	Term    string
	Kind    string // person | project | term | command
	Weight  float64
	SeenCnt int
}

// ---------------------------------------------------------------------------
// baseline now: Passthrough(origkindreturnback,   pos)
//
//  sametimeis kind  : 
//  1. **     pos now**( is   )--      underboundary; 
//  2. **  to "changebefore"   ** --     onface,   ly thenis   . 
//
//  data C1(       )   on cur**dayhowever ed**--
//  posis torevexample "      "   . 
// ---------------------------------------------------------------------------

// Passthrough      pos. 
type Passthrough struct{}

// Correct origkindreturnback,  produceoccur   Correction. 
func (Passthrough) Correct(req CorrectRequest) CorrectResult {
	return CorrectResult{Text: req.Raw}
}

// Observe baseline   . 
func (Passthrough) Observe(Feedback) error { return nil }

// Lexicon baseline has ityizestatus. 
func (Passthrough) Lexicon(string) Lexicon { return Lexicon{} }

//   periodconfirmbaselinefull connect . 
var _ Engine = Passthrough{}
