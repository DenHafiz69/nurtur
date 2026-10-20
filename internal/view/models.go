package view

import (
	"time"

	"nurtur/internal/feed"
)

// PageData is the single view model used by the dashboard, fragments, and
// out-of-band updates. Building it is cheap: a handful of small queries.
type PageData struct {
	Nonce      string
	Version    string
	BabyName   string
	Greeting   string
	TodayLabel string
	Now        time.Time
	Location   *time.Location

	// View selects the page body inside layout.html: "" or "dashboard",
	// "timeline", "settings".
	View string

	Active *ActiveView
	Last   *LastView
	Status StatusView
	Sleep  SleepView
	Today  TodaySummary
	Events []EventRow

	Timeline *TimelinePageView
	Settings *SettingsView
	Row      *EventRow

	// DeletedKey is the DOM id suffix for a deleted timeline row's placeholder.
	DeletedKey string

	// CSRF is set only when the optional passcode gate is enabled.
	CSRF        string
	AuthEnabled bool
	SyncChip    *SyncChipView

	// Login page state.
	LoginError string
	Next       string

	Toast     *Toast
	Edit      *EditView
	EditOpen  bool
	Sheet     string
	SheetOpen bool

	NursingSheet *NursingSheetView
	DiaperSheet  *DiaperSheetView
	DiaperEdit   *DiaperEditView
	SleepSheet   *SleepSheetView
	SleepEdit    *SleepEditView

	// TimerOOB adds hx-swap-oob when the timer is a companion fragment rather
	// than the primary response target.
	TimerOOB bool
}

// ActiveView is the running feed shown in the dock and header badge.
type ActiveView struct {
	ID            int64
	Side          feed.Side
	SideTitle     string
	Opposite      feed.Side
	OppositeTitle string
	StartedAt     time.Time
	ServerNow     time.Time
	Elapsed       time.Duration
	SplitLabel    string
}

// LastView is the most recent completed feed.
type LastView struct {
	SideTitle string
	Since     time.Time
	Elapsed   time.Duration
}

// StatusView is the three-card strip at the top of the dashboard.
type StatusView struct {
	Nursing StatusCard
	Diaper  StatusCard
	Sleep   StatusCard
}

// StatusCard is one glanceable card. ValueSince is set when the value should
// keep ticking client-side; ValueMode is "ago" or "duration".
type StatusCard struct {
	Kind           string
	Label          string
	Value          string
	ValueSince     time.Time
	ValueMode      string
	Detail         string
	Secondary      string
	SecondaryValue string
	SecondarySince *time.Time
	SecondaryMode  string
	HasData        bool
	Active         bool
	Tone           string
	Icon           string
	Href           string
}

// SleepView backs the dock's sleep pill.
type SleepView struct {
	Active    bool
	ID        int64
	StartedAt time.Time
	ServerNow time.Time
	Elapsed   time.Duration
}

// TodaySummary is the compact line above today's events.
type TodaySummary struct {
	Count       int
	CountLabel  string
	SplitLabel  string
	DiaperLabel string
	SleepLabel  string
	HasEvents   bool
}

// EventRow is one entry in today's unified list or the full timeline.
type EventRow struct {
	Kind         string // feed, diaper, sleep
	ID           int64
	Anchor       time.Time
	TimeLabel    string
	Title        string
	Detail       string
	Active       bool
	Icon         string
	EditURL      string
	EditTarget   string
	DeleteURL    string
	DeleteTarget string
	ActionLabel  string
	KindClass    string

	// Timeline rows are deleted in place and can be restored into the same
	// spot during the undo window.
	Timeline  bool
	RowID     string
	DeleteKey string
}

// TimelinePageView is one page of the unified history, grouped by local day.
type TimelinePageView struct {
	Days       []TimelineDay
	HasDays    bool
	HasMore    bool
	NextCursor string
	Count      int
}

// TimelineDay is one local-date group with its own summary.
type TimelineDay struct {
	Key     string
	Label   string
	Summary string
	Rows    []EventRow
}

// SyncChipView is the header's sync state indicator.
type SyncChipView struct {
	Label string
	Tone  string // ok, pending, warn, local
	Title string
}

// SettingsView backs the settings page.
type SettingsView struct {
	BabyName    string
	Timezone    string
	DBPath      string
	DBSize      string
	Version     string
	AuthEnabled bool
	NightStart  string
	NightEnd    string
	NightErr    string
	SyncMode    string
	SyncStatus  string
	SyncDetail  string
	LastPush    string
	LastPull    string
	Pending     string
	Degraded    bool
	ThemeNote   string
}

// Toast is a transient mutation result. An action turns it into a quick
// follow-up ("Add details", "Undo").
type Toast struct {
	Message      string
	Kind         string // success, info, warn, error
	ActionLabel  string
	ActionURL    string
	ActionMethod string // get (default) or post
	ActionTarget string
	DurationMS   int
}

// EditView backs the quick-fix / manual feed edit dialog.
type EditView struct {
	ID           int64
	Active       bool
	Title        string
	SideTitle    string
	StartInput   string
	EndInput     string
	LeftMinutes  int
	RightMinutes int

	StartErr  string
	EndErr    string
	LeftErr   string
	RightErr  string
	TotalsErr string
}

// ChipOption is one selectable detail in a sheet.
type ChipOption struct {
	Value    string
	Label    string
	Selected bool
}

// DiaperRow is a compact diaper entry in the diaper sheet.
type DiaperRow struct {
	ID        int64
	TimeLabel string
	Title     string
	Detail    string
	EditURL   string
}

// DiaperSheetView backs the diaper action sheet.
type DiaperSheetView struct {
	Recent     []DiaperRow
	HasWet     bool
	WetSince   time.Time
	DirtySince time.Time
	HasDirty   bool
	LastWet    string
	LastDirty  string
	ServerNow  time.Time
}

// DiaperEditView backs the optional details form.
type DiaperEditView struct {
	ID            int64
	KindTitle     string
	TimeInput     string
	Color         string
	Consistency   string
	Notes         string
	Colors        []ChipOption
	Consistencies []ChipOption
	TimeErr       string
	ColorErr      string
	ConsistErr    string
	NotesErr      string
}

// SleepRow is a compact sleep entry in the sleep sheet.
type SleepRow struct {
	ID        int64
	TimeLabel string
	Title     string
	Detail    string
	Active    bool
	EditURL   string
}

// SleepSheetView backs the sleep action sheet.
type SleepSheetView struct {
	Active     bool
	ID         int64
	StartedAt  time.Time
	ServerNow  time.Time
	Elapsed    time.Duration
	StartInput string
	EndInput   string
	StartErr   string
	EndErr     string
	NotesErr   string
	Recent     []SleepRow
	HasRecent  bool
	DayLabel   string
	NightLabel string
	TotalLabel string
}

// SleepEditView backs the sleep correction dialog.
type SleepEditView struct {
	ID         int64
	Active     bool
	StartInput string
	EndInput   string
	Notes      string
	StartErr   string
	EndErr     string
	NotesErr   string
}

// NursingSheetView backs the nursing action sheet.
type NursingSheetView struct {
	Active    bool
	ID        int64
	SideTitle string
	StartedAt time.Time
	ServerNow time.Time
	Elapsed   time.Duration
	HasLast   bool
	LastSide  string
	LastAgo   string
}
