package recommender

import (
	"context"
	"fmt"
	"testing"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/models"
)

// --- hardFilter ---

func TestHardFilter_RemovesOwned(t *testing.T) {
	p := &UserProfile{
		OwnedForeignIDs:     map[string]bool{"OWN": true},
		DismissedForeignIDs: map[string]bool{},
		ExcludedAuthors:     map[string]bool{},
	}
	candidates := []models.RecommendationCandidate{
		{ForeignID: "OWN", Title: "Already owned", RatingsCount: 100, Rating: 4.0},
		{ForeignID: "KEEP", Title: "Keep this", RatingsCount: 100, Rating: 4.0},
	}
	got := hardFilter(candidates, p)
	if len(got) != 1 || got[0].ForeignID != "KEEP" {
		t.Errorf("expected only KEEP, got %+v", got)
	}
}

func TestHardFilter_RemovesDismissed(t *testing.T) {
	p := &UserProfile{
		OwnedForeignIDs:     map[string]bool{},
		DismissedForeignIDs: map[string]bool{"DIS": true},
		ExcludedAuthors:     map[string]bool{},
	}
	candidates := []models.RecommendationCandidate{
		{ForeignID: "DIS", RatingsCount: 100, Rating: 4.0},
		{ForeignID: "OK", RatingsCount: 100, Rating: 4.0},
	}
	got := hardFilter(candidates, p)
	if len(got) != 1 || got[0].ForeignID != "OK" {
		t.Errorf("expected only OK, got %+v", got)
	}
}

func TestHardFilter_RemovesExcludedAuthor(t *testing.T) {
	p := &UserProfile{
		OwnedForeignIDs:     map[string]bool{},
		DismissedForeignIDs: map[string]bool{},
		ExcludedAuthors:     map[string]bool{"bad author": true},
	}
	candidates := []models.RecommendationCandidate{
		{ForeignID: "A", AuthorName: "Bad Author", RatingsCount: 100, Rating: 4.0}, // case-insensitive match
		{ForeignID: "B", AuthorName: "Good Author", RatingsCount: 100, Rating: 4.0},
		{ForeignID: "C", AuthorName: "", RatingsCount: 100, Rating: 4.0}, // no author → allowed
	}
	got := hardFilter(candidates, p)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(got), got)
	}
	for _, c := range got {
		if c.ForeignID == "A" {
			t.Error("excluded-author candidate should have been filtered")
		}
	}
}

func TestHardFilter_PopularityFilter(t *testing.T) {
	p := &UserProfile{
		OwnedForeignIDs:     map[string]bool{},
		DismissedForeignIDs: map[string]bool{},
		ExcludedAuthors:     map[string]bool{},
	}
	candidates := []models.RecommendationCandidate{
		{ForeignID: "A", Title: "No ratings at all", RatingsCount: 0, Rating: 0},
		{ForeignID: "B", Title: "Too few ratings", RatingsCount: 30, Rating: 4.5},
		{ForeignID: "C", Title: "Enough ratings but low score", RatingsCount: 50, Rating: 2.5},
		{ForeignID: "D", Title: "Enough ratings exactly 3.0", RatingsCount: 50, Rating: 3.0},
		{ForeignID: "E", Title: "Popular and well rated", RatingsCount: 200, Rating: 4.5},
		{ForeignID: "F", Title: "Popular but unrated in DB", RatingsCount: 200, Rating: 0.0},
	}
	got := hardFilter(candidates, p)

	wantPass := map[string]bool{"D": true, "E": true, "F": true}
	wantFilter := map[string]bool{"A": true, "B": true, "C": true}

	if len(got) != len(wantPass) {
		t.Fatalf("expected %d candidates, got %d: %+v", len(wantPass), len(got), got)
	}
	for _, c := range got {
		if wantFilter[c.ForeignID] {
			t.Errorf("candidate %q should have been filtered but was not", c.ForeignID)
		}
		if !wantPass[c.ForeignID] {
			t.Errorf("unexpected candidate %q in result", c.ForeignID)
		}
	}
}

