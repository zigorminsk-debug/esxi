// Package store persists only explicit non-secret host profiles and limited
// history summaries in the current user's application configuration folder.
package store

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
)

const maxProfiles = 100
const maxHistory = 200

type Profile struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	User    string `json:"user,omitempty"`
}
type Event struct {
	At       time.Time `json:"at"`
	Host     string    `json:"host"`
	Type     string    `json:"type"`
	Severity string    `json:"severity"`
	Summary  string    `json:"summary"`
}
type Store struct {
	mu  sync.Mutex
	dir string
}

func New(dir string) *Store { return &Store{dir: dir} }
func read(path string, v any) error {
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return errors.New("Не удалось прочитать локальные данные")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return errors.New("Файл локальных данных слишком велик или имеет неверный тип")
	}
	b, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil || len(b) > 1<<20 {
		return errors.New("Файл локальных данных слишком велик или недоступен")
	}
	if json.Unmarshal(b, v) != nil {
		return errors.New("Не удалось разобрать локальные данные")
	}
	return nil
}
func write(dir, name string, v any) error {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return errors.New("Не удалось создать папку локальных данных")
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return errors.New("Не удалось подготовить локальные данные")
	}
	b = append(b, '\n')
	// Atomic rename; unique temporary file stays in the same filesystem. On
	// Windows, Rename can replace an existing regular file with the Go runtime.
	f, e := os.CreateTemp(dir, ".tmp-*")
	if e != nil {
		return errors.New("Не удалось сохранить локальные данные")
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_ = f.Chmod(0600)
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e == nil {
		e = os.Rename(tmp, filepath.Join(dir, name))
	}
	if e != nil {
		return errors.New("Не удалось полностью сохранить локальные данные")
	}
	return nil
}
func clean(s string, max int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
	if len(s) > max {
		return ""
	}
	return s
}
func (s *Store) Profiles() ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := []Profile{}
	e := read(filepath.Join(s.dir, "hosts.json"), &p)
	return p, e
}
func (s *Store) SaveProfile(p Profile) ([]Profile, error) {
	p.Name = clean(p.Name, 160)
	p.User = clean(p.User, 256)
	_, address, e := inspect.ParseAddress(p.Address)
	if e != nil {
		return nil, e
	}
	p.Address = address
	if p.Name == "" {
		p.Name = address
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := []Profile{}
	if e = read(filepath.Join(s.dir, "hosts.json"), &items); e != nil {
		return nil, e
	}
	found := false
	for i := range items {
		if strings.EqualFold(items[i].Address, p.Address) {
			items[i] = p
			found = true
			break
		}
	}
	if !found {
		if len(items) >= maxProfiles {
			return nil, errors.New("В списке уже 100 хостов")
		}
		items = append(items, p)
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items, write(s.dir, "hosts.json", items)
}
func (s *Store) DeleteProfile(address string) ([]Profile, error) {
	_, address, e := inspect.ParseAddress(address)
	if e != nil {
		return nil, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := []Profile{}
	if e = read(filepath.Join(s.dir, "hosts.json"), &items); e != nil {
		return nil, e
	}
	out := []Profile{}
	for _, p := range items {
		if !strings.EqualFold(p.Address, address) {
			out = append(out, p)
		}
	}
	return out, write(s.dir, "hosts.json", out)
}
func (s *Store) History() ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := []Event{}
	e := read(filepath.Join(s.dir, "history.json"), &items)
	return items, e
}
func (s *Store) AddEvent(event Event) error {
	event.Host = clean(event.Host, 300)
	event.Type = clean(event.Type, 50)
	event.Severity = clean(event.Severity, 20)
	event.Summary = clean(event.Summary, 500)
	if event.Host == "" || event.Type == "" {
		return errors.New("Некорректная запись истории")
	}
	if event.At.IsZero() {
		event.At = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := []Event{}
	if e := read(filepath.Join(s.dir, "history.json"), &items); e != nil {
		return e
	}
	items = append([]Event{event}, items...)
	if len(items) > maxHistory {
		items = items[:maxHistory]
	}
	return write(s.dir, "history.json", items)
}
func (s *Store) ClearHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return write(s.dir, "history.json", []Event{})
}
