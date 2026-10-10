package staticassets

import (
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
)

const immutableMaxAge = "public, max-age=31536000, immutable"

type asset struct {
	data  []byte
	mod   int64
	ctype string
}

type Assets struct {
	prefix string
	fsys   fs.FS
	mutex  sync.Mutex
	cache  map[string]*asset
}

func New(dir, prefix string) *Assets {
	return &Assets{
		prefix: prefix,
		fsys:   os.DirFS(dir),
		cache:  map[string]*asset{},
	}
}

func (a *Assets) URL(name string) string {
	if a == nil {
		return name
	}
	as, err := a.get(name)
	if err != nil {
		return a.prefix + name
	}
	return a.prefix + strconv.FormatInt(as.mod, 10) + "/" + name
}

func (a *Assets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	want, name := splitVersion(r.PathValue("name"))
	if !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	as, err := a.get(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if want == strconv.FormatInt(as.mod, 10) {
		w.Header().Set("Cache-Control", immutableMaxAge)
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Set("Content-Type", as.ctype)
	w.Header().Set("Content-Length", strconv.Itoa(len(as.data)))
	w.Write(as.data)
}

func (a *Assets) get(name string) (*asset, error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if as, ok := a.cache[name]; ok {
		return as, nil
	}
	info, err := fs.Stat(a.fsys, name)
	if err != nil {
		return nil, err
	}
	data, err := fs.ReadFile(a.fsys, name)
	if err != nil {
		return nil, err
	}
	as := &asset{data: data, mod: info.ModTime().Unix()}
	as.ctype = mime.TypeByExtension(path.Ext(name))
	if as.ctype == "" {
		as.ctype = "application/octet-stream"
	}
	a.cache[name] = as
	return as, nil
}

func splitVersion(rest string) (version, name string) {
	head, tail, found := strings.Cut(rest, "/")
	if found {
		if _, err := strconv.ParseUint(head, 10, 64); err == nil {
			return head, tail
		}
	}
	return "", rest
}
