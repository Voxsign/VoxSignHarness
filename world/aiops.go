// Package world -- out  boundary type: pipe AIOps  closeconnectbecome" dependency  /     /    " numdata . 
//
//  data ASR-EXT-005(  +refer ): `aiops.peterzou.com`   `/api/*` is**read-only**  API, 
// in  zone noneed key. this packageread-only,  write,  call type. 
//
//  needrequire(ASR-EXT-005 §3.2): 
//
//	A1 onlycache numdata,   safety   ; cache heavy    
//	A2   numdata  source(endpoint+ gettimetime)+ status
//	A3 read   fail-open andtgt unknown(    cur" has")
//	A4 readconnect  200   curbecomewrite limit   
//	A5   path -- needneednewendpointtimeread face  fetch(DiscoverEndpoints)
//	A6   out   (   base URL)
//
// basefileonly **classtypeand  **;  as  gateway_impl.go. 
package world

import (
	"net/http"
	"sync"
	"time"
)

//  state(to  VHS-PROJMODEL-001 F2). 
const (
	StatusOK       = "ok"
	StatusUnknown  = "unknown"
	StatusInferred = "inferred" //  close  ,      
)

// Source     numdata   and gettimetime(A2). 
type Source struct {
	Endpoint  string `json:"endpoint"`
	FetchedAt string `json:"fetched_at"`
}

// Host is    (AIOps `/api/summary`   hosts[*]). 
type Host struct {
	Key      string            `json:"key"` // map  (e.g. trelva)
	Hostname string            `json:"hostname"`
	Purpose  string            `json:"purpose"`
	Domain   string            `json:"domain"`
	Status   string            `json:"status"`
	CPU      float64           `json:"cpu"`
	Mem      float64           `json:"mem"`
	Disk     float64           `json:"disk"`
	DiskFree float64           `json:"disk_free"`
	Ports    []int             `json:"ports"`
	Services int               `json:"services"`
	Env      string            `json:"env"`
	Role     string            `json:"role"`
	Region   map[string]string `json:"region,omitempty"`
	Network  map[string]string `json:"network,omitempty"`
}

// Summary is `/api/summary`  ** numdatafast **( keepstoreorigstart  body, A1). 
type Summary struct {
	Status string `json:"status"` // ok | unknown
	Source Source `json:"source"`
	Zone   string `json:"zone,omitempty"`
	TS     string `json:"ts,omitempty"`
	Hosts  []Host `json:"hosts"`
	// UnparsedHosts   **  no resolve **    key:     keep as unknown, 
	//       (A3: read    cur" has"). 
	UnparsedHosts []string `json:"unparsed_hosts,omitempty"`
	Note          string   `json:"note,omitempty"` //   origbecause(A3:     )
}

// Ledger is CI/CD      . 
type Ledger struct {
	TS     string `json:"ts"`
	Tag    string `json:"tag"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

// CICD is `/api/cicd/status`   numdatafast . 
type CICD struct {
	Status     string   `json:"status"`
	Source     Source   `json:"source"`
	CurrentTag string   `json:"current_tag"`
	Ledger     []Ledger `json:"ledger"`
	Note       string   `json:"note,omitempty"`
}

// Dependency is" dependency "   (  connectandin obj type dependencies). 
type Dependency struct {
	On     string `json:"on"`
	Kind   string `json:"kind"` // host | gateway | service(**  **is capability/grant, A4)
	For    string `json:"for"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
	Source Source `json:"source"`
}

// Boundary is"   to  "   . 
type Boundary struct {
	ID     string `json:"id"`
	Claim  string `json:"claim"`
	Source string `json:"source"`
	Status string `json:"status"`
}

// Service isbaselyserveservicenote table    (  numdata ;   time fail-open unknown). 
type Service struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Entry   string   `json:"entry"`
	Status  string   `json:"status"`
	Desc    string   `json:"desc"`
	Type    string   `json:"type"`
}

// cache onlystore** numdata**(A1); ClearCache   empty,  heavy . 
type cache struct {
	summary  *Summary
	cicd     *CICD
	services []Service
}

// Gateway is AIOps  close read-onlyclientuserend. 
type Gateway struct {
	BaseURL string
	Client  *http.Client
	now     func() time.Time
	lock    sync.Mutex
	cache   cache
}

// Option call clientuserend(  noteinuse;     A6    ly  end). 
type Option func(*Gateway)

// WithHTTPClient notein HTTP clientuserend(  use httptest). 
func WithHTTPClient(c *http.Client) Option { return func(g *Gateway) { g.Client = c } }

// WithClock noteintimeclock(  gettimetime  now). 
func WithClock(fn func() time.Time) Option { return func(g *Gateway) { g.now = fn } }

// NewGateway   clientuserend. baseURL is**unique**out   (A6). 
func NewGateway(baseURL string, opts ...Option) *Gateway {
	g := &Gateway{BaseURL: baseURL, now: time.Now}
	for _, o := range opts {
		o(g)
	}
	return g
}