func TestHardFilter_Dedupes(t *testing.T) {
	p := &UserProfile{
		OwnedForeignIDs:     map[string]bool{},
		DismissedForeignIDs: map[string]bool{},
		ExcludedAuthors:     map[string]bool{},
	}
	candidates := []models.RecommendationCandidate{
		{ForeignID: "X", Title: "first", RatingsCount: 100, Rating: 4.0},
		{ForeignID: "X", Title: "dup", RatingsCount: 100, Rating: 4.0},
		{ForeignID: "Y", RatingsCount: 100, Rating: 4.0},
	}
	got := hardFilter(candidates, p)
	if len(got) != 2 {
		t.Fatalf("expected 2 after dedup, got %d", len(got))
	}
	if got[0].ForeignID != "X" || got[0].Title != "first" {
		t.Errorf("first-wins dedup: got %+v", got[0])
	}
}

func TestClassifyDrop(t *testing.T) {
	p := &UserProfile{
		OwnedForeignIDs:     map[string]bool{"OWN": true},
		DismissedForeignIDs: map[string]bool{"DIS": true},
		ExcludedAuthors:     map[string]bool{"bad author": true},
		PreferredLanguage:   "eng",
	}
	tests := []struct {
		name string
		c    models.RecommendationCandidate
		want dropReason
	}{
		{"owned", models.RecommendationCandidate{ForeignID: "OWN", RatingsCount: 100, Rating: 4.0}, dropOwned},
		{"dismissed", models.RecommendationCandidate{ForeignID: "DIS", RatingsCount: 100, Rating: 4.0}, dropDismissed},
		{"excludedAuthor", models.RecommendationCandidate{ForeignID: "A", AuthorName: "Bad Author", RatingsCount: 100, Rating: 4.0}, dropExcludedAuthor},
		{"language", models.RecommendationCandidate{ForeignID: "B", Language: "spa", RatingsCount: 100, Rating: 4.0}, dropLanguage},
		{"lowRatingsCount", models.RecommendationCandidate{ForeignID: "C", RecType: models.RecTypeListCross, RatingsCount: 10, Rating: 4.0}, dropLowRatingsCount},
		// genre_popular now carries real OL ratings (search.json) → gated, but at a
		// modest bar: too few ratings drops (ratings-less classics/catalog noise),
		// a well-rated pick survives.
		{"genrePopularTooFewRatings", models.RecommendationCandidate{ForeignID: "GP1", RecType: models.RecTypeGenrePopular, Language: "eng", RatingsCount: 5, Rating: 4.2}, dropLowRatingsCount},
		{"genrePopularWellRated", models.RecommendationCandidate{ForeignID: "GP2", RecType: models.RecTypeGenrePopular, Language: "eng", RatingsCount: 40, Rating: 4.2}, dropReason("")},
		{"lowRating", models.RecommendationCandidate{ForeignID: "D", RatingsCount: 100, Rating: 2.5}, dropLowRating},
		{"collection", models.RecommendationCandidate{ForeignID: "E", Title: "The Complete Stories", RatingsCount: 100, Rating: 4.0}, dropCollection},
		{"trustedSourceSkipsRatingGate", models.RecommendationCandidate{ForeignID: "F", RecType: models.RecTypeAuthorNew, RatingsCount: 0, Rating: 0}, dropReason("")},
		{"keep", models.RecommendationCandidate{ForeignID: "G", Title: "A Fine Book", RatingsCount: 100, Rating: 4.0}, dropReason("")},
	}
	seen := map[string]bool{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyDrop(tt.c, p, seen); got != tt.want {
				t.Errorf("classifyDrop(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestClassifyDrop_LanguageCodeNormalization covers the headline bug: the
// preferred language is stored 2-letter ("en") but book tags are 3-letter
// ("eng"). The gate must treat them as equal, while still dropping genuinely
// foreign editions and passing empty (unknown) languages.
func TestClassifyDrop_LanguageCodeNormalization(t *testing.T) {
	p := &UserProfile{
		OwnedForeignIDs:     map[string]bool{},
		DismissedForeignIDs: map[string]bool{},
		ExcludedAuthors:     map[string]bool{},
		PreferredLanguage:   "en",
	}
	seen := map[string]bool{}
	cases := []struct {
		name string
		lang string
		want dropReason
	}{
		{"eng-matches-en", "eng", dropReason("")},
		{"en-matches-en", "en", dropReason("")},
		{"empty-passes", "", dropReason("")},
		{"dutch-dropped", "nld", dropLanguage},
		{"spanish-dropped", "spa", dropLanguage},
	}
	for _, tc := range cases {
		c := models.RecommendationCandidate{ForeignID: tc.name, Title: "x", Language: tc.lang, RatingsCount: 100, Rating: 4.0}
		if got := classifyDrop(c, p, seen); got != tc.want {
			t.Errorf("%s: lang=%q -> %q, want %q", tc.name, tc.lang, got, tc.want)
		}
	}
}

func TestClassifyDrop_NoPreferredLanguageDisablesFilter(t *testing.T) {
	p := &UserProfile{
		OwnedForeignIDs:     map[string]bool{},
		DismissedForeignIDs: map[string]bool{},
		ExcludedAuthors:     map[string]bool{},
		PreferredLanguage:   "",
	}
	c := models.RecommendationCandidate{ForeignID: "A", Title: "x", Language: "nld", RatingsCount: 100, Rating: 4.0}
	if got := classifyDrop(c, p, map[string]bool{}); got != "" {
		t.Errorf("empty PreferredLanguage should disable the language filter, got %q", got)
	}
}

// TestClassifyDrop_AnyLanguageDisablesFilter covers the "any" sentinel that the
// settings UI offers alongside "en". "any" means no language preference, so
// every language (including foreign) must pass — consistent with how
// models.ParseAllowedLanguages and indexer.FilterByLanguage treat "any".
func TestClassifyDrop_AnyLanguageDisablesFilter(t *testing.T) {
	p := &UserProfile{
		OwnedForeignIDs:     map[string]bool{},
		DismissedForeignIDs: map[string]bool{},
		ExcludedAuthors:     map[string]bool{},
		PreferredLanguage:   "any",
	}
	for _, lang := range []string{"eng", "nld", "spa", ""} {
		c := models.RecommendationCandidate{ForeignID: "x", Title: "x", Language: lang, RatingsCount: 100, Rating: 4.0}
		if got := classifyDrop(c, p, map[string]bool{}); got != "" {
			t.Errorf("PreferredLanguage=any must pass lang=%q, got drop %q", lang, got)
		}
	}
}

// TestDedupeByWork covers P0b: editions of the same work (shared DedupKey)
// collapse to the single best edition, preferring a preferred-language match,
// then ratings count. This is what removes the Spanish "Cuchillo de agua" in
// favor of the English "The Water Knife".
func TestDedupeByWork_KeepsBestEdition(t *testing.T) {
	p := &UserProfile{PreferredLanguage: "en"}
	cands := []models.RecommendationCandidate{
		{ForeignID: "ES", DedupKey: "water-knife", Title: "Cuchillo de agua", Language: "", Score: 0.35, RatingsCount: 10},
		{ForeignID: "EN", DedupKey: "water-knife", Title: "The Water Knife", Language: "eng", Score: 0.35, RatingsCount: 5000},
		{ForeignID: "OTHER", DedupKey: "other-work", Title: "Unrelated", Language: "eng", Score: 0.30},
		{ForeignID: "NODEDUP", DedupKey: "", Title: "No key", Language: "eng", Score: 0.20},
	}
	got := dedupeByWork(cands, p)
	if len(got) != 3 {
		t.Fatalf("expected 3 after work-dedup, got %d: %+v", len(got), got)
	}
	for _, c := range got {
		if c.ForeignID == "ES" {
			t.Error("Spanish edition should have been removed in favor of the English edition")
		}
	}
	// The kept edition of the shared work must be the English one.
	var keptWaterKnife string
	for _, c := range got {
		if c.DedupKey == "water-knife" {
			keptWaterKnife = c.ForeignID
		}
	}
	if keptWaterKnife != "EN" {
		t.Errorf("kept edition of shared work = %q, want EN", keptWaterKnife)
	}
}

func TestDedupeByWork_EmptyKeyPassesThrough(t *testing.T) {
	p := &UserProfile{PreferredLanguage: "en"}
	cands := []models.RecommendationCandidate{
		{ForeignID: "A", DedupKey: "", Title: "a", Score: 0.3},
		{ForeignID: "B", DedupKey: "", Title: "b", Score: 0.2},
	}
	got := dedupeByWork(cands, p)
	if len(got) != 2 {
		t.Fatalf("empty-DedupKey candidates must pass through untouched, got %d", len(got))
	}
}

// TestCapAuthorNew covers P0c: no single monitored author may flood the list.
func TestCapAuthorNew(t *testing.T) {
	a1 := int64(1)
	a2 := int64(2)
	cands := []models.RecommendationCandidate{
		{ForeignID: "a1-1", RecType: models.RecTypeAuthorNew, AuthorID: &a1, Score: 0.9},
		{ForeignID: "a1-2", RecType: models.RecTypeAuthorNew, AuthorID: &a1, Score: 0.8},
		{ForeignID: "a1-3", RecType: models.RecTypeAuthorNew, AuthorID: &a1, Score: 0.7},
		{ForeignID: "a1-4", RecType: models.RecTypeAuthorNew, AuthorID: &a1, Score: 0.6},
		{ForeignID: "a2-1", RecType: models.RecTypeAuthorNew, AuthorID: &a2, Score: 0.5},
		{ForeignID: "gp", RecType: models.RecTypeGenrePopular, Score: 0.4},
	}
	got := capAuthorNew(cands, 2)
	var ids []string
	for _, c := range got {
		ids = append(ids, c.ForeignID)
	}
	want := []string{"a1-1", "a1-2", "a2-1", "gp"}
	if len(ids) != len(want) {
		t.Fatalf("cap=2: got %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("cap=2 order: got %v, want %v", ids, want)
		}
	}
}

// TestCapDiscoveryByAuthorName covers the franchise-flood fix: discovery picks
// (genre_popular et al.) carry an author name but no AuthorID, so capAuthorNew
// can't reach them; a name-keyed cap stops one author (e.g. a full Harry Potter
// list) from monopolizing the reserved discovery slots.
func TestCapDiscoveryByAuthorName(t *testing.T) {
	cands := []models.RecommendationCandidate{
		{ForeignID: "hp1", RecType: models.RecTypeGenrePopular, AuthorName: "J. K. Rowling", Score: 0.20},
		{ForeignID: "hp2", RecType: models.RecTypeGenrePopular, AuthorName: "J. K. Rowling", Score: 0.19},
		{ForeignID: "hp3", RecType: models.RecTypeGenrePopular, AuthorName: "J. K. Rowling", Score: 0.18},
		{ForeignID: "hp4", RecType: models.RecTypeGenrePopular, AuthorName: "j. k. rowling", Score: 0.17}, // case-insensitive
		{ForeignID: "wells", RecType: models.RecTypeGenrePopular, AuthorName: "H. G. Wells", Score: 0.15},
		{ForeignID: "noname", RecType: models.RecTypeGenrePopular, AuthorName: "", Score: 0.14}, // no name → never capped
		{ForeignID: "an1", RecType: models.RecTypeAuthorNew, AuthorName: "Seth Godin", Score: 0.5},
		{ForeignID: "an2", RecType: models.RecTypeAuthorNew, AuthorName: "Seth Godin", Score: 0.49}, // author_new untouched by this cap
		{ForeignID: "an3", RecType: models.RecTypeAuthorNew, AuthorName: "Seth Godin", Score: 0.48},
	}
	got := capDiscoveryByAuthorName(cands, 2)
	var ids []string
	for _, c := range got {
		ids = append(ids, c.ForeignID)
	}
	want := []string{"hp1", "hp2", "wells", "noname", "an1", "an2", "an3"}
	if len(ids) != len(want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order/contents: got %v, want %v", ids, want)
		}
	}
}

// --- selectWithQuota (P1: reserved external-discovery quota) ---

func countDiscovery(cands []models.RecommendationCandidate) int {
	n := 0
	for _, c := range cands {
		if isDiscovery(c.RecType) {
			n++
		}
	}
	return n
}

// TestSelectWithQuota_ReservesDiscoverySlots is the core P1 case: many
// high-scoring author_new candidates and some low-scoring discovery picks; the
// quota must force the discovery picks in despite being below the score cut.
func TestSelectWithQuota_ReservesDiscoverySlots(t *testing.T) {
	var cands []models.RecommendationCandidate
	for i := 0; i < 100; i++ {
		cands = append(cands, models.RecommendationCandidate{
			ForeignID: fmt.Sprintf("an%d", i), RecType: models.RecTypeAuthorNew, Score: 0.5,
		})
	}
	for i := 0; i < 30; i++ {
		cands = append(cands, models.RecommendationCandidate{
			ForeignID: fmt.Sprintf("gp%d", i), RecType: models.RecTypeGenrePopular, Score: 0.1,
		})
	}
	got := selectWithQuota(cands, 20, 5)
	if len(got) != 20 {
		t.Fatalf("expected 20 selected, got %d", len(got))
	}
	if d := countDiscovery(got); d != 5 {
		t.Errorf("expected exactly 5 reserved discovery picks, got %d", d)
	}
	// Result must be score-sorted.
	for i := 1; i < len(got); i++ {
		if got[i-1].Score < got[i].Score {
			t.Fatalf("result not score-sorted at %d: %v > %v", i, got[i-1].Score, got[i].Score)
		}
	}
}

func TestSelectWithQuota_NoDiscoveryReturnsTopN(t *testing.T) {
	var cands []models.RecommendationCandidate
	for i := 0; i < 50; i++ {
		cands = append(cands, models.RecommendationCandidate{
			ForeignID: fmt.Sprintf("an%d", i), RecType: models.RecTypeAuthorNew, Score: float64(50 - i),
		})
	}
	got := selectWithQuota(cands, 20, 5)
	if len(got) != 20 {
		t.Fatalf("expected 20, got %d", len(got))
	}
	if countDiscovery(got) != 0 {
		t.Error("no discovery candidates exist; none should appear")
	}
	if got[0].Score != 50 {
		t.Errorf("expected highest-scored first, got %v", got[0].Score)
	}
}

func TestSelectWithQuota_FewerDiscoveryThanQuota(t *testing.T) {
	var cands []models.RecommendationCandidate
	for i := 0; i < 100; i++ {
		cands = append(cands, models.RecommendationCandidate{
			ForeignID: fmt.Sprintf("an%d", i), RecType: models.RecTypeAuthorNew, Score: 0.5,
		})
	}
	for i := 0; i < 3; i++ {
		cands = append(cands, models.RecommendationCandidate{
			ForeignID: fmt.Sprintf("gp%d", i), RecType: models.RecTypeGenrePopular, Score: 0.1,
		})
	}
	got := selectWithQuota(cands, 20, 5)
	if len(got) != 20 {
		t.Fatalf("expected 20, got %d", len(got))
	}
	if d := countDiscovery(got); d != 3 {
		t.Errorf("all 3 discovery picks should be included, got %d", d)
	}
}

func TestSelectWithQuota_PoolSmallerThanTotal(t *testing.T) {
	cands := []models.RecommendationCandidate{
		{ForeignID: "a", RecType: models.RecTypeAuthorNew, Score: 0.5},
		{ForeignID: "b", RecType: models.RecTypeGenrePopular, Score: 0.1},
	}
	got := selectWithQuota(cands, 20, 5)
	if len(got) != 2 {
		t.Fatalf("pool smaller than total should return all, got %d", len(got))
	}
}

func TestClassifyDrop_DupForeignID(t *testing.T) {
	p := &UserProfile{
		OwnedForeignIDs:     map[string]bool{},
		DismissedForeignIDs: map[string]bool{},
		ExcludedAuthors:     map[string]bool{},
	}
	c := models.RecommendationCandidate{ForeignID: "X", Title: "dup", RatingsCount: 100, Rating: 4.0}
	seen := map[string]bool{"X": true}
	if got := classifyDrop(c, p, seen); got != dropDupForeignID {
		t.Errorf("classifyDrop on seen ForeignID = %q, want %q", got, dropDupForeignID)
	}
}

// --- shared DB-integrated fixtures ---

func seedSeries(t *testing.T, f profileFixtures, foreignID, title string) *models.Series {
	t.Helper()
	s := &models.Series{
		ForeignID: foreignID,
		Title:     title,
	}
	if err := f.series.Create(context.Background(), s); err != nil {
		t.Fatalf("create series: %v", err)
	}
	return s
}

// --- GenerateSeries ---

func TestGenerateSeries_NoStartedSeries(t *testing.T) {
	f := newProfileFixtures(t)
	p := &UserProfile{
		SeriesState:     map[int64]SeriesState{},
		OwnedForeignIDs: map[string]bool{},
	}
	got := GenerateSeries(context.Background(), f.books, f.series, p)
	if len(got) != 0 {
		t.Errorf("expected no candidates with empty series state, got %d", len(got))
	}
}

func TestGenerateSeries_NextInSequence(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	a := seedAuthor(t, f, "Author", "OLA1", false)
	b1 := seedBook(t, f, a.ID, "OLB1", "Book 1", nil)
	b2 := seedBook(t, f, a.ID, "OLB2", "Book 2", nil)
	b3 := seedBook(t, f, a.ID, "OLB3", "Book 3", nil)

	s := seedSeries(t, f, "OLS1", "The Series")
	if err := f.series.LinkBook(ctx, s.ID, b1.ID, "1", true); err != nil {
		t.Fatal(err)
	}
	if err := f.series.LinkBook(ctx, s.ID, b2.ID, "2", true); err != nil {
		t.Fatal(err)
	}
	if err := f.series.LinkBook(ctx, s.ID, b3.ID, "3", true); err != nil {
		t.Fatal(err)
	}

	// User owns book 1 only; expect book 2 as next-in-sequence.
	p := &UserProfile{
		SeriesState: map[int64]SeriesState{
			s.ID: {SeriesID: s.ID, SeriesTitle: s.Title, MaxPosition: 1},
		},
		OwnedForeignIDs: map[string]bool{b1.ForeignID: true},
	}
	got := GenerateSeries(ctx, f.books, f.series, p)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %+v", len(got), got)
	}
	if got[0].ForeignID != b2.ForeignID {
		t.Errorf("expected next book %q, got %q", b2.ForeignID, got[0].ForeignID)
	}
	if got[0].RecType != models.RecTypeSeries {
		t.Errorf("RecType: got %q", got[0].RecType)
	}
	if got[0].SeriesID == nil || *got[0].SeriesID != s.ID {
		t.Error("SeriesID not populated")
	}
}

func TestGenerateSeries_FillGaps(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	a := seedAuthor(t, f, "Author", "OLA1", false)
	b1 := seedBook(t, f, a.ID, "OLB1", "Book 1", nil)
	b2 := seedBook(t, f, a.ID, "OLB2", "Book 2", nil)
	b3 := seedBook(t, f, a.ID, "OLB3", "Book 3", nil)

	s := seedSeries(t, f, "OLS1", "Gappy Series")
	_ = f.series.LinkBook(ctx, s.ID, b1.ID, "1", true)
	_ = f.series.LinkBook(ctx, s.ID, b2.ID, "2", true)
	_ = f.series.LinkBook(ctx, s.ID, b3.ID, "3", true)

	// User owns books 1 and 3; book 2 is a gap and book 4 would be next (doesn't exist).
	p := &UserProfile{
		SeriesState: map[int64]SeriesState{
			s.ID: {
				SeriesID:         s.ID,
				SeriesTitle:      s.Title,
				MaxPosition:      3,
				MissingPositions: []float64{2},
			},
		},
		OwnedForeignIDs: map[string]bool{b1.ForeignID: true, b3.ForeignID: true},
	}
	got := GenerateSeries(ctx, f.books, f.series, p)
	// Next-in-sequence after 3 does not exist; gap-fill returns book 2.
	found := false
	for _, c := range got {
		if c.ForeignID == b2.ForeignID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected gap-fill candidate for book 2, got %+v", got)
	}
}

// --- GenerateAuthorNew ---

func TestGenerateAuthorNew_MonitoredAuthor(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	a := seedAuthor(t, f, "Monitored", "OLA_M", true)
	b1 := seedBook(t, f, a.ID, "OLB1", "Wanted Book", nil) // status=wanted via helper
	// Owned book should be filtered.
	seedBook(t, f, a.ID, "OLB2", "Owned Book", nil)

	p := &UserProfile{
		MonitoredAuthors: map[int64]bool{a.ID: true},
		OwnedForeignIDs:  map[string]bool{"OLB2": true},
	}
	got := GenerateAuthorNew(ctx, f.books, f.authors, p)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %+v", len(got), got)
	}
	if got[0].ForeignID != b1.ForeignID {
		t.Errorf("expected %q, got %q", b1.ForeignID, got[0].ForeignID)
	}
	if got[0].RecType != models.RecTypeAuthorNew {
		t.Errorf("RecType: got %q", got[0].RecType)
	}
	if got[0].AuthorName != "Monitored" {
		t.Errorf("AuthorName: got %q", got[0].AuthorName)
	}
}

func TestGenerateAuthorNew_NoMonitoredAuthors(t *testing.T) {
	f := newProfileFixtures(t)
	p := &UserProfile{MonitoredAuthors: map[int64]bool{}}
	got := GenerateAuthorNew(context.Background(), f.books, f.authors, p)
	if len(got) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(got))
	}
}

