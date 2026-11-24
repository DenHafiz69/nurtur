// Package analytics aggregates feeds, bottles, pumps, solids, diapers, sleeps,
// and growth measurements into chart-ready view models for the Trends tab.
// It has no database or HTTP dependencies.
package analytics

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"nurtur/internal/clock"
	"nurtur/internal/diaper"
	"nurtur/internal/feed"
	"nurtur/internal/growth"
	"nurtur/internal/intake"
	"nurtur/internal/sleep"
	"nurtur/internal/view"
)

// Input supplies every raw domain slice and account preference needed to build
// the Trends page.
type Input struct {
	Now          time.Time
	Loc          *time.Location
	RangeDays    int
	GrowthMetric string
	GrowthUnit   growth.Unit
	VolumeUnit   intake.Unit
	FeedingMode  string
	TrackSolids  bool
	NightStart   time.Duration
	NightEnd     time.Duration
	BirthDate    time.Time
	Gender       string
	BabyName     string

	Feeds   []feed.Feed
	Bottles []intake.Bottle
	Pumps   []intake.Pump
	Solids  []intake.Solid
	Diapers []diaper.Diaper
	Sleeps  []sleep.Session
	Growth  []growth.Measurement
}

type dayWindow struct {
	start   time.Time
	end     time.Time
	key     string
	short   string
	fullDay string
}

// NormalizeRange clamps the requested window to 7, 14, or 30 days.
func NormalizeRange(days int) int {
	switch days {
	case 14:
		return 14
	case 30:
		return 30
	default:
		return 7
	}
}

// NormalizeGrowthMetric clamps the selected growth metric to weight, height, or head.
func NormalizeGrowthMetric(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "height":
		return "height"
	case "head":
		return "head"
	default:
		return "weight"
	}
}

// Build computes all charts, averages, routine strips, and the checkup snapshot.
func Build(in Input) *view.InsightsView {
	loc := in.Loc
	if loc == nil {
		loc = time.UTC
	}
	now := in.Now.UTC()
	days := NormalizeRange(in.RangeDays)
	metric := NormalizeGrowthMetric(in.GrowthMetric)
	windows := buildDayWindows(now, loc, days)

	growthView := buildGrowthInsights(in.Growth, metric, in.GrowthUnit, in.BirthDate, in.Gender, loc)
	feedingView := buildFeedingInsights(in.Feeds, in.Bottles, in.Pumps, windows, now, in.VolumeUnit, in.FeedingMode)
	sleepView := buildSleepInsights(in.Sleeps, windows, now, loc, in.NightStart, in.NightEnd)
	patternView := buildPatternStrip(in.Sleeps, in.Feeds, in.Bottles, windows, now, loc, in.NightStart, in.NightEnd)
	diaperView := buildDiaperInsights(in.Diapers, windows, now)
	solidsView := buildSolidsInsights(in.Solids, in.TrackSolids)
	snapshot := buildSnapshot(in.BabyName, days, growthView, feedingView, sleepView, diaperView)

	return &view.InsightsView{
		RangeDays:    days,
		RangeLabel:   fmt.Sprintf("Past %d days", days),
		GrowthMetric: metric,
		Growth:       growthView,
		Feeding:      feedingView,
		Sleep:        sleepView,
		Pattern:      patternView,
		Diapers:      diaperView,
		Solids:       solidsView,
		Snapshot:     snapshot,
	}
}

func buildDayWindows(now time.Time, loc *time.Location, days int) []dayWindow {
	localNow := now.In(loc)
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	out := make([]dayWindow, 0, days)
	for i := days - 1; i >= 0; i-- {
		d := todayStart.AddDate(0, 0, -i)
		start, end := clock.DayBounds(d, loc)
		label := d.Format("Mon")
		if days > 7 {
			label = d.Format("Jan 2")
		}
		if i == 0 {
			label = "Today"
		}
		out = append(out, dayWindow{
			start:   start,
			end:     end,
			key:     d.Format("2006-01-02"),
			short:   label,
			fullDay: d.Format("Mon, Jan 2"),
		})
	}
	return out
}

/* ---------- Growth ---------- */

