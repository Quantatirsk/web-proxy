package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var (
	target string // 目标域名
	port   int    // 代理端口
)

func main() {
	// 从环境变量获取目标域名
	target := os.Getenv("TARGET_URL")
	if target == "" {
		// 如果环境变量未设置，则默认使用 "https://api.openai.com"
		target = "https://api.openai.com"
	}

	// 从命令行参数获取代理端口
	flag.IntVar(&port, "port", 9000, "The proxy port.")
	flag.Parse()

	// 打印配置信息
	log.Println("Target domain: ", target)
	log.Println("Proxy port: ", port)

	http.HandleFunc("/", handleRequest)
	http.ListenAndServe(":"+strconv.Itoa(port), nil)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// 过滤无效URL
	_, err := url.Parse(r.URL.String())
	if err != nil {
		log.Println("Error parsing URL: ", err.Error())
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// 去掉环境前缀（针对腾讯云，如果包含的话，目前我只用到了test和release）
	newPath := strings.Replace(r.URL.Path, "/release", "", 1)
	newPath = strings.Replace(newPath, "/test", "", 1)

	// 拼接目标URL（带上查询字符串，如果有的话）
	// 如果请求中包含 X-Target-Host 头，则使用该头作为目标域名
	// 优先级 header > env variable
	var targetURL string
	if r.Header.Get("X-Target-Host") != "" {
		targetURL = "https://" + r.Header.Get("X-Target-Host") + newPath
	} else {
		targetURL = target + newPath
	}
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// 本地打印代理请求完整URL用于调试
	if os.Getenv("ENV") == "local" {
		fmt.Printf("Proxying request to: %s\n", targetURL)
	}

	// 修改请求头，将客户端IP地址设置为代理服务器IP地址
	// 从环境变量获取代理服务器IP
	proxyServerIP := os.Getenv("PROXY_SERVER_IP")

	if proxyServerIP == "" {
		// 如果环境变量没有获取到IP，尝试通过 curl ipconfig.me 获取
		cmd := exec.Command("curl", "ipconfig.me")
		output, err := cmd.Output()
		if err == nil {
			// 如果成功获取IP，设置到请求头
			proxyServerIP = strings.TrimSpace(string(output))
		} else {
			log.Println("Failed to get IP from ipconfig.me:", err)
		}
	}

	if proxyServerIP != "" {
		// 设置客户端IP地址到请求头
		r.Header.Set("X-Forwarded-For", proxyServerIP)
	}

	// 创建代理HTTP请求
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		log.Println("Error creating proxy request: ", err.Error())
		http.Error(w, "Error creating proxy request", http.StatusInternalServerError)
		return
	}

	// 将原始请求头复制到新请求中
	for headerKey, headerValues := range r.Header {
		for _, headerValue := range headerValues {
			proxyReq.Header.Add(headerKey, headerValue)
		}
	}

	// 默认超时时间设置为300s（应对长上下文）
	client := &http.Client{}

	// 发起代理请求
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Println("Error sending proxy request: ", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// 将响应头复制到代理响应头中
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// 将响应状态码设置为原始响应状态码
	w.WriteHeader(resp.StatusCode)

	// 将响应实体写入到响应流中（支持流式响应）
	buf := make([]byte, 1024)
	for {
		if n, err := resp.Body.Read(buf); err == io.EOF || n == 0 {
			return
		} else if err != nil {
			log.Println("error while reading resp body: ", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			if _, err = w.Write(buf[:n]); err != nil {
				log.Println("error while writing resp: ", err.Error())
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.(http.Flusher).Flush()
		}
	}
}
