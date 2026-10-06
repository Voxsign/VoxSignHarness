// l1.go -- L1 slow  clientuserend(deepseek-flash), **unique  objtgt**. 
//
//   triggersend onlyhas  (VHS-FASTSLOW-001 fix after): 
//
//	① JEV **  use**( time/  200/ form  --     ,  is" but   ")
//	② calluse voice **needneed    **
//
// `choice=ambiguous` **   list**:  is JEV giveout pos   "   " ⇒ clarificationuseuser. 
package route

import (
	"context"

	"voicesign-harness/modelcenter"
)

// L1Client use modelcenter   default   (deepseek-flash) nowslow  . 
type L1Client struct {
	Registry *modelcenter.Registry
	Channel  modelcenter.Channel
}

// Complete call  slow  . 
func (c *L1Client) Complete(ctx context.Context, prompt string) (string, error) {
	ch := c.Channel
	if ch == "" {
		ch = modelcenter.ChannelDefault
	}
	resp, err := c.Registry.Invoke(ctx, ch, prompt)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
