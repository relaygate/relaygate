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
	Root          string
	PullInterval  time.Duration
	HeartInterval time.Duration
	// AfterPull is optional hook (e.g. HotApply); may be nil for stub.
	// applied-version is updated only when AfterPull returns nil.
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

	fmt.Printf("节点 agent 已启动：主控=%s 拉取间隔=%s\n", client.ControlURL, opts.PullInterval)

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
		if applied := LocalAppliedVersion(opts.Root); applied != "" && applied == ver {
			fmt.Printf("本机已对齐版本 %s，跳过重复落地（避免反复应用内核/防火墙/网关）\n", ver)
			return
		}
		if opts.AfterPull == nil {
			fmt.Println("未配置本机应用钩子：已落盘，applied 未更新（需 HotApply 成功后才对齐）")
			return
		}
		if err := opts.AfterPull(opts.Root, ver); err != nil {
			fmt.Fprintf(os.Stderr, "本机应用：%v\n", err)
			return
		}
		if err := MarkApplied(opts.Root, ver); err != nil {
			fmt.Fprintf(os.Stderr, "记录已应用版本失败：%v\n", err)
			return
		}
		fmt.Printf("本机已对齐版本 %s\n", ver)
	}
	doHeart := func() {
		ver := LocalAppliedVersion(opts.Root)
		pullNow, err := client.Heartbeat(ver)
		if err != nil {
			fmt.Fprintf(os.Stderr, "心跳：%v\n", err)
			return
		}
		if pullNow {
			fmt.Println("主控要求本节点立即对齐已发布版本（单节点同步）")
			doPull()
		}
	}

	doPull()
	doHeart()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("节点 agent 已停止")
			return nil
		case <-pullTicker.C:
			doPull()
		case <-heartTicker.C:
			doHeart()
		}
	}
}
