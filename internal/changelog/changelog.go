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
		Version: "v0.5.5",
		Date:    "Nov 14, 2026",
		Title:   "One-tap app install",
		Summary: "Install Nurtur to your home screen directly from the landing page or Settings.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"An Install app button on the landing page and in Settings that triggers the browser's install prompt on Android, Chrome, Edge, and desktop.",
					"A quick two-step Add to Home Screen guide sheet for Safari on iPhone and iPad.",
				},
			},
		},
	},
	{
		Version: "v0.5.4",
		Date:    "Nov 14, 2026",
		Title:   "Live demo, updated landing page, and account recovery",
		Summary: "Share Nurtur with family using a no-login live demo, a refreshed landing page covering all trackers, and safer account management in Admin.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"A live demo mode (/demo) that lets visitors try the full app with sample data without logging in.",
					"Account soft-deletion with double confirmation, 30-day recovery, and automatic permanent purge in Admin.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"The landing page now highlights nursing, bottles, pumping, diapers, sleep, solids, and the baby album, with a collapsible mobile menu.",
					"The Last nursed badge now sits level without tilt.",
				},
			},
		},
	},
	{
		Version: "v0.5.3",
		Date:    "Nov 13, 2026",
		Title:   "Cleaner settings, even albums",
		Summary: "The Feeding Mode picker becomes a simple three-part control, and landscape photos now fill their album tiles. Your logs stay exactly as they are.",
		Sections: []Section{
			{
				Title: "Changed",
				Items: []string{
					"Feeding Mode in Settings is now a three-segment control: Direct Latch, Hybrid, or Exclusive Pump, with shorter labels that fit on a phone.",
					"Landscape album photos now crop to fill square grid tiles, so rows line up evenly instead of mixing tall and wide cards.",
				},
			},
		},
	},
	{
		Version: "v0.5.2",
		Date:    "Nov 13, 2026",
		Title:   "More ways to feed",
		Summary: "Bottles, pumping, and solid meals join Quick log as opt-in trackers. Nursing works exactly as before until you turn them on.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"A Feeding mode in Settings: Direct nursing, Hybrid, or Exclusive pumping. Bottle and pump buttons appear only in the modes that use them.",
					"Bottle logging for breast milk or formula, with 60, 90, and 120 ml presets (2, 3, and 4 oz) plus a custom amount and time.",
					"Pump sessions for the left side, right side, or both, with optional output and duration.",
					"A live pump timer in the Quick log dock that carries its minutes into the pump sheet when you tap Done & Log.",
					"An optional Solids & meals tracker with food, portion, reaction, and notes.",
					"Bottle, pump, and solid rows in Today and History, with edit and delete.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"The Quick log drawer groups its options by category with matching colours, so the growing set of trackers stays easy to scan.",
					"Saving tracker settings updates the Quick log dock right away, without a reload.",
					"In exclusive pump mode, Start sleep spans the full width and the left/right nursing controls are hidden.",
				},
			},
			{
				Title: "Backups",
				Items: []string{
					"JSON, CSV, and SQL exports include bottles, pumps, and solid meals.",
				},
			},
		},
	},
	{
		Version: "v0.5.1",
		Date:    "Nov 3, 2026",
		Title:   "Quieter checks, safer paths",
		Summary: "Behind-the-scenes hardening for photo storage and the feed tests. Nothing changes in daily use.",
		Sections: []Section{
			{
				Title: "Improved",
				Items: []string{
					"Photo storage reads now document and enforce the check that keeps object keys inside the storage root.",
					"The feed and album tests guard against missing values, so a regression fails the suite instead of panicking.",
				},
			},
		},
	},
	{
		Version: "v0.5.0",
		Date:    "Nov 2, 2026",
		Title:   "A home for the small moments",
		Summary: "The album arrives as a fourth tab: take or choose photos, browse them by day, open a calm full-screen viewer, fix a date or caption, and undo a delete.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"A fourth Album tab with camera-first capture and multi-select from the photo library.",
					"Browser-side resizing to a WebP display copy and a small WebP thumbnail, with a JPEG fallback for older browsers.",
					"Private photo storage on local disk or a private Cloudflare R2 bucket, proxied through Nurtur and scoped to the signed-in account.",
					"Whole-day browsing with sticky date headings, tile times, lazy loading, and earlier days loaded in place.",
					"A full-screen viewer with keyboard, buttons, and swipe navigation, plus a Save action for the full copy.",
					"Date and caption corrections, soft delete with Undo, and a 30-day retention sweep for deleted photo bytes.",
				},
			},
			{
				Title: "Fixed",
				Items: []string{
					"The Home-only quick-log button no longer reappears on History or Settings after a tab switch.",
				},
			},
			{
				Title: "Backups",
				Items: []string{
					"JSON and SQL exports now include album metadata, dates, and object keys. Photo bytes are backed up separately from the database.",
				},
			},
		},
	},
	{
		Version: "v0.4.17",
		Date:    "Nov 1, 2026",
		Title:   "The bottom edge, the browser's way",
		Summary: "The bottom nav sits flush on the screen edge again with the labels just above the home indicator.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"The shell extends past the iOS standalone viewport by exactly the safe-area inset, so the tab bar's background fills the strip below it instead of leaving a band of empty page.",
					"The row is placed with the browser's own safe-area value again, not with screen dimensions that mixed in the status bar.",
				},
			},
		},
	},
	{
		Version: "v0.4.16",
		Date:    "Nov 1, 2026",
		Title:   "Back on the bottom edge",
		Summary: "The tab labels and the quick-log button are fully visible again instead of slipping off the bottom of the screen on iOS.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"The extra bottom offset is gone: in an installed iOS web app the fixed shell already reaches the physical screen edge, so extending it pushed the tab labels and part of the + button below the visible screen.",
					"The bottom nav keeps a single safe-area padding again, leaving the labels just above the home indicator with the bar flush to the edge.",
				},
			},
		},
	},
	{
		Version: "v0.4.15",
		Date:    "Nov 1, 2026",
		Title:   "The bar sits on the edge",
		Summary: "The bottom nav stops floating above a strip of empty page on iOS and Android.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"The measured safe-area gap was applied as a positive offset, which pulled the fixed shell up and left a strip of background under the tab bar instead of extending it to the screen edge.",
					"The gap measurement now runs only on iOS, where the home-indicator strip can fall outside the layout viewport; Android keeps its full height.",
				},
			},
		},
	},
	{
		Version: "v0.4.14",
		Date:    "Nov 1, 2026",
		Title:   "Snug at the bottom edge again",
		Summary: "The tab row sits just above the home indicator again, and the bar stays flush because the shell now measures the real screen gap instead of guessing it.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"Too much empty room below the tab row on iOS is gone; the labels and the quick-log button sit a safe-area's height above the bottom edge again.",
					"The bottom bar reaches the physical screen edge on every iOS version, because the shell now measures the viewport gap instead of assuming how the standalone browser reports it.",
				},
			},
		},
	},
	{
		Version: "v0.4.13",
		Date:    "Oct 29, 2026",
		Title:   "Room for the last row",
		Summary: "The final entry in Today can now scroll clear of the bottom nav on iOS instead of resting half-hidden beneath it.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"The scroll area reserves the iOS home-indicator inset, so the last log row can be read in full above the tab bar.",
					"The bottom nav stays flush against the bottom of the screen.",
				},
			},
		},
	},
	{
		Version: "v0.4.12",
		Date:    "Oct 29, 2026",
		Title:   "A bottom bar that reaches the bottom",
		Summary: "On iOS the tab bar now sits flush against the physical screen edge again instead of floating above a strip of empty background.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"The app shell fills the home-indicator strip on iOS, so the bottom nav reaches the bottom of the screen.",
					"The nav labels stay clear of the home indicator, and the page still scrolls inside the shell without moving the header or tab bar.",
				},
			},
		},
	},
	{
		Version: "v0.4.11",
		Date:    "Oct 29, 2026",
		Title:   "A screen that stays put",
		Summary: "On iOS the header and bottom nav no longer drift with the page; only the day's feed scrolls.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"The app now fills exactly one screen on iOS, so the header cannot slide under the status bar and the bottom nav stays flush.",
					"Only the page body scrolls, which removes the small whole-screen shift when browsing the day.",
				},
			},
		},
	},
	{
		Version: "v0.4.10",
		Date:    "Oct 29, 2026",
		Title:   "An Album tab and a + that stays close",
		Summary: "A new Album tab joins the bottom nav, and the quick-log button now stays in the middle of every page — turning into an X when the log is open.",
		Sections: []Section{
			{
				Title: "Added",
				Items: []string{
					"An Album tab directly to the right of Home, with a calm empty page ready for photos.",
				},
			},
			{
				Title: "Changed",
				Items: []string{
					"The quick-log button now sits in the middle of the bottom nav on every page, including Settings.",
					"When quick logging is open the + rotates into an X; tap it to collapse the panel.",
				},
			},
		},
	},
	{
		Version: "v0.4.9",
		Date:    "Oct 27, 2026",
		Title:   "The quick log waits its turn",
		Summary: "Leaving Settings no longer flashes the quick log's floating button over the old page while the next tab loads.",
		Sections: []Section{
			{
				Title: "Fixed",
				Items: []string{
					"The quick log appears with the new tab instead of popping in over the page being left behind.",
					"An open quick log stays in place until Settings has actually loaded.",
				},
			},
		},
	},
	{
		Version: "v0.4.8",
		Date:    "Oct 27, 2026",
		Title:   "Tabs that keep up",
		Summary: "Switching between Home, History, and Settings no longer reloads the whole page, so the timer stays put and the next tab appears right away.",
		Sections: []Section{
			{
				Title: "Changed",
				Items: []string{
					"Tab switches replace only the page body; the header and quick log stay in place.",
					"A running timer keeps going across tabs without a reload or flash.",
					"The tab you tapped highlights straight away while its page loads.",
					"Back and forward move between tabs the same way.",
				},
			},
		},
	},
	{
		Version: "v0.4.7",
		Date:    "Oct 27, 2026",
		Title:   "Sleeping gets the same big card",
		Summary: "The quick log's sleeping card now matches the nursing card, with a large live timer and a clear Wake up button.",
		Sections: []Section{
			{
				Title: "Changed",
				Items: []string{
					"The sleeping card stands as tall as the nursing card.",
					"Sleeping shows the mascot, a live timer, and an edit button while it runs.",
					"Wake up is now its own button instead of the whole card.",
				},
			},
		},
	},
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
