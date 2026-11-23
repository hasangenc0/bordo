package chatstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func seeded(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.CreateSession(ctx, "a", "Ship it"); err != nil {
		t.Fatal(err)
	}
	for _, m := range [][2]string{{"user", "one"}, {"assistant", "r1"}, {"user", "two"}, {"assistant", "r2"}, {"user", "three"}} {
		if err := s.AppendMessage(ctx, "a", m[0], m[1]); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func contents(t *testing.T, s *Store, id string) []string {
	t.Helper()
	msgs, _ := s.GetMessages(context.Background(), id)
	var out []string
	for _, m := range msgs {
		out = append(out, m.Content)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTruncateFromUserDropsThatMessageAndAfter(t *testing.T) {
	s := seeded(t)
	if err := s.TruncateFromUser(context.Background(), "a", 1); err != nil {
		t.Fatal(err)
	}
	if got := contents(t, s, "a"); !eq(got, []string{"one", "r1"}) {
		t.Fatalf("got %v", got)
	}
}

func TestTruncateRejectsOutOfRange(t *testing.T) {
	s := seeded(t)
	for _, n := range []int{-1, 3, 99} {
		if err := s.TruncateFromUser(context.Background(), "a", n); !errors.Is(err, ErrNoSuchMessage) {
			t.Errorf("n=%d: got %v", n, err)
		}
	}
	if got := contents(t, s, "a"); len(got) != 5 {
		t.Fatalf("a rejected truncate must not delete anything: %v", got)
	}
}

func TestForkCopiesEarlierMessagesAndLeavesSourceAlone(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	f, err := s.Fork(ctx, "a", "b", 1)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "Ship it (fork)" {
		t.Errorf("title %q", f.Title)
	}
	if got := contents(t, s, "b"); !eq(got, []string{"one", "r1"}) {
		t.Fatalf("fork has %v", got)
	}
	if got := contents(t, s, "a"); len(got) != 5 {
		t.Fatalf("source changed: %v", got)
	}
	// Forking the very first message gives an empty chat to start over in.
	if _, err := s.Fork(ctx, "a", "c", 0); err != nil {
		t.Fatal(err)
	}
	if got := contents(t, s, "c"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if _, err := s.Fork(ctx, "a", "d", 7); !errors.Is(err, ErrNoSuchMessage) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.Fork(ctx, "nope", "e", 0); err == nil {
		t.Fatal("forking a missing chat must fail")
	}
}
