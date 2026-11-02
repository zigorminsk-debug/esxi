package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProfilesAndHistoryPersistWithoutPasswords(t *testing.T) {
	d := t.TempDir()
	s := New(d)
	p, e := s.SaveProfile(Profile{Name: "lab", Address: "192.0.2.1", User: "root"})
	if e != nil || len(p) != 1 {
		t.Fatalf("%+v %v", p, e)
	}
	p, e = s.SaveProfile(Profile{Name: "lab2", Address: "192.0.2.1:443", User: "admin"})
	if e != nil || len(p) != 1 {
		t.Fatal(p, e)
	}
	for i := 0; i < 205; i++ {
		if e = s.AddEvent(Event{At: time.Now(), Host: "192.0.2.1", Type: "проверка", Summary: "ok"}); e != nil {
			t.Fatal(e)
		}
	}
	h, e := New(d).History()
	if e != nil || len(h) != 200 {
		t.Fatalf("%d %v", len(h), e)
	}
	b, _ := os.ReadFile(filepath.Join(d, "hosts.json"))
	if strings.Contains(strings.ToLower(string(b)), "password") {
		t.Fatal("password persisted")
	}
	if _, e = s.DeleteProfile("192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	if e = s.ClearHistory(); e != nil {
		t.Fatal(e)
	}
	h, _ = s.History()
	if len(h) != 0 {
		t.Fatal(h)
	}
}
