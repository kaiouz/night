package main

var store []*Video

// 获取所有的视频信息
func Videos() []*Video {
	return store
}

// 保存视频信息
func SaveVideos(videos []*Video) {
	store = videos
}

// 添加视频
func AddVideo(video *Video) {
	store = append(store, video)
}