func buildGrowthInsights(
	rows []growth.Measurement,
	metric string,
	unit growth.Unit,
	birthDate time.Time,
	gender string,
	loc *time.Location,
) view.GrowthInsightsView {
	out := view.GrowthInsightsView{
		Metric:      metric,
		MetricLabel: growthMetricLabel(metric),
		UnitLabel:   growthUnitLabel(metric, unit),
	}

	type sample struct {
		at        time.Time
		metricVal float64 // kg or cm (for WHO)
		dispVal   float64 // display unit
		formatted string
	}
	var pts []sample
	for _, r := range rows {
		if r.DeletedAt != nil {
			continue
		}
		var mVal, dVal float64
		var formatted string
		switch metric {
		case "weight":
			if r.WeightG <= 0 {
				continue
			}
			mVal = float64(r.WeightG) / 1000.0
			dVal = mVal
			if unit == growth.UnitImperial {
				dVal = float64(r.WeightG) / gramsPerLb
			}
			formatted = growth.FormatWeight(r.WeightG, unit)
		case "height":
			if r.HeightMM <= 0 {
				continue
			}
			mVal = float64(r.HeightMM) / 10.0
			dVal = mVal
			if unit == growth.UnitImperial {
				dVal = mVal / cmPerInch
			}
			formatted = growth.FormatLength(r.HeightMM, unit)
		case "head":
			if r.HeadMM <= 0 {
				continue
			}
			mVal = float64(r.HeadMM) / 10.0
			dVal = mVal
			if unit == growth.UnitImperial {
				dVal = mVal / cmPerInch
			}
			formatted = growth.FormatLength(r.HeadMM, unit)
		}
		pts = append(pts, sample{
			at:        r.LoggedAt.UTC(),
			metricVal: mVal,
			dispVal:   dVal,
			formatted: formatted,
		})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].at.Before(pts[j].at) })

	out.Count = len(pts)
	if len(pts) == 0 {
		return out
	}
	out.HasData = true
	latest := pts[len(pts)-1]
	out.LatestValue = latest.formatted
	out.LatestDate = latest.at.In(loc).Format("Jan 2, 2006")

	if len(pts) >= 2 {
		prev := pts[len(pts)-2]
		diff := latest.dispVal - prev.dispVal
		sign := "+"
		if diff < 0 {
			sign = ""
		}
		daysBetween := int(math.Round(latest.at.Sub(prev.at).Hours() / 24.0))
		unitStr := out.UnitLabel
		if metric == "weight" {
			if daysBetween > 0 {
				out.DeltaLabel = fmt.Sprintf("%s%.2f %s in %dd", sign, diff, unitStr, daysBetween)
			} else {
				out.DeltaLabel = fmt.Sprintf("%s%.2f %s", sign, diff, unitStr)
			}
		} else {
			if daysBetween > 0 {
				out.DeltaLabel = fmt.Sprintf("%s%.1f %s in %dd", sign, diff, unitStr, daysBetween)
			} else {
				out.DeltaLabel = fmt.Sprintf("%s%.1f %s", sign, diff, unitStr)
			}
		}
	}

	canWHO := !birthDate.IsZero() && (gender == "boy" || gender == "girl")
	if canWHO {
		ageDays := latest.at.Sub(birthDate).Hours() / 24.0
		if pct := EstimatePercentile(metric, gender, ageDays, latest.metricVal); pct != "" {
			out.PercentileLabel = pct
		}
	}

	// Build SVG line chart against calendar dates.
	const (
		svgW = 320
		svgH = 176
		padL = 42.0
		padR = 14.0
		padT = 14.0
		padB = 28.0
	)
	plotW := float64(svgW) - padL - padR
	plotH := float64(svgH) - padT - padB

	minT := pts[0].at
	maxT := pts[len(pts)-1].at
	if maxT.Equal(minT) {
		// Single point: pad +/- 7 days so the point sits centered and WHO bands render.
		minT = minT.AddDate(0, 0, -7)
		maxT = maxT.AddDate(0, 0, 7)
	}
	spanSec := maxT.Sub(minT).Seconds()
	if spanSec <= 0 {
		spanSec = 1
	}

	// Sample WHO bands across the calendar span if within 0–24 months.
	var whoSamples []whoSamplePoint
	if canWHO {
		const steps = 16
		for i := 0; i <= steps; i++ {
			frac := float64(i) / float64(steps)
			t := minT.Add(time.Duration(frac*spanSec) * time.Second)
			ageDays := t.Sub(birthDate).Hours() / 24.0
			if band, ok := whoBandMetric(metric, gender, ageDays); ok {
				whoSamples = append(whoSamples, whoSamplePoint{
					t:   t,
					row: convertWHORow(band, metric, unit),
				})
			}
		}
	}

	minY := pts[0].dispVal
	maxY := pts[0].dispVal
	for _, p := range pts {
		if p.dispVal < minY {
			minY = p.dispVal
		}
		if p.dispVal > maxY {
			maxY = p.dispVal
		}
	}
	for _, ws := range whoSamples {
		if ws.row.p3 < minY {
			minY = ws.row.p3
		}
		if ws.row.p97 > maxY {
			maxY = ws.row.p97
		}
	}
	if maxY-minY < 0.2 {
		minY -= 0.5
		maxY += 0.5
	} else {
		pad := (maxY - minY) * 0.12
		minY -= pad
		maxY += pad
	}
	if minY < 0 {
		minY = 0
	}

	xOf := func(t time.Time) float64 {
		return padL + (t.Sub(minT).Seconds()/spanSec)*plotW
	}
	yOf := func(v float64) float64 {
		return padT + (1.0-(v-minY)/(maxY-minY))*plotH
	}

	chart := view.LineChart{
		Width:      svgW,
		Height:     svgH,
		PlotLeft:   padL,
		PlotRight:  float64(svgW) - padR,
		PlotTop:    padT,
		PlotBottom: float64(svgH) - padB,
	}

	if len(whoSamples) >= 2 {
		chart.HasWHO = true
		out.WHONote = "Shaded bands: WHO 3rd–97th & 15th–85th percentiles (dashed = 50th)"
		chart.BandOuterPoints = buildWHOPolygon(whoSamples, xOf, yOf, func(r whoRow) float64 { return r.p97 }, func(r whoRow) float64 { return r.p3 })
		chart.BandInnerPoints = buildWHOPolygon(whoSamples, xOf, yOf, func(r whoRow) float64 { return r.p85 }, func(r whoRow) float64 { return r.p15 })
		var medParts []string
		for _, ws := range whoSamples {
			medParts = append(medParts, fmt.Sprintf("%.1f,%.1f", xOf(ws.t), yOf(ws.row.p50)))
		}
		chart.MedianPoints = strings.Join(medParts, " ")
	}

	var lineParts []string
	var areaParts []string
	for _, p := range pts {
		x := xOf(p.at)
		y := yOf(p.dispVal)
		lineParts = append(lineParts, fmt.Sprintf("%.1f,%.1f", x, y))
		areaParts = append(areaParts, fmt.Sprintf("%.1f,%.1f", x, y))
		sub := ""
		if canWHO {
			sub = EstimatePercentile(metric, gender, p.at.Sub(birthDate).Hours()/24.0, p.metricVal)
		}
		chart.Points = append(chart.Points, view.ChartPoint{
			X:        round1(x),
			Y:        round1(y),
			TipTitle: p.at.In(loc).Format("Jan 2, 2006"),
			TipValue: p.formatted,
			TipSub:   sub,
		})
	}
	if len(lineParts) >= 2 {
		chart.LinePoints = strings.Join(lineParts, " ")
		firstX := xOf(pts[0].at)
		lastX := xOf(pts[len(pts)-1].at)
		areaParts = append(areaParts,
			fmt.Sprintf("%.1f,%.1f", lastX, chart.PlotBottom),
			fmt.Sprintf("%.1f,%.1f", firstX, chart.PlotBottom),
		)
		chart.AreaPoints = strings.Join(areaParts, " ")
	}

	// Y-axis ticks (3 ticks: min, mid, max).
	for i := 0; i <= 2; i++ {
		frac := float64(i) / 2.0
		val := minY + (maxY-minY)*frac
		lbl := fmt.Sprintf("%.1f", val)
		if metric == "weight" && maxY-minY < 3 {
			lbl = fmt.Sprintf("%.2f", val)
		}
		chart.YTicks = append(chart.YTicks, view.ChartTick{
			Pos:   round1(yOf(val)),
			Label: lbl,
		})
	}

	// X-axis calendar date ticks (start, mid, end).
	if len(pts) == 1 {
		chart.XTicks = []view.ChartTick{
			{Pos: round1(xOf(pts[0].at)), Label: pts[0].at.In(loc).Format("Jan 2")},
		}
	} else {
		midT := minT.Add(maxT.Sub(minT) / 2)
		chart.XTicks = []view.ChartTick{
			{Pos: round1(padL), Label: minT.In(loc).Format("Jan 2")},
			{Pos: round1(padL + plotW/2), Label: midT.In(loc).Format("Jan 2")},
			{Pos: round1(float64(svgW) - padR), Label: maxT.In(loc).Format("Jan 2")},
		}
	}

	out.Chart = chart
	return out
}

