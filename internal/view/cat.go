package view

import "html/template"

// Mochi is Nurtur's second mascot: a small, round cat. Like Pip, the poses are
// inline SVG so their palette follows the night theme, and Mochi is always
// decorative: aria-hidden and never the only signal for a state.

const (
	catTailBack template.HTML = `<path class="cat-tail" d="M91 82c10 2 15 9 13 17"/>`

	catTailCurl template.HTML = `<path class="cat-tail" d="M92 86c11 3 9 15-3 15H62"/>`

	catEars template.HTML = `
	<path class="cat-ear" d="M37 48C31 39 28 30 29 20c8 3 14 10 18 17Z"/>
	<path class="cat-ear-inner" d="M38 42c-3-6-5-11-5-15 4 2 7 5 10 9Z"/>
	<path class="cat-ear" d="M83 48c6-9 9-18 8-28-8 3-14 10-18 17Z"/>
	<path class="cat-ear-inner" d="M82 42c3-6 5-11 5-15-4 2-7 5-10 9Z"/>`

	catBody template.HTML = `
	<path class="pip-body" d="M60 35c18 0 33 14 33 33s-15 34-33 34-33-15-33-34 15-33 33-33Z"/>
	<path class="pip-outline" d="M60 35c18 0 33 14 33 33s-15 34-33 34-33-15-33-34 15-33 33-33Z"/>`

	catCheeks template.HTML = `
	<ellipse class="pip-cheek" cx="42" cy="77" rx="6.5" ry="3.6"/>
	<ellipse class="pip-cheek" cx="78" cy="77" rx="6.5" ry="3.6"/>`

	catWhiskers template.HTML = `
	<path class="cat-whisker" d="M23 62h13M23 70h12"/>
	<path class="cat-whisker" d="M97 62H84M97 70H85"/>`

	catNose template.HTML = `<path class="cat-nose" d="M56.5 70h7L60 74Z"/>`

	catMouth template.HTML = `<path class="pip-mouth" d="M60 74v2.5M60 76.5c-2.5 2.5-5.5 2.5-8 0M60 76.5c2.5 2.5 5.5 2.5 8 0"/>`
)

var catOpen = map[string]template.HTML{
	"awake":    `<svg class="pip-svg pip-awake" data-mascot="cat" viewBox="0 0 120 120" aria-hidden="true" focusable="false">`,
	"nursing":  `<svg class="pip-svg pip-nursing" data-mascot="cat" viewBox="0 0 120 120" aria-hidden="true" focusable="false">`,
	"sleeping": `<svg class="pip-svg pip-sleeping" data-mascot="cat" viewBox="0 0 120 120" aria-hidden="true" focusable="false">`,
	"empty":    `<svg class="pip-svg pip-empty" data-mascot="cat" viewBox="0 0 120 120" aria-hidden="true" focusable="false">`,
}

// Cat returns Mochi in the named pose: awake, nursing, sleeping, or empty.
func Cat(pose string) template.HTML {
	open, ok := catOpen[pose]
	if !ok {
		return ""
	}
	var content template.HTML
	switch pose {
	case "awake":
		content = catTailBack + catEars + catBody + catWhiskers + catNose + catMouth + catCheeks + `
			<circle class="pip-eye" cx="49" cy="63" r="3.4"/>
			<circle class="pip-eye" cx="71" cy="63" r="3.4"/>`

	case "nursing":
		content = catEars + catBody + `
			<path class="pip-arm" d="M31 72c-7 6-8 15-1 21 6 5 13 3 17-3"/>
			<path class="pip-arm" d="M89 72c7 6 8 15 1 21-6 5-13 3-17-3"/>
			<path class="pip-drop" d="M60 84c6 7.5 9.5 12 9.5 15A9.5 9.5 0 0 1 50.5 99C50.5 96 54 91.5 60 84Z"/>
			<path class="pip-drop-shine" d="M54.5 95c0-2.5 1.5-5 3.5-7"/>` +
			catWhiskers + catNose + catMouth + catCheeks + `
			<path class="pip-eye-line" d="M43 62c2 2.2 5 2.2 7 0"/>
			<path class="pip-eye-line" d="M70 62c2 2.2 5 2.2 7 0"/>`

	case "sleeping":
		content = catEars + catBody + catTailCurl + catWhiskers + catNose + catMouth + catCheeks + `
			<path class="pip-eye-line" d="M43 64c2.5 2.4 6 2.4 8.5 0"/>
			<path class="pip-eye-line" d="M68.5 64c2.5 2.4 6 2.4 8.5 0"/>
			<path class="pip-zzz" d="M85 24h10l-10 10h10"/>
			<path class="pip-zzz pip-zzz-small" d="M99 12h6l-6 6h6"/>`

	case "empty":
		content = catTailBack + catEars + catBody + `
			<path class="pip-arm" d="M29 78c-9 1-16-3-21-10"/>
			<path class="pip-arm" d="M91 78c9 1 16-3 21-10"/>` +
			catWhiskers + catNose + catCheeks + `
			<circle class="pip-eye" cx="49" cy="63" r="3.2"/>
			<circle class="pip-eye" cx="71" cy="63" r="3.2"/>
			<ellipse class="pip-mouth-fill" cx="60" cy="77" rx="3.2" ry="2.6"/>`
	}
	return open + content + `</svg>`
}
