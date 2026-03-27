package palpity

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	baseURL         = "https://app.palpity.io"
	rodoviaSlugBase = "rodovia-5-minutos-qu-"
)

type marketFetcher struct {
	httpClient *http.Client
}

func newMarketFetcher() *marketFetcher {
	return &marketFetcher{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:   &tls.Config{},
				ForceAttemptHTTP2: false,
				TLSNextProto:      make(map[string]func(string, *tls.Conn) http.RoundTripper),
			},
		},
	}
}

func (f *marketFetcher) fetchMarket(id int, slug string) (*Market, error) {
	urlPath := fmt.Sprintf("%s/live/%d-market/%s", baseURL, id, slug)

	req, err := http.NewRequest(http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "text/x-component")
	req.Header["RSC"] = []string{"1"}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch market: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(data[:min(len(data), 200)]))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return parseRSCPayload(data)
}

func (f *marketFetcher) fetchNextMarket(currentID int) (*Market, error) {
	nextID := currentID + 3
	nextSlug := fmt.Sprintf("%s%d", rodoviaSlugBase, nextID)
	return f.fetchMarket(nextID, nextSlug)
}

func (f *marketFetcher) discoverActiveMarket() (*Market, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create homepage request: %w", err)
	}

	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch homepage: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read homepage: %w", err)
	}

	html := string(data)
	idx := strings.Index(html, rodoviaSlugBase)
	if idx == -1 {
		return nil, fmt.Errorf("rodovia market not found on homepage")
	}

	fragment := html[idx:]
	endIdx := strings.IndexAny(fragment, `"'& <>/,`)
	if endIdx == -1 {
		return nil, fmt.Errorf("could not parse slug from homepage")
	}

	slug := fragment[:endIdx]
	idStr := strings.TrimPrefix(slug, rodoviaSlugBase)
	var id int
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		return nil, fmt.Errorf("parse market id from slug %q: %w", slug, err)
	}

	return f.fetchMarket(id, slug)
}

func parseRSCPayload(data []byte) (*Market, error) {
	body := string(data)

	const marker = `"initialData":{`
	idx := strings.Index(body, marker)
	if idx == -1 {
		return nil, fmt.Errorf("initialData not found in RSC payload")
	}

	jsonStart := idx + len(`"initialData":`)
	depth := 0
	jsonEnd := -1
	for i := jsonStart; i < len(body); i++ {
		switch body[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				jsonEnd = i + 1
				break
			}
		}
		if jsonEnd >= 0 {
			break
		}
	}

	if jsonEnd < 0 {
		return nil, fmt.Errorf("could not parse initialData boundaries")
	}

	raw := body[jsonStart:jsonEnd]

	var apiData marketAPIData
	if err := json.Unmarshal([]byte(raw), &apiData); err != nil {
		return nil, fmt.Errorf("parse initialData: %w", err)
	}

	closesAt, _ := time.Parse(time.RFC3339, apiData.ClosesAt)

	market := &Market{
		ID:                      apiData.ID,
		Slug:                    apiData.Slug,
		Title:                   apiData.Title,
		ClosesAt:                closesAt,
		ClosesAtRaw:             apiData.ClosesAt,
		RemainingSeconds:        apiData.RemainingSeconds,
		RemainingBettingSeconds: apiData.RemainingBettingSeconds,
		Live:                    apiData.Live,
		LiveType:                apiData.LiveType,
		Target:                  apiData.Target,
		MatchingSystem:          apiData.MatchingSystem,
		WinnerID:                apiData.WinnerID,
		Metadata:                apiData.Metadata,
		Selections:              apiData.Selections,
		GraphData:               apiData.GraphData,
	}

	return market, nil
}
