package httpapi

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"writers.studio/internal/llm"
)

type linkPreviewResolver map[string][]net.IP

func (r linkPreviewResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	result := make([]net.IPAddr, 0, len(r[host]))
	for _, address := range r[host] {
		result = append(result, net.IPAddr{IP: address})
	}
	return result, nil
}

type linkPreviewRoundTripper func(*http.Request) (*http.Response, error)

func (transport linkPreviewRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestParseLinkMetadata(t *testing.T) {
	title, description, siteName := parseLinkMetadata([]byte(`<!doctype html><html><head>
		<title> Fallback title </title>
		<meta property="og:title" content=" Preview   title ">
		<meta name="description" content=" A useful description ">
		<meta property="og:site_name" content="Example Research">
	</head></html>`))
	if title != "Preview title" || description != "A useful description" || siteName != "Example Research" {
		t.Fatalf("metadata = %q, %q, %q", title, description, siteName)
	}
	if title, description, siteName := parseLinkMetadata([]byte{}); title != "" || description != "" || siteName != "" {
		t.Fatalf("empty metadata = %q, %q, %q", title, description, siteName)
	}
	if got := truncateRunes("абвг", 3); got != "абв" {
		t.Fatalf("truncateRunes = %q", got)
	}
	if got := truncateRunes("short", 10); got != "short" {
		t.Fatalf("short truncateRunes = %q", got)
	}
}

func TestFetchLinkPreview(t *testing.T) {
	policy := llm.EndpointPolicy{
		AllowHTTP: true,
		Resolver:  linkPreviewResolver{"public.test": {net.ParseIP("203.0.113.10")}},
	}
	client := &http.Client{Transport: linkPreviewRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/html; charset=utf-8"}},
			Body: io.NopCloser(strings.NewReader(
				`<html><head><meta property="og:title" content="Title"><meta name="twitter:description" content="Description"></head></html>`,
			)),
			Request: request,
		}, nil
	})}
	preview, err := fetchLinkPreview(context.Background(), client, policy, "http://public.test/article")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Title != "Title" || preview.Description != "Description" || preview.SiteName != "public.test" || preview.FetchedAt == "" {
		t.Fatalf("preview = %#v", preview)
	}
	if _, err := fetchLinkPreview(context.Background(), client, policy, "http://127.0.0.1/private"); err == nil {
		t.Fatal("private preview URL was accepted")
	}
	if _, err := normalizedLinkURL("file:///private"); err == nil {
		t.Fatal("file URL was accepted")
	}
}

func TestFetchLinkPreviewRejectsUnsafeResponses(t *testing.T) {
	policy := llm.EndpointPolicy{
		AllowHTTP: true,
		Resolver:  linkPreviewResolver{"public.test": {net.ParseIP("203.0.113.10")}},
	}
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
	}{
		{name: "status", status: http.StatusNotFound, contentType: "text/html", body: "not found"},
		{name: "content type", status: http.StatusOK, contentType: "application/json", body: `{}`},
		{name: "body size", status: http.StatusOK, contentType: "text/html", body: strings.Repeat("x", maxLinkPreviewBody+1)},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			client := &http.Client{Transport: linkPreviewRoundTripper(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: item.status,
					Header:     http.Header{"Content-Type": {item.contentType}},
					Body:       io.NopCloser(strings.NewReader(item.body)),
					Request:    request,
				}, nil
			})}
			if _, err := fetchLinkPreview(context.Background(), client, policy, "http://public.test"); err == nil {
				t.Fatal("unsafe response was accepted")
			}
		})
	}
}

func TestLinkPreviewRedirectPolicy(t *testing.T) {
	policy := llm.EndpointPolicy{
		AllowHTTP: true,
		Resolver: linkPreviewResolver{
			"public.test":  {net.ParseIP("203.0.113.10")},
			"private.test": {net.ParseIP("10.0.0.1")},
		},
	}
	client := linkPreviewHTTPClient(policy)
	public, _ := http.NewRequest(http.MethodGet, "http://public.test/next", nil)
	if err := client.CheckRedirect(public, []*http.Request{{}, {}, {}}); err != nil {
		t.Fatalf("public redirect rejected: %v", err)
	}
	if err := client.CheckRedirect(public, []*http.Request{{}, {}, {}, {}}); err == nil {
		t.Fatal("redirect limit was not enforced")
	}
	private, _ := http.NewRequest(http.MethodGet, "http://private.test/secret", nil)
	if err := client.CheckRedirect(private, nil); err == nil {
		t.Fatal("private redirect was accepted")
	}
}
