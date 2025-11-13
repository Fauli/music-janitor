# MusicBrainz API Reference Documentation

**Version**: API v2 (Web Service v2)
**Base URL**: `https://musicbrainz.org/ws/2/`
**Documentation**: https://musicbrainz.org/doc/MusicBrainz_API

---

## Table of Contents

- [Overview](#overview)
- [Rate Limiting](#rate-limiting)
- [Authentication](#authentication)
- [Request Format](#request-format)
- [Response Format](#response-format)
- [Artist Search](#artist-search)
- [Artist Lookup](#artist-lookup)
- [Common Issues](#common-issues)
- [Best Practices](#best-practices)
- [Error Handling](#error-handling)

---

## Overview

The MusicBrainz API provides access to the MusicBrainz Database, which contains information about:
- Artists, albums, tracks, recordings
- Relationships between entities
- Tags, ratings, and user-submitted data
- MusicBrainz Identifiers (MBIDs)

### Core Entities

| Entity | Endpoint | Description |
|--------|----------|-------------|
| Artist | `/ws/2/artist/` | Musicians, bands, orchestras |
| Release | `/ws/2/release/` | Albums, singles, compilations |
| Recording | `/ws/2/recording/` | Unique versions of songs |
| Release Group | `/ws/2/release-group/` | Logical album groupings |
| Work | `/ws/2/work/` | Compositional works |
| Label | `/ws/2/label/` | Record labels |
| Area | `/ws/2/area/` | Geographic areas |

---

## Rate Limiting

### Rate Limit Rules

**CRITICAL**: MusicBrainz enforces **strict rate limiting** to protect server resources.

| Type | Limit | Scope |
|------|-------|-------|
| **IP-Based** | **1 request/second (average)** | Per source IP address |
| **User-Agent** | 50 requests/second | Specific user agents (varies) |
| **Global** | 300 requests/second | All requests combined |

### Rate Limit Behavior

When limits are exceeded:
1. **All requests are rejected** (not partial - 100% rejection)
2. HTTP `503 Service Unavailable` is returned
3. Rejection continues until rate drops below limit
4. **Repeated violations may result in IP blocking**

### Checking Order

Rate limiting checks occur in this order:
1. User-Agent check
2. Source IP check
3. Global limit check

If any check fails, the request is **immediately denied**.

### Example Violation Scenario

```
Scenario: 4 requests/second
Result: 100% of requests declined (not 75%)
Recovery: Wait until rate drops to ≤1 req/sec
```

### Implementation Requirements

```go
// Minimum delay between requests
const RateLimit = 1 * time.Second

// Use a ticker or rate limiter
rateLimiter := time.NewTicker(RateLimit)
defer rateLimiter.Stop()

// Wait before each request
<-rateLimiter.C
// Make request
```

**Important**: Do not use synchronized timing (e.g., all clients waking at midnight). Spread requests throughout the day.

---

## Authentication

### Data Retrieval (Read-Only)

**No authentication required** for:
- Searching artists, releases, recordings
- Looking up entities by MBID
- Reading metadata

**Requirements**:
- Valid User-Agent header (mandatory)
- Rate limiting compliance

### Data Submission (Write Operations)

**HTTP Digest Authentication required** for:
- Submitting new data
- Editing existing data
- Adding ratings/tags

**Credentials**:
- Same as MusicBrainz website login
- Realm: `"musicbrainz.org"`

**Additional Requirements**:
- `client` parameter with format: `"application-version"`
- Content-Type: `"application/xml; charset=utf-8"`

---

## Request Format

### Required Headers

```http
GET /ws/2/artist/?query=beatles&fmt=json HTTP/1.1
Host: musicbrainz.org
User-Agent: MLC-MusicLibraryCleaner/1.7.0 (https://github.com/franz/music-janitor)
Accept: application/json
```

| Header | Required | Description | Example |
|--------|----------|-------------|---------|
| `User-Agent` | **YES** | Identifies your application | `"AppName/1.0 (contact@example.com)"` |
| `Accept` | Recommended | Response format preference | `"application/json"` |

### User-Agent Format

**Recommended format**:
```
AppName/Version (contact-info)
```

**Examples**:
```
MLC-MusicLibraryCleaner/1.7.0 (https://github.com/user/repo)
MyTagger/2.1.0 (support@example.com)
AwesomePlayer/1.0.0 (http://awesome.example.com)
```

**Why it matters**:
- MusicBrainz may contact you if your app misbehaves
- Apps can be throttled individually by User-Agent
- Generic/missing User-Agents may be blocked

### Query Parameters

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `query` | string | Lucene search query | - |
| `fmt` | string | Response format: `json` or `xml` | `xml` |
| `limit` | integer | Results per page (1-100) | `25` |
| `offset` | integer | Pagination offset | `0` |
| `inc` | string | Include subqueries (comma-separated) | - |

---

## Response Format

### Format Selection

**XML (default)**:
```bash
curl "https://musicbrainz.org/ws/2/artist/?query=beatles"
```

**JSON (two methods)**:

1. Via Accept header:
```bash
curl -H "Accept: application/json" \
  "https://musicbrainz.org/ws/2/artist/?query=beatles"
```

2. Via query parameter:
```bash
curl "https://musicbrainz.org/ws/2/artist/?query=beatles&fmt=json"
```

**Note**: If both are specified, `fmt=` parameter takes precedence.

### JSON Response Structure

All search responses follow this pattern:

```json
{
  "created": "2025-01-13T12:34:56.789Z",
  "count": 42,
  "offset": 0,
  "<entities>": [ /* array of entity objects */ ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `created` | ISO 8601 timestamp | When response was generated |
| `count` | integer | Total results available |
| `offset` | integer | Current pagination offset |
| `<entities>` | array | Entity-specific array (e.g., `artists`) |

---

## Artist Search

### Endpoint

```
GET /ws/2/artist/?query={LUCENE_QUERY}&fmt=json&limit={N}
```

### Search Fields

| Field | Description | Example |
|-------|-------------|---------|
| `artist` | Artist name (diacritics ignored) | `artist:beatles` |
| `artistaccent` | Artist name (diacritics preserved) | `artistaccent:björk` |
| `alias` | Any alias (diacritics ignored) | `alias:fab` |
| `primary_alias` | Primary aliases only | `primary_alias:"the beatles"` |
| `sortname` | Sort name | `sortname:"beatles, the"` |
| `type` | Artist type | `type:group` |
| `country` | ISO 3166-1 alpha-2 code | `country:GB` |
| `area` | Main associated area | `area:liverpool` |
| `beginarea` | Begin area | `beginarea:liverpool` |
| `endarea` | End area | `endarea:london` |
| `begin` | Begin date (YYYY-MM-DD) | `begin:1960` |
| `end` | End date (YYYY-MM-DD) | `end:1970-04-10` |
| `ended` | Ended flag | `ended:true` |
| `gender` | Gender | `gender:male` |
| `comment` | Disambiguation comment | `comment:"uk rock band"` |
| `ipi` | IPI code | `ipi:00053019745` |
| `isni` | ISNI code | `isni:0000000121707484` |
| `tag` | Attached tags | `tag:rock` |
| `arid` | MusicBrainz ID | `arid:b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d` |

**Default search**: If no field is specified, searches `artist`, `alias`, and `sortname`.

### Query Syntax (Lucene)

```
Simple:        artist:beatles
AND:           artist:beatles AND country:GB
OR:            artist:beatles OR artist:stones
NOT:           artist:beatles NOT type:person
Phrase:        artist:"the beatles"
Wildcard:      artist:beat*
Fuzzy:         artist:beatls~
Range:         begin:[1960 TO 1970]
Boost:         artist:beatles^2
Grouping:      (artist:beatles OR artist:stones) AND country:GB
```

### Artist Search Response

**Request**:
```bash
GET /ws/2/artist/?query=beatles&fmt=json&limit=1
```

**Response**:
```json
{
  "created": "2025-01-13T10:30:00.123Z",
  "count": 23,
  "offset": 0,
  "artists": [
    {
      "id": "b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d",
      "type": "Group",
      "type-id": "e431f5f6-b5d2-343d-8b36-72607fffb74b",
      "score": 100,
      "name": "The Beatles",
      "sort-name": "Beatles, The",
      "country": "GB",
      "area": {
        "id": "8a754a16-0027-3a29-b6d7-2b40ea0481ed",
        "type": "Country",
        "type-id": "06dd0ae4-8c74-30bb-b43d-95dcedf961de",
        "name": "United Kingdom",
        "sort-name": "United Kingdom",
        "life-span": {
          "ended": null
        }
      },
      "begin-area": {
        "id": "c249c30e-88ab-4b2f-a745-96a25bd7afee",
        "type": "City",
        "type-id": "6fd8f29a-3d0a-32fc-980d-ea697b69da78",
        "name": "Liverpool",
        "sort-name": "Liverpool",
        "life-span": {
          "ended": null
        }
      },
      "disambiguation": "UK rock band, \"The Fab Four\"",
      "isnis": ["0000000121707484"],
      "life-span": {
        "begin": "1960",
        "end": "1970-04-10",
        "ended": true
      },
      "aliases": [
        {
          "sort-name": "Beatles",
          "name": "Beatles",
          "locale": null,
          "type": null,
          "primary": null,
          "begin-date": null,
          "end-date": null
        }
      ],
      "tags": [
        {
          "count": 12,
          "name": "rock"
        }
      ]
    }
  ]
}
```

### Artist Object Structure

| Field | Type | Description | Nullable |
|-------|------|-------------|----------|
| `id` | string (UUID) | MusicBrainz ID (MBID) | No |
| `name` | string | Artist name | No |
| `sort-name` | string | Sortable name | No |
| `score` | **integer** | Search relevance (0-100) | No |
| `type` | string | `Person`, `Group`, `Orchestra`, `Choir`, `Character`, `Other` | Yes |
| `type-id` | string (UUID) | Type identifier | Yes |
| `country` | string | ISO 3166-1 alpha-2 code | Yes |
| `gender` | string | `Male`, `Female`, `Other`, `Not applicable` | Yes |
| `disambiguation` | string | Disambiguation comment | Yes |
| `area` | object | Main associated area | Yes |
| `begin-area` | object | Formation/birth area | Yes |
| `end-area` | object | Dissolution/death area | Yes |
| `life-span` | object | Active period | Yes |
| `aliases` | array | Alternative names | Yes |
| `tags` | array | User-submitted tags | Yes |
| `isnis` | array[string] | ISNI codes | Yes |
| `ipis` | array[string] | IPI codes | Yes |

### Score Field ⚠️ IMPORTANT

**Data Type**: `integer` (NOT string)

```json
{
  "score": 100
}
```

**Common Bug**: Using `json:"score,string"` in Go struct tags

**Incorrect**:
```go
type Artist struct {
    Score int `json:"score,string"` // ❌ WRONG - causes decode error
}
```

**Correct**:
```go
type Artist struct {
    Score int `json:"score"` // ✅ CORRECT - score is an integer
}
```

**Error message when using `,string` tag**:
```
json: invalid use of ,string struct tag, trying to unmarshal unquoted value into int
```

**Historical Context**: Older MusicBrainz API documentation examples showed score as a string (`"score": "100"`), but the current API returns it as an integer. Always test with live API responses.

---

## Artist Lookup

### Endpoint

```
GET /ws/2/artist/{MBID}?fmt=json&inc={INCLUDES}
```

### Available Includes

Include additional data using `inc` parameter (comma-separated):

| Include | Description | Example |
|---------|-------------|---------|
| `aliases` | Artist aliases | `inc=aliases` |
| `recordings` | Artist's recordings | `inc=recordings` |
| `releases` | Artist's releases | `inc=releases` |
| `release-groups` | Release groups | `inc=release-groups` |
| `works` | Musical works | `inc=works` |
| `annotation` | Artist annotation | `inc=annotation` |
| `tags` | Tags | `inc=tags` |
| `ratings` | User ratings | `inc=ratings` |
| `url-rels` | URL relationships | `inc=url-rels` |
| `artist-rels` | Artist relationships | `inc=artist-rels` |

**Multiple includes**:
```
inc=aliases+tags+url-rels
```

### Lookup Example

**Request**:
```bash
GET /ws/2/artist/b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d?fmt=json&inc=aliases
```

**Response**:
```json
{
  "id": "b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d",
  "name": "The Beatles",
  "sort-name": "Beatles, The",
  "type": "Group",
  "type-id": "e431f5f6-b5d2-343d-8b36-72607fffb74b",
  "country": "GB",
  "disambiguation": "UK rock band, \"The Fab Four\"",
  "life-span": {
    "begin": "1960",
    "end": "1970-04-10",
    "ended": true
  },
  "aliases": [
    {
      "sort-name": "Beatles",
      "name": "Beatles",
      "locale": null,
      "type": null,
      "primary": null,
      "begin-date": null,
      "end-date": null
    },
    {
      "sort-name": "ビートルズ",
      "type-id": "894afba6-2816-3c24-8072-eadb66bd04bc",
      "name": "ビートルズ",
      "locale": "ja",
      "type": "Artist name",
      "primary": true,
      "begin-date": null,
      "end-date": null
    }
  ]
}
```

**Note**: Lookup responses do NOT include a `score` field (only search results have scores).

---

## Common Issues

### Issue 1: Score Field Type Error

**Error**:
```
json: invalid use of ,string struct tag, trying to unmarshal unquoted value into int
```

**Cause**: Using `json:"score,string"` when MusicBrainz returns integer.

**Solution**:
```go
// Change from:
Score int `json:"score,string"`

// To:
Score int `json:"score"`
```

### Issue 2: Rate Limit 503 Errors

**Error**:
```
503 Service Unavailable
```

**Causes**:
1. Exceeding 1 request/second average
2. Missing User-Agent header
3. Generic/blocked User-Agent
4. Server maintenance

**Solutions**:
```go
// Use rate limiter
rateLimiter := time.NewTicker(1 * time.Second)

// Set proper User-Agent
req.Header.Set("User-Agent", "MyApp/1.0 (contact@example.com)")

// Implement exponential backoff
if resp.StatusCode == 503 {
    time.Sleep(5 * time.Second)
    // Retry
}
```

### Issue 3: Empty Results

**Problem**: Search returns no results for known artist.

**Causes**:
1. Query syntax error
2. Too specific query
3. Typo in artist name

**Solutions**:
```go
// Start with simple query
query = "artist:beatles"

// Use fuzzy search
query = "artist:beatls~"  // Tolerates typos

// Check if field is specified correctly
query = url.QueryEscape("beatles")  // Searches all default fields
```

### Issue 4: Missing Aliases

**Problem**: Search result doesn't include aliases.

**Cause**: Search endpoint returns limited data by default.

**Solution**:
```go
// After finding artist in search:
if artist.Score >= 90 && len(artist.Aliases) == 0 {
    // Fetch full artist with aliases
    fullArtist, _ := client.LookupArtist(ctx, artist.ID)
    if fullArtist != nil {
        artist.Aliases = fullArtist.Aliases
    }
}
```

---

## Best Practices

### 1. Caching

**Recommended**: Cache MusicBrainz responses locally

```go
type ArtistCache struct {
    mu    sync.RWMutex
    cache map[string]*CachedArtist
}

type CachedArtist struct {
    Artist    *Artist
    FetchedAt time.Time
}

func (c *ArtistCache) Get(name string) (*Artist, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    cached, ok := c.cache[strings.ToLower(name)]
    if !ok {
        return nil, false
    }

    // Cache for 30 days (metadata changes rarely)
    if time.Since(cached.FetchedAt) > 30*24*time.Hour {
        return nil, false
    }

    return cached.Artist, true
}
```

**Why**: Metadata changes infrequently, caching reduces API load.

### 2. Score Thresholds

**Recommended score thresholds**:

| Score | Confidence | Action |
|-------|------------|--------|
| 100 | Perfect match | Use immediately |
| 90-99 | High confidence | Use (recommended minimum) |
| 80-89 | Medium confidence | Consider using |
| 70-79 | Low confidence | Require user confirmation |
| <70 | Very low | Reject, use original name |

**Implementation**:
```go
if artist.Score < 90 {
    // Don't use low-confidence matches
    return originalName, nil
}
```

### 3. Error Handling

**Implement retry logic**:

```go
func (c *Client) SearchArtistWithRetry(ctx context.Context, name string) (*Artist, error) {
    maxRetries := 3
    baseDelay := 2 * time.Second

    for attempt := 0; attempt < maxRetries; attempt++ {
        artist, err := c.SearchArtist(ctx, name)

        if err == nil {
            return artist, nil
        }

        // Only retry on 503 errors
        if strings.Contains(err.Error(), "503") {
            delay := baseDelay * time.Duration(1<<attempt) // Exponential backoff
            time.Sleep(delay)
            continue
        }

        // Don't retry other errors
        return nil, err
    }

    return nil, fmt.Errorf("max retries exceeded")
}
```

### 4. Batch Operations

**Problem**: Need to normalize 10,000 artist names.

**Bad approach**:
```go
// ❌ Sequential requests (takes 10,000 seconds = 2.7 hours)
for _, artist := range artists {
    normalized, _ := client.NormalizeArtist(ctx, artist)
}
```

**Good approach**:
```go
// ✅ Deduplicate + cache + rate limit
uniqueArtists := deduplicateArtists(artists)
results := make(map[string]string)

for _, artist := range uniqueArtists {
    // Check cache first
    if cached, ok := cache.Get(artist); ok {
        results[artist] = cached
        continue
    }

    // Rate-limited API call
    normalized, _ := client.NormalizeArtist(ctx, artist)
    results[artist] = normalized
    cache.Set(artist, normalized)

    // Respect rate limit
    time.Sleep(1 * time.Second)
}
```

### 5. User-Agent Versioning

**Update User-Agent with version changes**:

```go
const (
    AppName    = "MLC-MusicLibraryCleaner"
    AppVersion = "1.7.0"
    AppContact = "https://github.com/franz/music-janitor"
)

var UserAgent = fmt.Sprintf("%s/%s (%s)", AppName, AppVersion, AppContact)
```

**Benefits**:
- Track which version is causing issues
- MusicBrainz can contact you about problems
- Debugging is easier

---

## Error Handling

### HTTP Status Codes

| Code | Meaning | Action |
|------|---------|--------|
| `200` | Success | Process response |
| `400` | Bad Request | Fix query syntax |
| `404` | Not Found | Entity doesn't exist |
| `503` | Service Unavailable | Rate limit or maintenance - retry with backoff |
| `5xx` | Server Error | Temporary - retry later |

### Error Response Structure

**404 Not Found**:
```json
{
  "error": "Not Found"
}
```

**503 Service Unavailable** (rate limit):
```
No JSON body, just HTTP 503 status
```

### Handling Strategies

```go
func handleMBResponse(resp *http.Response) error {
    switch resp.StatusCode {
    case 200:
        return nil

    case 400:
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("bad request: %s", string(body))

    case 404:
        return fmt.Errorf("not found")

    case 503:
        return &RateLimitError{
            Message: "rate limit exceeded or service unavailable",
            RetryAfter: 5 * time.Second,
        }

    default:
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
    }
}

type RateLimitError struct {
    Message    string
    RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
    return e.Message
}
```

---

## Testing Considerations

### Use Real API Sparingly

**Don't**:
```go
// ❌ Test calls real API on every run
func TestSearchArtist(t *testing.T) {
    client := NewClient()
    artist, _ := client.SearchArtist(context.Background(), "Beatles")
    assert.NotNil(t, artist)
}
```

**Do**:
```go
// ✅ Use recorded responses or mocks
func TestSearchArtist(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{
            "count": 1,
            "offset": 0,
            "artists": [{
                "id": "b10bbbfc-cf9e-42e0-be17-e2c3e1d2600d",
                "name": "The Beatles",
                "score": 100
            }]
        }`))
    }))
    defer server.Close()

    client := &Client{httpClient: server.Client()}
    // Override baseURL to point to test server
    // Test with mock data
}
```

### Rate Limit Testing

```go
// Test that rate limiter works
func TestRateLimiting(t *testing.T) {
    client := NewClient()
    defer client.Close()

    start := time.Now()

    // Make 3 requests
    for i := 0; i < 3; i++ {
        client.waitForRateLimit()
    }

    elapsed := time.Since(start)

    // Should take at least 2 seconds (3 requests - 1)
    assert.GreaterOrEqual(t, elapsed, 2*time.Second)
}
```

---

## Debugging Checklist

When MusicBrainz API calls fail:

- [ ] Check User-Agent header is set and meaningful
- [ ] Verify rate limiting (≤1 req/sec)
- [ ] Confirm request URL is properly encoded
- [ ] Check `fmt=json` parameter for JSON responses
- [ ] Verify score field is `int`, not `int json:"score,string"`
- [ ] Test query syntax in browser: `https://musicbrainz.org/ws/2/artist/?query=beatles&fmt=json`
- [ ] Check network connectivity to musicbrainz.org
- [ ] Review response body for error messages
- [ ] Verify HTTP status code handling
- [ ] Check for nil pointer dereferences on optional fields
- [ ] Confirm context timeout is reasonable (≥30s recommended)

---

## Additional Resources

- **Official API Docs**: https://musicbrainz.org/doc/MusicBrainz_API
- **Search Documentation**: https://musicbrainz.org/doc/MusicBrainz_API/Search
- **Rate Limiting**: https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting
- **Examples**: https://musicbrainz.org/doc/MusicBrainz_API/Examples
- **Community Forums**: https://community.metabrainz.org/
- **GitHub (Server)**: https://github.com/metabrainz/musicbrainz-server
- **MusicBrainz Picard** (reference implementation): https://picard.musicbrainz.org/

---

## Changelog

### 2025-01-13
- Initial documentation created
- Confirmed score field is integer (not string)
- Documented rate limiting behavior
- Added comprehensive error handling guidance

---

**Last Updated**: 2025-01-13
**API Version**: v2
**Document Maintainer**: music-janitor project
