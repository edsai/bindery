package models

import "time"

// Recommendation type constants.
const (
	RecTypeSeries       = "series"
	RecTypeAuthorNew    = "author_new"
	RecTypeGenreSimilar = "genre_similar"
	RecTypeGenrePopular = "genre_popular"
	RecTypeListCross    = "list_cross"
	RecTypeSerendipity  = "serendipity"
)

// Recommendation is a persisted book recommendation shown on the Discover page.
type Recommendation struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"userId"`
	ForeignID    string     `json:"foreignId"`
	RecType      string     `json:"recType"`
	Title        string     `json:"title"`
	AuthorName   string     `json:"authorName"`
	AuthorID     *int64     `json:"authorId,omitempty"`
	ImageURL     string     `json:"imageUrl"`
	Description  string     `json:"description"`
	Genres       []string   `json:"genres"`
	Rating       float64    `json:"rating"`
	RatingsCount int        `json:"ratingsCount"`
	ReleaseDate  *time.Time `json:"releaseDate"`
	Language     string     `json:"language"`
	MediaType    string     `json:"mediaType"`
	Score        float64    `json:"score"`
	Reason       string     `json:"reason"`
	SeriesID     *int64     `json:"seriesId,omitempty"`
	SeriesPos    string     `json:"seriesPos"`
	Dismissed    bool       `json:"dismissed"`
	BatchID      string     `json:"batchId"`
	CreatedAt    time.Time  `json:"createdAt"`
}

// RecommendationCandidate is the shared struct used by both the recommender
// engine (to produce candidates) and the db layer (to persist them). Lives in
// models to break the circular import between internal/recommender and internal/db.
type RecommendationCandidate struct {
	ForeignID    string
	RecType      string
	Title        string
	AuthorName   string
	AuthorID     *int64
	ImageURL     string
	Description  string
	Genres       []string
	Rating       float64
	RatingsCount int
	ReleaseDate  *time.Time
	Language     string
	MediaType    string
	SeriesID     *int64
	SeriesPos    string
	Score        float64
	Reason       string
	// DedupKey is the canonical cross-edition work key (#940), copied from the
	// source Book. The recommender uses it to collapse multiple editions of the
	// same work to a single best edition. Empty for candidates built from
	// external sources (e.g. OpenLibrary) that have no Book row.
	DedupKey string
}
