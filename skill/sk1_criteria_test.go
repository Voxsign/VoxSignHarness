// sk1_criteria_test.go -- SK-1"list     "** to** data(prevent"emptyed":   Lead   R3   ). 
package skill

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fakeSkillService(t *testing.T, calls *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		switch r.URL.Path {
		case "/api/skill/skills":
			_ = json.NewEncoder(w).Encode(map[string]any{"skills": []map[string]any{
				{"id": "arch-guardian", "version": "v0.1.0", "state": "active"},
				{"id": "arch-review", "version": "v0.1.0", "state": "active"},
			}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": filepath.Base(r.URL.Path), "knowhow": map[string]any{}})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ① inizeafterlist   ⇒ ** endcallusenum = 0**(andfirst keepinizefilestore and empty --  thenthenisemptyed)
func TestSK1ListAfterInternalizeMakesZeroRemoteCalls(t *testing.T) {
	calls := 0
	srv := fakeSkillService(t, &calls)
	st := NewStore(t.TempDir(), time.Hour)
	f := &Fetcher{BaseURL: srv.URL, Calls: &calls}

	n, err := Internalize(context.Background(), f, st, DefaultWhitelist)
	if err != nil || n == 0 {
		t.Fatalf("[SK-1] 内化失败或为空: n=%d err=%v", n, err)
	}
	// **before : inizefilestore and empty**(preventemptyed)
	b, err := os.ReadFile(filepath.Join(st.Dir, "index.json"))
	if err != nil || len(b) == 0 {
		t.Fatalf("[SK-1] 内化文件不存在/为空 ⇒ 后面的断言会空过: %v", err)
	}
	before := calls
	got, status, err := ListOffline(context.Background(), st, f)
	if err != nil || status != StatusOK || len(got) == 0 {
		t.Fatalf("[SK-1] 离线列表失败: %v %s %d", err, status, len(got))
	}
	if calls != before {
		t.Errorf("[SK-1] 已内化却仍联网: %d → %d", before, calls)
	}
}

// ② **revexample**:   inizefile ⇒ **    **( then   rootbase   )
func TestSK1WithoutInternalizeMustFetch(t *testing.T) {
	calls := 0
	srv := fakeSkillService(t, &calls)
	st := NewStore(t.TempDir(), time.Hour) // emptyobj  ⇒ noinize
	f := &Fetcher{BaseURL: srv.URL, Calls: &calls}
	got, _, err := ListOffline(context.Background(), st, f)
	if err != nil {
		t.Fatalf("[SK-1 反例] 无内化时应回退拉取: %v", err)
	}
	if len(got) == 0 {
		t.Errorf("[SK-1 反例] 拉取未返回技能")
	}
	if calls == 0 {
		t.Errorf("[SK-1 反例] 无内化却一次都没联网 ⇒ 判据是空过")
	}
}
