package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alvinunreal/lazyskills/internal/compat"
)

// Skill represents the sanitized/mapped registry skill fields.
type Skill struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Slug        string `json:"slug"`
	Source      string `json:"source"`
	Installs    int    `json:"installs"`
	Invalid     bool   `json:"invalid,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// Client represents the skills.sh registry API client.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient returns a registry Client initialized with base URL from environment or fallback, and a default timeout.
func NewClient() *Client {
	baseURL := os.Getenv("SKILLS_API_URL")
	if baseURL == "" {
		baseURL = "https://skills.sh"
	}
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 9 * time.Second,
		},
	}
}

type apiResponse struct {
	Skills []struct {
		ID       string `json:"id"`
		SkillID  string `json:"skillId"`
		Name     string `json:"name"`
		Installs int    `json:"installs"`
		Source   string `json:"source"`
	} `json:"skills"`
}

// Search queries the registry API for matching skills.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Skill, error) {
	if len(query) < 2 {
		return nil, nil
	}

	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("SKILLS_API_URL")
		if baseURL == "" {
			baseURL = "https://skills.sh"
		}
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 9 * time.Second,
		}
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL %q: %w", baseURL, err)
	}

	u := parsedURL.ResolveReference(&url.URL{
		Path: "/api/search",
	})
	q := url.Values{}
	q.Set("q", query)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registry search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected registry response status: %d", resp.StatusCode)
	}

	var res apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to parse registry json response: %w", err)
	}

	skills := make([]Skill, 0, len(res.Skills))
	for _, raw := range res.Skills {
		slugRaw := raw.SkillID
		if slugRaw == "" {
			slugRaw = raw.Name
		}
		id := compat.SanitizeMetadata(raw.ID)
		displayName := compat.SanitizeMetadata(raw.Name)
		slug := compat.SanitizeMetadata(slugRaw)
		source := compat.SanitizeMetadata(raw.Source)
		invalid, reason := invalidCommandFields(slugRaw, raw.Source, slug, source)
		skills = append(skills, Skill{
			ID:          id,
			DisplayName: displayName,
			Slug:        slug,
			Source:      source,
			Installs:    raw.Installs,
			Invalid:     invalid,
			Reason:      reason,
		})
	}

	// Sort by installs desc if API order is not reliable.
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Installs > skills[j].Installs
	})

	return skills, nil
}

func invalidCommandFields(rawSlug, rawSource, slug, source string) (bool, string) {
	if rawSlug == "" || rawSource == "" {
		return true, "registry result is missing an install slug or source"
	}
	if rawSlug != slug {
		return true, "registry skill slug contains unsafe characters"
	}
	if rawSource != source {
		return true, "registry source contains unsafe characters"
	}
	if strings.HasPrefix(slug, "-") || strings.HasPrefix(source, "-") {
		return true, "registry install slug or source is option-like"
	}
	return false, ""
}
