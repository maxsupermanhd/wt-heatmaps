package frontend

import (
	"fmt"
	"runtime/debug"
	"slices"
	"strings"
)

func BRString(i int) string {
	if i == 0 {
		return "1.0"
	}
	rem := i % 3
	switch rem {
	case 1:
		rem = 3
	case 2:
		rem = 7
	}
	return fmt.Sprintf("%d.%d", 1+i/3, rem)
}

func BRNumber(i int) float32 {
	if i == 0 {
		return 1
	}
	rem := i % 3
	switch rem {
	case 1:
		rem = 3
	case 2:
		rem = 7
	}
	return float32(1+i/3) + float32(rem)/10
}

func KillDeathRatio(kills, deaths int) string {
	if deaths == 0 {
		return "∞"
	}
	return fmt.Sprintf("%.2f", float64(kills)/float64(deaths))
}

func GetVCSSummary(s []debug.BuildSetting) string {
	slices.SortFunc(s, func(a, b debug.BuildSetting) int {
		return strings.Compare(a.Key, b.Key)
	})
	ret := make([]string, 0, 4)
	for _, v := range s {
		switch v.Key {
		case "vcs":
			ret = append(ret, v.Value)
		case "vcs.revision":
			if len(v.Value) > 7 {
				ret = append(ret, v.Value[:7])
			} else {
				ret = append(ret, v.Value)
			}
		case "vcs.time":
			ret = append(ret, v.Value)
		case "vcs.modified":
			if v.Value == "true" {
				ret = append(ret, "(modified)")
			}
		}
	}
	return strings.Join(ret, " ")
}
