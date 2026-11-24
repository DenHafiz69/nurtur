package analytics

import (
	"fmt"
	"math"

	"nurtur/internal/growth"
)

// whoRow holds WHO 2006 Child Growth Standards reference values for one
// integer month of age (0..24 months): 3rd, 15th, 50th, 85th, and 97th
// percentiles in metric units (kg for weight, cm for length/head).
type whoRow struct {
	p3  float64
	p15 float64
	p50 float64
	p85 float64
	p97 float64
}

const (
	daysPerMonth = 30.4375
	gramsPerLb   = 453.59237
	cmPerInch    = 2.54
)

// WHO Weight-for-age (kg), 0–24 months.
var whoWeightBoys = [25]whoRow{
	{2.5, 2.9, 3.3, 3.9, 4.3},
	{3.4, 3.9, 4.5, 5.1, 5.7},
	{4.4, 4.9, 5.6, 6.3, 7.0},
	{5.1, 5.7, 6.4, 7.2, 7.9},
	{5.6, 6.2, 7.0, 7.8, 8.6},
	{6.1, 6.7, 7.5, 8.4, 9.2},
	{6.4, 7.1, 7.9, 8.8, 9.7},
	{6.7, 7.4, 8.3, 9.2, 10.2},
	{7.0, 7.7, 8.6, 9.6, 10.5},
	{7.2, 8.0, 8.9, 9.9, 10.9},
	{7.5, 8.2, 9.2, 10.2, 11.2},
	{7.7, 8.4, 9.4, 10.5, 11.5},
	{7.8, 8.6, 9.6, 10.8, 11.8},
	{8.0, 8.8, 9.9, 11.0, 12.1},
	{8.2, 9.0, 10.1, 11.3, 12.4},
	{8.4, 9.2, 10.3, 11.5, 12.7},
	{8.5, 9.4, 10.5, 11.7, 12.9},
	{8.7, 9.6, 10.7, 12.0, 13.2},
	{8.9, 9.8, 10.9, 12.2, 13.5},
	{9.0, 10.0, 11.1, 12.5, 13.7},
	{9.2, 10.1, 11.3, 12.7, 14.0},
	{9.3, 10.3, 11.5, 12.9, 14.3},
	{9.5, 10.5, 11.8, 13.2, 14.5},
	{9.7, 10.7, 12.0, 13.4, 14.8},
	{9.8, 10.8, 12.2, 13.6, 15.1},
}

var whoWeightGirls = [25]whoRow{
	{2.4, 2.8, 3.2, 3.7, 4.2},
	{3.2, 3.6, 4.2, 4.8, 5.4},
	{4.0, 4.5, 5.1, 5.8, 6.5},
	{4.6, 5.2, 5.8, 6.6, 7.4},
	{5.1, 5.7, 6.4, 7.3, 8.1},
	{5.5, 6.1, 6.9, 7.8, 8.7},
	{5.8, 6.5, 7.3, 8.2, 9.2},
	{6.1, 6.8, 7.6, 8.6, 9.6},
	{6.3, 7.0, 7.9, 9.0, 10.0},
	{6.6, 7.3, 8.2, 9.3, 10.4},
	{6.8, 7.5, 8.5, 9.6, 10.7},
	{7.0, 7.7, 8.7, 9.9, 11.0},
	{7.1, 7.9, 8.9, 10.1, 11.3},
	{7.3, 8.1, 9.2, 10.4, 11.6},
	{7.5, 8.3, 9.4, 10.6, 11.9},
	{7.7, 8.5, 9.6, 10.9, 12.2},
	{7.8, 8.7, 9.8, 11.1, 12.5},
	{8.0, 8.9, 10.0, 11.4, 12.7},
	{8.2, 9.1, 10.2, 11.6, 13.0},
	{8.3, 9.2, 10.4, 11.8, 13.3},
	{8.5, 9.4, 10.6, 12.1, 13.5},
	{8.7, 9.6, 10.9, 12.3, 13.8},
	{8.8, 9.8, 11.1, 12.5, 14.1},
	{9.0, 10.0, 11.3, 12.8, 14.3},
	{9.2, 10.2, 11.5, 13.0, 14.6},
}

