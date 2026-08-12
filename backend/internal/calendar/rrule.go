package calendar

import (
	"strconv"
	"strings"
	"time"
)

// expandRRuleInstances returns occurrence start/end pairs in UTC for a basic
// RRULE (DAILY / WEEKLY) overlapping [from, to). Empty rrule → single instance.
func expandRRuleInstances(rrule string, allDay bool, startUTC, endUTC time.Time, from, to time.Time) [][2]time.Time {
	rrule = strings.TrimSpace(rrule)
	base := [][2]time.Time{{startUTC, endUTC}}
	if rrule == "" {
		if startUTC.Before(to) && endUTC.After(from) {
			return base
		}
		return nil
	}
	freq, interval, count, until, byDays := parseSimpleRRule(rrule)
	if freq == "" {
		if startUTC.Before(to) && endUTC.After(from) {
			return base
		}
		return nil
	}
	if interval < 1 {
		interval = 1
	}
	dur := endUTC.Sub(startUTC)
	if dur <= 0 {
		if allDay {
			dur = 24 * time.Hour
		} else {
			dur = time.Hour
		}
	}

	var out [][2]time.Time
	cur := startUTC
	emitted := 0
	maxEmit := 400
	for emitted < maxEmit {
		if count > 0 && emitted >= count {
			break
		}
		if !until.IsZero() && cur.After(until) {
			break
		}
		occEnd := cur.Add(dur)
		if cur.Before(to) && occEnd.After(from) {
			if len(byDays) == 0 || weekdayAllowed(cur.Weekday(), byDays) {
				out = append(out, [2]time.Time{cur, occEnd})
			}
		}
		if !occEnd.After(from) && cur.After(to) {
			break
		}
		emitted++
		if len(byDays) > 0 && (freq == "WEEKLY" || freq == "DAILY") {
			cur = cur.AddDate(0, 0, 1)
			continue
		}
		switch freq {
		case "DAILY":
			cur = cur.AddDate(0, 0, interval)
		case "WEEKLY":
			cur = cur.AddDate(0, 0, 7*interval)
		case "MONTHLY":
			cur = cur.AddDate(0, interval, 0)
		default:
			return out
		}
		// Stop if we've walked far past the window with no more useful hits.
		if cur.After(to.AddDate(0, 0, 14)) && len(out) > 0 {
			break
		}
		if cur.After(to.AddDate(2, 0, 0)) {
			break
		}
	}
	return out
}

func parseSimpleRRule(rrule string) (freq string, interval, count int, until time.Time, byDays []time.Weekday) {
	interval = 1
	parts := strings.Split(rrule, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToUpper(strings.TrimSpace(kv[0]))
		v := strings.TrimSpace(kv[1])
		switch k {
		case "FREQ":
			freq = strings.ToUpper(v)
		case "INTERVAL":
			if n, err := strconv.Atoi(v); err == nil {
				interval = n
			}
		case "COUNT":
			if n, err := strconv.Atoi(v); err == nil {
				count = n
			}
		case "UNTIL":
			until = parseUntil(v)
		case "BYDAY":
			byDays = parseByDay(v)
		}
	}
	return freq, interval, count, until, byDays
}

func parseUntil(v string) time.Time {
	v = strings.TrimSpace(v)
	layouts := []string{
		time.RFC3339,
		"20060102T150405Z",
		"20060102T150405",
		"20060102",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func parseByDay(v string) []time.Weekday {
	var out []time.Weekday
	for _, part := range strings.Split(v, ",") {
		part = strings.ToUpper(strings.TrimSpace(part))
		// Strip leading ordinal (+1MO)
		for len(part) > 2 && (part[0] == '+' || part[0] == '-' || (part[0] >= '0' && part[0] <= '9')) {
			part = part[1:]
		}
		switch part {
		case "SU":
			out = append(out, time.Sunday)
		case "MO":
			out = append(out, time.Monday)
		case "TU":
			out = append(out, time.Tuesday)
		case "WE":
			out = append(out, time.Wednesday)
		case "TH":
			out = append(out, time.Thursday)
		case "FR":
			out = append(out, time.Friday)
		case "SA":
			out = append(out, time.Saturday)
		}
	}
	return out
}

func weekdayAllowed(day time.Weekday, allowed []time.Weekday) bool {
	for _, a := range allowed {
		if a == day {
			return true
		}
	}
	return false
}
