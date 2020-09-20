package main

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestVideoInfo(t *testing.T) {
	v, err := VideoInfo("ffprobe", "/Users/zoukai/Downloads/ff7.mp4")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("%v", v)
	v, err = VideoInfo("ffprobe", "/Users/zoukai/Downloads/my.mp4")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("%v", v)
}

func TestVideoThumbnails(t *testing.T) {
	type args struct {
		ffmpeg string
		path   string
		outDir string
		fps    string
		width  int
		height int
	}

	arg := args{
		ffmpeg: "ffmpeg",
		path:   "/Users/zoukai/Downloads/my.mp",
		outDir: "/Users/zoukai/Downloads",
		fps:    "1/1800",
		width:  640,
		height: 360,
	}

	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "缩略图",
			args: arg,
			want: []string{
				"/Users/zoukai/Downloads/thums/thum001.jpg",
				"/Users/zoukai/Downloads/thums/thum002.jpg",
				"/Users/zoukai/Downloads/thums/thum003.jpg",
				"/Users/zoukai/Downloads/thums/thum004.jpg",
				"/Users/zoukai/Downloads/thums/thum005.jpg",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := videoThumbnails(context.Background(), tt.args.ffmpeg, tt.args.path, tt.args.outDir, tt.args.fps, tt.args.width, tt.args.height, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("videoThumbnails() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("videoThumbnails() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVideoThumbnailsSprite(t *testing.T) {
	thumbs := make([]string, 100)
	for i := 0; i < len(thumbs); i++ {
		thumbs[i] = "/Users/zoukai/Downloads/DSC_3100.JPG"
	}
	out := "/Users/zoukai/Downloads/sprite/thumbs.jpg"
	sprite, err := videoThumbnailsSprite(thumbs, out, 1600, 900, 10, 10)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("%+v", sprite)
}

func TestVideoThumbnailsProgress(t *testing.T) {
	ps := &ProgressServer{}
	err := ps.Start()
	if err != nil {
		t.Fatalf("%+v", err)
	}
	defer ps.Stop()

	vi, err := VideoInfo("ffprobe", "/Users/zoukai/Downloads/ff7.mp4")
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ps.SetProgressSource(&ProgressSource{Duration: vi.Duration})

	_, err = videoThumbnails(context.Background(), "ffmpeg", "/Users/zoukai/Downloads/ff7.mp4", "/Users/zoukai/Downloads",
		"50/261", 160, 90, ps.Addr())

	if err != nil {
		t.Fatalf("%+v", err)
	}
}

func TestGenVideoThumbnail(t *testing.T) {
	ps := &ProgressServer{}
	err := ps.Start()
	if err != nil {
		t.Fatalf("%+v", err)
	}
	defer ps.Stop()

	vi, err := VideoInfo("ffprobe", "/Users/zoukai/Downloads/ff7.mp4")
	if err != nil {
		t.Fatalf("%+v", err)
	}

	ps.SetProgressSource(&ProgressSource{Duration: vi.Duration})

	_, err = GenVideoPreview(context.Background(), vi.Duration, "ffmpeg", "/Users/zoukai/Downloads/ff7.mp4", "/Users/zoukai/Downloads/thumbstest",
		ps.Addr(), PreviewConfig{
			5, 100, 1600, 900, 412, 232, 160, 90,
		})

	if err != nil {
		t.Fatalf("%+v", err)
	}
}

func TestConsole(t *testing.T) {
	for i := 0; i < 100; i++ {
		time.Sleep(time.Millisecond * 300)
		fmt.Printf( "进度: %v", i)
	}
}
