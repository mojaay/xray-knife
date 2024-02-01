package detector

import (
	"context"
	"fmt"
	"time"
	"xray-knife/detector/output"
	"xray-knife/detector/ping"
	"xray-knife/detector/service"
	"xray-knife/speedtester/cloudflare"
	"xray-knife/xray"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
)

type Detector interface {
}

type Detective struct {
	config *conf.Config
	*output.Config
	*ping.PingHttp

	DownloadSpeed   float32 `csv:"download"` // mbps
	UploadSpeed     float32 `csv:"upload"`   // mbps
	SpeedtestAmount uint32
}

func NewInstance(ctx context.Context, protocol xray.Protocol) (*Detective, error) {

	outboundDetourConfig, err := protocol.BuildOutboundDetourConfig(true)
	if err != nil {
		return nil, err
	}

	jsonConfig := &conf.Config{
		LogConfig: &conf.LogConfig{
			LogLevel:  "none",
			AccessLog: "none",
			ErrorLog:  "none",
			DNSLog:    false,
		},
		OutboundConfigs: []conf.OutboundDetourConfig{*outboundDetourConfig},
	}

	clientConfig, err := jsonConfig.Build()
	// built, err := outboundDetourConfig.Build()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	server, err := core.NewWithContext(ctx, clientConfig)
	if err != nil {
		return nil, err
	}
	return &Detective{
		config: jsonConfig,
		PingHttp: &ping.PingHttp{
			Result: ping.PingResult{},
			Service: service.Service{
				Server: server,
			},
		},
		SpeedtestAmount: 10000,
	}, nil
}

func (d *Detective) Close() error {
	return d.Server.Close()
}

func (d *Detective) Speed() {
	d.measureSpeed(true, &d.DownloadSpeed)
	d.measureSpeed(false, &d.UploadSpeed)
}

func (d *Detective) measureSpeed(isDownload bool, speed *float32) {
	startTime := time.Now()
	_, _, err := xray.CoreHTTPRequestCustom(d.Server, time.Duration(20000)*time.Millisecond,
		cloudflare.Speedtest.MakeDownloadHTTPRequest(isDownload, d.SpeedtestAmount*1000))
	if err == nil {
		timeTaken := time.Since(startTime).Milliseconds()
		*speed = (float32((d.SpeedtestAmount*1000)*8) / (float32(timeTaken) / float32(1000.0))) / float32(1000000.0)
	}
}
