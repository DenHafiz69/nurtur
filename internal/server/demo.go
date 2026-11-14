package server

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"net/http"
	"time"

	"nurtur/internal/clock"
	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/intake"
	"nurtur/internal/store"
	"nurtur/internal/view"
)

func (s *Server) handleDemo(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if _, _, _, ok := s.auth.session(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.startDemoSession(w, r)
}

func (s *Server) handleDemoReset(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil || !isDemoFrom(r.Context()) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.startDemoSession(w, r)
}

func (s *Server) handleDemoExit(w http.ResponseWriter, r *http.Request) {
	if s.auth != nil {
		s.auth.clearSession(w, r)
	}
	target := "/landing"
	if s.auth != nil && s.auth.enabled() {
		target = "/"
	}
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) startDemoSession(w http.ResponseWriter, r *http.Request) {
	demoID, err := newDemoAccountID()
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.seedDemoAccount(r.Context(), demoID); err != nil {
		s.serverError(w, r, err)
		return
	}
	_, csrf, err := s.auth.newDemoSession(demoID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.auth.setDemoSession(w, r, demoID, csrf)
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func newDemoAccountID() (int64, error) {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	n := int64(binary.BigEndian.Uint32(raw[:]) & 0x7fffffff)
	return -(1000 + n), nil
}

func (s *Server) seedDemoAccount(ctx context.Context, demoID int64) error {
	demoCtx := store.WithAccount(ctx, demoID)
	now := s.clock.Now().UTC()
	dayStart, _ := clock.DayBounds(now, s.loc)

	settings := map[string]string{
		settingBabyName:      "Aisyah",
		settingBabyBirthDate: now.AddDate(0, -4, -12).Format("2006-01-02"),
		settingBabyGender:    babyGenderGirl,
		settingMascot:        view.MascotPip,
		settingFeedingMode:   "hybrid",
		settingTrackBottles:  "1",
		settingTrackPumps:    "1",
		settingTrackSolids:   "1",
		settingVolumeUnit:    string(intake.UnitML),
	}
	if s.version != "" {
		settings[settingChangelogSeen] = s.version
	}
	if err := s.store.SetSettings(demoCtx, settings); err != nil {
		return err
	}

	// Place sample events within today so Today's summary and timeline feel
	// alive at any hour of the day.
	elapsedToday := now.Sub(dayStart)
	if elapsedToday < 35*time.Minute {
		// Just after midnight: space a few recent logs over the last 25 minutes.
		return s.seedDemoEvents(demoCtx, now,
			now.Add(-24*time.Minute),
			now.Add(-20*time.Minute), now.Add(-14*time.Minute),
			now.Add(-12*time.Minute),
			now.Add(-9*time.Minute), now.Add(-6*time.Minute),
			now.Add(-4*time.Minute),
			now.Add(-2*time.Minute),
		)
	}

	scale := func(d time.Duration) time.Time {
		if d > elapsedToday-5*time.Minute {
			d = elapsedToday - 5*time.Minute
		}
		return now.Add(-d)
	}

	sleepStart := scale(3 * time.Hour)
	sleepEnd := sleepStart.Add(48 * time.Minute)
	if !sleepEnd.Before(now.Add(-10 * time.Minute)) {
		sleepEnd = sleepStart.Add(elapsedToday / 6)
	}
	feedStart := scale(95 * time.Minute)
	feedEnd := feedStart.Add(11 * time.Minute)
	if !feedEnd.Before(now.Add(-5 * time.Minute)) {
		feedEnd = feedStart.Add(5 * time.Minute)
	}

	return s.seedDemoEvents(demoCtx, now,
		scale(4*time.Hour),
		sleepStart, sleepEnd,
		scale(2*time.Hour+15*time.Minute),
		feedStart, feedEnd,
		scale(45*time.Minute),
		scale(18*time.Minute),
	)
}

func (s *Server) seedDemoEvents(
	ctx context.Context,
	now time.Time,
	solidAt, sleepStart, sleepEnd, pumpAt, feedStart, feedEnd, bottleAt, diaperAt time.Time,
) error {
	if _, err := s.store.CreateSolid(ctx, "Oatmeal & banana", intake.PortionTaste, intake.ReactionNone, solidAt, now, "First spoonfuls"); err != nil {
		return err
	}
	if _, err := s.store.CreateSleep(ctx, sleepStart, sleepEnd, "Morning nap", now); err != nil {
		return err
	}
	if _, err := s.store.CreatePump(ctx, intake.SideBoth, 110, 15, pumpAt, now, "Morning session"); err != nil {
		return err
	}
	f, err := s.store.CreateFeed(ctx, feed.SideRight, feedStart)
	if err != nil {
		return err
	}
	if _, err := s.store.StopFeed(ctx, f.ID, feedEnd); err != nil {
		return err
	}
	if _, _, err := s.store.CreateBottle(ctx, intake.MilkBreast, 90, bottleAt, now, "Warmed", false); err != nil {
		return err
	}
	if _, _, err := s.store.CreateDiaper(ctx, diaper.Wet, diaperAt); err != nil {
		return err
	}
	return nil
}