type whoSamplePoint struct {
	t   time.Time
	row whoRow
}

func buildWHOPolygon(
	items []whoSamplePoint,
	xOf func(time.Time) float64,
	yOf func(float64) float64,
	topFn, botFn func(whoRow) float64,
) string {
	parts := make([]string, 0, len(items)*2)
	for _, ws := range items {
		parts = append(parts, fmt.Sprintf("%.1f,%.1f", xOf(ws.t), yOf(topFn(ws.row))))
	}
	for i := len(items) - 1; i >= 0; i-- {
		ws := items[i]
		parts = append(parts, fmt.Sprintf("%.1f,%.1f", xOf(ws.t), yOf(botFn(ws.row))))
	}
	return strings.Join(parts, " ")
}

func growthMetricLabel(metric string) string {
	switch metric {
	case "height":
		return "Height / length"
	case "head":
		return "Head circumference"
	default:
		return "Weight"
	}
}

func growthUnitLabel(metric string, unit growth.Unit) string {
	if metric == "weight" {
		if unit == growth.UnitImperial {
			return "lb"
		}
		return "kg"
	}
	if unit == growth.UnitImperial {
		return "in"
	}
	return "cm"
}

/* ---------- Feeding & Milk ---------- */

func buildFeedingInsights(
	feeds []feed.Feed,
	bottles []intake.Bottle,
	pumps []intake.Pump,
	windows []dayWindow,
	now time.Time,
	volUnit intake.Unit,
	feedingMode string,
) view.FeedingInsightsView {
	n := len(windows)
	out := view.FeedingInsightsView{
		ShowPumps: feedingMode == "hybrid" || feedingMode == "exclusive_pump" || len(pumps) > 0,
	}
	if n == 0 {
		return out
	}

	breastML := make([]float64, n)
	formulaML := make([]float64, n)
	nurseMin := make([]float64, n)
	var (
		totalBreastML  int
		totalFormulaML int
		totalPumpML    int
		totalFeeds     int
		totalLeft      time.Duration
		totalRight     time.Duration
	)

	for _, b := range bottles {
		if b.DeletedAt != nil {
			continue
		}
		idx := dayIndex(windows, b.LoggedAt)
		if idx < 0 {
			continue
		}
		if b.MilkType == intake.MilkFormula {
			formulaML[idx] += float64(b.AmountML)
			totalFormulaML += b.AmountML
		} else {
			breastML[idx] += float64(b.AmountML)
			totalBreastML += b.AmountML
		}
	}
	for _, p := range pumps {
		if p.DeletedAt != nil {
			continue
		}
		if dayIndex(windows, p.LoggedAt) >= 0 {
			totalPumpML += p.AmountML
		}
	}
	for _, f := range feeds {
		if f.DeletedAt != nil {
			continue
		}
		idx := dayIndex(windows, f.StartedAt)
		if idx < 0 {
			continue
		}
		left, right := f.Totals(now)
		totalFeeds++
		totalLeft += left
		totalRight += right
		nurseMin[idx] += (left + right).Minutes()
	}

	totalMilkML := totalBreastML + totalFormulaML
	if totalMilkML > 0 {
		out.HasBottles = true
		avgML := int(math.Round(float64(totalMilkML) / float64(n)))
		out.AvgMilkPerDay = intake.FormatVolume(avgML, volUnit) + " / day"
		if totalBreastML > 0 && totalFormulaML > 0 {
			out.MilkSplitLabel = fmt.Sprintf("%s breast · %s formula",
				intake.FormatVolume(int(math.Round(float64(totalBreastML)/float64(n))), volUnit),
				intake.FormatVolume(int(math.Round(float64(totalFormulaML)/float64(n))), volUnit),
			)
		} else if totalFormulaML > 0 {
			out.MilkSplitLabel = "Formula"
		} else {
			out.MilkSplitLabel = "Breast milk"
		}
		chart := buildBarChart(windows, breastML, formulaML, "Breast milk", "Formula", func(v float64) string {
			return intake.FormatVolume(int(math.Round(v)), volUnit)
		}, func(v float64) string {
			if volUnit == intake.UnitOZ {
				return fmt.Sprintf("%.0foz", v/29.5735)
			}
			return fmt.Sprintf("%.0f", v)
		})
		out.MilkChart = &chart
	}

	if totalFeeds > 0 {
		out.HasFeeds = true
		out.AvgFeedsPerDay = fmt.Sprintf("%.1f / day", float64(totalFeeds)/float64(n))
		totalDur := totalLeft + totalRight
		avgPerDay := time.Duration(float64(totalDur) / float64(n))
		avgPerFeed := time.Duration(float64(totalDur) / float64(totalFeeds))
		out.AvgNursePerDay = view.FormatDuration(avgPerDay) + " / day"
		out.AvgFeedDuration = view.FormatDuration(avgPerFeed) + " avg"
		if totalDur > 0 {
			out.HasBalance = true
			out.LeftPct = int(math.Round(float64(totalLeft) / float64(totalDur) * 100))
			out.RightPct = 100 - out.LeftPct
		}
		chart := buildBarChart(windows, nurseMin, nil, "Nursing", "", func(v float64) string {
			return view.FormatDuration(time.Duration(math.Round(v)) * time.Minute)
		}, func(v float64) string {
			return fmt.Sprintf("%.0fm", v)
		})
		out.NurseChart = &chart
	}

	if totalPumpML > 0 {
		out.HasPumps = true
		avgPump := int(math.Round(float64(totalPumpML) / float64(n)))
		out.AvgPumpPerDay = intake.FormatVolume(avgPump, volUnit) + " / day"
		if totalMilkML > 0 {
			avgNet := int(math.Round(float64(totalPumpML-totalMilkML) / float64(n)))
			switch {
			case avgNet > 0:
				out.NetPumpLabel = "+" + intake.FormatVolume(avgNet, volUnit) + "/day surplus"
			case avgNet < 0:
				out.NetPumpLabel = "-" + intake.FormatVolume(-avgNet, volUnit) + "/day vs bottles"
			default:
				out.NetPumpLabel = "Balanced with bottles"
			}
		}
	}

	return out
}

