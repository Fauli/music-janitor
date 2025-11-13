package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/franz/music-janitor/internal/util"
)

const (
	// BaseURL is the MusicBrainz API base URL
	BaseURL = "https://musicbrainz.org/ws/2"

	// UserAgent identifies this application to MusicBrainz
	// MusicBrainz requires a proper user agent
	UserAgent = "MLC-MusicLibraryCleaner/1.3.0 (https://github.com/franz/music-janitor)"

	// RateLimit is the maximum requests per second (MusicBrainz requirement)
	RateLimit = 1 * time.Second
)

// Client handles MusicBrainz API requests with rate limiting
type Client struct {
	httpClient  *http.Client
	userAgent   string
	rateLimiter *time.Ticker
	lastRequest time.Time
}

// NewClient creates a new MusicBrainz API client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		userAgent:   UserAgent,
		rateLimiter: time.NewTicker(RateLimit),
		lastRequest: time.Now().Add(-RateLimit), // Allow first request immediately
	}
}

// Close releases resources used by the client
func (c *Client) Close() {
	if c.rateLimiter != nil {
		c.rateLimiter.Stop()
	}
}

// ArtistSearchResult represents a search result from MusicBrainz
type ArtistSearchResult struct {
	Artists []Artist `json:"artists"`
	Count   int      `json:"count"`
	Offset  int      `json:"offset"`
	Created string   `json:"created"`
}

// Artist represents an artist from MusicBrainz
type Artist struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	SortName       string    `json:"sort-name"`
	Score          int       `json:"score"` // MusicBrainz returns as integer
	Type           string    `json:"type"`
	Country        string    `json:"country"`
	Disambiguation string    `json:"disambiguation"`
	Aliases        []Alias   `json:"aliases"`
	LifeSpan       *LifeSpan `json:"life-span"`
}

// Alias represents an artist alias
type Alias struct {
	Name      string `json:"name"`
	SortName  string `json:"sort-name"`
	Locale    string `json:"locale"`
	Type      string `json:"type"`
	Primary   *bool  `json:"primary"`
	BeginDate string `json:"begin-date"`
	EndDate   string `json:"end-date"`
}

// LifeSpan represents an artist's active period
type LifeSpan struct {
	Begin string `json:"begin"`
	End   string `json:"end"`
	Ended bool   `json:"ended"`
}

