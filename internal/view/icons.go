package view

import "html/template"

// icons are a small hand-drawn set: 24px, 2.25px rounded strokes. They are
// inline so they inherit currentColor and need no extra requests. Every
// fragment below is a trusted compile-time constant.
var icons = map[string]template.HTML{
	"drop":         `<path d="M12 3.4c3.1 4 5.4 6.8 5.4 9.5a5.4 5.4 0 0 1-10.8 0c0-2.7 2.3-5.5 5.4-9.5Z"/>`,
	"drop-left":    `<path d="M12 3.4c3.1 4 5.4 6.8 5.4 9.5a5.4 5.4 0 0 1-10.8 0c0-2.7 2.3-5.5 5.4-9.5Z"/><path d="M13.7 15.4H10.3l2.1-3.1"/>`,
	"drop-right":   `<path d="M12 3.4c3.1 4 5.4 6.8 5.4 9.5a5.4 5.4 0 0 1-10.8 0c0-2.7 2.3-5.5 5.4-9.5Z"/><path d="M10.3 15.4h3.4l-2.1-3.1"/>`,
	"check":        `<path d="m5 12.6 4.2 4.2L19 6.8"/>`,
	"pencil":       `<path d="M4 20l4.2-1L18.6 8.6a2 2 0 0 0 0-2.8l-1.4-1.4a2 2 0 0 0-2.8 0L4 14.8 3 19l1 1Z"/><path d="m13.6 5.6 4.4 4.4"/>`,
	"trash":        `<path d="M4 7h16"/><path d="M9 7V4.5h6V7"/><path d="M6.5 7l.8 12.2A1.8 1.8 0 0 0 9.1 21h5.8a1.8 1.8 0 0 0 1.8-1.8L17.5 7"/><path d="M10 11v6M14 11v6"/>`,
	"sun":          `<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>`,
	"moon":         `<path d="M20 14.2A8.2 8.2 0 0 1 9.8 4 8.5 8.5 0 1 0 20 14.2Z"/>`,
	"close":        `<path d="m6 6 12 12M18 6 6 18"/>`,
	"star":         `<path d="M12 3l2.2 5.1 5.5.5-4.2 3.7 1.3 5.4L12 14.9l-4.8 2.8 1.3-5.4-4.2-3.7 5.5-.5L12 3Z"/>`,
	"alert":        `<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5v5"/><circle cx="12" cy="16.4" r=".6" fill="currentColor" stroke="none"/>`,
	"bell":         `<path d="M6.5 16.5h11l-1.2-2.1a2 2 0 0 1-.3-1V10a4 4 0 0 0-8 0v3.4a2 2 0 0 1-.3 1l-1.2 2.1Z"/><path d="M10 19a2 2 0 0 0 4 0"/>`,
	"clock":        `<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/>`,
	"cloud":        `<path d="M7 18h9.5a4 4 0 0 0 .3-8 5.5 5.5 0 0 0-10.5 1.6A3.3 3.3 0 0 0 7 18Z"/>`,
	"diaper":       `<path d="M5 8h14l-1.1 6a4 4 0 0 1-4 3.4h-3.8a4 4 0 0 1-4-3.4L5 8Z"/><path d="M8 8V6.5A1.5 1.5 0 0 1 9.5 5h5A1.5 1.5 0 0 1 16 6.5V8"/>`,
	"dirty":        `<path d="M7 16h9.5a3.5 3.5 0 0 0 .4-7 5 5 0 0 0-9.7 1.4A3 3 0 0 0 7 16Z"/><circle cx="9.5" cy="19.4" r=".8" fill="currentColor" stroke="none"/><circle cx="13.2" cy="20.4" r=".6" fill="currentColor" stroke="none"/>`,
	"plus":         `<path d="M12 5v14M5 12h14"/>`,
	"arrow-left":   `<path d="M19 12H5M11 6l-6 6 6 6"/>`,
	"arrow-right":  `<path d="M5 12h14M13 6l6 6-6 6"/>`,
	"chevron-down": `<path d="m6 9 6 6 6-6"/>`,
	"history":      `<path d="M4 12a8 8 0 1 0 2.4-5.7"/><path d="M4 4.5V9h4.5"/><path d="M12 8.2V12l3.2 2"/>`,
	"settings":     `<path d="M3.5 7.5h9M17 7.5h3.5M3.5 16.5h4M12 16.5h8.5"/><circle cx="14.6" cy="7.5" r="2.4"/><circle cx="9.6" cy="16.5" r="2.4"/>`,
	"download":     `<path d="M12 4v11"/><path d="m7.5 10.5 4.5 4.5 4.5-4.5"/><path d="M5 19.5h14"/>`,
	"logout":       `<path d="M14.5 4.5H6a1.5 1.5 0 0 0-1.5 1.5v12A1.5 1.5 0 0 0 6 19.5h8.5"/><path d="M18 12H9.5"/><path d="m15 9 3 3-3 3"/>`,
	"lock":         `<rect x="5" y="10.5" width="14" height="9" rx="2.4"/><path d="M8.5 10.5V8a3.5 3.5 0 0 1 7 0v2.5"/><circle cx="12" cy="15" r="1"/>`,
	"link":         `<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1.7 1.7"/><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1.7-1.7"/>`,
	"copy":         `<rect x="8.5" y="8.5" width="11" height="11" rx="2"/><path d="M5.5 15.5h-1a2 2 0 0 1-2-2v-9a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`,
}

const (
	iconOpen  template.HTML = `<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">`
	iconClose template.HTML = `</svg>`
)

// Icon returns an inline SVG for the named icon. Unknown names render nothing;
// every control also carries a text label or aria-label.
func Icon(name string) template.HTML {
	body, ok := icons[name]
	if !ok {
		return ""
	}
	return iconOpen + body + iconClose
}
