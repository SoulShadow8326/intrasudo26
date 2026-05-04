package template

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Renderer struct {
	root       string
	mu         sync.RWMutex
	templates  *template.Template
	lastLoaded time.Time
}

func New(root string) (*Renderer, error) {
	r := &Renderer{root: root}
	if err := r.loadTemplates(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Renderer) loadTemplates() error {
	pattern := filepath.Join(r.root, "*.html")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	t := template.New("root").Funcs(template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"toJSON": func(v any) template.JS {
			raw, _ := json.Marshal(v)
			return template.JS(string(raw))
		},
		"formatTime": func(ts int64) string {
			if ts == 0 {
				return ""
			}
			return time.Unix(ts, 0).Local().Format("02 Jan 2006 15:04")
		},
		"upper": strings.ToUpper,
		"inc":   func(i int) int { return i + 1 },
	})

	if _, err := t.ParseFiles(files...); err != nil {
		return err
	}

	var latest time.Time
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates = t
	r.lastLoaded = latest
	return nil
}

func (r *Renderer) shouldReload() bool {
	if os.Getenv("DEV_TEMPLATES") == "" {//set this env var for dev work for html reload without having to restart the server(remove in prod will cause it to rapidly be recalled each time html is served)
		return false
	}
	pattern := filepath.Join(r.root, "*.html")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return false
	}
	var latest time.Time
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	r.mu.RLock()
	loaded := r.lastLoaded
	r.mu.RUnlock()
	return latest.After(loaded)
}

func (r *Renderer) Render(w io.Writer, name string, data any) error {
	if r.shouldReload() {
		if err := r.loadTemplates(); err != nil {
			log.Printf("could not reload templates: %v", err)
		}
	}

	r.mu.RLock()
	t := r.templates
	r.mu.RUnlock()

	if t == nil {
		return fmt.Errorf("templates not loaded")
	}
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("render %s: %w", name, err)
	}
	return nil
}
