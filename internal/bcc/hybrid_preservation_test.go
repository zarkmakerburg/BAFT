package bcc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type hybridNativePageConfig struct {
	Mode           string `json:"mode"`
	CSRF           string `json:"csrf"`
	CSRFHeader     string `json:"csrfHeader"`
	OperationsHTML string `json:"operationsHTML"`
	OperationsJS   string `json:"operationsJS"`
}

// Decode the real per-request page configuration, not a source-code substring.
func hybridPageConfig(t *testing.T, page string) hybridNativePageConfig {
	t.Helper()
	var config hybridNativePageConfig
	_, tail, ok := strings.Cut(page, "<script>window.BAFT_UI=")
	if !ok {
		t.Fatal("native page configuration missing")
	}
	raw, _, ok := strings.Cut(tail, ";</script>")
	if !ok {
		t.Fatal("native page configuration delimiter missing")
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatalf("native page configuration JSON: %v", err)
	}
	return config
}

func TestHybridPreservesNativeOperationsAndSharedSession(t *testing.T) {
	r := newAccessRig(t)
	cookie, csrf := r.session()
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(cookie)
		return r.do(req)
	}
	page := get(r.base())
	if page.Code != http.StatusOK || page.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("default authenticated page or no-store policy missing")
	}
	config := hybridPageConfig(t, page.Body.String())
	if config.Mode != "dashboard" || config.CSRF != csrf || config.CSRFHeader != csrfHeader {
		t.Fatal("default page lost native session/CSRF configuration")
	}
	for _, id := range []string{"kNodes", "kUp", "kDown", "kUnknown", "historyCanvas", "tnPlan", "chRows", "rdResult", "pmMatrix", "jobs"} {
		if !strings.Contains(config.OperationsHTML, `id="`+id+`"`) {
			t.Errorf("default BCC lost operational control %s", id)
		}
	}
	for _, name := range []string{"loadNodes", "loadTunnels", "loadHistory", "loadAll"} {
		if !strings.Contains(config.OperationsJS, "function "+name+"(") {
			t.Errorf("default BCC lost operational loader %s", name)
		}
	}
	classic := get(r.base() + "classic/")
	if classic.Code != http.StatusOK || !strings.Contains(classic.Body.String(), `C="`+csrf+`"`) {
		t.Fatal("classic fallback lost the shared session/CSRF token")
	}
	for _, name := range []string{"bcc-app.css", "bcc-app.js", "bcc-map.js", "bcc-operations-v3.js", "bcc-earth-v3.js", "bcc-welcome-globe-v3.js", "assets/world-land.js"} {
		asset := get(r.base() + "v3/" + name)
		if asset.Code != http.StatusOK || asset.Body.Len() == 0 || strings.Contains(asset.Body.String(), csrf) {
			t.Errorf("asset missing or contains per-session secret: %s", name)
		}
	}
	// Invalid JSON exercises authorization without creating any tunnel or job.
	for _, value := range []string{"", "wrong", csrf} {
		req := httptest.NewRequest(http.MethodPost, r.base()+"api/tunnels/plan", strings.NewReader("{"))
		req.AddCookie(cookie)
		if value != "" {
			req.Header.Set(csrfHeader, value)
		}
		want := http.StatusUnauthorized
		if value == csrf {
			want = http.StatusBadRequest
		}
		if response := r.do(req); response.Code != want {
			t.Errorf("shared-session plan authorization: status %d, want %d", response.Code, want)
		}
	}
}

func TestHybridDoesNotExposeOperationsAfterLogout(t *testing.T) {
	r := newAccessRig(t)
	cookie, _ := r.session()
	req := httptest.NewRequest(http.MethodPost, r.base()+"logout", nil)
	req.AddCookie(cookie)
	if response := r.do(req); response.Code != http.StatusSeeOther {
		t.Fatal("logout failed")
	}
	req = httptest.NewRequest(http.MethodGet, r.base(), nil)
	req.AddCookie(cookie)
	page := r.do(req)
	config := hybridPageConfig(t, page.Body.String())
	if page.Code != http.StatusOK || config.Mode != "welcome" || config.CSRF != "" || config.OperationsHTML != "" || config.OperationsJS != "" {
		t.Fatal("logged-out page exposed native operations or CSRF")
	}
	req = httptest.NewRequest(http.MethodGet, r.base()+"classic/", nil)
	req.AddCookie(cookie)
	if classic := r.do(req); classic.Code != http.StatusOK || strings.Contains(classic.Body.String(), `id="tnPlan"`) {
		t.Fatal("logged-out classic fallback exposed operational controls")
	}
}
