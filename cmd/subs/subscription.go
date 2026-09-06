package subs

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/imroc/req/v3"
	"github.com/lilendian0x00/xray-knife/v11/pkg/subscription"
)

type Subscription struct {
	Remark      string
	Url         string
	UserAgent   string
	Method      string
	ConfigLinks []string
	Proxy       string
	MaxBytes    int64
	MaxLinks    int
	Timeout     time.Duration
}

// FetchAll retains the CLI API. Library callers should use subscription.Fetch
// to inject their own context, HTTP client, limits, and request policy.
func (s *Subscription) FetchAll() ([]string, error) {
	return s.FetchAllContext(context.Background())
}

func (s *Subscription) FetchAllContext(ctx context.Context) ([]string, error) {
	if s.Method == "" {
		s.Method = http.MethodGet
	}
	client := req.C().ImpersonateChrome().DisableAutoReadResponse()
	defer client.GetClient().CloseIdleConnections()
	if s.Proxy != "" {
		client.SetProxyURL(s.Proxy)
	}
	headers := client.Headers.Clone()
	if s.UserAgent != "" {
		headers.Set("User-Agent", s.UserAgent)
	}
	result, err := subscription.Fetch(ctx, client.GetClient(), s.Url, subscription.FetchOptions{
		DecodeOptions: subscription.DecodeOptions{MaxBytes: s.MaxBytes, MaxLinks: s.MaxLinks},
		Method:        s.Method,
		Headers:       headers,
		Timeout:       s.Timeout,
	})
	if err != nil {
		return nil, err
	}
	if !result.NotModified {
		s.ConfigLinks = result.Links
	}
	return s.ConfigLinks, nil
}

func (s *Subscription) RemoveDuplicate(verbose bool) {
	// Remove duplicates using hashmap (hashed keys)
	allKeys := make(map[string]bool)
	var list []string
	for _, item := range s.ConfigLinks {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	if verbose {
		log.Printf("Removed %d duplicate configs!\n", len(s.ConfigLinks)-len(list))
	}
	s.ConfigLinks = list
}
