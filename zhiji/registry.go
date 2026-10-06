// registry.go --    ·    per-model   note table(   v1.0 §6.4 / §08 model-registry  ly). 
//
// by type restrict  androuteby numdatabase (M1 before   ,  dependency GPU/ close): 
//   -   type -> keep   (      )
//   -   type ->     (dependency  backpatch)
// keep izeto model_registry.json(JSON file, and Store sameobj   ). 
package zhiji

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// CompressionDensity       (   §6.4:  keep  /    ). 
type CompressionDensity string

const (
	DensityConservative CompressionDensity = "conservative" //   type:       
	DensityAggressive   CompressionDensity = "aggressive"   //   type: dependency  backpatch
)

// ModelProfile   type    (   §6.4 charseg). 
type ModelProfile struct {
	ID                string             `json:"id"`
	NominalWindow     int                `json:"nominal_window"`      // tgtcalledonunder   (token)
	EffectiveWindow   int                `json:"effective_window"`    //   has   (token)
	LostInMiddle      float64            `json:"lost_in_middle"`      // Lost-in-middle     0–1
	FormatPref        string             `json:"format_pref"`         //  form  (json|markdown|xml…)
	InstructionFollow float64            `json:"instruction_follow"`  // refer      0–1
	PriceClass        string             `json:"price_class"`         //    (cheap|mid|premium)
	Strengths         []string           `json:"strengths"`           //   tgt 
	Density           CompressionDensity `json:"density"`             //       
}

// Registry per-model   note table(line safesafety, JSON keep ize). 
type Registry struct {
	path      string
	mu        sync.RWMutex
	profiles  map[string]ModelProfile
	defaultID string
}

// NewRegistry   /  note table(dir  store     ). 
func NewRegistry(dir string) (*Registry, error) {
	if dir == "" {
		return nil, errors.New("zhiji: 注册表目录不能为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	r := &Registry{path: filepath.Join(dir, "model_registry.json"), profiles: map[string]ModelProfile{}}
	b, err := os.ReadFile(r.path)
	if err == nil {
		if err := json.Unmarshal(b, &r.profiles); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return r, nil
}

// Register note /changenew type  . 
func (r *Registry) Register(p ModelProfile) error {
	if p.ID == "" {
		return errors.New("zhiji: 模型 ID 不能为空")
	}
	if p.Density == "" {
		p.Density = DensityConservative //    typedefaultkeep   (  risk   )
	}
	r.mu.Lock()
	r.profiles[p.ID] = p
	if r.defaultID == "" {
		r.defaultID = p.ID
	}
	r.mu.Unlock()
	return r.save()
}

// SetDefault   default type(routeby bot). 
// note : save() in  get RLock,  place  first Unlock again save--
// RWMutex   heavyin, keepwrite againread i.e.   (and Register()  first Unlock after save same ). 
func (r *Registry) SetDefault(id string) error {
	r.mu.Lock()
	if _, ok := r.profiles[id]; !ok {
		r.mu.Unlock()
		return ErrNotFound
	}
	r.defaultID = id
	r.mu.Unlock()
	return r.save()
}

// Get get  type  . 
func (r *Registry) Get(id string) (ModelProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[id]
	if !ok {
		return ModelProfile{}, ErrNotFound
	}
	return p, nil
}

// Default default type(routeby bot). 
func (r *Registry) Default() (ModelProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defaultID == "" {
		return ModelProfile{}, ErrNotFound
	}
	return r.profiles[r.defaultID], nil
}

// List safety   (by ID   ,    out). 
func (r *Registry) List() []ModelProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelProfile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// CompressionPolicy       (   §6.4: by type restrict    in). 
func (r *Registry) CompressionPolicy(id string) (CompressionDensity, error) {
	p, err := r.Get(id)
	if err != nil {
		return "", err
	}
	return p.Density, nil
}

func (r *Registry) save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, err := json.MarshalIndent(r.profiles, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}
