package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

type cacheInfo struct {
	Mod    map[string]time.Time `json:"mod"`
	Videos map[string]*Video    `json:"videos"`
}

func (c *cacheInfo) ModTime(path string) time.Time {
	return c.Mod[path]
}

func (c *cacheInfo) AddModTime(path string, time time.Time) {
	c.Mod[path] = time
}

func (c *cacheInfo) AddVideo(video *Video) {
	c.Videos[video.Path] = video
}

func (c *cacheInfo) RemoveVideo(path string) {
	delete(c.Videos, path)
}

func (c *cacheInfo) AllVideos() []*Video {
	vs := make([]*Video, len(c.Videos))
	i := 0
	for _, v := range c.Videos {
		vs[i] = v
		i++
	}
	return vs
}

func (c *cacheInfo) RemoveNotExistVideos() {
	for k, _ := range c.Videos {
		if !IsFileExists(k) {
			delete(c.Videos, k)
			delete(c.Mod, k)
		}
	}
}

func (c *cacheInfo) IsExists(path string) bool {
	v := c.Videos[path]
	if v == nil {
		return false
	}
	if v.Preview == nil {
		return false
	}
	if v.Preview.Thumbs == nil {
		return false
	}
	return IsFileExists(v.Preview.Cover) && IsFileExists(v.Preview.Thumbs.Path)
}

func (c *cacheInfo) IsEmpty() bool {
	return len(c.Mod) <= 0 && len(c.Videos) <= 0
}

func (c *cacheInfo) Read(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.WithMessage(err, "读取缓存信息失败")
	}
	err = json.Unmarshal(data, c)
	if err != nil {
		return errors.WithMessage(err, "解析缓存信息失败")
	}
	return nil
}

func (c *cacheInfo) Write(path string) error {
	if c.IsEmpty() {
		return nil
	}
	data, err := json.Marshal(c)
	if err != nil {
		return errors.WithMessage(err, "生成缓存信息失败")
	}
	err = MkParentDir(path)
	if err != nil {
		return errors.WithMessage(err, "创建缓存信息失败")
	}
	err = ioutil.WriteFile(path, data, os.ModePerm)
	if err != nil {
		return errors.WithMessage(err, "写入缓存信息失败")
	}
	return nil
}

var (
	cache  = &cacheInfo{Mod: map[string]time.Time{}, Videos: map[string]*Video{}}
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
)

func init() {
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
}

func complete() {
	close(done)
}

// 停止扫描目录生成
func StopScanVideos() {
	cancel()
	<-done
}

// 扫描目录生成资源
func ScanVideos(videoDirs []string, cacheDir string, ffprobe, ffmpeg string) {
	cacheF := filepath.Join(cacheDir, "cache.json")
	if IsFileExists(cacheF) {
		err := cache.Read(cacheF)
		if err != nil {
			fmt.Printf("缓存信息读取失败了: %+v\n", err)
		} else {
			// 移除不存在的视频信息
			cache.RemoveNotExistVideos()
		}
	}

	var videos []string
	for _, resDir := range videoDirs {
		filepath.Walk(resDir, func(path string, info os.FileInfo, err error) error {
			// 如果取消就停止
			if Canceled(ctx) {
				return filepath.SkipDir
			}

			fmt.Printf("扫描: %v.", path)

			if err != nil {
				fmt.Printf("文件或目录错误, %v, 跳过\n", path)
				return nil
			}

			if info.IsDir() {
				if info.ModTime() == cache.ModTime(path) {
					fmt.Println("无需更新, 跳过")
					return filepath.SkipDir
				}
				// 换个行，输出好看
				fmt.Println()
				//cache.AddModTime(path, info.ModTime())
				return nil
			}

			if !IsVideo(path) {
				fmt.Println("不是视频，跳过")
				return nil
			}

			if !cache.IsExists(path) {
				fmt.Println("不存在, 待生成")
				videos = append(videos, path)
				cache.AddModTime(path, info.ModTime())
				return nil
			}

			if info.ModTime() == cache.ModTime(path) {
				fmt.Println("无需更新, 跳过")
				return nil
			}

			// 要生成信息, 把旧的信息移除
			cache.RemoveVideo(path)
			videos = append(videos, path)

			return nil
		})
	}
	// 保存到仓库
	if vs := cache.AllVideos(); len(vs) > 0 {
		SaveVideos(vs)
	}
	writeCache(cacheF)

	if Canceled(ctx) {
		complete()
		return
	}

	// 提交生成视频信息的任务
	if len(videos) <= 0 {
		complete()
		return
	}

	videosIn := make(chan string)
	go genVideoInfoWork(ctx, videosIn, len(videos), cacheDir, cacheF, ffprobe, ffmpeg)
	for _, v := range videos {
		select {
		case <-ctx.Done():
			close(videosIn)
			return
		case videosIn <- v:
		}
	}
}

func writeCache(cacheF string) {
	err := cache.Write(cacheF)
	if err != nil {
		fmt.Printf("写入缓存信息失败: %+v\n", err)
	}
}

func addCacheVideo(video *Video) {
	cache.AddVideo(video)
	AddVideo(video)
}

func genVideoInfoWork(ctx context.Context, videoIn <-chan string, count int, cacheDir, cacheF string, ffprobe, ffmpeg string) {
	defer func() {
		writeCache(cacheF)
		complete()
	}()

	ps := &ProgressServer{}
	err := ps.Start()
	if err != nil {
		fmt.Printf("启动进度服务器错误: %+v\n", err)
		return
	}
	defer ps.Stop()

	i := 1
	for {
		select {
		case v, ok := <-videoIn:
			if !ok {
				return
			}
			video, err := genVideoInfo(ctx, ffprobe, ffmpeg, v, cacheDir, count, i, ps)
			i++
			if err != nil {
				fmt.Printf("视频信息生成失败: %+v\n", err)
				continue
			}
			addCacheVideo(video)
		case <-ctx.Done():
			return
		}
	}
}

func genVideoInfo(ctx context.Context, ffprobe, ffmpeg, path, cacheDir string, count, cur int, ps *ProgressServer) (*Video, error) {
	v, err := VideoInfo(ffprobe, path)
	if err != nil {
		return nil, err
	}

	previewDir, err := ioutil.TempDir(cacheDir, "")
	if err != nil {
		return nil, errors.WithMessage(err, "生成预览目录失败")
	}

	fmt.Printf("进度: %v\n", path)
	ps.SetProgressSource(&ProgressSource{
		Duration: v.Duration,
		ProgressCb: func(p *Progress) {
			fmt.Printf("进度: %d/%d, %v\r", cur, count, time.Duration(float64(p.Duration-p.OutTime)/p.Speed))
		},
	})

	cw, ch := AdjustAspectRatio(v.Width, v.Height, 412, 232)
	v.Preview, err = GenVideoPreview(ctx, v.Duration, ffmpeg, path, previewDir, ps.Addr(), PreviewConfig{
		spf: 5, maxF: 100, width: 1600, height: 900, cW: cw, cH: ch, perW: 160, perH: 90,
	})
	if err != nil {
		return nil, err
	}
	return v, nil
}
