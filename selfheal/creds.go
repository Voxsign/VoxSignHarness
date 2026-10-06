package selfheal

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"voicesign-harness/config"
)

// modelCenterCredsFile is typein   file(~/.modelcenter/creds.txt). 
// file statecompat kind:  valueto "MAIN_KEY=sk-mc-xxx" or   "sk-mc-xxx"; 
//     ,  ed # note  , getfirst  sk-mc- openhead  token. 
//
// safesafety line: key    in code   / day  / commit / error  . base numonlyreturnbackchar  give
// provider       Authorization head,       . 
const modelCenterCredsFile = ".modelcenter/creds.txt"

// ReadModelCenterKey from ~/.modelcenter/creds.txt resolve  typein  key. 
// file store  / no sk-mc- token -> returnback ""(calluse data   disconnect  ed,    ). 
func ReadModelCenterKey() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	f, err := os.Open(filepath.Join(home, modelCenterCredsFile))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024), 64*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		//  valueto state: get '='  side. 
		if _, rhs, ok := strings.Cut(line, "="); ok {
			line = strings.TrimSpace(rhs)
		}
		//  in  has id/empty , getfirst  sk-mc- openhead token. 
		for _, tok := range strings.Fields(line) {
			tok = strings.Trim(tok, `"' `)
			if strings.HasPrefix(tok, "sk-mc-") {
				return tok
			}
		}
	}
	return ""
}

// PrepareDiagKey   provider.NewRegistry ofbeforecalluse, tovoice   diag provider     : 
//  1.  time  :   form   TimeoutMs timedefault 30000ms(   etc Global.LLMTimeoutMs=60s); 
//      time->   returnbackerror-> disconnect    ed,   disconnect chain. 
//  2.   : if  form   api_key, from typein   filepatch . 
//
// already form   api_key   diag provider[ firstuse ],  beoverwrite; 
//      -> diag keepkeepempty key(aftercontinue Chat 401 ->  disconnect    ed). 
//  voice  diag provider ->    ( disconnect  body ed,  chain change). 
func PrepareDiagKey(cfg *config.Config) {
	if cfg == nil {
		return
	}
	p, ok := cfg.DiagProvider()
	if !ok {
		return
	}
	// ① diag default 30s  time  (  at key   ;  form   overwrite). 
	if p.TimeoutMs <= 0 {
		for i := range cfg.Providers {
			if cfg.Providers[i].Name == p.Name {
				cfg.Providers[i].TimeoutMs = DiagDefaultTimeoutMs
				break
			}
		}
	}
	// ②   patch ( form api_key  first). 
	if strings.TrimSpace(p.APIKey) != "" {
		return
	}
	if key := ReadModelCenterKey(); key != "" {
		for i := range cfg.Providers {
			if cfg.Providers[i].Name == p.Name {
				cfg.Providers[i].APIKey = key
				break
			}
		}
	}
}

// DiagDefaultTimeoutMs is diag provider   form   TimeoutMs time default time(30s). 
const DiagDefaultTimeoutMs = 30000
