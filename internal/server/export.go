package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/sleep"
	"nurtur/internal/view"
)

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	feeds, diapers, sleeps, settings, err := s.exportData(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	now := s.clock.Now().UTC()

	type segmentJSON struct {
		ID        int64  `json:"id"`
		Side      string `json:"side"`
		StartedAt string `json:"started_at"`
		EndedAt   string `json:"ended_at,omitempty"`
	}
	type feedJSON struct {
		ID        int64         `json:"id"`
		StartedAt string        `json:"started_at"`
		EndedAt   string        `json:"ended_at,omitempty"`
		LastSide  string        `json:"last_side,omitempty"`
		Notes     string        `json:"notes,omitempty"`
		CreatedAt string        `json:"created_at"`
		DeletedAt string        `json:"deleted_at,omitempty"`
		Left      int64         `json:"left_minutes"`
		Right     int64         `json:"right_minutes"`
		Segments  []segmentJSON `json:"segments"`
	}
	type diaperJSON struct {
		ID          int64  `json:"id"`
		LoggedAt    string `json:"logged_at"`
		Type        string `json:"type"`
		Color       string `json:"color,omitempty"`
		Consistency string `json:"consistency,omitempty"`
		Notes       string `json:"notes,omitempty"`
		CreatedAt   string `json:"created_at"`
		DeletedAt   string `json:"deleted_at,omitempty"`
	}
	type sleepJSON struct {
		ID        int64  `json:"id"`
		StartedAt string `json:"started_at"`
		EndedAt   string `json:"ended_at,omitempty"`
		Notes     string `json:"notes,omitempty"`
		CreatedAt string `json:"created_at"`
		DeletedAt string `json:"deleted_at,omitempty"`
	}

	payload := struct {
		ExportedAt string            `json:"exported_at"`
		Version    string            `json:"version"`
		BabyName   string            `json:"baby_name"`
		Timezone   string            `json:"timezone"`
		Feeds      []feedJSON        `json:"feeds"`
		Diapers    []diaperJSON      `json:"diapers"`
		Sleeps     []sleepJSON       `json:"sleeps"`
		Settings   map[string]string `json:"settings"`
	}{
		ExportedAt: now.Format(time.RFC3339),
		Version:    s.version,
		BabyName:   s.babyName,
		Timezone:   s.loc.String(),
		Settings:   settings,
	}

	for _, f := range feeds {
		left, right := f.Totals(now)
		item := feedJSON{
			ID:        f.ID,
			StartedAt: view.ISO(f.StartedAt),
			EndedAt:   isoPtr(f.EndedAt),
			LastSide:  string(f.LastSide),
			Notes:     f.Notes,
			CreatedAt: view.ISO(f.CreatedAt),
			DeletedAt: isoPtr(f.DeletedAt),
			Left:      int64(left / time.Minute),
			Right:     int64(right / time.Minute),
		}
		for _, seg := range f.Segments {
			item.Segments = append(item.Segments, segmentJSON{
				ID:        seg.ID,
				Side:      string(seg.Side),
				StartedAt: view.ISO(seg.StartedAt),
				EndedAt:   isoPtr(seg.EndedAt),
			})
		}
		payload.Feeds = append(payload.Feeds, item)
	}
	for _, d := range diapers {
		payload.Diapers = append(payload.Diapers, diaperJSON{
			ID:          d.ID,
			LoggedAt:    view.ISO(d.LoggedAt),
			Type:        string(d.Kind),
			Color:       string(d.Color),
			Consistency: string(d.Consistency),
			Notes:       d.Notes,
			CreatedAt:   view.ISO(d.CreatedAt),
			DeletedAt:   isoPtr(d.DeletedAt),
		})
	}
	for _, session := range sleeps {
		payload.Sleeps = append(payload.Sleeps, sleepJSON{
			ID:        session.ID,
			StartedAt: view.ISO(session.StartedAt),
			EndedAt:   isoPtr(session.EndedAt),
			Notes:     session.Notes,
			CreatedAt: view.ISO(session.CreatedAt),
			DeletedAt: isoPtr(session.DeletedAt),
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="nurtur-%s.json"`, now.In(s.loc).Format("2006-01-02")))
	w.Header().Set("Cache-Control", "no-store")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	feeds, diapers, sleeps, _, err := s.exportData(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	now := s.clock.Now().UTC()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="nurtur-%s.csv"`, now.In(s.loc).Format("2006-01-02")))
	w.Header().Set("Cache-Control", "no-store")

	out := csv.NewWriter(w)
	_ = out.Write([]string{
		"kind", "id", "started_at", "ended_at", "left_minutes", "right_minutes",
		"last_side", "type", "color", "consistency", "notes", "created_at", "deleted_at",
	})

	for _, f := range feeds {
		left, right := f.Totals(now)
		_ = out.Write([]string{
			"feed", strconv.FormatInt(f.ID, 10), view.ISO(f.StartedAt), isoPtr(f.EndedAt),
			strconv.FormatInt(int64(left/time.Minute), 10), strconv.FormatInt(int64(right/time.Minute), 10),
			string(f.LastSide), "", "", "", f.Notes, view.ISO(f.CreatedAt), isoPtr(f.DeletedAt),
		})
	}
	for _, d := range diapers {
		_ = out.Write([]string{
			"diaper", strconv.FormatInt(d.ID, 10), view.ISO(d.LoggedAt), "",
			"", "", "", string(d.Kind), string(d.Color), string(d.Consistency), d.Notes,
			view.ISO(d.CreatedAt), isoPtr(d.DeletedAt),
		})
	}
	for _, session := range sleeps {
		_ = out.Write([]string{
			"sleep", strconv.FormatInt(session.ID, 10), view.ISO(session.StartedAt), isoPtr(session.EndedAt),
			"", "", "", "", "", "", session.Notes, view.ISO(session.CreatedAt), isoPtr(session.DeletedAt),
		})
	}
	out.Flush()
}

func (s *Server) handleExportSQL(w http.ResponseWriter, r *http.Request) {
	feeds, diapers, sleeps, settings, err := s.exportData(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	now := s.clock.Now().UTC()

	w.Header().Set("Content-Type", "application/sql; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="nurtur-%s.sql"`, now.In(s.loc).Format("2006-01-02")))
	w.Header().Set("Cache-Control", "no-store")

	var b strings.Builder
	b.WriteString("-- Nurtur logical export. Replay into a fresh Turso/SQLite database.\n")
	fmt.Fprintf(&b, "-- Exported %s (version %s, timezone %s)\n\n", now.Format(time.RFC3339), s.version, s.loc.String())
	b.WriteString("BEGIN TRANSACTION;\n\n")
	b.WriteString("CREATE TABLE IF NOT EXISTS schema_migrations (\n    version    TEXT PRIMARY KEY,\n    applied_at TEXT NOT NULL\n);\n\n")

	if s.migrations != nil {
		if err := writeSchema(&b, s.migrations); err != nil {
			s.serverError(w, r, err)
			return
		}
	}

	for _, f := range feeds {
		fmt.Fprintf(&b, "INSERT INTO feeds (id, started_at, ended_at, last_side, notes, created_at, deleted_at) VALUES (%d, %s, %s, %s, %s, %s, %s);\n",
			f.ID, sqlText(view.ISO(f.StartedAt)), sqlTimePtr(f.EndedAt), sqlNullable(string(f.LastSide)),
			sqlNullable(f.Notes), sqlText(view.ISO(f.CreatedAt)), sqlTimePtr(f.DeletedAt))
		for _, seg := range f.Segments {
			fmt.Fprintf(&b, "INSERT INTO feed_segments (id, feed_id, side, started_at, ended_at) VALUES (%d, %d, %s, %s, %s);\n",
				seg.ID, seg.FeedID, sqlText(string(seg.Side)), sqlText(view.ISO(seg.StartedAt)), sqlTimePtr(seg.EndedAt))
		}
	}
	for _, d := range diapers {
		fmt.Fprintf(&b, "INSERT INTO diapers (id, logged_at, type, color, consistency, notes, created_at, deleted_at) VALUES (%d, %s, %s, %s, %s, %s, %s, %s);\n",
			d.ID, sqlText(view.ISO(d.LoggedAt)), sqlText(string(d.Kind)), sqlNullable(string(d.Color)),
			sqlNullable(string(d.Consistency)), sqlNullable(d.Notes), sqlText(view.ISO(d.CreatedAt)), sqlTimePtr(d.DeletedAt))
	}
	for _, session := range sleeps {
		fmt.Fprintf(&b, "INSERT INTO sleeps (id, started_at, ended_at, notes, created_at, deleted_at) VALUES (%d, %s, %s, %s, %s, %s);\n",
			session.ID, sqlText(view.ISO(session.StartedAt)), sqlTimePtr(session.EndedAt),
			sqlNullable(session.Notes), sqlText(view.ISO(session.CreatedAt)), sqlTimePtr(session.DeletedAt))
	}
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "INSERT INTO settings (key, value) VALUES (%s, %s);\n", sqlText(key), sqlText(settings[key]))
	}

	// Mark migrations as applied so the app does not try to recreate tables.
	if s.migrations != nil {
		versions, err := migrationVersions(s.migrations)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		for _, version := range versions {
			fmt.Fprintf(&b, "INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (%s, %s);\n",
				sqlText(version), sqlText(now.Format(time.RFC3339)))
		}
	}

	b.WriteString("\nCOMMIT;\n")
	_, _ = io.WriteString(w, b.String())
}

func (s *Server) exportData(r *http.Request) ([]feed.Feed, []diaper.Diaper, []sleep.Session, map[string]string, error) {
	ctx := r.Context()
	feeds, err := s.store.AllFeeds(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	diapers, err := s.store.AllDiapers(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	sleeps, err := s.store.AllSleeps(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return feeds, diapers, sleeps, settings, nil
}

func writeSchema(b *strings.Builder, migrations fs.FS) error {
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		return fmt.Errorf("server: read migrations for export: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := fs.ReadFile(migrations, name)
		if err != nil {
			return fmt.Errorf("server: read migration %s: %w", name, err)
		}
		b.WriteString("-- " + name + "\n")
		b.Write(body)
		b.WriteString("\n\n")
	}
	return nil
}

func migrationVersions(migrations fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		return nil, fmt.Errorf("server: read migrations: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func isoPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return view.ISO(*t)
}

func sqlText(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func sqlNullable(value string) string {
	if value == "" {
		return "NULL"
	}
	return sqlText(value)
}

func sqlTimePtr(t *time.Time) string {
	if t == nil {
		return "NULL"
	}
	return sqlText(view.ISO(*t))
}