// WHO Length-for-age (cm), 0–24 months.
var whoHeightBoys = [25]whoRow{
	{46.3, 47.9, 49.9, 51.8, 53.4},
	{51.1, 52.7, 54.7, 56.7, 58.4},
	{54.7, 56.4, 58.4, 60.5, 62.2},
	{57.6, 59.3, 61.4, 63.5, 65.3},
	{60.0, 61.7, 63.9, 66.0, 67.8},
	{61.9, 63.7, 65.9, 68.1, 69.9},
	{63.6, 65.4, 67.6, 69.8, 71.6},
	{65.1, 66.9, 69.2, 71.4, 73.2},
	{66.5, 68.3, 70.6, 72.9, 74.7},
	{67.7, 69.6, 72.0, 74.3, 76.2},
	{69.0, 70.9, 73.3, 75.6, 77.6},
	{70.2, 72.1, 74.5, 76.9, 78.9},
	{71.3, 73.3, 75.7, 78.2, 80.2},
	{72.4, 74.4, 76.9, 79.4, 81.5},
	{73.4, 75.5, 78.0, 80.6, 82.7},
	{74.4, 76.5, 79.1, 81.8, 83.9},
	{75.4, 77.5, 80.2, 82.9, 85.1},
	{76.3, 78.5, 81.2, 84.0, 86.2},
	{77.2, 79.5, 82.3, 85.1, 87.3},
	{78.1, 80.4, 83.2, 86.1, 88.4},
	{78.9, 81.3, 84.2, 87.1, 89.5},
	{79.7, 82.2, 85.1, 88.1, 90.5},
	{80.5, 83.0, 86.0, 89.0, 91.6},
	{81.3, 83.8, 86.9, 90.0, 92.6},
	{82.1, 84.6, 87.8, 90.9, 93.6},
}

var whoHeightGirls = [25]whoRow{
	{45.6, 47.2, 49.1, 51.1, 52.7},
	{50.0, 51.7, 53.7, 55.7, 57.4},
	{53.2, 55.0, 57.1, 59.2, 60.9},
	{55.8, 57.6, 59.8, 62.0, 63.8},
	{58.0, 59.9, 62.1, 64.3, 66.2},
	{59.9, 61.8, 64.0, 66.3, 68.2},
	{61.5, 63.4, 65.7, 68.1, 70.0},
	{62.9, 64.9, 67.3, 69.6, 71.6},
	{64.3, 66.3, 68.7, 71.1, 73.2},
	{65.6, 67.6, 70.1, 72.6, 74.7},
	{66.8, 68.9, 71.5, 74.0, 76.1},
	{68.0, 70.2, 72.8, 75.3, 77.5},
	{69.2, 71.3, 74.0, 76.6, 78.9},
	{70.3, 72.5, 75.2, 77.9, 80.2},
	{71.3, 73.6, 76.4, 79.1, 81.4},
	{72.4, 74.7, 77.5, 80.3, 82.7},
	{73.3, 75.7, 78.6, 81.5, 83.9},
	{74.3, 76.7, 79.7, 82.6, 85.0},
	{75.2, 77.7, 80.7, 83.7, 86.2},
	{76.2, 78.7, 81.7, 84.8, 87.3},
	{77.0, 79.6, 82.7, 85.8, 88.4},
	{77.9, 80.5, 83.7, 86.8, 89.4},
	{78.7, 81.4, 84.6, 87.8, 90.5},
	{79.6, 82.2, 85.5, 88.8, 91.5},
	{80.3, 83.1, 86.4, 89.7, 92.5},
}

// WHO Head circumference-for-age (cm), 0–24 months.
var whoHeadBoys = [25]whoRow{
	{32.1, 33.1, 34.5, 35.8, 36.9},
	{35.1, 36.1, 37.3, 38.5, 39.5},
	{36.9, 37.9, 39.1, 40.4, 41.4},
	{38.3, 39.3, 40.5, 41.8, 42.8},
	{39.4, 40.4, 41.6, 42.9, 43.9},
	{40.3, 41.3, 42.6, 43.9, 44.9},
	{41.0, 42.1, 43.3, 44.6, 45.7},
	{41.7, 42.7, 44.0, 45.3, 46.4},
	{42.2, 43.2, 44.5, 45.8, 46.9},
	{42.6, 43.7, 45.0, 46.3, 47.4},
	{43.0, 44.1, 45.4, 46.7, 47.8},
	{43.4, 44.4, 45.8, 47.1, 48.2},
	{43.7, 44.8, 46.1, 47.4, 48.5},
	{43.9, 45.0, 46.3, 47.7, 48.8},
	{44.2, 45.3, 46.6, 47.9, 49.0},
	{44.4, 45.5, 46.8, 48.2, 49.3},
	{44.6, 45.7, 47.0, 48.4, 49.5},
	{44.8, 45.9, 47.2, 48.6, 49.7},
	{45.0, 46.1, 47.4, 48.8, 49.9},
	{45.1, 46.2, 47.6, 48.9, 50.1},
	{45.3, 46.4, 47.7, 49.1, 50.2},
	{45.4, 46.5, 47.9, 49.3, 50.4},
	{45.6, 46.7, 48.0, 49.4, 50.5},
	{45.7, 46.8, 48.1, 49.5, 50.7},
	{45.8, 46.9, 48.3, 49.7, 50.8},
}

