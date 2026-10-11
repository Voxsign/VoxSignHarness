// JSONL event-sourced FileStore: every state change is appended as one JSON
// record and fsynced before the in-memory map is updated. On Open the log is
// replayed, so a process restart (including a crash after the external call
// but before the status write) leaves the action visible as PENDING/UNKNOWN for
// the Reconciler instead of silently replaying it.
//
// The format is one Record object per line (the latest line for a key wins on
// replay). It stores correlation metadata only — never message bodies or keys.

package action

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// FileStore is a durable, concurrency-safe Store backed by an append-only
// JSONL file. It uses only the Go standard library.
type FileStore struct {
	mu   sync.Mutex
	path string
	rows map[recKey]*Record
}

// OpenFileStore opens (creating if needed) and replays a JSONL action log.
func OpenFileStore(path string) (*FileStore, error) {
	fs := &FileStore{path: path, rows: map[recKey]*Record{}}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fs, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev storeEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			// A torn final line from a crash is ignored; earlier committed lines
			// (each fsynced whole) remain authoritative.
			continue
		}
		cp := ev.Rec
		fs.rows[recKey{cp.Tenant, cp.Key}] = &cp
	}
	return fs, sc.Err()
}

type storeEvent struct {
	Rec Record `json:"rec"`
}

// persist appends and fsyncs one state record. On error the in-memory map is
// left untouched, so a failed durable write can never masquerade as success.
func (fs *FileStore) persist(r Record) error {
	f, err := os.OpenFile(fs.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	b, err := json.Marshal(storeEvent{Rec: r})
	if err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (fs *FileStore) snapshot(k recKey) Record {
	if r := fs.rows[k]; r != nil {
		return *r
	}
	return Record{}
}

// Acquire implements Store.
func (fs *FileStore) Acquire(sel Selector, owner string, lease time.Duration, now time.Time) (Record, AcquireDecision, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	k := recKey{sel.Tenant, sel.Key}
	out, dec := acquireLogic(fs.rows[k], sel, owner, lease, now)
	if dec == DecisionAcquired {
		if err := fs.persist(out); err != nil {
			return fs.snapshot(k), dec, err
		}
		cp := out
		fs.rows[k] = &cp
	}
	return out, dec, nil
}

// Mark implements Store.
func (fs *FileStore) Mark(tenant, key string, expectVersion int64, mutate func(*Record), now time.Time) (Record, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	k := recKey{tenant, key}
	r, ok := fs.rows[k]
	if !ok {
		return Record{}, ErrNotFound
	}
	out, err := markLogic(*r, expectVersion, mutate, now)
	if err != nil {
		return out, err
	}
	if err := fs.persist(out); err != nil {
		return *r, err
	}
	cp := out
	fs.rows[k] = &cp
	return out, nil
}

// Get implements Store.
func (fs *FileStore) Get(tenant, key string) (Record, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	r, ok := fs.rows[recKey{tenant, key}]
	if !ok {
		return Record{}, ErrNotFound
	}
	return *r, nil
}

// Reopen implements Store.
func (fs *FileStore) Reopen(tenant, key string, expectVersion int64, now time.Time) (Record, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	k := recKey{tenant, key}
	r, ok := fs.rows[k]
	if !ok {
		return Record{}, ErrNotFound
	}
	out, err := reopenLogic(*r, expectVersion, now)
	if err != nil {
		return out, err
	}
	if err := fs.persist(out); err != nil {
		return *r, err
	}
	cp := out
	fs.rows[k] = &cp
	return out, nil
}
