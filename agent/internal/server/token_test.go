package server

import "testing"

func TestTokenOK(t *testing.T) {
	const want = "s3cret-token"
	cases := []struct {
		name         string
		header, qry  string
		wantAccepted bool
	}{
		{"bearer header", "Bearer s3cret-token", "", true},
		{"query token", "", "s3cret-token", true},
		{"wrong bearer", "Bearer nope", "", false},
		{"missing prefix", "s3cret-token", "", false},
		{"wrong query", "", "nope", false},
		{"nothing", "", "", false},
		{"prefix of token", "Bearer s3cret", "", false},
		{"token with suffix", "Bearer s3cret-token-x", "", false},
	}
	for _, c := range cases {
		if got := tokenOK(c.header, c.qry, want); got != c.wantAccepted {
			t.Errorf("%s: tokenOK = %v, want %v", c.name, got, c.wantAccepted)
		}
	}
}
