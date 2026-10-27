// Package changelog holds Nurtur's release notes.
//
// The list is hand-written and shipped inside the binary so the notes work
// offline and survive restarts. Add a new Release at the top whenever the build
// version changes; the server shows every entry newer than the version an
// account has already seen.
package changelog

import (
	"regexp"
	"strings"
)

// Release is one version's worth of notes.
type Release struct {
	Version  string
	Date     string
	Title    string
	Summary  string
	Sections []Section
}

// Section groups related notes under a short heading such as "Added".
type Section struct {
	Title string
	Items []string
}

// releases is newest first. Keep it in that order: the server relies on it to
// work out which notes an account has not seen yet.
var releases = []Release{
	{
		Version: "v0.4.6",
		Date:    "Oct 27, 2026",
		Title:   "A quieter quick log",
		Summary: "The quick log now stays focused on whatever is running, and the Theme button keeps working after other settings change.",
		Sections: []Section{
			{
				Title: "Changed",
				Items: []string{
					"While nursing or sleeping, the quick log hides the options that don't apply.",
					"Every option comes back as soon as the feed or sleep is done.",
				},
			},
			{
				Title: "Fixed",
				Items: []string{
					"The Theme button in Settings still works after another setting is saved.",
				},
			},
		},
	},
	{
		Version: "v0.4.5",
		Date:    "Oct 27, 2026",
		Title:   "The bottom menu sits where it belongs",
		Summary: "On iPhone, the bottom menu now reaches the screen edge instead of floating above it.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"The bottom menu fills the home-indicator area on iOS instead of leaving a gap below it.",
				},
			},
		},
	},
	{
		Version: "v0.4.4",
		Date:    "Oct 27, 2026",
		Title:   "One thumb, three places",
		Summary: "A bottom menu keeps Home, History, and Settings one tap away.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"A bottom menu with Home, History, and Settings.",
					"The current screen stays highlighted, and quick logging still sits just above the menu.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"Today no longer needs its own History and Settings links.",
					"The quick-log button and its drawer sit above the menu, within easy reach of a thumb.",
				},
			},
			{
				Title: "Removed",
				Items: []string{
					"The small status chip in the header, for a calmer screen. Sync details are still in Settings.",
				},
			},
		},
	},
	{
		Version: "v0.4.3",
		Date:    "Oct 27, 2026",
		Title:   "Quick actions from the running row",
		Summary: "Tapping the running activity in Today now opens the same quick-log menu as the floating + button.",
		Sections: []Section{
			{
				Title: "Changed",
				Items: []string{
					"Tap the running nursing or sleep row in Today to open the quick log.",
					"Pause, switch sides, finish, wake, or log a diaper without leaving the row.",
					"The pencil and trash on that row still edit or delete it.",
				},
			},
		},
	},
	{
		Version: "v0.4.2",
		Date:    "Oct 27, 2026",
		Title:   "The nursing clock keeps its time",
		Summary: "Resuming a paused feed now continues from the time already nursed instead of starting at zero.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"Resuming a paused feed no longer resets the timer to 00:00.",
					"The timer, header badge, and nursing status card show the total time on the current side.",
					"Switching back to a side continues that side's running total.",
				},
			},
		},
	},
	{
		Version: "v0.4.1",
		Date:    "Oct 27, 2026",
		Title:   "Pause the timer, keep the time",
		Summary: "Nursing can now be paused when your baby unlatches, without losing the time already counted.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"A Pause button on the nursing timer.",
					"Resume on the same side, or choose the other side to carry on there.",
					"Paused minutes stay out of the nursing totals.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"The header badge, status card, history row, nursing sheet, and edit sheet show when a feed is paused.",
					"The nursing alert stops pinging while paused and does not replay old milestones after resuming.",
				},
			},
		},
	},
	{
		Version: "v0.4.0",
		Date:    "Oct 24, 2026",
		Title:   "Invite the people you trust",
		Summary: "A small admin screen generates one-use magic links, so nobody has to type the shared signup key.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"Admin screen at /admin, signed in as admin with the password from NURTUR_ADMIN_PASSWORD.",
					"Generate a magic link with an optional name and a 1, 7, or 30 day expiry.",
					"Magic links open signup directly, skip the shared secret key, and stop working after one account is created.",
					"See pending, used, expired, and revoked invitations, and revoke an active link.",
					"A read-only account list shows who has joined.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"The signup form hides the secret-key field when it is opened from an invitation.",
				},
			},
		},
	},
	{
		Version: "v0.3.8",
		Date:    "Oct 24, 2026",
		Title:   "Update notes, in the app",
		Summary: "Nurtur now tells you what changed after each update — and keeps the whole history.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"A What's new sheet appears the first time you open Nurtur after a version change.",
					"Close it with Got it; Nurtur remembers the version you've seen.",
					"Settings links to the full changelog, from v0.1.0 onward.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"Release notes are stored with the app, so they work offline and survive restarts.",
				},
			},
		},
	},
	{
		Version: "v0.3.7",
		Date:    "Oct 24, 2026",
		Title:   "Mochi joins the nursery",
		Summary: "A second mascot, a shorter README, and a quieter cloud sync default.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"Mochi the cat, a second mascot you can choose in Settings.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"The README is now a product overview instead of setup notes.",
					"Cloud pull interval now defaults to 5 minutes.",
				},
			},
			{
				Title: "Fixed",
				Items: []string{
					"Removed the stale timezone environment note from Settings.",
				},
			},
		},
	},
	{
		Version: "v0.3.6",
		Date:    "Oct 23, 2026",
		Title:   "A steadier place to return",
		Summary: "The drawer remembers how you left it, and notifications open the app.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"Nurtur remembers whether the quick-log drawer was open or closed.",
					"Tapping a notification returns to the installed app when it is already open.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"Recorded the project's commit, tag, and push policy.",
				},
			},
		},
	},
	{
		Version: "v0.3.5",
		Date:    "Oct 23, 2026",
		Title:   "A ping you can tune",
		Summary: "Choose when the nursing alert pings, and two small fixes around it.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"Configurable nursing alert: set a threshold from 0 to 120 minutes, or turn it off.",
				},
			},
			{
				Title: "Fixed",
				Items: []string{
					"Theme toggle no longer needs two clicks to leave night mode.",
					"The header name refreshes immediately after saving it.",
				},
			},
		},
	},
	{
		Version: "v0.3.4",
		Date:    "Oct 23, 2026",
		Title:   "Steadier sync, softer nights",
		Summary: "A warmer night theme, a calmer History dock, and a sync deadlock fixed.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"Warm plum night theme with lifted neutral cards.",
					"The quick-log dock collapses into a drawer on History.",
				},
			},
			{
				Title: "Fixed",
				Items: []string{
					"The sync worker no longer deadlocks after network failures.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"CI test packages run serially to avoid a Turso cache race.",
				},
			},
		},
	},
	{
		Version: "v0.3.3",
		Date:    "Oct 21, 2026",
		Title:   "A little more context",
		Summary: "Onboarding asks for the baby's age and gender, plus a local Docker shortcut.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"Onboarding now asks for the baby's age and gender.",
					"New make docker-up target for local container builds.",
				},
			},
		},
	},
	{
		Version: "v0.3.2",
		Date:    "Oct 21, 2026",
		Title:   "Repair older installs",
		Summary: "A fix for installs whose settings table missed a Turso sync.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"The settings table is repaired when a Turso sync misses the table rename.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"Recorded the project's commit, tag, and push policy.",
				},
			},
		},
	},
	{
		Version: "v0.3.1",
		Date:    "Oct 21, 2026",
		Title:   "Fresh styles every time",
		Summary: "Updates can no longer pair new HTML with an old stylesheet.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"Static CSS and JS URLs are versioned, so an update can't serve stale styles from a cache.",
				},
			},
		},
	},
	{
		Version: "v0.3.0",
		Date:    "Oct 21, 2026",
		Title:   "History that adds up",
		Summary: "Each day in History now leads with its breastfeeding totals.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"Per-day breastfeeding summary above each day's history.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"History days start collapsed to their summary; expand one to see every row.",
					"Service-worker cache bumped for the new summary styles.",
				},
			},
		},
	},
	{
		Version: "v0.2.0",
		Date:    "Oct 21, 2026",
		Title:   "One household, many phones",
		Summary: "Local accounts now keep each carer's baby profile and care log private.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"Per-account data scoping and multi-user signup.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"Settings and care logs are private to each local account.",
					"Stopped tracking generated graphify artifacts.",
				},
			},
		},
	},
	{
		Version: "v0.1.0",
		Date:    "Oct 21, 2026",
		Title:   "Hello, Nurtur",
		Summary: "The first release: a calm, one-thumb nursery companion.",
		Sections: []Section{
			{
				Title: "Nursing",
				Items: []string{
					"One-tap left/right timer that survives screen locks, refreshes, and restarts.",
					"Switch sides without stopping the feed; Last nursed stays in the header.",
					"Manual start and end edits with validation, plus quick minus-five and plus-five corrections.",
				},
			},
			{
				Title: "Diapers and sleep",
				Items: []string{
					"Wet, dirty, or both in a single tap, with optional colour, consistency, and notes.",
					"Sleep sessions with live wake windows and a day/night split.",
				},
			},
			{
				Title: "At a glance",
				Items: []string{
					"Dashboard status cards for nursing, diapers, and sleep.",
					"A unified history timeline with in-place delete and eight-second undo.",
				},
			},
			{
				Title: "Everyday use",
				Items: []string{
					"Night-first theme, thumb-zone dock, and no page jumps while logging.",
					"Installable PWA shell with an offline page and local-first background sync.",
					"JSON, CSV, and replayable SQL exports.",
					"Optional local accounts, editable settings, and a self-hosted Docker image.",
				},
			},
		},
	},
}

