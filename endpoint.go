package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"strings"
)

type ResultCode struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

// 写入json到响应
func WriteJson(w http.ResponseWriter, v interface{}) {
	bytes, err := json.Marshal(v)
	if err != nil {
		log.Printf("返回结果序列化错误, rc: %v, err: %+v", v, err)
		if bytes, err = json.Marshal(&ResultCode{Code: -1, Msg: "结果序列化错误"}); err != nil {
			log.Printf("无法序列化错误")
			http.Error(w, "无法序列化", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	_, err = w.Write(bytes)
	if err != nil {
		log.Printf("无法写入http响应: %+v", err)
	}
}

// 写入成功结果
func OkCode(w http.ResponseWriter, v interface{}) {
	rc := ResultCode{Code: 0, Data: v}
	WriteJson(w, rc)
}

// 写入失败结果
func ErrorCode(w http.ResponseWriter, msg string) {
	rc := &ResultCode{Code: -1, Msg: msg}
	WriteJson(w, rc)
}

// 获取所有的资源
func GetAllResources(w http.ResponseWriter, r *http.Request) {
	res := Videos()
	OkCode(w, res)
}

// 获取资源内容
func GetContent(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	http.ServeFile(w, r, p)
}

var (
	srv http.Server
)

// http 方法
const (
	GET = "GET"
)

// 跨域中间件
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()

		if origin := r.Header.Get("Origin"); origin != "" {
			header.Set("Access-Control-Allow-Origin", origin)

			if r.Method == "OPTIONS" {
				// Preflight request
				if allowMethod := r.Header.Get("Access-Control-Request-Method"); allowMethod != "" {
					header.Set("Access-Control-Allow-Methods", allowMethod)
				}
				if allowHeaders := r.Header["Access-Control-Request-Headers"]; len(allowHeaders) > 0 {
					header.Set("Access-Control-Allow-Headers", strings.Join(allowHeaders, ", "))
				}
				header.Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func Start(port int) {
	r := mux.NewRouter()
	r.HandleFunc("/resources", GetAllResources).Methods(GET)
	r.HandleFunc("/content", GetContent).Methods(GET)
	r.Handle("/", http.RedirectHandler("/web", http.StatusMovedPermanently))
	r.PathPrefix("/web").Handler(http.StripPrefix("/web", http.FileServer(AssetFile())))

	log.Printf("http server listen at :%v", port)
	srv = http.Server{Addr: fmt.Sprintf(":%d", port), Handler: cors(r)}
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("http server exit with error: %+v", err)
	}
}

func Stop(ctx context.Context) {
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("http server shutdown error: %+v", err)
	}
}
