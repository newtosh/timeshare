package onepassword

import (
	"sort"
	"strings"
)

// SuggestMatches ranks items against query and returns up to limit,
// best-first. An exact case-insensitive substring match always outranks a
// typo-tolerant (Levenshtein-distance) match. Used to give a "did you
// mean" hint when a user-supplied item name/ID doesn't resolve — 1Password
// item titles aren't guaranteed unique, so this is a hint toward the right
// item(s), not a resolver.
func SuggestMatches(query string, items []Item, limit int) []Item {
	type scored struct {
		item  Item
		score int // lower is better; substring matches get a large bonus
	}

	q := strings.ToLower(query)
	var candidates []scored
	for _, it := range items {
		title := strings.ToLower(it.Title)
		switch {
		case strings.Contains(title, q):
			candidates = append(candidates, scored{item: it, score: -1000 + len(title)})
		default:
			dist := levenshtein(q, title)
			// Reject anything wildly dissimilar in length/content — a
			// distance close to the longer string's length is noise, not
			// a plausible typo.
			maxLen := len(q)
			if len(title) > maxLen {
				maxLen = len(title)
			}
			// Inclusive at the threshold by design: a candidate exactly
			// at half the longer string's length is still kept, not
			// rejected — only strictly worse candidates are dropped.
			if maxLen == 0 || dist > maxLen/2 {
				continue
			}
			candidates = append(candidates, scored{item: it, score: dist})
		}
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score < candidates[j].score })

	if limit > len(candidates) {
		limit = len(candidates)
	}
	result := make([]Item, limit)
	for i := 0; i < limit; i++ {
		result[i] = candidates[i].item
	}
	return result
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
