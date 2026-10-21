package httpapi

import (
	"slices"

	"cal/apps/api/internal/store"
)

// paletteColors are the colour tokens the web app styles (--c-* in
// styles.css). Colours written through the API must be one of them.
var paletteColors = []string{"slate", "mint", "sky", "violet", "amber", "orange", "rose", "red"}

func validColor(c string) bool { return slices.Contains(paletteColors, c) }

// nextFeedColor picks the first colour no existing feed uses, so new
// calendars are told apart without choosing one; once all are taken it cycles.
func nextFeedColor(feeds []store.Feed) string {
	order := []string{"sky", "mint", "violet", "amber", "orange", "rose", "red", "slate"}
	used := map[string]bool{}
	for _, f := range feeds {
		used[f.Color] = true
	}
	for _, c := range order {
		if !used[c] {
			return c
		}
	}
	return order[len(feeds)%len(order)]
}