func TestGenerateAuthorNew_SkipsNonWantedStatus(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	a := seedAuthor(t, f, "Auth", "OLA", true)
	b := &models.Book{
		ForeignID:        "OLDOWN",
		AuthorID:         a.ID,
		Title:            "Downloaded",
		SortTitle:        "Downloaded",
		Status:           models.BookStatusDownloaded,
		MetadataProvider: "openlibrary",
		Monitored:        true,
	}
	if err := f.books.Create(ctx, b); err != nil {
		t.Fatal(err)
	}

	p := &UserProfile{
		MonitoredAuthors: map[int64]bool{a.ID: true},
		OwnedForeignIDs:  map[string]bool{},
	}
	got := GenerateAuthorNew(ctx, f.books, f.authors, p)
	if len(got) != 0 {
		t.Errorf("downloaded books should not produce candidates, got %+v", got)
	}
}

// --- GenerateGenreSimilar ---

func TestGenerateGenreSimilar_SkipsStartedSeries(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	a := seedAuthor(t, f, "Auth", "OLA", false)
	b1 := seedBook(t, f, a.ID, "OLB1", "Book 1", []string{"fantasy"})
	b2 := seedBook(t, f, a.ID, "OLB2", "Book 2", []string{"fantasy"})

	s := seedSeries(t, f, "OLS1", "Started Series")
	_ = f.series.LinkBook(ctx, s.ID, b1.ID, "1", true)
	_ = f.series.LinkBook(ctx, s.ID, b2.ID, "2", true)

	// User already started this series.
	p := &UserProfile{
		GenreWeights: map[string]float64{"fantasy": 1.0},
		SeriesState: map[int64]SeriesState{
			s.ID: {SeriesID: s.ID, MaxPosition: 1},
		},
		OwnedForeignIDs: map[string]bool{b1.ForeignID: true},
	}
	got := GenerateGenreSimilar(ctx, f.books, f.series, p)
	if len(got) != 0 {
		t.Errorf("should skip books in started series, got %+v", got)
	}
}

