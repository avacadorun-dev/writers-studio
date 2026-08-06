package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"golang.org/x/net/html"

	"writers.studio/internal/llm"
)

const (
	maxLinkPreviewURLs = 20
	maxLinkPreviewBody = 1 << 20
)

type linkPreviewRequest struct {
	URLs []string `json:"urls"`
}

type linkPreview struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	SiteName    string `json:"siteName"`
	FetchedAt   string `json:"fetchedAt"`
}

func (s *Server) handleLinkPreviews(response http.ResponseWriter, request *http.Request) {
	var input linkPreviewRequest
	if !decodeJSON(response, request, &input) {
		return
	}
	if len(input.URLs) == 0 || len(input.URLs) > maxLinkPreviewURLs {
		writeError(response, http.StatusUnprocessableEntity, "invalid_link_preview_request", "Передайте от 1 до 20 ссылок")
		return
	}
	if _, err := s.store.GetProjectOutline(
		request.Context(), currentUser(request.Context()).ID, chi.URLParam(request, "projectID"),
	); err != nil {
		handleStoreError(response, err)
		return
	}

	policy := llm.EndpointPolicy{AllowHTTP: true}
	client := linkPreviewHTTPClient(policy)
	previews := make([]linkPreview, 0, len(input.URLs))
	for _, rawURL := range input.URLs {
		preview, err := fetchLinkPreview(request.Context(), client, policy, rawURL)
		if err != nil {
			writeError(response, http.StatusUnprocessableEntity, "link_preview_failed", "Не удалось безопасно получить preview ссылки")
			return
		}
		previews = append(previews, preview)
	}
	writeJSON(response, http.StatusOK, map[string]any{"previews": previews})
}

func linkPreviewHTTPClient(policy llm.EndpointPolicy) *http.Client {
	client := policy.HTTPClient(6 * time.Second)
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errors.New("too many redirects")
		}
		_, err := policy.Validate(request.Context(), request.URL.String())
		return err
	}
	return client
}

func fetchLinkPreview(
	ctx context.Context,
	client *http.Client,
	policy llm.EndpointPolicy,
	rawURL string,
) (linkPreview, error) {
	parsed, err := policy.Validate(ctx, rawURL)
	if err != nil {
		return linkPreview{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return linkPreview{}, err
	}
	request.Header.Set("User-Agent", "WritersStudio-LinkPreview/1.0")
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	result, err := client.Do(request)
	if err != nil {
		return linkPreview{}, err
	}
	defer result.Body.Close()
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return linkPreview{}, fmt.Errorf("unexpected status %d", result.StatusCode)
	}
	mediaType := strings.ToLower(strings.Split(result.Header.Get("Content-Type"), ";")[0])
	if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
		return linkPreview{}, errors.New("link is not HTML")
	}
	body, err := io.ReadAll(io.LimitReader(result.Body, maxLinkPreviewBody+1))
	if err != nil || len(body) > maxLinkPreviewBody {
		return linkPreview{}, errors.New("link preview body is too large")
	}
	title, description, siteName := parseLinkMetadata(body)
	finalURL := result.Request.URL
	if finalURL == nil {
		finalURL = parsed
	}
	if siteName == "" {
		siteName = finalURL.Hostname()
	}
	return linkPreview{
		URL:         finalURL.String(),
		Title:       truncateRunes(title, 200),
		Description: truncateRunes(description, 500),
		SiteName:    truncateRunes(siteName, 120),
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func parseLinkMetadata(body []byte) (title, description, siteName string) {
	document, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return "", "", ""
	}
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "title" && title == "" && node.FirstChild != nil {
			title = normalizedMetadataText(node.FirstChild.Data)
		}
		if node.Type == html.ElementNode && node.Data == "meta" {
			attributes := map[string]string{}
			for _, attribute := range node.Attr {
				attributes[strings.ToLower(attribute.Key)] = attribute.Val
			}
			key := strings.ToLower(attributes["property"])
			if key == "" {
				key = strings.ToLower(attributes["name"])
			}
			value := normalizedMetadataText(attributes["content"])
			switch key {
			case "og:title", "twitter:title":
				if value != "" {
					title = value
				}
			case "og:description", "twitter:description", "description":
				if description == "" && value != "" {
					description = value
				}
			case "og:site_name":
				if value != "" {
					siteName = value
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return title, description, siteName
}

func normalizedMetadataText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func normalizedLinkURL(raw string) (*url.URL, error) {
	return llm.EndpointPolicy{AllowHTTP: true}.Validate(context.Background(), raw)
}
