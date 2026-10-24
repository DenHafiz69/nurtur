package view

import "html/template"

// Supported mascots. The stored setting value is the same string the templates
// use; unknown values fall back to Pip so an older or hand-edited install still
// renders a complete page.
const (
	MascotPip = "pip"
	MascotCat = "cat"
)

// NormalizeMascot returns a supported mascot name, defaulting to Pip.
func NormalizeMascot(name string) string {
	if name == MascotCat {
		return MascotCat
	}
	return MascotPip
}

// MascotLabel is the friendly display name used on the picker and in copy.
func MascotLabel(name string) string {
	switch NormalizeMascot(name) {
	case MascotCat:
		return "Mochi"
	default:
		return "Pip"
	}
}

// Figure renders one mascot pose by name. It is exposed to templates as the
// "mascot" function so pickers can preview both characters.
func Figure(name, pose string) template.HTML {
	switch NormalizeMascot(name) {
	case MascotCat:
		return Cat(pose)
	default:
		return Pip(pose)
	}
}

// Mascot renders the page's chosen mascot in the named pose.
func (p *PageData) Mascot(pose string) template.HTML {
	return Figure(p.MascotName, pose)
}
