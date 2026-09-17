package onepassword

import "testing"

func TestSuggestMatchesExactSubstringRanksFirst(t *testing.T) {
	items := []Item{
		{ID: "id1", Title: "Database URL (Staging)"},
		{ID: "id2", Title: "Database URL (Prod)"},
		{ID: "id3", Title: "Stripe Key"},
	}

	got := SuggestMatches("Database URL", items, 3)

	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(got), got)
	}
	for _, m := range got {
		if m.Title == "Stripe Key" {
			t.Fatalf("did not expect Stripe Key in matches: %v", got)
		}
	}
}

func TestSuggestMatchesTypoTolerant(t *testing.T) {
	items := []Item{
		{ID: "id1", Title: "Stripe Key"},
		{ID: "id2", Title: "Database URL"},
	}

	got := SuggestMatches("Stirpe Key", items, 3) // transposed letters

	if len(got) == 0 {
		t.Fatal("expected at least one typo-tolerant match")
	}
	if got[0].Title != "Stripe Key" {
		t.Fatalf("expected Stripe Key to rank first, got %v", got)
	}
}

func TestSuggestMatchesRespectsLimit(t *testing.T) {
	items := []Item{
		{ID: "id1", Title: "Key One"},
		{ID: "id2", Title: "Key Two"},
		{ID: "id3", Title: "Key Three"},
		{ID: "id4", Title: "Key Four"},
	}

	got := SuggestMatches("Key", items, 2)

	if len(got) != 2 {
		t.Fatalf("expected limit of 2, got %d", len(got))
	}
}

func TestSuggestMatchesNoMatchReturnsEmpty(t *testing.T) {
	items := []Item{
		{ID: "id1", Title: "Stripe Key"},
	}

	got := SuggestMatches("zzzzzzzzzzzzzzzzzzzz", items, 3)

	if len(got) != 0 {
		t.Fatalf("expected no matches, got %v", got)
	}
}

func TestLevenshteinKnownDistances(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "", 3},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSuggestMatchesKeepsCandidateExactlyAtThreshold(t *testing.T) {
	// query and title are both 8 chars, differing in exactly 4 positions:
	// levenshtein("abcdefgh", "abcdxxxx") == 4, and maxLen/2 == 4. The
	// threshold check is `dist > maxLen/2`, so a dist-4 candidate is kept.
	if got := levenshtein("abcdefgh", "abcdxxxx"); got != 4 {
		t.Fatalf("test setup assumption wrong: got distance %d, want 4", got)
	}
	items := []Item{{ID: "id1", Title: "abcdxxxx"}}

	got := SuggestMatches("abcdefgh", items, 3)

	if len(got) != 1 {
		t.Fatalf("expected the exactly-at-threshold candidate to be kept, got %v", got)
	}
}

func TestSuggestMatchesRejectsCandidateJustOverThreshold(t *testing.T) {
	// levenshtein("abcdefgh", "abcxxxxx") == 5, one over the same
	// threshold of 4 used above — must be rejected.
	if got := levenshtein("abcdefgh", "abcxxxxx"); got != 5 {
		t.Fatalf("test setup assumption wrong: got distance %d, want 5", got)
	}
	items := []Item{{ID: "id1", Title: "abcxxxxx"}}

	got := SuggestMatches("abcdefgh", items, 3)

	if len(got) != 0 {
		t.Fatalf("expected the just-over-threshold candidate to be rejected, got %v", got)
	}
}
