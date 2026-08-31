package backtest

import "strings"

// DownsampleEquity reduces curve points for UI transfer (keeps first/last + trade events).
func DownsampleEquity(points []EquityPoint, max int) []EquityPoint {
	if max < 4 || len(points) <= max {
		return points
	}
	keep := make(map[int]struct{}, max)
	keep[0] = struct{}{}
	keep[len(points)-1] = struct{}{}
	for i, p := range points {
		ev := p.Event
		if ev != "" && ev != "start" && !strings.HasPrefix(ev, "mark:") {
			keep[i] = struct{}{}
		}
	}
	if len(keep) < max {
		step := float64(len(points)-1) / float64(max-1)
		for i := 0; i < max; i++ {
			idx := int(float64(i) * step)
			if idx >= len(points) {
				idx = len(points) - 1
			}
			keep[idx] = struct{}{}
		}
	}
	indices := make([]int, 0, len(keep))
	for i := range keep {
		indices = append(indices, i)
	}
	sortInts(indices)
	if len(indices) > max {
		// Always keep endpoints; sample the middle.
		mid := make([]int, 0, len(indices)-2)
		for _, idx := range indices {
			if idx != 0 && idx != len(points)-1 {
				mid = append(mid, idx)
			}
		}
		indices = []int{0}
		if len(mid) > max-2 {
			step := float64(len(mid)-1) / float64(max-3)
			for i := 0; i < max-2; i++ {
				idx := int(float64(i) * step)
				if idx >= len(mid) {
					idx = len(mid) - 1
				}
				indices = append(indices, mid[idx])
			}
		} else {
			indices = append(indices, mid...)
		}
		indices = append(indices, len(points)-1)
		sortInts(indices)
	}
	out := make([]EquityPoint, len(indices))
	for i, idx := range indices {
		out[i] = points[idx]
	}
	return out
}

func sortInts(a []int) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[j] < a[i] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}
