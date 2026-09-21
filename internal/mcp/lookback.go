// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

var lookbackPattern = regexp.MustCompile(`^([1-9][0-9]*)\s*(h|hours?|d|days?|w|weeks?|mo|months?)$`)

// lookbackCutoff returns a UTC publication cutoff, or zero when omitted.
func lookbackCutoff(req mcp.CallToolRequest, now time.Time) (time.Time, error) {
	raw, present := req.GetArguments()["lookback"]
	if !present {
		return time.Time{}, nil
	}
	invalid := fmt.Errorf("lookback must be a positive whole number with a unit: h/hours, d/days, w/weeks, or mo/months (e.g. 24h, 7d, 2w, 3mo)")
	value, ok := raw.(string)
	if !ok {
		return time.Time{}, invalid
	}
	parts := lookbackPattern.FindStringSubmatch(strings.ToLower(strings.TrimSpace(value)))
	if parts == nil {
		return time.Time{}, invalid
	}
	n, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("lookback is too large")
	}
	now = now.UTC()
	if parts[2] == "mo" || strings.HasPrefix(parts[2], "month") {
		// Start at day one so AddDate cannot roll a short target month forward.
		if n > int64((now.Year()-1)*12+int(now.Month())-1) {
			return time.Time{}, fmt.Errorf("lookback reaches before year 1")
		}
		first := time.Date(now.Year(), now.Month(), 1, now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC).AddDate(0, -int(n), 0)
		lastDay := first.AddDate(0, 1, -1).Day()
		return first.AddDate(0, 0, min(now.Day(), lastDay)-1), nil
	}
	unit := time.Hour
	switch parts[2][0] {
	case 'd':
		unit *= 24
	case 'w':
		unit *= 24 * 7
	}
	if n > math.MaxInt64/int64(unit) {
		return time.Time{}, fmt.Errorf("lookback is too large")
	}
	return now.Add(-time.Duration(n) * unit), nil
}