func TestGenreSimilar_UnstartedSeries(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	a := seedAuthor(t, f, "Auth", "OLA", false)
	b1 := seedBook(t, f, a.ID, "OLB1", "Book 1", []string{"fantasy"})
	b2 := seedBook(t, f, a.ID, "OLB2", "Book 2", []string{"fantasy"})

	s := seedSeries(t, f, "OLS1", "Unstarted")
	_ = f.series.LinkBook(ctx, s.ID, b1.ID, "1", true)
	_ = f.series.LinkBook(ctx, s.ID, b2.ID, "2", true)

	p := &UserProfile{
		GenreWeights:    map[string]float64{"fantasy": 1.0},
		SeriesState:     map[int64]SeriesState{},
		OwnedForeignIDs: map[string]bool{},
	}
	got := GenerateGenreSimilar(ctx, f.books, f.series, p)
	if len(got) == 0 {
		t.Error("expected candidates from unstarted series")
	}
	for _, c := range got {
		if c.RecType != models.RecTypeGenreSimilar {
			t.Errorf("RecType: got %q", c.RecType)
		}
		if c.Reason == "" {
			t.Error("expected non-empty Reason")
		}
	}
}

// --- GenerateSerendipity ---

func TestGenerateSerendipity_FiltersOwned(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	a := seedAuthor(t, f, "Auth", "OLA", false)
	owned := seedBook(t, f, a.ID, "OLOWN", "Owned", nil)
	unowned := seedBook(t, f, a.ID, "OLNEW", "Unowned", nil)

	s := seedSeries(t, f, "OLS1", "Series")
	_ = f.series.LinkBook(ctx, s.ID, owned.ID, "1", true)
	_ = f.series.LinkBook(ctx, s.ID, unowned.ID, "2", true)

	p := &UserProfile{
		GenreWeights:    map[string]float64{"fantasy": 1.0},
		OwnedForeignIDs: map[string]bool{owned.ForeignID: true},
	}
	got := GenerateSerendipity(ctx, f.books, f.series, p, 10)
	for _, c := range got {
		if c.ForeignID == owned.ForeignID {
			t.Errorf("owned book should not appear: %+v", c)
		}
		if c.RecType != models.RecTypeSerendipity {
			t.Errorf("RecType: got %q", c.RecType)
		}
	}
}

