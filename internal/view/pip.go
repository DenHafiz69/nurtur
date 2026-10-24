package view

import "html/template"

// Pip is Nurtur's sleepy little sprout. The poses are inline SVG so their
// palette can follow the night theme via CSS custom properties. Pip is always
// decorative: aria-hidden, never the only signal for a state.

const pipLeaves = `
	<path class="pip-stem" d="M60 41V30"/>
	<path class="pip-leaf" d="M59 33C53 18 61 8 78 7c2 14-6 24-19 26Z"/>
	<path class="pip-vein" d="M62 30c5-7 10-13 16-18"/>`

const pipBody = `
	<path class="pip-body" d="M60 35c19 0 34 15 34 34S79 102 60 102 26 88 26 69s15-34 34-34Z"/>
	<path class="pip-outline" d="M60 35c19 0 34 15 34 34S79 102 60 102 26 88 26 69s15-34 34-34Z"/>`

const pipCheeks = `
	<ellipse class="pip-cheek" cx="43" cy="75" rx="6.5" ry="3.6"/>
	<ellipse class="pip-cheek" cx="77" cy="75" rx="6.5" ry="3.6"/>`

// pipOpen holds the trusted opening tag for each pose. Keeping them typed and
// precomposed means no user-supplied string ever reaches the SVG wrapper.
const pipViewBox = `" data-mascot="pip" viewBox="0 0 120 120" aria-hidden="true" focusable="false">`

var pipOpen = map[string]template.HTML{
	"awake":    `<svg class="pip-svg pip-awake` + pipViewBox,
	"nursing":  `<svg class="pip-svg pip-nursing` + pipViewBox,
	"sleeping": `<svg class="pip-svg pip-sleeping` + pipViewBox,
	"empty":    `<svg class="pip-svg pip-empty` + pipViewBox,
}

func pipSVG(pose string, content template.HTML) template.HTML {
	open, ok := pipOpen[pose]
	if !ok {
		return ""
	}
	return open + content + `</svg>`
}

// Pip returns the named mascot pose: awake, nursing, sleeping, or empty.
func Pip(pose string) template.HTML {
	switch pose {
	case "awake":
		return pipSVG("awake", pipLeaves+pipBody+pipCheeks+`
			<circle class="pip-eye" cx="49" cy="64" r="3.4"/>
			<circle class="pip-eye" cx="71" cy="64" r="3.4"/>
			<path class="pip-mouth" d="M55 76c2.5 3 7.5 3 10 0"/>`)

	case "nursing":
		return pipSVG("nursing", pipLeaves+`
			<path class="pip-arm" d="M28 68c-7 7-7 17 0 23 6 5 14 3 18-3"/>
			<path class="pip-arm" d="M92 68c7 7 7 17 0 23-6 5-14 3-18-3"/>`+
			pipBody+`
			<path class="pip-drop" d="M60 54c8 10 12 15.5 12 20.5A12 12 0 0 1 48 74.5C48 69.5 52 64 60 54Z"/>
			<path class="pip-drop-shine" d="M55 71c0-3 1.5-5.5 4-8"/>`+pipCheeks+`
			<path class="pip-eye-line" d="M43 63c2 2 5 2 7 0"/>
			<path class="pip-eye-line" d="M70 63c2 2 5 2 7 0"/>
			<path class="pip-mouth" d="M55 78c2.5 3 7.5 3 10 0"/>`)

	case "sleeping":
		return pipSVG("sleeping", pipLeaves+`
			<path class="pip-zzz" d="M85 24h10l-10 10h10"/>
			<path class="pip-zzz pip-zzz-small" d="M99 12h6l-6 6h6"/>`+
			pipBody+pipCheeks+`
			<path class="pip-eye-line" d="M43 65c2.5 2.4 6 2.4 8.5 0"/>
			<path class="pip-eye-line" d="M68.5 65c2.5 2.4 6 2.4 8.5 0"/>
			<path class="pip-mouth" d="M56 77c2 1.6 6 1.6 8 0"/>`)

	case "empty":
		return pipSVG("empty", pipLeaves+`
			<path class="pip-arm" d="M29 76c-9 1-16-3-21-10"/>
			<path class="pip-arm" d="M91 76c9 1 16-3 21-10"/>`+
			pipBody+pipCheeks+`
			<circle class="pip-eye" cx="49" cy="64" r="3.2"/>
			<circle class="pip-eye" cx="71" cy="64" r="3.2"/>
			<ellipse class="pip-mouth-fill" cx="60" cy="77" rx="3.2" ry="2.6"/>`)

	default:
		return ""
	}
}
