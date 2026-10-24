package view

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"time"
)

// Renderer owns the parsed template set. Templates are parsed once at startup
// from the embedded filesystem.
type Renderer struct {
	tmpl *template.Template
}

// NewRenderer parses layout.html, dashboard.html, and every partial.
// assetVersion is appended to asset URLs so a deploy can never pair new HTML
// with a stylesheet still sitting in the browser or service-worker cache.
func NewRenderer(fsys fs.FS, assetVersion string) (*Renderer, error) {
	funcs := template.FuncMap{
		"icon":        Icon,
		"mascot":      Figure,
		"mascotLabel": MascotLabel,
		"duration":    FormatDuration,
		"timer":       FormatTimer,
		"iso":         ISO,
		"minutes":     Minutes,
		"feedCount":   FeedCountLabel,
		"diaperCount": DiaperCountLabel,
		"sleepLabel":  SleepTotalLabel,
		"asset": func(path string) string {
			if assetVersion == "" {
				return path
			}
			return path + "?v=" + assetVersion
		},
	}
	tmpl, err := template.New("nurtur").Funcs(funcs).ParseFS(fsys, "*.html", "partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("view: parse templates: %w", err)
	}
	return &Renderer{tmpl: tmpl}, nil
}

// Execute renders one named template.
func (r *Renderer) Execute(w io.Writer, name string, data any) error {
	if err := r.tmpl.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("view: render %s: %w", name, err)
	}
	return nil
}

// ISO formats a UTC timestamp for data attributes and client-side timers.
func ISO(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// Clock is the local wall-clock label used in lists.
func (p *PageData) Clock(t time.Time) string {
	return FormatClock(t, p.location())
}

// TimelineHasToday reports whether the current history page contains today's
// group, so quick-log forms can ask the server to update that group in place.
func (p *PageData) TimelineHasToday() bool {
	return p.View == "timeline" && p.Timeline != nil && p.Timeline.HasToday
}

// DateInput formats a datetime-local value in the configured timezone.
func (p *PageData) DateInput(t time.Time) string {
	return t.In(p.location()).Format("2006-01-02T15:04:05")
}

// Ago renders a server-side relative label; timer.js refreshes it in place.
func (p *PageData) Ago(t time.Time) string {
	return FormatAgo(p.Now, t)
}

// DayLabel is the long local date heading.
func (p *PageData) DayLabel(t time.Time) string {
	return t.In(p.location()).Format("Monday, January 2")
}

func (p *PageData) location() *time.Location {
	if p.Location == nil {
		return time.Local
	}
	return p.Location
}