/* ---------- Sleep ---------- */

func buildSleepInsights(
	sleeps []sleep.Session,
	windows []dayWindow,
	now time.Time,
	loc *time.Location,
	nightStart, nightEnd time.Duration,
) view.SleepInsightsView {
	n := len(windows)
	var out view.SleepInsightsView
	if n == 0 {
		return out
	}

	nightHours := make([]float64, n)
	dayHours := make([]float64, n)
	var (
		totalNight     time.Duration
		totalDay       time.Duration
		longestStretch time.Duration
		longestAt      time.Time
		liveSleeps     []sleep.Session
	)

	for _, s := range sleeps {
		if s.DeletedAt != nil {
			continue
		}
		liveSleeps = append(liveSleeps, s)
		end := now
		if s.EndedAt != nil {
			end = *s.EndedAt
		}
		if !end.After(s.StartedAt) {
			continue
		}

		// Attribute overlap with each day window.
		single := []sleep.Session{s}
		for i, w := range windows {
			oStart := s.StartedAt
			if oStart.Before(w.start) {
				oStart = w.start
			}
			oEnd := end
			if oEnd.After(w.end) {
				oEnd = w.end
			}
			if !oEnd.After(oStart) {
				continue
			}
			split := sleep.SplitRange(single, w.start, w.end, now, loc, nightStart, nightEnd)
			nightHours[i] += split.Night.Hours()
			dayHours[i] += split.Day.Hours()
			totalNight += split.Night
			totalDay += split.Day
		}

		// Longest uninterrupted night stretch (any session overlapping night hours).
		fullSplit := sleep.SplitRange(single, s.StartedAt, end, now, loc, nightStart, nightEnd)
		if fullSplit.Night > 0 {
			dur := end.Sub(s.StartedAt)
			if dur > longestStretch {
				longestStretch = dur
				longestAt = s.StartedAt
			}
		}
	}

	if totalNight+totalDay == 0 {
		return out
	}
	out.HasData = true
	out.AvgTotalPerDay = view.FormatDuration(time.Duration(float64(totalNight+totalDay) / float64(n)))
	out.AvgNightPerDay = view.FormatDuration(time.Duration(float64(totalNight) / float64(n)))
	out.AvgDayPerDay = view.FormatDuration(time.Duration(float64(totalDay) / float64(n)))
	if longestStretch > 0 {
		out.LongestStretch = view.FormatDuration(longestStretch)
		out.LongestDate = longestAt.In(loc).Format("Jan 2")
	}

	// Average daytime wake window: gaps between consecutive completed sleeps in [20m, 8h].
	sort.Slice(liveSleeps, func(i, j int) bool { return liveSleeps[i].StartedAt.Before(liveSleeps[j].StartedAt) })
	var wakeSum time.Duration
	var wakeCount int
	for i := 0; i < len(liveSleeps)-1; i++ {
		if liveSleeps[i].EndedAt == nil {
			continue
		}
		gap := liveSleeps[i+1].StartedAt.Sub(*liveSleeps[i].EndedAt)
		if gap >= 20*time.Minute && gap <= 8*time.Hour {
			wakeSum += gap
			wakeCount++
		}
	}
	if wakeCount > 0 {
		out.AvgWakeWindow = view.FormatDuration(wakeSum / time.Duration(wakeCount))
	}

	out.Chart = buildBarChart(windows, nightHours, dayHours, "Night", "Day", func(v float64) string {
		return view.FormatDuration(time.Duration(math.Round(v*60)) * time.Minute)
	}, func(v float64) string {
		return fmt.Sprintf("%.0fh", v)
	})
	return out
}