var versionPattern = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)`)

// NormalizeVersion reduces a build version such as "v0.3.7-4-gabc123",
// "v0.3.7-dirty", or "0.3.7" to "v0.3.7". It returns "" for "dev", a bare
// commit hash, or anything else that is not a semantic version.
func NormalizeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "dev") {
		return ""
	}
	match := versionPattern.FindStringSubmatch(raw)
	if match == nil {
		return ""
	}
	return "v" + match[1]
}

// All returns every release, newest first.
func All() []Release {
	out := make([]Release, len(releases))
	copy(out, releases)
	return out
}

// Latest returns the newest release. It returns false when the list is empty.
func Latest() (Release, bool) {
	if len(releases) == 0 {
		return Release{}, false
	}
	return releases[0], true
}

// Unseen returns the notes an account should see when moving from seenVersion
// to currentVersion. It returns nil for a dev build, an unchanged version, or
// a version that has no notes yet.
func Unseen(currentVersion, seenVersion string) []Release {
	current := NormalizeVersion(currentVersion)
	seen := NormalizeVersion(seenVersion)
	if current == "" || current == seen {
		return nil
	}

	currentIndex := versionIndex(current)
	if currentIndex < 0 {
		// The binary is newer than the last written note. There is nothing new
		// to announce, so leave the page alone.
		return nil
	}

	if seen == "" {
		// First visit after this feature shipped: show the running release
		// only, not the entire back catalogue.
		return []Release{releases[currentIndex]}
	}

	seenIndex := versionIndex(seen)
	switch {
	case seenIndex < 0:
		// The stored version predates the changelog. Show everything up to and
		// including the running release.
		out := make([]Release, currentIndex+1)
		copy(out, releases[:currentIndex+1])
		return out
	case seenIndex <= currentIndex:
		// Same version, or a downgrade. Nothing to announce.
		return nil
	default:
		out := make([]Release, seenIndex-currentIndex)
		copy(out, releases[currentIndex:seenIndex])
		return out
	}
}

func versionIndex(version string) int {
	for i, release := range releases {
		if release.Version == version {
			return i
		}
	}
	return -1
}
