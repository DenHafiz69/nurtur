package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nurtur/internal/analytics"
	"nurtur/internal/clock"
	"nurtur/internal/view"
)

func parseInsightsRange(raw string) int {
	raw = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), "d")
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 7
	}
	return analytics.NormalizeRange(n)
}

func (s *Server) buildInsights(ctx context.Context, rangeDays int, growthMetric string) (*view.InsightsView, error) {
	now := s.clock.Now().UTC()
	days := analytics.NormalizeRange(rangeDays)
	metric := analytics.NormalizeGrowthMetric(growthMetric)

	localNow := now.In(s.loc)
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, s.loc)
	rangeStart, _ := clock.DayBounds(todayStart.AddDate(0, 0, -(days-1)), s.loc)
	_, rangeEnd := clock.DayBounds(todayStart, s.loc)

	feeds, err := s.store.FeedsInRange(ctx, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}
	bottles, err := s.store.BottlesInRange(ctx, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}
	pumps, err := s.store.PumpsInRange(ctx, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}
	solids, err := s.store.SolidsInRange(ctx, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}
	diapers, err := s.store.DiapersInRange(ctx, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}
	sleeps, err := s.store.SleepsInRange(ctx, rangeStart, rangeEnd, now)
	if err != nil {
		return nil, err
	}
	growthRows, err := s.store.LiveGrowth(ctx)
	if err != nil {
		return nil, err
	}

	profile, err := s.currentBabyProfile(ctx)
	if err != nil {
		return nil, err
	}
	trackers := s.trackerSettings(ctx)
	nightStart, nightEnd := s.nightWindow(ctx)

	return analytics.Build(analytics.Input{
		Now:          now,
		Loc:          s.loc,
		RangeDays:    days,
		GrowthMetric: metric,
		GrowthUnit:   s.growthUnit(ctx),
		VolumeUnit:   s.volumeUnit(ctx),
		FeedingMode:  trackers.FeedingMode,
		TrackSolids:  trackers.Solids,
		NightStart:   nightStart,
		NightEnd:     nightEnd,
		BirthDate:    profile.BirthDate,
		Gender:       profile.Gender,
		BabyName:     profile.Name,
		Feeds:        feeds,
		Bottles:      bottles,
		Pumps:        pumps,
		Solids:       solids,
		Diapers:      diapers,
		Sleeps:       sleeps,
		Growth:       growthRows,
	}), nil
}

func (s *Server) handleInsightsPage(w http.ResponseWriter, r *http.Request) {
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rangeDays := parseInsightsRange(r.URL.Query().Get("range"))
	metric := analytics.NormalizeGrowthMetric(r.URL.Query().Get("growth"))
	insights, err := s.buildInsights(r.Context(), rangeDays, metric)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "insights"
	data.Insights = insights
	s.renderPage(w, r, http.StatusOK, data)
}

func (s *Server) handleInsightsPartial(w http.ResponseWriter, r *http.Request) {
	rangeDays := parseInsightsRange(r.URL.Query().Get("range"))
	metric := analytics.NormalizeGrowthMetric(r.URL.Query().Get("growth"))
	insights, err := s.buildInsights(r.Context(), rangeDays, metric)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data, err := s.buildPage(r.Context(), nonceFrom(r.Context()), nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.View = "insights"
	data.Insights = insights
	s.writeFragments(w, r, http.StatusOK, "insights-body", data, false, nil, "", false, "", "")
}