/* ---------- 24-Hour Pattern Strip ---------- */

func buildPatternStrip(
	sleeps []sleep.Session,
	feeds []feed.Feed,
	bottles []intake.Bottle,
	windows []dayWindow,
	now time.Time,
	loc *time.Location,
	nightStart, nightEnd time.Duration,
) view.PatternStripView {
	// Show up to the most recent 14 days (7 days when range=7) so rows stay readable.
	sliceStart := 0
	if len(windows) > 14 {
		sliceStart = len(windows) - 14
	}
	subset := windows[sliceStart:]
	rows := make([]view.PatternDayRow, 0, len(subset))
	hasAny := false

	// Newest day on top so today is immediately visible.
	for i := len(subset) - 1; i >= 0; i-- {
		w := subset[i]
		dayDur := w.end.Sub(w.start).Seconds()
		if dayDur <= 0 {
			dayDur = 86400
		}
		row := view.PatternDayRow{
			Label:   w.short,
			DateKey: w.key,
		}
		for _, s := range sleeps {
			if s.DeletedAt != nil {
				continue
			}
			end := now
			if s.EndedAt != nil {
				end = *s.EndedAt
			}
			oStart := s.StartedAt
			if oStart.Before(w.start) {
				oStart = w.start
			}
			oEnd := end
			if oEnd.After(w.end) {
				oEnd = w.end
			}
			if !oEnd.After(oStart) {
				continue
			}
			leftPct := oStart.Sub(w.start).Seconds() / dayDur * 100
			widthPct := oEnd.Sub(oStart).Seconds() / dayDur * 100
			if widthPct < 1.2 {
				widthPct = 1.2
			}
			split := sleep.SplitRange([]sleep.Session{s}, oStart, oEnd, now, loc, nightStart, nightEnd)
			kind := "sleep"
			if split.Night >= split.Day {
				kind = "night-sleep"
			}
			row.Blocks = append(row.Blocks, view.PatternBlock{
				LeftPct:  round1(leftPct),
				WidthPct: round1(widthPct),
				Kind:     kind,
				Tip: fmt.Sprintf("Sleep %s–%s (%s)",
					view.FormatClock(oStart, loc),
					view.FormatClock(oEnd, loc),
					view.FormatDuration(oEnd.Sub(oStart)),
				),
			})
			hasAny = true
		}
		for _, f := range feeds {
			if f.DeletedAt != nil || f.StartedAt.Before(w.start) || !f.StartedAt.Before(w.end) {
				continue
			}
			pct := f.StartedAt.Sub(w.start).Seconds() / dayDur * 100
			row.Markers = append(row.Markers, view.PatternMarker{
				LeftPct: round1(pct),
				Kind:    "feed",
				Tip:     "Nursed at " + view.FormatClock(f.StartedAt, loc),
			})
			hasAny = true
		}
		for _, b := range bottles {
			if b.DeletedAt != nil || b.LoggedAt.Before(w.start) || !b.LoggedAt.Before(w.end) {
				continue
			}
			pct := b.LoggedAt.Sub(w.start).Seconds() / dayDur * 100
			row.Markers = append(row.Markers, view.PatternMarker{
				LeftPct: round1(pct),
				Kind:    "bottle",
				Tip:     "Bottle at " + view.FormatClock(b.LoggedAt, loc),
			})
			hasAny = true
		}
		rows = append(rows, row)
	}
	return view.PatternStripView{
		HasData: hasAny,
		Days:    rows,
	}
}

