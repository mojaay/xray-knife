package detector

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"xray-knife/detector/output"
	"xray-knife/xray"
)

type Tester struct {
	Ctx      context.Context
	Parallel uint16 // max 65535
	Timeout  time.Duration
}

func (t *Tester) Run(urls []string) {

	ctx, cancel := context.WithCancel(t.Ctx)
	defer cancel()

	// 创建channel
	guard := make(chan int, t.Parallel)
	//jobs := make(chan config.Protocol, t.Parallel)
	ch := make(chan *Detective, len(urls))
	// 创建wg
	var wg sync.WaitGroup

	startTime := time.Now()
	// 启动goroutine
	for idx, url := range urls {
		protocol, err := xray.ParseXrayConfig(url)
		if err != nil {
			fmt.Println(err.Error())
			continue
		}

		select {
		case <-ctx.Done():
			fmt.Println(ctx.Err())
			return
		case guard <- idx:
			wg.Add(1)
			go func() {
				defer wg.Done()
				test(ctx, protocol, t.Timeout, ch)
				<-guard
			}()
		}
		//fmt.Println("par", runtime.NumGoroutine())
	}

	// 等待所有goroutine完成
	wg.Wait()
	// 关闭channel
	close(ch)

	// 遍历channel，输出结果
	for result := range ch {

		cfg, err := output.ConvertOutputConfig(result.config)
		if err != nil {
			fmt.Println("error:", err)
		}
		encodedJSON, err := json.Marshal(cfg)
		if err != nil {
			fmt.Println("error:", err)
		}
		fmt.Println("encode:")
		fmt.Println(string(encodedJSON))
		//fmt.Println("result OK -", result)
		fmt.Println(result.Result.String())
		fmt.Printf(
			"Speed test - Download: %.2f Mbps, Upload: %.2f Mbps\n",
			result.DownloadSpeed, result.UploadSpeed)
		fmt.Println()
	}
	fmt.Println("time used -", time.Since(startTime))
}

func test(ctx context.Context, protocol xray.Protocol, timeout time.Duration, ch chan<- *Detective) {

	_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	inst, err := NewInstance(_ctx, protocol)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	defer inst.Close()

	if err := inst.Ping(timeout); err != nil {
		return
	}
	inst.Speed()

	ch <- inst
}
