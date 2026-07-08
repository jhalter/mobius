package hotline

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemFileStore is an in-memory FileStore. It exists to prove that the FileStore abstraction is
// genuinely backend-agnostic (no *os.File, no direct os/filepath calls leak through) and to
// provide a fast, hermetic test fixture that needs no temp directory.
//
// It models storage as a flat keyspace of cleaned absolute paths, deriving directory structure
// from key prefixes the way an object store would. It is intentionally simple: a single mutex
// guards all state and it is not tuned for concurrent throughput. It is not production storage.
//
// Symlink/ReadLink return errors.ErrUnsupported, mirroring how a real object-store backend
// would behave — Hotline aliases simply aren't available on such backends.
type MemFileStore struct {
	mu    sync.Mutex
	nodes map[string]*memNode // keyed by cleaned absolute path
}

type memNode struct {
	data    []byte
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

// NewMemFileStore returns an empty in-memory FileStore.
func NewMemFileStore() *MemFileStore {
	return &MemFileStore{nodes: make(map[string]*memNode)}
}

var _ FileStore = (*MemFileStore)(nil)

func cleanPath(name string) string {
	return filepath.Clean(name)
}

// ensureParents registers directory nodes for every ancestor of name so that ReadDir/Stat on a
// parent directory works after a nested write, matching os.MkdirAll-on-write semantics that the
// file library relies on (uploads never explicitly Mkdir the FileRoot).
func (m *MemFileStore) ensureParents(name string) {
	dir := filepath.Dir(name)
	for {
		if dir == "." || dir == string(filepath.Separator) || dir == "" {
			return
		}
		if _, ok := m.nodes[dir]; !ok {
			m.nodes[dir] = &memNode{mode: fs.ModeDir | 0755, modTime: time.Now(), isDir: true}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func (m *MemFileStore) Mkdir(name string, perm fs.FileMode) error {
	name = cleanPath(name)

	m.mu.Lock()
	defer m.mu.Unlock()

	if n, ok := m.nodes[name]; ok && !n.isDir {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrExist}
	}
	m.ensureParents(name)
	m.nodes[name] = &memNode{mode: fs.ModeDir | perm, modTime: time.Now(), isDir: true}
	return nil
}

func (m *MemFileStore) Stat(name string) (fs.FileInfo, error) {
	name = cleanPath(name)

	m.mu.Lock()
	defer m.mu.Unlock()

	n, ok := m.nodes[name]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return newMemFileInfo(name, n), nil
}

func (m *MemFileStore) Open(name string) (io.ReadCloser, error) {
	name = cleanPath(name)

	m.mu.Lock()
	defer m.mu.Unlock()

	n, ok := m.nodes[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if n.isDir {
		return nil, &fs.PathError{Op: "open", Path: name, Err: errors.New("is a directory")}
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), n.data...))), nil
}

func (m *MemFileStore) ReadFile(name string) ([]byte, error) {
	name = cleanPath(name)

	m.mu.Lock()
	defer m.mu.Unlock()

	n, ok := m.nodes[name]
	if !ok || n.isDir {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return append([]byte(nil), n.data...), nil
}

func (m *MemFileStore) ReadDir(name string) ([]fs.DirEntry, error) {
	name = cleanPath(name)

	m.mu.Lock()
	defer m.mu.Unlock()

	if n, ok := m.nodes[name]; ok && !n.isDir {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: errors.New("not a directory")}
	}

	var entries []fs.DirEntry
	for p, n := range m.nodes {
		if p == name {
			continue
		}
		if filepath.Dir(p) == name {
			entries = append(entries, fs.FileInfoToDirEntry(newMemFileInfo(p, n)))
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (m *MemFileStore) ReadLink(name string) (string, error) {
	return "", &fs.PathError{Op: "readlink", Path: name, Err: errors.ErrUnsupported}
}

func (m *MemFileStore) Symlink(oldname, newname string) error {
	return &fs.PathError{Op: "symlink", Path: newname, Err: errors.ErrUnsupported}
}

// Walk mirrors filepath.Walk: it visits root and all descendants in lexical order. It supports
// filepath.SkipDir returned from fn.
func (m *MemFileStore) Walk(root string, fn filepath.WalkFunc) error {
	root = cleanPath(root)

	m.mu.Lock()
	paths := make([]string, 0, len(m.nodes))
	for p := range m.nodes {
		if p == root || strings.HasPrefix(p, root+string(filepath.Separator)) {
			paths = append(paths, p)
		}
	}
	// Snapshot the nodes we intend to visit so fn can call back into the store without deadlock.
	snapshot := make(map[string]*memNode, len(paths))
	for _, p := range paths {
		snapshot[p] = m.nodes[p]
	}
	_, rootExists := m.nodes[root]
	m.mu.Unlock()

	if !rootExists {
		return fn(root, nil, &fs.PathError{Op: "lstat", Path: root, Err: fs.ErrNotExist})
	}

	sort.Strings(paths)

	var skipped []string
	for _, p := range paths {
		// Honor SkipDir: skip anything under a directory whose walk fn returned SkipDir.
		skip := false
		for _, s := range skipped {
			if p == s || strings.HasPrefix(p, s+string(filepath.Separator)) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		info := newMemFileInfo(p, snapshot[p])
		err := fn(p, info, nil)
		if err != nil {
			if errors.Is(err, filepath.SkipDir) && info.IsDir() {
				skipped = append(skipped, p)
				continue
			}
			return err
		}
	}

	return nil
}

func (m *MemFileStore) Create(name string) (io.WriteCloser, error) {
	return m.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
}

func (m *MemFileStore) OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
	name = cleanPath(name)
	return &memWriter{
		store:  m,
		name:   name,
		perm:   perm,
		append: flag&os.O_APPEND != 0,
	}, nil
}

func (m *MemFileStore) WriteFile(name string, data []byte, perm fs.FileMode) error {
	name = cleanPath(name)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.ensureParents(name)
	m.nodes[name] = &memNode{data: append([]byte(nil), data...), mode: perm, modTime: time.Now()}
	return nil
}

func (m *MemFileStore) Rename(oldpath, newpath string) error {
	oldpath = cleanPath(oldpath)
	newpath = cleanPath(newpath)

	m.mu.Lock()
	defer m.mu.Unlock()

	n, ok := m.nodes[oldpath]
	if !ok {
		return &fs.PathError{Op: "rename", Path: oldpath, Err: fs.ErrNotExist}
	}
	m.ensureParents(newpath)
	m.nodes[newpath] = n
	delete(m.nodes, oldpath)
	return nil
}

func (m *MemFileStore) Remove(name string) error {
	name = cleanPath(name)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.nodes[name]; !ok {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrNotExist}
	}
	delete(m.nodes, name)
	return nil
}

func (m *MemFileStore) RemoveAll(path string) error {
	path = cleanPath(path)

	m.mu.Lock()
	defer m.mu.Unlock()

	for p := range m.nodes {
		if p == path || strings.HasPrefix(p, path+string(filepath.Separator)) {
			delete(m.nodes, p)
		}
	}
	return nil
}

// memWriter buffers writes and commits them to the store on Close, supporting append semantics.
type memWriter struct {
	store  *MemFileStore
	name   string
	perm   fs.FileMode
	append bool
	buf    bytes.Buffer
	closed bool
}

func (w *memWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fs.ErrClosed
	}
	return w.buf.Write(p)
}

func (w *memWriter) Close() error {
	if w.closed {
		return fs.ErrClosed
	}
	w.closed = true

	w.store.mu.Lock()
	defer w.store.mu.Unlock()

	w.store.ensureParents(w.name)

	data := w.buf.Bytes()
	if w.append {
		if existing, ok := w.store.nodes[w.name]; ok && !existing.isDir {
			data = append(append([]byte(nil), existing.data...), data...)
		}
	}
	w.store.nodes[w.name] = &memNode{
		data:    append([]byte(nil), data...),
		mode:    w.perm,
		modTime: time.Now(),
	}
	return nil
}

// memFileInfo implements fs.FileInfo over a memNode.
type memFileInfo struct {
	name string
	node *memNode
}

func newMemFileInfo(path string, n *memNode) memFileInfo {
	return memFileInfo{name: filepath.Base(path), node: n}
}

func (fi memFileInfo) Name() string       { return fi.name }
func (fi memFileInfo) Size() int64        { return int64(len(fi.node.data)) }
func (fi memFileInfo) Mode() fs.FileMode  { return fi.node.mode }
func (fi memFileInfo) ModTime() time.Time { return fi.node.modTime }
func (fi memFileInfo) IsDir() bool        { return fi.node.isDir }
func (fi memFileInfo) Sys() any           { return nil }
