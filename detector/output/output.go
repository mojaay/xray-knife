package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xtls/xray-core/infra/conf"
)

type InboundDetourConfig struct {
	Protocol       string                              `json:"protocol"`
	PortList       *conf.PortList                      `json:"port"`
	ListenOn       *conf.Address                       `json:"listen"`
	Settings       *json.RawMessage                    `json:"settings"`
	Tag            string                              `json:"tag"`
	Allocation     *conf.InboundDetourAllocationConfig `json:"allocate"`
	StreamSetting  *conf.StreamConfig                  `json:"streamSettings"`
	DomainOverride *conf.StringList                    `json:"domainOverride"`
	SniffingConfig *conf.SniffingConfig                `json:"sniffing"`
}

type OutboundDetourConfig struct {
	Tag           string            `json:"tag"`
	Protocol      string            `json:"protocol"`
	SendThrough   *conf.Address     `json:"sendThrough,omitempty"`
	Settings      *json.RawMessage  `json:"settings,omitempty"`
	StreamSetting *StreamConfig     `json:"streamSettings,omitempty"`
	ProxySettings *conf.ProxyConfig `json:"proxySettings,omitempty"`
	MuxSettings   *conf.MuxConfig   `json:"mux,omitempty"`
}

type StreamConfig struct {
	Network         *conf.TransportProtocol  `json:"network,omitempty"`
	Security        string                   `json:"security"`
	TLSSettings     *conf.TLSConfig          `json:"tlsSettings,omitempty"`
	REALITYSettings *conf.REALITYConfig      `json:"realitySettings,omitempty"`
	TCPSettings     *conf.TCPConfig          `json:"tcpSettings,omitempty"`
	KCPSettings     *conf.KCPConfig          `json:"kcpSettings,omitempty"`
	WSSettings      *conf.WebSocketConfig    `json:"wsSettings,omitempty"`
	HTTPSettings    *conf.HTTPConfig         `json:"httpSettings,omitempty"`
	DSSettings      *conf.DomainSocketConfig `json:"dsSettings,omitempty"`
	QUICSettings    *conf.QUICConfig         `json:"quicSettings,omitempty"`
	SocketSettings  *conf.SocketConfig       `json:"sockopt,omitempty"`
	GRPCConfig      *conf.GRPCConfig         `json:"grpcSettings,omitempty"`
	GUNConfig       *conf.GRPCConfig         `json:"gunSettings,omitempty"`
}

type Config struct {
	LogConfig       *conf.LogConfig         `json:"log,omitempty"`
	RouterConfig    *conf.RouterConfig      `json:"routing,omitempty"`
	DNSConfig       *conf.DNSConfig         `json:"dns,omitempty"`
	Transport       *conf.TransportConfig   `json:"transport,omitempty"`
	Policy          *conf.PolicyConfig      `json:"policy,omitempty"`
	API             *conf.APIConfig         `json:"api,omitempty"`
	Metrics         *conf.MetricsConfig     `json:"metrics,omitempty"`
	Stats           *conf.StatsConfig       `json:"stats,omitempty"`
	Reverse         *conf.ReverseConfig     `json:"reverse,omitempty"`
	FakeDNS         *conf.FakeDNSConfig     `json:"fakeDns,omitempty"`
	Observatory     *conf.ObservatoryConfig `json:"observatory,omitempty"`
	InboundConfigs  []InboundDetourConfig   `json:"inbounds,omitempty"`
	OutboundConfigs []OutboundDetourConfig  `json:"outbounds,omitempty"`
}

func ConvertOutputConfig(config *conf.Config, section ...string) (*Config, error) {
	outputConfig := &Config{}

	if len(section) == 0 {
		section = []string{"inbound", "outbound"}
	}

	for _, v := range section {

		switch strings.ToLower(v) {
		case "log":
			outputConfig.LogConfig = config.LogConfig
		case "routing":
			outputConfig.RouterConfig = config.RouterConfig
		case "dns":
			outputConfig.DNSConfig = config.DNSConfig
		case "transport":
			outputConfig.Transport = config.Transport
		case "policy":
			outputConfig.Policy = config.Policy
		case "api":
			outputConfig.API = config.API
		case "metrics":
			outputConfig.Metrics = config.Metrics
		case "stats":
			outputConfig.Stats = config.Stats
		case "inbound":
			for _, idc := range config.InboundConfigs {
				inboundConfig := &InboundDetourConfig{
					Tag:            idc.Tag,
					Protocol:       idc.Protocol,
					PortList:       idc.PortList,
					ListenOn:       idc.ListenOn,
					Settings:       idc.Settings,
					Allocation:     idc.Allocation,
					StreamSetting:  idc.StreamSetting,
					DomainOverride: idc.DomainOverride,
					SniffingConfig: idc.SniffingConfig,
				}
				outputConfig.InboundConfigs = append(outputConfig.InboundConfigs, *inboundConfig)
			}
		case "outbound":

			for _, odc := range config.OutboundConfigs {
				outboundConfig := &OutboundDetourConfig{
					Tag:      "", // odc.Tag,
					Protocol: odc.Protocol,
					Settings: odc.Settings,
					StreamSetting: &StreamConfig{
						Network:         odc.StreamSetting.Network,
						Security:        odc.StreamSetting.Security,
						TLSSettings:     odc.StreamSetting.TLSSettings,
						REALITYSettings: odc.StreamSetting.REALITYSettings,
						TCPSettings:     odc.StreamSetting.TCPSettings,
						KCPSettings:     odc.StreamSetting.KCPSettings,
						WSSettings:      odc.StreamSetting.WSSettings,
						HTTPSettings:    odc.StreamSetting.HTTPSettings,
						DSSettings:      odc.StreamSetting.DSSettings,
						QUICSettings:    odc.StreamSetting.QUICSettings,
						SocketSettings:  odc.StreamSetting.SocketSettings,
						GRPCConfig:      odc.StreamSetting.GRPCConfig,
						GUNConfig:       odc.StreamSetting.GUNConfig,
					},
					SendThrough:   odc.SendThrough,
					ProxySettings: odc.ProxySettings,
					MuxSettings:   odc.MuxSettings,
				}
				outputConfig.OutboundConfigs = append(outputConfig.OutboundConfigs, *outboundConfig)
			}
		default:
			return nil, fmt.Errorf("unsupported config type: %s", v)
		}
	}

	return outputConfig, nil
}
