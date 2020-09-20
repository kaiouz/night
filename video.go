package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/nfnt/resize"
	"github.com/pkg/errors"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Video struct {
	Name     string        `json:"name"`
	Path     string        `json:"path"`
	Duration time.Duration `json:"duration"`
	Width    int           `json:"width"`
	Height   int           `json:"height"`
	Preview  *VideoPreview `json:"preview"`
}

// 获取视频的信息
func VideoInfo(ffprobe string, path string) (*Video, error) {
	cmd := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=height,width", "-of", "csv=s=x:p=0", "-show_format", "-print_format", "json", path)
	out, err := cmd.Output()

	if err != nil {
		// 命令执行错误
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, errors.Errorf("执行错误:%s\n%s\n%s\n", cmd.String(), ee.Error(), ee.Stderr)
		} else {
			// 其他io错误
			return nil, errors.WithStack(err)
		}
	}

	vfj := struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}{}
	err = json.Unmarshal(out, &vfj)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	duration, err := strconv.ParseFloat(vfj.Format.Duration, 64)
	if err != nil {
		return nil, errors.WithMessage(err, "视频时长解析错误")
	}

	video := &Video{
		Name:     FileName(path),
		Path:     path,
		Duration: time.Millisecond * time.Duration(duration*1000),
	}
	if len(vfj.Streams) > 0 {
		video.Width = vfj.Streams[0].Width
		video.Height = vfj.Streams[0].Height
	}

	return video, nil
}

type VideoPreview struct {
	Cover  string       `json:"cover"`
	Thumbs *ThumbSprite `json:"thumbs"`
}

type PreviewConfig struct {
	spf, maxF, width, height, cW, cH, perW, perH int
}

// 生辰视频缩略图
func GenVideoPreview(ctx context.Context, duration time.Duration, ffmpeg, path, outDir, progressUrl string, pc PreviewConfig) (*VideoPreview, error) {
	var fps string
	if time.Duration(pc.spf*pc.maxF)*time.Second > duration {
		fps = fmt.Sprintf("%d/%d", 1, pc.spf)
	} else {
		fps = fmt.Sprintf("%d/%d", pc.maxF, duration/time.Second)
	}

	thumbDir := filepath.Join(outDir, "thumbs")
	thumbs, err := videoThumbnails(ctx, ffmpeg, path, thumbDir, fps, pc.cW, pc.cH, progressUrl)
	// 删除所有的临时缩略图
	defer os.RemoveAll(thumbDir)
	if err != nil {
		return nil, err
	}

	// 复制中间图作为封面
	cover := filepath.Join(outDir, "cover.jpg")
	err = CopyFile(thumbs[len(thumbs)/2], cover)
	if err != nil {
		return nil, errors.WithMessage(err, "生成封面错误")
	}

	vts, err := videoThumbnailsSprite(thumbs, filepath.Join(outDir, "thumbs.jpg"), pc.width, pc.height, pc.width/pc.perW, pc.height/pc.perH)
	if err != nil {
		return nil, err
	}

	return &VideoPreview{
		Cover:  cover,
		Thumbs: vts,
	}, nil
}

// 视频缩略图
func videoThumbnails(ctx context.Context, ffmpeg, path, thumbDir, fps string, width, height int, progressUrl string) ([]string, error) {
	size := fmt.Sprintf("%dx%d", width, height)
	out := filepath.Join(thumbDir, "thum%03d.jpg")

	// 确保目录已创建
	err := os.MkdirAll(thumbDir, os.ModePerm)
	if err != nil {
		return nil, errors.WithMessage(err, "无法创建缩略图目录")
	}

	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-v", "error", "-progress", progressUrl, "-i", path, "-vf", "fps="+fps, "-s", size, out)
	_, err = cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			// 命令执行错误, 从标准错误中获取真正的错误
			return nil, errors.Errorf("执行错误: %s\n%s\n%s\n", cmd.String(), ee.Error(), ee.Stderr)
		} else {
			// 其他io错误
			return nil, errors.WithStack(err)
		}
	}

	thumbs, err := ioutil.ReadDir(thumbDir)
	if err != nil {
		return nil, errors.WithMessage(err, "读取缩略图失败")
	}

	thumbsPaths := make([]string, len(thumbs))
	for i := 0; i < len(thumbs); i++ {
		thumbsPaths[i] = filepath.Join(thumbDir, thumbs[i].Name())
	}

	// 排序
	sort.Strings(thumbsPaths)

	return thumbsPaths, nil
}

type Progress struct {
	Duration time.Duration
	OutTime  time.Duration
	Speed    float64
}

type progressCollector struct {
	duration time.Duration
	progress *Progress
}

func (p *progressCollector) Collect(kv string) (*Progress, bool) {
	//frame=1
	//fps=0.0
	//stream_0_0_q=1.9
	//bitrate=N/A
	//total_size=N/A
	//out_time_ms=1800000000
	//out_time=00:30:00.000000
	//dup_frames=0
	//drop_frames=0
	//speed=25.6x
	//progress=continue
	if p.progress == nil {
		p.progress = &Progress{Duration: p.duration}
	}
	s := strings.Split(kv, "=")

	switch s[0] {
	case "out_time_ms":
		if ot, err := strconv.ParseInt(strings.TrimSpace(s[1]), 10, 64); err == nil {
			p.progress.OutTime = time.Duration(ot) * time.Microsecond
		}
	case "speed":
		ss := strings.TrimSpace(s[1])
		ss = ss[:len(ss)-1]
		if speed, err := strconv.ParseFloat(ss, 64); err == nil {
			p.progress.Speed = speed
		}
	case "progress":
		progress := p.progress
		p.progress = nil
		return progress, true
	}

	return nil, false
}