func TestGenerateSerendipity_RespectsCount(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	a := seedAuthor(t, f, "Auth", "OLA", false)
	for i, fid := range []string{"A", "B", "C", "D", "E"} {
		b := seedBook(t, f, a.ID, "OL"+fid, "T"+fid, []string{"horror"})
		s := seedSeries(t, f, "S"+fid, "Series")
		_ = f.series.LinkBook(ctx, s.ID, b.ID, "1", true)
		_ = i
	}

	p := &UserProfile{
		GenreWeights:    map[string]float64{"fantasy": 1.0},
		OwnedForeignIDs: map[string]bool{},
	}
	got := GenerateSerendipity(ctx, f.books, f.series, p, 2)
	if len(got) > 2 {
		t.Errorf("expected at most 2, got %d", len(got))
	}
}

// --- Engine.Run ---

func TestEngine_Run_Disabled(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	// Without the "recommendations.enabled" setting, Run is a no-op.
	e := New(f.books, f.authors, f.series, f.recs, f.settings)
	if err := e.Run(ctx, f.userID); err != nil {
		t.Fatalf("Run: %v", err)
	}
	recs, err := f.recs.List(ctx, f.userID, "", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("disabled run should produce no recs, got %d", len(recs))
	}
}

func TestEngine_Run_EnabledEmptyLibrary(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	if err := f.settings.Set(ctx, "recommendations.enabled", "true"); err != nil {
		t.Fatalf("set: %v", err)
	}
	e := New(f.books, f.authors, f.series, f.recs, f.settings)
	if err := e.Run(ctx, f.userID); err != nil {
		t.Fatalf("Run: %v", err)
	}
	recs, err := f.recs.List(ctx, f.userID, "", 100, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("empty library should yield 0 recs, got %d", len(recs))
	}
}

