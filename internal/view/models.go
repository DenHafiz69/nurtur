package view

import (
	"time"

	"nurtur/internal/feed"
)

// PageData is the single view model used by the dashboard, fragments, and
// out-of-band updates. Building it is cheap (three small queries).
type PageData struct {
	Nonce      string
	Version    string
	BabyName   string
	Greeting   string
	TodayLabel string
	Now        time.Time
	Location   *time.Location

	Active *ActiveView
	Last   *LastView
	Today  TodaySummary
	Feeds  []FeedRow

	Toast    *Toast
	Edit     *EditView
	EditOpen bool

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

// TodaySummary is the compact strip above Today's list.
type TodaySummary struct {
	Count      int
	CountLabel string
	SplitLabel string
}

// FeedRow is one entry in today's feed list.
type FeedRow struct {
	ID            int64
	Active        bool
	StartLabel    string
	StartedAt     time.Time
	TotalLabel    string
	SplitLabel    string
	LastSide      feed.Side
	LastSideTitle string
}

// Toast is a transient mutation result. Undo arrives in phase 3.
type Toast struct {
	Message string
	Kind    string // success, info, warn, error
}

// EditView backs the quick-fix / manual edit dialog.
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