type ProgressServer struct {
	addr   string
	ln     net.Listener
	source *ProgressSource
	sync.Mutex
}

type ProgressSource struct {
	Duration time.Duration
	ProgressCb func(*Progress)
}

func (p *ProgressServer) SetProgressSource(source *ProgressSource) {
	p.Lock()
	p.source = source
	p.Unlock()
}

func (p *ProgressServer) NextProgressSource() *ProgressSource {
	p.Lock()
	source := p.source
	p.source = nil
	p.Unlock()
	return source
}

func (p *ProgressServer) Addr() string {
	return p.addr
}

func (p *ProgressServer) Start() error {
	ln, err := net.Listen("tcp4", "127.0.0.1:")
	if err != nil {
		return errors.WithStack(err)
	}
	p.ln = ln

	progressFn := func(conn net.Conn, source *ProgressSource) {
		defer conn.Close()
		r := bufio.NewReader(conn)
		prefix := ""
		pc := &progressCollector{duration: source.Duration}
		for {
			line, isPrefix, err := r.ReadLine()
			// 遇到错误, 退出
			if err != nil {
				return
			}
			if isPrefix {
				prefix += string(line)
				continue
			}
			kv := prefix + string(line)
			prefix = ""
			if p, ok := pc.Collect(kv); ok {
				source.ProgressCb(p)
			}
		}
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("接收进度连接失败: %+v", errors.WithStack(err))
				continue
			}
			go progressFn(conn, p.NextProgressSource())
		}
	}()

	p.addr = ln.Addr().Network() + "://" + ln.Addr().String()
	fmt.Printf("进度服务地址: %v\n", p.addr)
	return nil
}

func (p *ProgressServer) Stop() error {
	return p.ln.Close()
}

type ThumbSprite struct {
	Path        string `json:"path"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	ThumbWidth  int    `json:"thumbWidth"`
	ThumbHeight int    `json:"thumbHeight"`
	Count       int    `json:"count"`
}

// 生成精灵图
func videoThumbnailsSprite(thumbs []string, out string, width, height, rows, cols int) (*ThumbSprite, error) {
	// 包含的缩略图数量
	nums := int(math.Min(float64(rows*cols), float64(len(thumbs))))

	// 每个缩略图的尺寸
	thumbWidth := width / cols
	thumbHeight := height / rows

	// 生成一张背景图, 默认背景是黑的
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))

	// 逐个把缩略图画在背景图上
	for i := 0; i < nums; i++ {
		col := i % cols
		row := i / cols
		rect := image.Rect(col*thumbWidth, row*thumbHeight, (col+1)*thumbWidth, (row+1)*thumbHeight)
		err := drawThumb(canvas, rect, thumbs[i])
		if err != nil {
			return nil, errors.WithMessagef(err, "绘制缩略图错误, thumb: %v", thumbs[i])
		}
	}

	err := os.MkdirAll(filepath.Dir(out), os.ModePerm)
	if err != nil {
		return nil, errors.WithMessage(err, "精灵图目录创建失败")
	}
	f, err := os.Create(out)
	defer f.Close()
	if err != nil {
		return nil, errors.WithMessagef(err, "精灵图创建失败")
	}
	err = jpeg.Encode(f, canvas, &jpeg.Options{Quality: 80})
	if err != nil {
		return nil, errors.WithMessagef(err, "精灵图写入失败")
	}

	return &ThumbSprite{
		Path:        out,
		Width:       width,
		Height:      height,
		ThumbWidth:  thumbWidth,
		ThumbHeight: thumbHeight,
		Count:       nums,
	}, nil
}

func drawThumb(dst draw.Image, r image.Rectangle, thumbPath string) error {
	thumbWidth := r.Dx()
	thumbHeight := r.Dy()

	thumb, err := os.Open(thumbPath)
	if err != nil {
		return errors.WithStack(err)
	}
	defer thumb.Close()

	config, _, err := image.DecodeConfig(thumb)
	if err != nil {
		return errors.WithStack(err)
	}
	thumb.Seek(0, io.SeekStart)

	// 调整原始缩略图的尺寸，适应目标位置的宽高
	// 如果宽高小于目标，居中
	adjustW, adjustH := AdjustAspectRatio(config.Width, config.Height, thumbWidth, thumbHeight)
	var dstRect image.Rectangle
	if thumbHeight > adjustH {
		// 垂直居中
		dy := (thumbHeight - adjustH) / 2
		dstRect = image.Rect(r.Min.X, r.Min.Y+dy, r.Max.X, r.Max.Y-dy)
	} else if thumbWidth > adjustW {
		// 水平居中
		dx := (thumbWidth - adjustW) / 2
		dstRect = image.Rect(r.Min.X+dx, r.Min.Y, r.Max.X-dx, r.Max.Y)
	} else {
		dstRect = r
	}

	img, _, err := image.Decode(thumb)
	if err != nil {
		return errors.WithStack(err)
	}
	// 缩放图片
	adjustImg := resize.Resize(uint(adjustW), uint(adjustH), img, resize.NearestNeighbor)

	// 绘制
	draw.Draw(dst, dstRect, adjustImg, image.Point{}, draw.Src)

	return nil
}

func AdjustAspectRatio(width, height, tw, th int) (int, int) {
	var adjustW, adjustH int
	if width > tw*height/th {
		// 原图尺寸相对于目标尺寸太宽了，适应宽度，等比缩放高度
		adjustW = tw
		adjustH = adjustW * height / width
	} else if height > th*width/tw {
		// 原图尺寸相对于目标尺寸太高了，适应高度，等比缩放宽度
		adjustH = th
		adjustW = adjustH * width / height
	} else {
		// 同比例
		adjustW = tw
		adjustH = th
	}
	return adjustW, adjustH
}
