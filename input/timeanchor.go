package input

// timeanchor.go -- timetime ptresolve (SPEC v2    36  lybase ). 
//
// [pseudocode logic layer]
// module  : from baseinresolve  langtimetimetable ( day/ day/afterday/ day/beforeday/ X/under X)as bodydayperiod. 
//  in:  base;   timetime now(resolve moment, baselytime ). 
//  out: hint(orig table ), date(YYYY-MM-DD), ambiguous(is   clarification). 
// control flow: 
//   1.  to  :  day/ day/afterday/ day/beforeday(   ), get basein   in. 
//   2.  period pt(   formchar    ,    CJK charnode    ): 
//      to   periodname  : underunder X(+2 ) / under  periodX, under periodX, under X(+1 ) / base X,   X(+0) /  X(+0); 
//       inget"    , same  get form  ";  X andobjtgt at day ->   under  + ambiguous=true. 
//   3.  to   vs  period pt -> get  changebeforeer. 
// errorhandle: no in -> safetyempty; "under month"" id" resolve (SPEC v2    );  month/ yearby AddDate   ; 
//    periodname  :  periodday/ day/ periodday ->  day. 

import (
	"strings"
	"time"
)

var weekdayNames = map[string]time.Weekday{
	"周一":  time.Monday,
	"周二":  time.Tuesday,
	"周三":  time.Wednesday,
	"周四":  time.Thursday,
	"周五":  time.Friday,
	"周六":  time.Saturday,
	"周日":  time.Sunday,
	"星期天": time.Sunday,
	"周天":  time.Sunday,
	"星期日": time.Sunday,
}

// absoluteAnchors  to   pt(by first  list; first ini.e.returnback). 
type absoluteAnchor struct {
	hint string
	days int //  to now  daynum  
}

var absoluteAnchors = []absoluteAnchor{
	{"今天", 0},
	{"明天", 1},
	{"后天", 2},
	{"昨天", -1},
	{"前天", -2},
}

// weekdayPat is   period pt form(pat asfinish    ). 
type weekdayPat struct {
	pat  string
	off  int  //    : 0=base  1=under  2=underunder 
	bare bool //   X(nobefore fix )
}

// weekdayPatterns as periodname     form. 
// note :   use"under "+"  " connect(  to char"under   ")--
//  howevertable in" "char and("under  "= under+ + ), because by"dayafter "  ("under "+day -> under  ). 
func weekdayPatterns(name string) []weekdayPat {
	runes := []rune(name)
	day := string(runes[len(runes)-1])
	return []weekdayPat{
		{"下下周" + day, 2, false},
		{"下个星期" + day, 1, false},
		{"下星期" + day, 1, false},
		{"下周" + day, 1, false},
		{"本周" + day, 0, false},
		{"这周" + day, 0, false},
		{name, 0, true}, //   X(  /  /…/ day/ periodday/ day/ periodday)
	}
}

// ResolveTimeAnchor resolve  basein first timetime pt. 
// returnbackvalue: hint orig , date(YYYY-MM-DD, no inas ""), ambiguous(is   clarification). 
func ResolveTimeAnchor(text string, now time.Time) (hint, date string, ambiguous bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", false
	}

	// 1.  to  (   in)
	absIdx, absHint, absDays := -1, "", 0
	for _, a := range absoluteAnchors {
		if i := strings.Index(text, a.hint); i >= 0 && (absIdx < 0 || i < absIdx) {
			absIdx, absHint, absDays = i, a.hint, a.days
		}
	}

	// 2.  period pt(   form;     , same    form first)
	wdIdx, wdHint, wdOffset, wdBare, wdName := -1, "", 0, false, ""
	for name := range weekdayNames {
		for _, p := range weekdayPatterns(name) {
			if i := strings.Index(text, p.pat); i >= 0 {
				if wdIdx < 0 || i < wdIdx || (i == wdIdx && len(p.pat) > len(wdHint)) {
					wdIdx, wdHint, wdOffset, wdBare, wdName = i, p.pat, p.off, p.bare, name
				}
			}
		}
	}

	// 3. getchange beforeer
	switch {
	case wdIdx >= 0 && (absIdx < 0 || wdIdx < absIdx):
		d, _ := resolveWeekday(now, weekdayNames[wdName], wdOffset)
		//   izetocurday ptagain  (AddDate keep moment,  then"  "curday 10:00  triggersend  )
		dMid := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if wdBare {
			switch {
			case dMid.Before(today):
				d, ambiguous = d.AddDate(0, 0, 7), true // base  dayalreadyed ->   under  +   
			case dMid.Equal(today):
				ambiguous = true //  as day(e.g.   "  ")->  day vs under    , clarification  
			}
		}
		return wdHint, d.Format("2006-01-02"), ambiguous

	case absIdx >= 0:
		d := now.AddDate(0, 0, absDays)
		return absHint, d.Format("2006-01-02"), false
	}

	return "", "", false
}

// resolveWeekday returnbackby now    asbaseapprove,   weekOffset  (0=base ,1=under ,2=underunder )  wd dayperiod. 
func resolveWeekday(now time.Time, wd time.Weekday, weekOffset int) (time.Time, bool) {
	mon := mondayOf(now)
	target := mon.AddDate(0, 0, (int(wd)+6)%7+7*weekOffset) //   raisestart:   =0 …  day=6
	return target, false
}

// mondayOf returnback now       (  raisestart  ISO semantic). 
func mondayOf(now time.Time) time.Time {
	offset := (int(now.Weekday()) + 6) % 7 //   =0
	return now.AddDate(0, 0, -offset)
}