/* ---------- Diapers ---------- */

func buildDiaperInsights(diapers []diaper.Diaper, windows []dayWindow, now time.Time) view.DiaperInsightsView {
	n := len(windows)
	var out view.DiaperInsightsView
	if n == 0 {
		return out
	}

	wetCounts := make([]float64, n)
	dirtyCounts := make([]float64, n)
	var (
		totalWet   int
		totalDirty int
		totalAll   int
		lastDirty  time.Time
	)

	for _, d := range diapers {
		if d.DeletedAt != nil {
			continue
		}
		idx := dayIndex(windows, d.LoggedAt)
		if idx < 0 {
			continue
		}
		totalAll++
		switch d.Kind {
		case diaper.Wet:
			wetCounts[idx]++
			totalWet++
		case diaper.Dirty:
			dirtyCounts[idx]++
			totalDirty++
			if d.LoggedAt.After(lastDirty) {
				lastDirty = d.LoggedAt
			}
		case diaper.Both:
			wetCounts[idx]++
			dirtyCounts[idx]++
			totalWet++
			totalDirty++
			if d.LoggedAt.After(lastDirty) {
				lastDirty = d.LoggedAt
			}
		}
	}

	if totalAll == 0 {
		return out
	}
	out.HasData = true
	out.AvgTotalPerDay = fmt.Sprintf("%.1f / day", float64(totalAll)/float64(n))
	out.AvgWetPerDay = fmt.Sprintf("%.1f / day", float64(totalWet)/float64(n))
	out.AvgDirtyPerDay = fmt.Sprintf("%.1f / day", float64(totalDirty)/float64(n))
	if !lastDirty.IsZero() {
		daysAgo := int(now.Sub(lastDirty).Hours() / 24.0)
		if daysAgo >= 2 {
			out.LastDirtyNote = fmt.Sprintf("Last dirty diaper: %d days ago", daysAgo)
		}
	}
	out.Chart = buildBarChart(windows, wetCounts, dirtyCounts, "Wet", "Dirty", func(v float64) string {
		return fmt.Sprintf("%.0f", v)
	}, func(v float64) string {
		return fmt.Sprintf("%.0f", v)
	})
	return out
}

