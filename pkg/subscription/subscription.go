// Package subscription fetches and decodes share-link lists without CLI or database state.
// Callers control network access and validate the resulting configs.
package subscription

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultMaxBytes int64 = 64 << 20
	DefaultMaxLinks       = 100000
	DefaultTimeout        = 30 * time.Second
)

var (
	ErrTooLarge      = errors.New("subscription exceeds size limit")
	ErrTooManyLinks  = errors.New("subscription exceeds link limit")
	ErrInvalidFormat = errors.New("subscription must contain a plain or base64-encoded share-link list")
)

type DecodeOptions struct {
	// Zero selects the default; negative limits are invalid.
	MaxBytes int64
	MaxLinks int
}

// Decode reads plain or base64 lists, preserving duplicates and malformed config options.
// Invalid documents and limit breaches return an error, never a partial list.
func Decode(body []byte, opts DecodeOptions) ([]string, error) {
	maxBytes, maxLinks, err := limits(opts)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, ErrTooLarge
	}
	body = trimBody(body)
	if len(body) == 0 {
		return []string{}, nil
	}
	if !bytes.Contains(body, []byte("://")) {
		// Allow base64 wrapped across lines by subscription providers.
		encoded := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, string(body))
		decoded := false
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			if b, err := enc.DecodeString(encoded); err == nil {
				body = trimBody(b)
				decoded = true
				break
			}
		}
		if !decoded {
			return nil, ErrInvalidFormat
		}
	}
	if !utf8.Valid(body) {
		return nil, ErrInvalidFormat
	}
	links := make([]string, 0)
	for line := range strings.SplitSeq(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		scheme, value, ok := strings.Cut(line, "://")
		if !ok || value == "" || !validScheme(scheme) {
			return nil, ErrInvalidFormat
		}
		if len(links) == maxLinks {
			return nil, ErrTooManyLinks
		}
		links = append(links, line)
	}
	return links, nil
}

func trimBody(body []byte) []byte {
	return bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(body), []byte("\xef\xbb\xbf")))
}

func validScheme(s string) bool {
	if s == "" || !asciiLetter(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !asciiLetter(c) && !(c >= '0' && c <= '9') && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

func asciiLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func limits(opts DecodeOptions) (int64, int, error) {
	if opts.MaxBytes < 0 || opts.MaxLinks < 0 {
		return 0, 0, errors.New("subscription limits must not be negative")
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	if opts.MaxLinks == 0 {
		opts.MaxLinks = DefaultMaxLinks
	}
	return opts.MaxBytes, opts.MaxLinks, nil
}

type FetchOptions struct {
	DecodeOptions
	Method  string // Default GET.
	Headers http.Header
	Timeout time.Duration // Overall request/body deadline; zero defaults to 30s.
}

type FetchResult struct {
	Links        []string
	StatusCode   int
	NotModified  bool // HTTP 304: preserve the previous source snapshot.
	ETag         string
	LastModified string
}

// Fetch downloads a bounded list; a nil client uses http.DefaultClient.
// The caller's client controls redirects and network access. HTTP 304 preserves the snapshot.
func Fetch(ctx context.Context, client *http.Client, rawURL string, opts FetchOptions) (*FetchResult, error) {
	maxBytes, _, err := limits(opts.DecodeOptions)
	if err != nil {
		return nil, err
	}
	// The extra byte used to detect overflow must itself fit in int64.
	if maxBytes == math.MaxInt64 || opts.Timeout < 0 {
		return nil, errors.New("invalid subscription fetch limits")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("subscription URL must be HTTP or HTTPS with a host")
	}
	if opts.Timeout == 0 {
		opts.Timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	if opts.Method == "" {
		opts.Method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, opts.Method, u.String(), nil)
	if err != nil {
		return nil, errors.New("invalid subscription request")
	}
	if opts.Headers != nil {
		req.Header = opts.Headers.Clone()
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &fetchError{err: err}
	}
	defer resp.Body.Close()
	result := &FetchResult{
		StatusCode:   resp.StatusCode,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	if resp.StatusCode == http.StatusNotModified {
		result.NotModified = true
		return result, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subscription server returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, ErrTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, &fetchError{err: err}
	}
	result.Links, err = Decode(body, opts.DecodeOptions)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Preserve errors.Is/As (e.g. cancellation) without printing a URL/token from
// net/http's *url.Error or a custom transport's error in ordinary logs.
type fetchError struct{ err error }

func (e *fetchError) Error() string { return "subscription request failed" }
func (e *fetchError) Unwrap() error { return e.err }
