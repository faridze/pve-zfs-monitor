package release

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	MaxManifestBodyBytes  = 1 << 20 // 1 MiB
	MaxSignatureBodyBytes = 256 << 10
	MaxArtifactBodyBytes  = 64 << 20 // 64 MiB
	maxHTTPRedirects      = 3
)

// ValidateHTTPSURL rejects non-HTTPS release endpoints.
func ValidateHTTPSURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("release URL must use https:// with host")
	}
	if u.User != nil {
		return fmt.Errorf("release URL must not embed credentials")
	}
	return nil
}

// SecureHTTPClient fetches release objects over HTTPS with bounded bodies and safe redirects.
type SecureHTTPClient struct {
	Client    *http.Client
	UserAgent string
}

func (c SecureHTTPClient) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxHTTPRedirects {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to non-https URL blocked")
			}
			return nil
		},
	}
}

func (c SecureHTTPClient) Get(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error) {
	if err := ValidateHTTPSURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	if maxBytes <= 0 {
		maxBytes = MaxArtifactBodyBytes
	}
	lim := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(lim)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d byte limit", maxBytes)
	}
	return data, nil
}

// ManifestClient adapts SecureHTTPClient to HTTPClient with manifest size bound.
func ManifestClient(c SecureHTTPClient) HTTPClient {
	return boundedClient{c, MaxManifestBodyBytes}
}

// ArtifactClient adapts SecureHTTPClient to HTTPClient with artifact size bound.
func ArtifactClient(c SecureHTTPClient) HTTPClient {
	return boundedClient{c, MaxArtifactBodyBytes}
}

type boundedClient struct {
	SecureHTTPClient
	max int64
}

func (b boundedClient) Get(ctx context.Context, url string) ([]byte, error) {
	return b.SecureHTTPClient.Get(ctx, url, b.max)
}