/* ---------- Solids ---------- */

func buildSolidsInsights(solids []intake.Solid, enabled bool) *view.SolidsInsightsView {
	var live []intake.Solid
	for _, s := range solids {
		if s.DeletedAt == nil {
			live = append(live, s)
		}
	}
	if !enabled && len(live) == 0 {
		return nil
	}
	out := &view.SolidsInsightsView{
		HasData:    len(live) > 0,
		TotalMeals: len(live),
	}
	if len(live) == 0 {
		return out
	}

	type acc struct {
		display  string
		count    int
		reaction string
	}
	byFood := make(map[string]*acc)
	for _, s := range live {
		key := strings.ToLower(strings.TrimSpace(s.Food))
		if key == "" {
			continue
		}
		entry, ok := byFood[key]
		if !ok {
			entry = &acc{display: strings.TrimSpace(s.Food)}
			byFood[key] = entry
		}
		entry.count++
		if s.Reaction == intake.ReactionAllergic {
			entry.reaction = "Allergic reaction"
		} else if s.Reaction == intake.ReactionMild && entry.reaction == "" {
			entry.reaction = "Mild reaction"
		}
	}
	out.UniqueFoods = len(byFood)

	all := make([]view.SolidFoodStat, 0, len(byFood))
	for _, e := range byFood {
		stat := view.SolidFoodStat{Food: e.display, Count: e.count, Reaction: e.reaction}
		all = append(all, stat)
		if e.reaction != "" {
			out.Reactions = append(out.Reactions, stat)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Food < all[j].Food
	})
	if len(all) > 5 {
		all = all[:5]
	}
	out.TopFoods = all
	sort.Slice(out.Reactions, func(i, j int) bool { return out.Reactions[i].Food < out.Reactions[j].Food })
	return out
}

/* ---------- Pediatrician Snapshot ---------- */

func buildSnapshot(
	babyName string,
	days int,
	gr view.GrowthInsightsView,
	fd view.FeedingInsightsView,
	sl view.SleepInsightsView,
	dp view.DiaperInsightsView,
) view.SnapshotInsightsView {
	label := fmt.Sprintf("Past %d days", days)
	var items []view.SnapshotItem
	if fd.HasBottles {
		items = append(items, view.SnapshotItem{Label: "Avg milk", Value: fd.AvgMilkPerDay})
	}
	if fd.HasFeeds {
		items = append(items, view.SnapshotItem{Label: "Avg nursing", Value: fd.AvgFeedsPerDay + " (" + fd.AvgNursePerDay + ")"})
	}
	if sl.HasData {
		val := sl.AvgTotalPerDay + " / day"
		if sl.LongestStretch != "" {
			val += " · longest " + sl.LongestStretch
		}
		items = append(items, view.SnapshotItem{Label: "Avg sleep", Value: val})
	}
	if dp.HasData {
		items = append(items, view.SnapshotItem{Label: "Diapers", Value: dp.AvgWetPerDay + " wet · " + dp.AvgDirtyPerDay + " dirty"})
	}
	if gr.HasData {
		val := gr.LatestValue + " (" + gr.LatestDate + ")"
		if gr.PercentileLabel != "" {
			val += " · " + gr.PercentileLabel
		}
		items = append(items, view.SnapshotItem{Label: "Latest " + strings.ToLower(gr.MetricLabel), Value: val})
	}

	var b strings.Builder
	title := "Baby summary"
	if strings.TrimSpace(babyName) != "" {
		title = strings.TrimSpace(babyName) + " — summary"
	}
	fmt.Fprintf(&b, "%s (%s)\n", title, label)
	for _, it := range items {
		fmt.Fprintf(&b, "• %s: %s\n", it.Label, it.Value)
	}
	return view.SnapshotInsightsView{
		RangeLabel: label,
		Items:      items,
		CopyText:   strings.TrimSpace(b.String()),
	}
}

