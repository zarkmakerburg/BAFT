package bcc

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCommandCenterV3NativeSession(t *testing.T) {
	r := newAccessRig(t)
	for _, p := range []string{"/v3/", "/v3/assets/baft-brand.png"} {
		if rr := r.do(httptest.NewRequest("GET", p, nil)); rr.Code != 404 {
			t.Fatalf("public route exposed: %s %d", p, rr.Code)
		}
	}
	rr := r.do(httptest.NewRequest("GET", r.base(), nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"mode":"welcome"`) {
		t.Fatal("welcome mode missing")
	}
	if !strings.Contains(rr.Body.String(), `src="`+r.base()+`v3/bcc-app.js"`) || strings.Contains(rr.Body.String(), `="./`) {
		t.Fatal("page assets are not served under v3/")
	}
	if old := r.do(httptest.NewRequest("GET", r.base()+"v3/", nil)); old.Code != 303 || old.Header().Get("Location") != r.base() {
		t.Fatalf("old v3/ bookmark is not sent to the Command Center: %d", old.Code)
	}
	if tpl := r.do(httptest.NewRequest("GET", r.base()+"v3/bcc-app.html", nil)); tpl.Code != 404 {
		t.Fatal("page template served without its configuration")
	}
	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("security headers missing")
	}
	if asset := r.do(httptest.NewRequest("GET", r.base()+"v3/assets/baft-brand.png", nil)); asset.Code != 200 || asset.Header().Get("Content-Type") != "image/png" {
		t.Fatal("brand asset missing")
	}
	if source := r.do(httptest.NewRequest("GET", r.base()+"v3/assets.go", nil)); source.Code != 404 {
		t.Fatal("source exposed")
	}
	if post := r.do(httptest.NewRequest("POST", r.base()+"v3/bcc-app.js", nil)); post.Code != 405 {
		t.Fatal("asset permits mutation")
	}
	form := url.Values{"username": {r.creds.Username}, "password": {"wrong"}}
	req := httptest.NewRequest("POST", r.base()+"login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	bad := r.do(req)
	if bad.Code != 401 || !strings.Contains(bad.Body.String(), `"mode":"signin"`) {
		t.Fatal("native failed-login page missing")
	}
	form.Set("password", r.creds.Password)
	req = httptest.NewRequest("POST", r.base()+"login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	good := r.do(req)
	if good.Code != 303 || good.Header().Get("Location") != r.base() {
		t.Fatal("login redirect missing")
	}
	var cookie *http.Cookie
	for _, c := range good.Result().Cookies() {
		if c.Name == sessionCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("native session missing")
	}
	req = httptest.NewRequest("GET", r.base(), nil)
	req.AddCookie(cookie)
	page := r.do(req)
	if !strings.Contains(page.Body.String(), `"mode":"dashboard"`) {
		t.Fatal("authenticated mode missing")
	}
	// The previous dashboard stays reachable with the same session.
	req = httptest.NewRequest("GET", r.base()+"classic/", nil)
	req.AddCookie(cookie)
	if classic := r.do(req).Body.String(); !strings.Contains(classic, "Recent Jobs") || !strings.Contains(classic, `C="`) {
		t.Fatal("classic dashboard missing")
	}
	req = httptest.NewRequest("GET", r.base()+"api/overview", nil)
	req.AddCookie(cookie)
	if r.do(req).Code != 200 {
		t.Fatal("session overview GET failed")
	}
	req = httptest.NewRequest("GET", r.base()+"api/alerts", nil)
	req.AddCookie(cookie)
	if r.do(req).Code != 200 {
		t.Fatal("session alerts GET failed")
	}
	req = httptest.NewRequest("GET", r.base()+"api/nodes", nil)
	req.AddCookie(cookie)
	if r.do(req).Code != 200 {
		t.Fatal("session API failed")
	}
	req = httptest.NewRequest("POST", r.base()+"api/nodes", strings.NewReader("{}"))
	req.AddCookie(cookie)
	if r.do(req).Code != 401 {
		t.Fatal("write without CSRF was not rejected")
	}
	req = httptest.NewRequest("POST", r.base()+"logout", nil)
	req.AddCookie(cookie)
	if r.do(req).Code != 303 {
		t.Fatal("logout failed")
	}
	req = httptest.NewRequest("GET", r.base()+"api/nodes", nil)
	req.AddCookie(cookie)
	if r.do(req).Code != 401 {
		t.Fatal("logout left session active")
	}
}

func TestCommandCenterV3LoginFailureStates(t *testing.T) {
	r := newAccessRig(t)
	req := httptest.NewRequest("POST", r.base()+"login", strings.NewReader("password="+strings.Repeat("x", 5000)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := r.do(req)
	if rr.Code != 400 || !strings.Contains(rr.Body.String(), `"mode":"signin"`) || !strings.Contains(rr.Body.String(), `"error":"bad request"`) {
		t.Fatal("oversized request lost native error state")
	}
	for i := 0; i < 20; i++ {
		r.app.loginLim.failed(r.creds.Username, r.clock)
	}
	req = httptest.NewRequest("POST", r.base()+"login", strings.NewReader("username="+url.QueryEscape(r.creds.Username)+"&password=wrong"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = r.do(req)
	if rr.Code != 429 || rr.Header().Get("Retry-After") == "" || !strings.Contains(rr.Body.String(), `"mode":"signin"`) {
		t.Fatal("limited login lost native error state")
	}
}

func TestClassicDashboardKeepsItsOwnSignIn(t *testing.T) {
	r := newAccessRig(t)
	rr := r.do(httptest.NewRequest("GET", r.base()+"classic/", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `href="`+r.base()+`login?ui=classic"`) || strings.Contains(rr.Body.String(), "Recent Jobs") {
		t.Fatalf("classic welcome: %d", rr.Code)
	}
	form := url.Values{"username": {r.creds.Username}, "password": {r.creds.Password}}
	req := httptest.NewRequest("POST", r.base()+"login?ui=classic", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if good := r.do(req); good.Code != 303 || good.Header().Get("Location") != r.base()+"classic/" {
		t.Fatalf("classic login redirect: %d %s", good.Code, good.Header().Get("Location"))
	}
	form.Set("password", "wrong")
	req = httptest.NewRequest("POST", r.base()+"login?ui=classic", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if bad := r.do(req); bad.Code != 401 || !strings.Contains(bad.Body.String(), "Wrong username or password.") {
		t.Fatalf("classic failed login: %d", bad.Code)
	}
}
