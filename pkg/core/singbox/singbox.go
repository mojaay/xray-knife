package singbox

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/lilendian0x00/xray-knife/v11/pkg/core/protocol"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter/certificate"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/adapter/inbound"
	boxOutbound "github.com/sagernet/sing-box/adapter/outbound"
	boxService "github.com/sagernet/sing-box/adapter/service"
	"github.com/sagernet/sing-box/dns"
	dnsTransport "github.com/sagernet/sing-box/dns/transport"
	"github.com/sagernet/sing-box/dns/transport/hosts"
	"github.com/sagernet/sing-box/dns/transport/local"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/hysteria2"
	"github.com/sagernet/sing-box/protocol/shadowsocks"
	"github.com/sagernet/sing-box/protocol/socks"
	"github.com/sagernet/sing-box/protocol/trojan"
	"github.com/sagernet/sing-box/protocol/vless"
	"github.com/sagernet/sing-box/protocol/vmess"
	"github.com/sagernet/sing-box/protocol/wireguard"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/service"
)

type Core struct {
	Inbound *option.Inbound

	// Log
	Verbose bool
	Log     logger.ContextLogger

	AllowInsecure bool

	// BindInterface, when set, pins all outbound sing-box dials to the
	// named OS interface via RouteOptions.DefaultInterface.
	BindInterface string
}

func (c *Core) Name() string {
	return "singbox"
}

type ServiceOption = func(c *Core)

func WithInbound(inbound protocol.Protocol) ServiceOption {
	return func(c *Core) {
		in := inbound.(Protocol)
		c.Inbound = in.CraftInboundOptions()
	}
}

func WithCustomLogLevel(logOptions option.LogOptions) ServiceOption {
	return func(c *Core) {
		l, _ := log.New(log.Options{
			Options: logOptions,
		})
		c.Log = l.Logger()
	}
}

// WithBindInterface configures the OS interface to bind outbound
// sing-box dials to. Empty string disables binding.
func WithBindInterface(iface string) ServiceOption {
	return func(c *Core) {
		c.BindInterface = iface
	}
}

