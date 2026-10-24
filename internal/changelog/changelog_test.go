package changelog

import (
	"fmt"
	"testing"
)

func TestReleasesAreNewestFirstAndUnique(t *testing.T) {
	if len(releases) == 0 {
		t.Fatal("no releases")
	}
	seen := make(map[string]bool, len(releases))
	for i, release := range releases {
		if release.Version == "" || NormalizeVersion(release.Version) != release.Version {
			t.Errorf("release %d has invalid version %q", i, release.Version)
		}
		if seen[release.Version] {
			t.Errorf("duplicate version %q", release.Version)
		}
		seen[release.Version] = true
		if release.Date == "" || release.Title == "" || release.Summary == "" {
			t.Errorf("release %s is missing a date, title, or summary", release.Version)
		}
		if len(release.Sections) == 0 {
			t.Errorf("release %s has no sections", release.Version)
		}
		for _, section := range release.Sections {
			if section.Title == "" || len(section.Items) == 0 {
				t.Errorf("release %s has an empty section", release.Version)
			}
		}
		if i > 0 && compareVersions(release.Version, releases[i-1].Version) >= 0 {
			t.Errorf("release order is not newest first: %s after %s", release.Version, releases[i-1].Version)
		}
	}
}

func compareVersions(a, b string) int {
	var amaj, amin, apatch, bmaj, bmin, bpatch int
	_, _ = fmt.Sscanf(a, "v%d.%d.%d", &amaj, &amin, &apatch)
	_, _ = fmt.Sscanf(b, "v%d.%d.%d", &bmaj, &bmin, &bpatch)
	switch {
	case amaj != bmaj:
		return amaj - bmaj
	case amin != bmin:
		return amin - bmin
	default:
		return apatch - bpatch
	}
}

func TestNormalizeVersion(t *testing.T) {
	cases := map[string]string{
		"v0.3.7":           "v0.3.7",
		"0.3.7":            "v0.3.7",
		"v0.3.7-dirty":     "v0.3.7",
		"v0.3.7-4-gabc123": "v0.3.7",
		"v0.3.7-rc1":       "v0.3.7",
		"dev":              "",
		"":                 "",
		"abc1234":          "",
		"v0.3":             "",
		"v0.3.7 extra":     "v0.3.7",
	}
	for input, want := range cases {
		if got := NormalizeVersion(input); got != want {
			t.Errorf("NormalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUnseen(t *testing.T) {
	latest, ok := Latest()
	if !ok {
		t.Fatal("no latest release")
	}

	if got := Unseen("dev", ""); len(got) != 0 {
		t.Errorf("dev build returned %d releases", len(got))
	}
	if got := Unseen(latest.Version, latest.Version); len(got) != 0 {
		t.Errorf("same version returned %d releases", len(got))
	}
	if got := Unseen(latest.Version, ""); len(got) != 1 || got[0].Version != latest.Version {
		t.Errorf("first visit = %v, want only %s", versions(got), latest.Version)
	}
	if got := Unseen(latest.Version, "v9.9.9"); len(got) != 1 || got[0].Version != latest.Version {
		t.Errorf("unknown seen version = %v, want only %s", versions(got), latest.Version)
	}

	// Moving from an older known release shows every release in between,
	// newest first, and never the one already seen.
	seen := releases[len(releases)-1].Version
	got := Unseen(latest.Version, seen)
	if len(got) != len(releases)-1 {
		t.Fatalf("unseen count = %d, want %d", len(got), len(releases)-1)
	}
	if got[0].Version != latest.Version {
		t.Errorf("unseen starts at %s, want %s", got[0].Version, latest.Version)
	}
	for _, release := range got {
		if release.Version == seen {
			t.Errorf("unseen includes already-seen %s", seen)
		}
	}
}

func versions(releases []Release) []string {
	out := make([]string, len(releases))
	for i, release := range releases {
		out[i] = release.Version
	}
	return out
}