var whoHeadGirls = [25]whoRow{
	{31.7, 32.7, 33.9, 35.1, 36.1},
	{34.3, 35.3, 36.5, 37.8, 38.8},
	{36.0, 37.0, 38.3, 39.5, 40.5},
	{37.2, 38.2, 39.5, 40.8, 41.9},
	{38.2, 39.3, 40.6, 41.9, 43.0},
	{39.0, 40.1, 41.5, 42.8, 43.9},
	{39.7, 40.8, 42.2, 43.5, 44.6},
	{40.3, 41.4, 42.8, 44.2, 45.3},
	{40.8, 41.9, 43.4, 44.8, 45.9},
	{41.3, 42.4, 43.8, 45.2, 46.4},
	{41.7, 42.8, 44.2, 45.6, 46.8},
	{42.0, 43.1, 44.6, 46.0, 47.2},
	{42.3, 43.5, 44.9, 46.3, 47.5},
	{42.6, 43.7, 45.2, 46.6, 47.8},
	{42.8, 44.0, 45.4, 46.9, 48.1},
	{43.1, 44.2, 45.7, 47.1, 48.3},
	{43.3, 44.4, 45.9, 47.3, 48.5},
	{43.5, 44.6, 46.1, 47.5, 48.7},
	{43.6, 44.8, 46.2, 47.7, 48.9},
	{43.8, 45.0, 46.4, 47.9, 49.1},
	{44.0, 45.1, 46.6, 48.1, 49.3},
	{44.1, 45.3, 46.7, 48.2, 49.4},
	{44.3, 45.4, 46.9, 48.4, 49.6},
	{44.4, 45.6, 47.0, 48.5, 49.7},
	{44.6, 45.7, 47.2, 48.7, 49.9},
}

func whoTable(metric, gender string) (*[25]whoRow, bool) {
	switch metric {
	case "weight":
		if gender == "boy" {
			return &whoWeightBoys, true
		}
		if gender == "girl" {
			return &whoWeightGirls, true
		}
	case "height":
		if gender == "boy" {
			return &whoHeightBoys, true
		}
		if gender == "girl" {
			return &whoHeightGirls, true
		}
	case "head":
		if gender == "boy" {
			return &whoHeadBoys, true
		}
		if gender == "girl" {
			return &whoHeadGirls, true
		}
	}
	return nil, false
}

// whoBandMetric returns the interpolated WHO percentiles in metric units (kg or cm)
// for ageDays in [0, 24*30.4375].
func whoBandMetric(metric, gender string, ageDays float64) (whoRow, bool) {
	tbl, ok := whoTable(metric, gender)
	if !ok || ageDays < 0 {
		return whoRow{}, false
	}
	months := ageDays / daysPerMonth
	if months > 24 {
		return whoRow{}, false
	}
	lo := int(math.Floor(months))
	hi := int(math.Ceil(months))
	if lo < 0 {
		lo = 0
	}
	if hi > 24 {
		hi = 24
	}
	if lo == hi {
		return tbl[lo], true
	}
	frac := months - float64(lo)
	a, b := tbl[lo], tbl[hi]
	lerp := func(x, y float64) float64 { return x + (y-x)*frac }
	return whoRow{
		p3:  lerp(a.p3, b.p3),
		p15: lerp(a.p15, b.p15),
		p50: lerp(a.p50, b.p50),
		p85: lerp(a.p85, b.p85),
		p97: lerp(a.p97, b.p97),
	}, true
}

func convertWHORow(r whoRow, metric string, unit growth.Unit) whoRow {
	if unit != growth.UnitImperial {
		return r
	}
	factor := 1.0 / cmPerInch
	if metric == "weight" {
		factor = 1000.0 / gramsPerLb
	}
	return whoRow{
		p3:  r.p3 * factor,
		p15: r.p15 * factor,
		p50: r.p50 * factor,
		p85: r.p85 * factor,
		p97: r.p97 * factor,
	}
}

// EstimatePercentile returns a calm human-readable WHO percentile estimate
// for a single metric value (kg or cm) at ageDays.
func EstimatePercentile(metric, gender string, ageDays, valMetric float64) string {
	band, ok := whoBandMetric(metric, gender, ageDays)
	if !ok || valMetric <= 0 {
		return ""
	}
	type anchor struct {
		val float64
		pct float64
	}
	anchors := []anchor{
		{band.p3, 3},
		{band.p15, 15},
		{band.p50, 50},
		{band.p85, 85},
		{band.p97, 97},
	}
	if valMetric < anchors[0].val {
		return "< 3rd %ile (WHO)"
	}
	if valMetric > anchors[len(anchors)-1].val {
		return "> 97th %ile (WHO)"
	}
	for i := 0; i < len(anchors)-1; i++ {
		if valMetric >= anchors[i].val && valMetric <= anchors[i+1].val {
			span := anchors[i+1].val - anchors[i].val
			pct := anchors[i].pct
			if span > 0 {
				pct += (valMetric - anchors[i].val) / span * (anchors[i+1].pct - anchors[i].pct)
			}
			rounded := int(math.Round(pct))
			return fmt.Sprintf("~%s %%ile (WHO)", ordinal(rounded))
		}
	}
	return ""
}

func ordinal(n int) string {
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 13 {
		return fmt.Sprintf("%dth", n)
	}
	switch n % 10 {
	case 1:
		return fmt.Sprintf("%dst", n)
	case 2:
		return fmt.Sprintf("%dnd", n)
	case 3:
		return fmt.Sprintf("%drd", n)
	default:
		return fmt.Sprintf("%dth", n)
	}
}
