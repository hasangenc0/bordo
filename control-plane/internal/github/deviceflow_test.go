package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeviceFlowConnects(t *testing.T) {
	var polls int32
	var scope string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/device":
			scope = r.PostForm.Get("scope")
			_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"ABCD-1234","verification_uri":"https://github.com/login/device","expires_in":60,"interval":1}`))
		case "/token":
			if atomic.AddInt32(&polls, 1) == 1 {
				_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"gho_abc","refresh_token":"rt","expires_in":28800}`))
		}
	}))
	defer gh.Close()
	oldD, oldT := oauthDeviceURL, oauthTokenURL
	oauthDeviceURL, oauthTokenURL = gh.URL+"/device", gh.URL+"/token"
	defer func() { oauthDeviceURL, oauthTokenURL = oldD, oldT }()

	s := testStore(t)
	d := NewDeviceFlow(s)
	if d.Status().Status != "idle" {
		t.Fatalf("initial status = %q", d.Status().Status)
	}
	ds, err := d.Start("cid")
	if err != nil {
		t.Fatal(err)
	}
	if ds.UserCode != "ABCD-1234" || ds.VerificationURI == "" {
		t.Fatalf("start = %+v", ds)
	}
	if scope != UserScopes {
		t.Fatalf("scope = %q", scope)
	}
	if d.Status().Status != "pending" {
		t.Fatalf("status after start = %q", d.Status().Status)
	}

	deadline := time.Now().Add(10 * time.Second)
	for d.Status().Status == "pending" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := d.Status().Status; got != "connected" {
		t.Fatalf("final status = %q (%+v)", got, d.Status())
	}
	tok, _, err := s.LoadUserToken(context.Background())
	if err != nil || tok != "gho_abc" {
		t.Fatalf("stored token = %q, err = %v", tok, err)
	}
}

func TestDeviceFlowDenied(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/device" {
			_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"X","verification_uri":"u","expires_in":60,"interval":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	}))
	defer gh.Close()
	oldD, oldT := oauthDeviceURL, oauthTokenURL
	oauthDeviceURL, oauthTokenURL = gh.URL+"/device", gh.URL+"/token"
	defer func() { oauthDeviceURL, oauthTokenURL = oldD, oldT }()

	d := NewDeviceFlow(testStore(t))
	if _, err := d.Start("cid"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for d.Status().Status == "pending" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := d.Status().Status; got != "denied" {
		t.Fatalf("status = %q", got)
	}
}

func TestDeviceFlowStartRejectedByGitHub(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"Not Found"}`))
	}))
	defer gh.Close()
	old := oauthDeviceURL
	oauthDeviceURL = gh.URL
	defer func() { oauthDeviceURL = old }()

	d := NewDeviceFlow(testStore(t))
	if _, err := d.Start("bad"); err == nil {
		t.Fatal("expected an error")
	}
	if d.Status().Status != "idle" {
		t.Fatalf("status = %q", d.Status().Status)
	}
}
