package view

import (
	"strings"
	"testing"
)

func TestCatPosesRender(t *testing.T) {
	for _, pose := range []string{"awake", "nursing", "sleeping", "empty"} {
		t.Run(pose, func(t *testing.T) {
			svg := string(Cat(pose))
			if !strings.Contains(svg, `data-mascot="cat"`) {
				t.Fatalf("cat %s missing marker: %s", pose, svg)
			}
			if !strings.Contains(svg, `class="pip-svg pip-`+pose) {
				t.Fatalf("cat %s missing pose class: %s", pose, svg)
			}
			if !strings.HasSuffix(svg, "</svg>") {
				t.Fatalf("cat %s is not closed: %s", pose, svg)
			}
		})
	}
	if got := Cat("unknown"); got != "" {
		t.Fatalf("unknown cat pose = %q, want empty", got)
	}
}

func TestMascotFigureDispatch(t *testing.T) {
	if got := string(Figure(MascotCat, "awake")); !strings.Contains(got, `data-mascot="cat"`) {
		t.Fatalf("Figure(cat) = %s", got)
	}
	if got := string(Figure(MascotPip, "awake")); !strings.Contains(got, `data-mascot="pip"`) {
		t.Fatalf("Figure(pip) = %s", got)
	}
	if got := string(Figure("unknown", "awake")); !strings.Contains(got, `data-mascot="pip"`) {
		t.Fatalf("Figure(unknown) should fall back to pip: %s", got)
	}
}

func TestNormalizeMascot(t *testing.T) {
	cases := map[string]string{
		MascotPip: MascotPip,
		MascotCat: MascotCat,
		"":        MascotPip,
		"CAT":     MascotPip, // normalization is the server's job
		"dog":     MascotPip,
	}
	for in, want := range cases {
		if got := NormalizeMascot(in); got != want {
			t.Errorf("NormalizeMascot(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPageMascotFollowsSelection(t *testing.T) {
	cat := (&PageData{MascotName: MascotCat}).Mascot("sleeping")
	if !strings.Contains(string(cat), `data-mascot="cat"`) {
		t.Fatalf("selected cat not rendered: %s", cat)
	}
	pip := (&PageData{}).Mascot("awake")
	if !strings.Contains(string(pip), `data-mascot="pip"`) {
		t.Fatalf("default mascot is not pip: %s", pip)
	}
}
