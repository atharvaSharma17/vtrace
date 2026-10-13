package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var mu sync.Mutex

func Atomic(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return Write(path, append(b, '\n'))
}
func Write(path string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".atomic-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
func Append(path string, v any) error {
	mu.Lock()
	defer mu.Unlock()
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.Write(append(b, '\n')); e != nil {
		return e
	}
	return f.Sync()
}
func Read[T any](path string) ([]T, error) {
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var out []T
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 4*1024*1024)
	for s.Scan() {
		var v T
		if e = json.Unmarshal(s.Bytes(), &v); e != nil {
			return nil, fmt.Errorf("corrupt JSONL %s line %d: %w", path, len(out)+1, e)
		}
		out = append(out, v)
	}
	return out, s.Err()
}
func Lock(path string) (func(), error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, fmt.Errorf("host/run lock exists or inaccessible: %w; inspect owner before manual removal", e)
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(path) }, nil
}
