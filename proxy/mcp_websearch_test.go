package proxy

import (
	"context"
	"io"
	"kiro-proxy/config"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebSearchContinuesWithoutProfileARNWhenLookupFails(t *testing.T) {
	if err := config.Init(filepath.Join(t.TempDir(), "config.json")); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	var mcpCalls int
	var sentARN string
	kiroRestHttpStore.Store(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.HasSuffix(req.URL.Path, "/ListAvailableProfiles") {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(strings.NewReader(`{"message":"Improperly formed request.","reason":"REQUEST_BODY_INVALID"}`)),
					Header:     make(http.Header),
				}, nil
			}
			mcpCalls++
			sentARN = req.Header.Get("x-amzn-kiro-profile-arn")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":"1","result":{"content":[{"type":"text","text":"{\"results\":[{\"title\":\"T\",\"link\":\"https://example.com\",\"description\":\"S\"}]}"}]}}`)),
				Header: make(http.Header),
			}, nil
		}),
	})
	t.Cleanup(func() { InitKiroHttpClient("") })

	results, err := performKiroWebSearch(context.Background(), &config.Account{
		ID:          "websearch-builderid-no-profile",
		Provider:    "BuilderId",
		AuthMethod:  "idc",
		Region:      "us-east-1",
		AccessToken: "access-token",
	}, "kiro")
	if err != nil {
		t.Fatalf("web search failed instead of degrading: %v", err)
	}
	if len(results) != 1 || mcpCalls != 1 {
		t.Fatalf("results=%v mcpCalls=%d", results, mcpCalls)
	}
	if sentARN != "" {
		t.Fatalf("profile ARN header sent without a resolved profile: %q", sentARN)
	}
}

func TestParseMCPWebSearchResponseHandlesWrappedJSONAndNumericDates(t *testing.T) {
	data := []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"result": {
			"content": [{
				"type": "text",
				"text": "{\"results\":[{\"title\":\"Result\",\"link\":\"https://example.com\",\"description\":\"Snippet\",\"publishedDate\":1710000000}]}"
			}]
		}
	}`)

	results, err := parseMCPWebSearchResponse(data)
	if err != nil {
		t.Fatalf("parseMCPWebSearchResponse: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %#v", results)
	}
	if results[0].Title != "Result" || results[0].URL != "https://example.com" || results[0].Snippet != "Snippet" || results[0].PublishedDate != "1710000000" {
		t.Fatalf("unexpected parsed result: %#v", results[0])
	}
}

func TestOpenAIHostedWebSearchToolConvertsForKiro(t *testing.T) {
	tools, nameMap := convertOpenAITools([]OpenAITool{{Type: "web_search_preview_2025_03_11"}})
	if len(tools) != 1 {
		t.Fatalf("expected hosted web search tool conversion, got %#v", tools)
	}
	if got := tools[0].ToolSpecification.Name; got != kiroWebSearchToolName {
		t.Fatalf("expected %q, got %q", kiroWebSearchToolName, got)
	}
	if nameMap[kiroWebSearchToolName] != webSearchToolName {
		t.Fatalf("expected hosted web search name map, got %#v", nameMap)
	}
}

func TestFormatWebSearchResultsCapsAndIncludesSourceFields(t *testing.T) {
	results := []WebSearchResult{
		{Title: "One", URL: "https://one.example", Snippet: "A", PublishedDate: "2026-06-02"},
		{Title: "Two", URL: "https://two.example", Snippet: "B"},
		{Title: "Three"},
		{Title: "Four"},
		{Title: "Five"},
		{Title: "Six"},
	}
	formatted := formatWebSearchResults(capWebSearchResults(results))
	if strings.Contains(formatted, "Six") {
		t.Fatalf("expected results to be capped, got %q", formatted)
	}
	for _, want := range []string{"One", "URL: https://one.example", "Published: 2026-06-02", "A"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("expected formatted result to contain %q, got %q", want, formatted)
		}
	}
}
