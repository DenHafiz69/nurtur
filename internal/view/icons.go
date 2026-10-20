package view

import "html/template"

// icons are a small hand-drawn set: 24px, 2.25px rounded strokes. They are
// inline so they inherit currentColor and need no extra requests.
var icons = map[string]string{
	"drop":        `<path d="M12 3.4c3.1 4 5.4 6.8 5.4 9.5a5.4 5.4 0 0 1-10.8 0c0-2.7 2.3-5.5 5.4-9.5Z"/>`,
	"drop-left":   `<path d="M12 3.4c3.1 4 5.4 6.8 5.4 9.5a5.4 5.4 0 0 1-10.8 0c0-2.7 2.3-5.5 5.4-9.5Z"/><path d="M13.7 15.4H10.3l2.1-3.1"/>`,
	"drop-right":  `<path d="M12 3.4c3.1 4 5.4 6.8 5.4 9.5a5.4 5.4 0 0 1-10.8 0c0-2.7 2.3-5.5 5.4-9.5Z"/><path d="M10.3 15.4h3.4l-2.1-3.1"/>`,
	"check":       `<path d="m5 12.6 4.2 4.2L19 6.8"/>`,
	"pencil":      `<path d="M4 20l4.2-1L18.6 8.6a2 2 0 0 0 0-2.8l-1.4-1.4a2 2 0 0 0-2.8 0L4 14.8 3 19l1 1Z"/><path d="m13.6 5.6 4.4 4.4"/>`,
	"trash":       `<path d="M4 7h16"/><path d="M9 7V4.5h6V7"/><path d="M6.5 7l.8 12.2A1.8 1.8 0 0 0 9.1 21h5.8a1.8 1.8 0 0 0 1.8-1.8L17.5 7"/><path d="M10 11v6M14 11v6"/>`,
	"sun":         `<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>`,
	"moon":        `<path d="M20 14.2A8.2 8.2 0 0 1 9.8 4 8.5 8.5 0 1 0 20 14.2Z"/>`,
	"close":       `<path d="m6 6 12 12M18 6 6 18"/>`,
	"star":        `<path d="M12 3l2.2 5.1 5.5.5-4.2 3.7 1.3 5.4L12 14.9l-4.8 2.8 1.3-5.4-4.2-3.7 5.5-.5L12 3Z"/>`,
	"alert":       `<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5v5"/><circle cx="12" cy="16.4" r=".6" fill="currentColor" stroke="none"/>`,
	"clock":       `<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/>`,
	"cloud":       `<path d="M7 18h9.5a4 4 0 0 0 .3-8 5.5 5.5 0 0 0-10.5 1.6A3.3 3.3 0 0 0 7 18Z"/>`,
	"arrow-left":  `<path d="M19 12H5M11 6l-6 6 6 6"/>`,
	"arrow-right": `<path d="M5 12h14M13 6l6 6-6 6"/>`,
}

// Icon returns an inline SVG for the named icon. Unknown names render nothing;
// every control also carries a text label or aria-label.
func Icon(name string) template.HTML {
	body, ok := icons[name]
	if !ok {
		return ""
	}
	return template.HTML(`<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">` + body + `</svg>`)
}