// SearchArtist searches for an artist by name
// Returns the best matching artist with aliases
func (c *Client) SearchArtist(ctx context.Context, name string) (*Artist, error) {
	if name == "" {
		return nil, fmt.Errorf("artist name cannot be empty")
	}

	// Wait for rate limit
	c.waitForRateLimit()

	// Build query URL
	query := url.QueryEscape(name)
	urlStr := fmt.Sprintf("%s/artist/?query=%s&fmt=json&limit=5", BaseURL, query)

	util.DebugLog("MusicBrainz API: searching for artist '%s'", name)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode == 503 {
		return nil, fmt.Errorf("MusicBrainz service unavailable (503) - rate limit exceeded or maintenance")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result ArtistSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Return best match
	if len(result.Artists) == 0 {
		util.DebugLog("MusicBrainz: no results for '%s'", name)
		return nil, nil
	}

	// Get the top result
	artist := &result.Artists[0]
	util.DebugLog("MusicBrainz: found '%s' (score: %d, MBID: %s)", artist.Name, artist.Score, artist.ID)

	// If we got a good match but no aliases, fetch them separately
	if artist.Score >= 90 && len(artist.Aliases) == 0 {
		enriched, err := c.LookupArtist(ctx, artist.ID)
		if err != nil {
			util.WarnLog("Failed to fetch aliases for %s: %v", artist.Name, err)
		} else if enriched != nil {
			artist.Aliases = enriched.Aliases
		}
	}

	return artist, nil
}

// LookupArtist retrieves full artist details including aliases by MBID
func (c *Client) LookupArtist(ctx context.Context, mbid string) (*Artist, error) {
	if mbid == "" {
		return nil, fmt.Errorf("MBID cannot be empty")
	}

	// Wait for rate limit
	c.waitForRateLimit()

	// Build lookup URL with aliases included
	urlStr := fmt.Sprintf("%s/artist/%s?fmt=json&inc=aliases", BaseURL, mbid)

	util.DebugLog("MusicBrainz API: looking up artist %s", mbid)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode == 503 {
		return nil, fmt.Errorf("MusicBrainz service unavailable (503)")
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("artist not found (404)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var artist Artist
	if err := json.NewDecoder(resp.Body).Decode(&artist); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	util.DebugLog("MusicBrainz: retrieved '%s' with %d aliases", artist.Name, len(artist.Aliases))

	return &artist, nil
}

// GetCanonicalName returns the canonical name and all aliases for an artist
func (c *Client) GetCanonicalName(ctx context.Context, artistName string) (canonical string, aliases []string, err error) {
	if artistName == "" {
		return "", nil, fmt.Errorf("artist name cannot be empty")
	}

	artist, err := c.SearchArtist(ctx, artistName)
	if err != nil {
		return "", nil, err
	}

	if artist == nil {
		// No match found - use original name
		return artistName, nil, nil
	}

	// Only use MusicBrainz result if score is high enough (>= 90%)
	if artist.Score < 90 {
		util.DebugLog("MusicBrainz: low confidence match (%d) for '%s', using original", artist.Score, artistName)
		return artistName, nil, nil
	}

	// Canonical name from MusicBrainz
	canonical = artist.Name

	// Extract all aliases
	aliases = make([]string, 0, len(artist.Aliases))
	for _, alias := range artist.Aliases {
		if alias.Name != "" && alias.Name != canonical {
			aliases = append(aliases, alias.Name)
		}
	}

	util.DebugLog("MusicBrainz: '%s' -> canonical: '%s', %d aliases", artistName, canonical, len(aliases))

	return canonical, aliases, nil
}

// NormalizeArtistWithMB normalizes an artist name using MusicBrainz data
// Returns the normalized canonical name
func (c *Client) NormalizeArtistWithMB(ctx context.Context, artistName string) (string, error) {
	canonical, _, err := c.GetCanonicalName(ctx, artistName)
	if err != nil {
		return "", err
	}
	return canonical, nil
}

// IsAlias checks if a given name is an alias of the canonical artist name
func (c *Client) IsAlias(ctx context.Context, name1, name2 string) (bool, string, error) {
	if name1 == "" || name2 == "" {
		return false, "", fmt.Errorf("names cannot be empty")
	}

	// Quick check - if they're the same, no need to query
	if strings.EqualFold(name1, name2) {
		return true, name1, nil
	}

	// Look up both names
	canonical1, aliases1, err := c.GetCanonicalName(ctx, name1)
	if err != nil {
		return false, "", err
	}

	canonical2, aliases2, err := c.GetCanonicalName(ctx, name2)
	if err != nil {
		return false, "", err
	}

	// Check if canonical names match
	if strings.EqualFold(canonical1, canonical2) {
		return true, canonical1, nil
	}

	// Check if name2 is an alias of name1
	for _, alias := range aliases1 {
		if strings.EqualFold(alias, name2) || strings.EqualFold(alias, canonical2) {
			return true, canonical1, nil
		}
	}

	// Check if name1 is an alias of name2
	for _, alias := range aliases2 {
		if strings.EqualFold(alias, name1) || strings.EqualFold(alias, canonical1) {
			return true, canonical2, nil
		}
	}

	return false, "", nil
}

// waitForRateLimit ensures we don't exceed MusicBrainz rate limit (1 req/sec)
func (c *Client) waitForRateLimit() {
	// Wait for next tick
	<-c.rateLimiter.C
	c.lastRequest = time.Now()
}