func TestEngine_Run_WithMonitoredAuthorProducesCandidate(t *testing.T) {
	f := newProfileFixtures(t)
	ctx := context.Background()

	if err := f.settings.Set(ctx, "recommendations.enabled", "true"); err != nil {
		t.Fatal(err)
	}

	a := seedAuthor(t, f, "Mon", "OLA_M", true)
	// Wanted book from monitored author, not owned.
	seedBook(t, f, a.ID, "OLW", "Wanted", []string{"fantasy"})

	e := New(f.books, f.authors, f.series, f.recs, f.settings)
	if err := e.Run(ctx, f.userID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	recs, err := f.recs.List(ctx, f.userID, "", 100, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, r := range recs {
		if r.ForeignID == "OLW" {
			found = true
		}
	}
	if !found {
		t.Errorf("wanted book from monitored author should appear as a recommendation")
	}
}

func TestEngine_WithClients(t *testing.T) {
	f := newProfileFixtures(t)
	e := New(f.books, f.authors, f.series, f.recs, f.settings)

	ol := &fakeSubjectFetcher{}
	hc := &fakeWishlistFetcher{}

	if ret := e.WithOLClient(ol); ret != e {
		t.Error("WithOLClient should return the engine")
	}
	if ret := e.WithHCClient(hc); ret != e {
		t.Error("WithHCClient should return the engine")
	}
	if e.olClient != ol {
		t.Error("olClient not wired")
	}
	if e.hcClient != hc {
		t.Error("hcClient not wired")
	}
}

// Ensure the package compiles with the db import path above.
var _ = db.OpenMemory
