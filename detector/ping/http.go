package ping

import (
	"context"
	"errors"
	"fmt"
	corenet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/core"
	"net"
	"net/http"
	"time"
	"xray-knife/detector/service"
)

const (
	GoogleStatic = "http://www.gstatic.com/generate_204"
	Google       = "http://www.google.com/generate_204"
)

type ResultTime struct {
	elapsedTime time.Duration
	maxTime     time.Duration
	minTime     time.Duration
}

type PingResult struct {
	count      uint
	successful uint
	failed     uint
	ResultTime
}

func (r *PingResult) CountIncrease() {
	r.count += 1
}
func (r *PingResult) CountReduce() {
	r.count -= 1
}
func (r *PingResult) success(timeDuration time.Duration) {
	r.CountIncrease()
	r.successful += 1

	if timeDuration > r.maxTime {
		r.maxTime = timeDuration
	}
	if timeDuration < r.minTime || r.minTime == 0 {
		r.minTime = timeDuration
	}

	r.elapsedTime += timeDuration
}
func (r *PingResult) fail() {
	r.CountIncrease()
	r.failed += 1
}
func (r *PingResult) AverageTime() time.Duration {
	return r.elapsedTime / time.Duration(r.successful)
}

func (result *PingResult) String() string {
	return fmt.Sprintf("Ping[http] %d times, elapsed: %s, max: %s, min: %s, avg: %s, success: %d, failed: %d",
		result.count,
		result.elapsedTime, result.maxTime, result.minTime, result.AverageTime(), result.successful, result.failed)
}

type PingHttp struct {
	//client  *http.Client
	Result PingResult
	//Server *core.Instance
	service.Service
}

func (h *PingHttp) client(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dest, err := corenet.ParseDestination(fmt.Sprintf("%s:%s", network, addr))
				if err != nil {
					return nil, err
				}
				return core.Dial(ctx, h.Server, dest)
			},
		},
	}
}
func (h *PingHttp) Ping(timeout time.Duration) error {
	client := h.client(timeout)
	if h.client == nil {
		return errors.New("invalid Client")
	}

	startTime := time.Now()
	if resp, err := client.Get(Google); err != nil {
		h.Result.fail()
		return err
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			h.Result.fail()
			return errors.New("invalid status code")
		}
	}
	h.Result.success(time.Since(startTime))
	return nil
}
