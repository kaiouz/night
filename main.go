package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	port := flag.Int("p", 8080, "http端口")
	dir := flag.String("d", "", "扫描目录")
	cacheDir := flag.String("c", "", "缓存目录")
	ffprobe := flag.String("ffprobe", "ffprobe", "ffprobe")
	ffmpeg := flag.String("ffmpeg", "ffmpeg", "ffmpeg")
	flag.Parse()

	go Start(*port)

	go ScanVideos([]string{*dir}, *cacheDir, *ffprobe, *ffmpeg)

	// 等待退出
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	<-c

	// 停止正在运行的服务
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wg := sync.WaitGroup{}

	// 停止http服务
	wg.Add(1)
	go func() {
		Stop(ctx)
		wg.Done()
	}()

	// 停止扫描
	wg.Add(1)
	go func() {
		StopScanVideos()
		wg.Done()
	}()

	wg.Wait()
}
