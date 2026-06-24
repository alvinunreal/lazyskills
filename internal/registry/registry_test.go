package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSearchSuccessAndSorting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"skills": [
				{"id": "owner/one/skill-1", "skillId": "skill-1", "name": "Skill One", "installs": 10, "source": "github.com/one"},
				{"id": "owner/two/skill-2", "skillId": "skill-2", "name": "Skill Two", "installs": 99, "source": "github.com/two"}
			]
		}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
	}

	skills, err := client.Search(context.Background(), "test-query", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	// Verify sorting by installs desc
	if skills[0].Slug != "skill-2" || skills[1].Slug != "skill-1" {
		t.Errorf("expected skill-2 first (99 installs) then skill-1 (10 installs), got order: %s, %s", skills[0].Slug, skills[1].Slug)
	}

	if skills[0].DisplayName != "Skill Two" || skills[0].Source != "github.com/two" || skills[0].Installs != 99 {
		t.Errorf("unexpected fields for skill[0]: %+v", skills[0])
	}
	if skills[0].ID != "owner/two/skill-2" {
		t.Errorf("expected registry id to be preserved separately, got %q", skills[0].ID)
	}
}

func TestSearchFallsBackToNameForMissingSkillID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"skills": [
				{"id": "owner/repo/friendly-skill", "name": "friendly-skill", "installs": 10, "source": "owner/repo"}
			]
		}`))
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL}
	skills, err := client.Search(context.Background(), "friendly", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 || skills[0].Slug != "friendly-skill" {
		t.Fatalf("expected name fallback slug, got %+v", skills)
	}
}

func TestSearchEscaping(t *testing.T) {
	var requestedRawQuery string
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"skills": []}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
	}

	query := "hello & world / ?"
	_, err := client.Search(context.Background(), query, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if requestedPath != "/api/search" {
		t.Errorf("expected path /api/search, got %q", requestedPath)
	}

	// Let's check query values
	values, err := url.ParseQuery(requestedRawQuery)
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}
	if values.Get("q") != query {
		t.Errorf("expected query parameter q=%q, got %q", query, values.Get("q"))
	}
	if values.Get("limit") != "3" {
		t.Errorf("expected limit=3, got %q", values.Get("limit"))
	}
}

func TestSearchNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
	}

	_, err := client.Search(context.Background(), "query", 5)
	if err == nil {
		t.Fatal("expected error for non-200 response, but got nil")
	}
}

func TestSearchMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{ "skills": [{ "id": "1", "name": "bad`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
	}

	_, err := client.Search(context.Background(), "query", 5)
	if err == nil {
		t.Fatal("expected error for malformed json, but got nil")
	}
}

func TestSearchEnvBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"skills": []}`))
	}))
	defer server.Close()

	t.Setenv("SKILLS_API_URL", server.URL)
	client := NewClient()

	if client.BaseURL != server.URL {
		t.Errorf("expected BaseURL from env %q, got %q", server.URL, client.BaseURL)
	}

	_, err := client.Search(context.Background(), "query", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchShortQueryNoRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("expected no request to be sent for short query")
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
	}

	skills, err := client.Search(context.Background(), "a", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skills != nil {
		t.Errorf("expected nil skills for short query, got %+v", skills)
	}
}

func TestSearchSanitization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"skills": [
				{"id": "id\u001b[31;1m-sanitized", "skillId": "slug\u001b[31m", "name": "name\nwith\rnewline", "installs": 5, "source": "source\u0000sanitized"}
			]
		}`))
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
	}

	skills, err := client.Search(context.Background(), "query", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	s := skills[0]
	if s.ID != "id-sanitized" {
		t.Errorf("expected sanitized id %q, got %q", "id-sanitized", s.ID)
	}
	if s.Slug != "slug" {
		t.Errorf("expected sanitized display slug %q, got %q", "slug", s.Slug)
	}
	if s.DisplayName != "name withnewline" {
		t.Errorf("expected sanitized display name %q, got %q", "name withnewline", s.DisplayName)
	}
	if s.Source != "sourcesanitized" {
		t.Errorf("expected sanitized source %q, got %q", "sourcesanitized", s.Source)
	}
	if !s.Invalid || s.Reason == "" {
		t.Errorf("expected unsafe command fields to mark result invalid, got %+v", s)
	}
}
