package cmd

import "testing"

func TestIsLocalServer(t *testing.T) {
	cases := map[string]bool{
		"http://localhost:7401":    true,
		"http://127.0.0.1:7401":    true,
		"http://[::1]:7401":        true,
		"https://bordo.example.io": false,
		"http://10.0.0.5:7401":     false,
		"://bad":                   false,
	}
	for in, want := range cases {
		if got := isLocalServer(in); got != want {
			t.Errorf("isLocalServer(%q) = %v, want %v", in, got, want)
		}
	}
}