/* ---------- Shared SVG Bar Chart Builder ---------- */

func buildBarChart(
	windows []dayWindow,
	primary []float64,
	secondary []float64,
	primaryLegend, secondaryLegend string,
	formatVal func(float64) string,
	formatTick func(float64) string,
) view.BarChart {
	const (
		svgW = 320
		svgH = 148
		padL = 38.0
		padR = 10.0
		padT = 14.0
		padB = 26.0
	)
	plotW := float64(svgW) - padL - padR
	plotH := float64(svgH) - padT - padB
	n := len(windows)

	maxVal := 0.0
	sumVal := 0.0
	hasSecondary := false
	for i := 0; i < n; i++ {
		sec := 0.0
		if i < len(secondary) {
			sec = secondary[i]
			if sec > 0 {
				hasSecondary = true
			}
		}
		tot := primary[i] + sec
		if tot > maxVal {
			maxVal = tot
		}
		sumVal += tot
	}
	if maxVal <= 0 {
		maxVal = 1
	} else {
		maxVal *= 1.15
	}

	chart := view.BarChart{
		Width:           svgW,
		Height:          svgH,
		PlotLeft:        padL,
		PlotRight:       float64(svgW) - padR,
		PlotTop:         padT,
		PlotBottom:      float64(svgH) - padB,
		PrimaryLegend:   primaryLegend,
		SecondaryLegend: secondaryLegend,
		HasSecondary:    hasSecondary,
	}

	yOf := func(v float64) float64 {
		return padT + (1.0-v/maxVal)*plotH
	}

	if n > 0 && sumVal > 0 {
		avg := sumVal / float64(n)
		chart.HasAvg = true
		chart.AvgY = round1(yOf(avg))
		chart.AvgLabel = "Avg " + formatVal(avg)
	}

	slotW := plotW / float64(max(n, 1))
	barW := slotW * 0.62
	if barW > 26 {
		barW = 26
	}
	if barW < 4 {
		barW = 4
	}

	labelStep := 1
	if n > 14 {
		labelStep = 5
	} else if n > 7 {
		labelStep = 2
	}

	for i, w := range windows {
		p := primary[i]
		sec := 0.0
		if i < len(secondary) {
			sec = secondary[i]
		}
		x := padL + float64(i)*slotW + (slotW-barW)/2
		pH := (p / maxVal) * plotH
		sH := (sec / maxVal) * plotH
		pY := chart.PlotBottom - pH
		sY := pY - sH

		tipVal := formatVal(p + sec)
		if hasSecondary && p > 0 && sec > 0 {
			tipVal = fmt.Sprintf("%s (%s %s · %s %s)",
				formatVal(p+sec),
				formatVal(p), strings.ToLower(primaryLegend),
				formatVal(sec), strings.ToLower(secondaryLegend),
			)
		}

		showLbl := i%labelStep == 0 || i == n-1
		chart.Bars = append(chart.Bars, view.ChartBar{
			X:          round1(x),
			Width:      round1(barW),
			PrimaryY:   round1(pY),
			PrimaryH:   round1(pH),
			SecondaryY: round1(sY),
			SecondaryH: round1(sH),
			Label:      w.short,
			ShowLabel:  showLbl,
			TipTitle:   w.fullDay,
			TipValue:   tipVal,
		})
	}

	for i := 0; i <= 2; i++ {
		val := maxVal * float64(i) / 2.0
		chart.YTicks = append(chart.YTicks, view.ChartTick{
			Pos:   round1(yOf(val)),
			Label: formatTick(val),
		})
	}

	return chart
}

func dayIndex(windows []dayWindow, t time.Time) int {
	for i, w := range windows {
		if !t.Before(w.start) && t.Before(w.end) {
			return i
		}
	}
	return -1
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
