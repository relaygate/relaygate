package agent

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// RunOptions configures agent run loop.
type RunOptions struct {
	Root         string
	PullInterval time.Duration
	HeartInterval time.Duration
	// AfterPull is optional hook (e.g. HotApply); may be nil for stub.
	AfterPull func(root, version string) error
}

// Run loops: heartbeat + periodic pull until SIGINT/SIGTERM.
func Run(opts RunOptions) error {
	if opts.PullInterval <= 0 {
		opts.PullInterval = 30 * time.Second
	}
	if opts.HeartInterval <= 0 {
		opts.HeartInterval = 15 * time.Second
	}
	client, err := LoadClientFromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("节点代理已启动：主控=%s 拉取间隔=%s\n", client.PrimaryURL, opts.PullInterval)

	pullTicker := time.NewTicker(opts.PullInterval)
	heartTicker := time.NewTicker(opts.HeartInterval)
	defer pullTicker.Stop()
	defer heartTicker.Stop()

	doPull := func() {
		ver, err := client.PullOnce(opts.Root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "拉取：%v\n", err)
			return
		}
		fmt.Printf("已拉取配置版本 %s\n", ver)
		if opts.AfterPull != nil {
			if err := opts.AfterPull(opts.Root, ver); err != nil {
				fmt.Fprintf(os.Stderr, "本机应用：%v\n", err)
			}
		}
	}
	doHeart := func() {
		ver := LocalAppliedVersion(opts.Root)
		if err := client.Heartbeat(ver); err != nil {
			fmt.Fprintf(os.Stderr, "心跳：%v\n", err)
		}
	}

	doPull()
	doHeart()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("节点代理已停止")
			return nil
		case <-pullTicker.C:
			doPull()
		case <-heartTicker.C:
			doHeart()
		}
	}
}