func NewSingboxService(verbose bool, allowInsecure bool, opts ...ServiceOption) *Core {
	s := &Core{
		Inbound:       nil,
		Verbose:       verbose,
		AllowInsecure: allowInsecure,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.Log == nil {
		l, _ := log.New(log.Options{
			Options: option.LogOptions{Disabled: true},
		})
		s.Log = l.Logger()
	}

	if verbose {
		l, _ := log.New(log.Options{
			Options: option.LogOptions{
				Disabled: false,
				Level:    "trace",
			},
		})
		s.Log = l.Logger()
	}

	return s
}

// applyBind injects the configured BindInterface into the option tree
// so every outbound dial is pinned to that OS interface.
func (c *Core) applyBind(opts *option.Options) {
	if c == nil || c.BindInterface == "" {
		return
	}
	if opts.Route == nil {
		opts.Route = &option.RouteOptions{}
	}
	opts.Route.DefaultInterface = c.BindInterface
}

// boxContext registers every protocol xray-knife can drive with sing-box and
// attaches the registries to ctx. WireGuard has been an endpoint rather than
// an outbound since sing-box 1.14, so it goes into the endpoint registry; the
// outbound manager still resolves endpoint tags, so detour/Final/Outbound(tag)
// keep working for it.
func boxContext(ctx context.Context) context.Context {
	ctx = service.ContextWithDefaultRegistry(ctx)
	outboundRegistry := boxOutbound.NewRegistry()
	hysteria2.RegisterOutbound(outboundRegistry)
	shadowsocks.RegisterOutbound(outboundRegistry)
	socks.RegisterOutbound(outboundRegistry)
	trojan.RegisterOutbound(outboundRegistry)
	vless.RegisterOutbound(outboundRegistry)
	vmess.RegisterOutbound(outboundRegistry)
	endpointRegistry := endpoint.NewRegistry()
	wireguard.RegisterEndpoint(endpointRegistry)
	// sing-box 1.14 builds a "local" DNS server as the default fallback and
	// looks its transport up in this registry, so an empty registry fails
	// box.New with "transport type not found: local". Register the plain
	// transports (no build-tag-gated ones such as QUIC or DHCP).
	dnsRegistry := dns.NewTransportRegistry()
	dnsTransport.RegisterTCP(dnsRegistry)
	dnsTransport.RegisterUDP(dnsRegistry)
	dnsTransport.RegisterTLS(dnsRegistry)
	dnsTransport.RegisterHTTPS(dnsRegistry)
	hosts.RegisterTransport(dnsRegistry)
	local.RegisterTransport(dnsRegistry)
	return box.Context(ctx, inbound.NewRegistry(), outboundRegistry, endpointRegistry,
		dnsRegistry, boxService.NewRegistry(), certificate.NewRegistry())
}

// placeOutbounds files crafted outbounds into the right sing-box list:
// WireGuard options are endpoint options and must live in Endpoints, everything
// else in Outbounds. Tags are preserved so callers can keep addressing them.
func placeOutbounds(opts *option.Options, outbounds ...option.Outbound) {
	for _, out := range outbounds {
		if _, isEndpoint := out.Options.(*option.WireGuardEndpointOptions); isEndpoint {
			opts.Endpoints = append(opts.Endpoints, option.Endpoint{Type: out.Type, Tag: out.Tag, Options: out.Options})
			continue
		}
		opts.Outbounds = append(opts.Outbounds, out)
	}
}

type FakeInstance struct {
}

func (f *FakeInstance) Start() error {
	return nil
}

func (f *FakeInstance) Close() error {
	return nil
}

func (c *Core) SetInbound(inbound protocol.Protocol) error {
	i := inbound.(Protocol)
	c.Inbound = i.CraftInboundOptions()
	return nil
}

func (c *Core) MakeInstance(ctx context.Context, outbound protocol.Protocol) (protocol.Instance, error) {
	out := outbound.(Protocol)

	outOpts, err := out.CraftOutboundOptions(c.AllowInsecure)
	if err != nil {
		return nil, err
	}

	opts := option.Options{
		Inbounds: []option.Inbound{},
		Log: &option.LogOptions{
			Disabled: true,
		},
	}
	placeOutbounds(&opts, *outOpts)

	if c.Verbose {
		opts.Log = &option.LogOptions{
			Disabled: false,
			Level:    "trace",
		}
	}

	if c.Inbound != nil {
		opts.Inbounds = append(opts.Inbounds, *c.Inbound)
	}

	c.applyBind(&opts)

	singboxInstance, err := box.New(box.Options{
		Options: opts,
		Context: boxContext(ctx),
	})

	if err != nil {
		return nil, err
	}

	return singboxInstance, nil
}

func (c *Core) MakeHttpClient(ctx context.Context, outbound protocol.Protocol, maxDelay time.Duration) (*http.Client, protocol.Instance, error) {
	out := outbound.(Protocol)

	outOpts, err := out.CraftOutboundOptions(c.AllowInsecure)
	if err != nil {
		return nil, nil, err
	}
	outboundTag := "http_client_outbound"
	outOpts.Tag = outboundTag

	opts := option.Options{
		Inbounds: []option.Inbound{},
		Log: &option.LogOptions{
			Disabled: true,
		},
	}
	placeOutbounds(&opts, *outOpts)
	if c.Verbose {
		opts.Log = &option.LogOptions{
			Disabled: false,
			Level:    "trace",
		}
	}

	c.applyBind(&opts)

	ctx = boxContext(ctx)

	instance, err := box.New(box.Options{
		Options: opts,
		Context: ctx,
	})
	if err != nil {
		return nil, nil, err
	}

	if err := instance.Start(); err != nil {
		return nil, nil, err
	}

	// Retrieve the outbound adapter from the router
	outboundAdapter, ok := instance.Outbound().Outbound(outboundTag)
	if !ok {
		var available []string
		for _, o := range instance.Outbound().Outbounds() {
			available = append(available, fmt.Sprintf("%s(%s)", o.Tag(), o.Type()))
		}
		instance.Close()
		return nil, nil, fmt.Errorf("outbound adapter not found for tag: %s. Available: %v", outboundTag, available)
	}

	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return outboundAdapter.DialContext(ctx, network, M.ParseSocksaddr(addr))
	}

	if out.Name() == protocol.WireguardIdentifier {
		dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, lookupErr := net.LookupIP(host)
			if lookupErr != nil {
				return nil, lookupErr
			}
			return outboundAdapter.DialContext(ctx, network, M.ParseSocksaddr(ips[0].To4().String()+":"+port))
		}
	}

	tr := &http.Transport{
		DisableKeepAlives: true,
		DialContext:       withEOFNormalization(dial),
	}

	return &http.Client{
		Transport: tr,
		Timeout:   maxDelay,
	}, instance, nil
}

//
//func (c *Core) MakeDial() func(ctx context.Context, v *Instance, dest net.Destination) (net.Conn, error) {
//
//}
