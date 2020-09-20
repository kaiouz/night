package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// 是否是个目录, 发生错误或者不存在也返回false
func IsDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

// 是否文件，检查文件是否存在
func IsFileExists(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		return false
	}
	return true
}

// 文件名字
func FileName(path string) string {
	name := filepath.Base(path)
	ext := filepath.Ext(name)
	if ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

// 复制文件
func CopyFile(source, dest string) error {
	err := MkParentDir(dest)
	if err != nil {
		return err
	}
	df, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer df.Close()

	sf, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sf.Close()

	_, err = io.Copy(df, sf)
	return err
}

func MkParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), os.ModePerm)
}

// 文件的mime类型
func Mime(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	// 仅使用前512个字节来检测内容类型。
	buffer := make([]byte, 512)
	_, err = f.Read(buffer)
	if err != nil {
		return "", err
	}
	// 使用 net/http 包中的的DectectContentType函数,它将始终返回有效的 MIME 类型
	// 对于没有匹配的未知类型，将返回 "application/octet-stream"
	contentType := http.DetectContentType(buffer)
	return contentType, nil
}

// 是否视频文件
func IsVideo(path string) bool {
	mime, err := Mime(path)
	if err != nil {
		return false
	}
	return strings.Contains(mime, "video")
}

func Canceled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
