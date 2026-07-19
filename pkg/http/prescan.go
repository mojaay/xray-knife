package http

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alitto/pond/v2"

	"github.com/lilendian0x00/xray-knife/v10/pkg/core"
	"github.com/lilendian0x00/xray-knife/v10/pkg/core/protocol"
	"github.com/lilendian0x00/xray-knife/v10/pkg/netbind"
)

const (
	// defaultPrescanWorkers is the fallback concurrency for TCP dials. A plain
	// SYN handshake is cheap (no core instance), so this can be far higher than
	// the full-test thread count.
	defaultPrescanWorkers = 512
	// defaultPrescanTimeout bounds a single TCP dial.
	defaultPrescanTimeout = 2 * time.Second
)

// PrescanOptions configures the TCP reachability pre-check.
type PrescanOptions struct {
	// Workers is the number of concurrent TCP dials. Values <= 0 use the
	// default, and it is capped at the number of unique endpoints.
	Workers int
	// Timeout is the per-endpoint TCP dial timeout. Values <= 0 use the default.
	Timeout time.Duration
	// BindInterface pins dials to a specific OS interface, mirroring the
	// examiner so reachability matches the real test path. Empty disables it.
	BindInterface string
}

// PrescanResult summarizes a single pre-scan pass.
type PrescanResult struct {
	// Reachable holds the links that survived the pre-check (TCP-reachable
	// plus bypassed), in the original input order.
	Reachable []string
	// TCPReachable is the number of links kept because their endpoint answered
	// a TCP dial.
	TCPReachable int
	// Bypassed is the number of links kept without dialing (UDP-based
	// protocols, or links that could not be parsed / had no dialable endpoint).
	Bypassed int
	// FilteredOut is the number of links dropped as unreachable.
	FilteredOut int
	// UniqueEndpoints is the number of distinct host:port pairs dialed.
	UniqueEndpoints int
}

// RunPrescan performs a fast TCP reachability pre-check over links and returns
// the subset worth handing to the full HTTP test.
//
// Links are grouped by their destination host:port so each unique endpoint is
// dialed only once; the verdict is then fanned back out to every config that
// shares it. UDP-based protocols (Hysteria2, WireGuard) and UDP transports
// (mKCP, QUIC) cannot be TCP-probed, so they bypass the check and are kept.
// Links that fail to parse or lack a dialable endpoint are also kept, so the
// full test can report them as broken exactly as it would without a pre-scan.
//
// onStart, if non-nil, is called once with the number of unique endpoints
// before dialing begins. onProgress, if non-nil, is called once per endpoint
// dialed. Both may be nil.
func RunPrescan(ctx context.Context, c core.Core, links []string, opts PrescanOptions, onStart func(uniqueEndpoints int), onProgress func()) (*PrescanResult, error) {
	// Interface binding is resolved once so a bad --bind value fails fast.
	binder, err := netbind.New(opts.BindInterface)
	if err != nil {
		return nil, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultPrescanTimeout
	}

	// Decide each link's fate up front: "" means bypass (keep without dialing),
	// otherwise the value is the "host:port" to dial. Grouping happens via uniq.
	endpoints := make([]string, len(links))
	uniq := make(map[string]struct{})
	res := &PrescanResult{}
	for i, link := range links {
		ep := endpointForLink(c, link)
		endpoints[i] = ep
		if ep == "" {
			continue
		}
		uniq[ep] = struct{}{}
	}
	res.UniqueEndpoints = len(uniq)

	if onStart != nil {
		onStart(len(uniq))
	}

	// Dial each unique endpoint once, concurrently.
	reachable := make(map[string]bool, len(uniq))
	var mu sync.Mutex

	workers := opts.Workers
	if workers <= 0 {
		workers = defaultPrescanWorkers
	}
	if len(uniq) > 0 && workers > len(uniq) {
		workers = len(uniq)
	}
	if workers < 1 {
		workers = 1
	}

	pool := pond.NewPool(workers)
	defer pool.Stop()
	group := pool.NewGroupContext(ctx)

	for ep := range uniq {
		endpoint := ep
		group.Submit(func() {
			ok := dialTCP(group.Context(), binder, endpoint, timeout)
			mu.Lock()
			reachable[endpoint] = ok
			mu.Unlock()
			if onProgress != nil {
				onProgress()
			}
		})
	}
	group.Wait()

	// Assemble survivors, preserving input order.
	res.Reachable = make([]string, 0, len(links))
	for i, link := range links {
		ep := endpoints[i]
		if ep == "" {
			res.Bypassed++
			res.Reachable = append(res.Reachable, link)
			continue
		}
		if reachable[ep] {
			res.TCPReachable++
			res.Reachable = append(res.Reachable, link)
		} else {
			res.FilteredOut++
		}
	}

	return res, nil
}

// endpointForLink returns the "host:port" to TCP-dial for a config link, or ""
// when the link should bypass the TCP check (UDP-based, unparseable, or no
// dialable endpoint).
func endpointForLink(c core.Core, link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	proto, err := c.CreateProtocol(link)
	if err != nil {
		return ""
	}
	if err := proto.Parse(); err != nil {
		return ""
	}
	gc := proto.ConvertToGeneralConfig()
	if isUDPBased(gc) {
		return ""
	}
	// Address may already be bracketed for IPv6 (e.g. "[::1]"); strip so
	// net.JoinHostPort does not double-wrap.
	host := strings.TrimSpace(gc.Address)
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if host == "" || gc.Port == "" {
		return ""
	}
	if _, err := strconv.Atoi(strings.TrimSpace(gc.Port)); err != nil {
		// Non-numeric port: let the full test report it as broken.
		return ""
	}
	return net.JoinHostPort(host, strings.TrimSpace(gc.Port))
}

// isUDPBased reports whether a config runs over UDP (or is otherwise not
// TCP-dialable), so a TCP pre-check would produce a false negative.
func isUDPBased(gc protocol.GeneralConfig) bool {
	switch strings.ToLower(gc.Protocol) {
	case protocol.Hysteria2Identifier, "hy2", protocol.WireguardIdentifier, protocol.TunIdentifier:
		return true
	}
	// The transport network lives in different GeneralConfig fields depending
	// on the protocol (vmess -> Network, vless/trojan -> Type), so check both.
	for _, n := range []string{gc.Network, gc.Type} {
		switch strings.ToLower(strings.TrimSpace(n)) {
		case "kcp", "mkcp", "quic":
			return true
		}
	}
	return false
}

// dialTCP performs a single TCP handshake to endpoint and reports success.
// It honors the optional interface binder (no-op when nil/disabled).
func dialTCP(ctx context.Context, binder *netbind.Binder, endpoint string, timeout time.Duration) bool {
	d := &net.Dialer{Timeout: timeout}
	binder.ApplyDialer(d)
	conn, err := d.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
