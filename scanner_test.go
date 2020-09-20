package main

import "testing"

func TestScanVideos(t *testing.T) {
	ScanVideos([]string{"/Users/zoukai/Downloads"}, "/Users/zoukai/temp/", "ffprobe", "ffmpeg")
	<-done
}