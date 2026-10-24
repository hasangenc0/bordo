package tools

import (
	"fmt"
	"testing"
)

const wlJSON = `{"regions":[{"region":"eu","reported_at":"t","urls":["http://1.2.3.4:30775"],"pods":[
 {"name":"old","ready":true,"image":"img:old"},
 {"name":"new","ready":%s,"reason":"%s","image":"img:new"}]}]}`

func podsJSON(ready, reason string) []byte {
	return []byte(fmt.Sprintf(wlJSON, ready, reason))
}

func TestRolloutState(t *testing.T) {
	// The old pod is ready but is not the release's image: still waiting.
	if done, st := rolloutState("deployed", "img:new", "eu", podsJSON("false", "")); done || st["outcome"] != "waiting" {
		t.Fatalf("old pod only: done=%v %v", done, st)
	}
	if done, st := rolloutState("deployed", "img:new", "eu", podsJSON("true", "")); !done || st["outcome"] != "serving" {
		t.Fatalf("new pod ready: done=%v %v", done, st)
	}
	// Still reconciling: a ready pod of the new image is not enough until the release is deployed.
	if done, _ := rolloutState("reconciling", "img:new", "eu", podsJSON("true", "")); done {
		t.Fatal("must wait for the release to be deployed")
	}
	if done, st := rolloutState("deployed", "img:new", "eu", podsJSON("false", "ImagePullBackOff")); !done || st["outcome"] != "stuck" {
		t.Fatalf("stuck pod: done=%v %v", done, st)
	}
	if done, st := rolloutState("failed", "img:new", "eu", nil); !done || st["outcome"] != "failed" {
		t.Fatalf("failed release: done=%v %v", done, st)
	}
	if done, _ := rolloutState("deployed", "img:new", "eu", nil); done {
		t.Fatal("no workload report yet must keep waiting")
	}
}

func TestRolloutStateOutdatedReconciler(t *testing.T) {
	old := []byte(`{"regions":[{"region":"eu","pods":[{"name":"a","ready":true}]}]}`)
	done, st := rolloutState("deployed", "img:new", "eu", old)
	if !done || st["outcome"] != "unknown" {
		t.Fatalf("pods without images must not read as serving or waiting forever: done=%v %v", done, st)
	}
}
