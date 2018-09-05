package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luoyunpeng/monitor/common"
	"github.com/luoyunpeng/monitor/container"
	"github.com/luoyunpeng/monitor/handler"
)

var (
	port string
)

func init() {
	if len(common.HostIPs) == 0 {
		panic("at least one host must given")
	}
	if runtime.NumCPU() >= 4 {
		numProces := runtime.NumCPU() / 2
		runtime.GOMAXPROCS(numProces)
		println("[ monitor ] set max processor to ", numProces)
	}
	flag.StringVar(&port, "port", ":8080", "base image use to create container")
	flag.Parse()
}

func main() {
	router := gin.Default()
	router.Use(cors)
	v1 := router.Group("")

	v1.GET("/container/stats/:id", handler.ContainerStats)
	v1.GET("/container/metric/mem/:id", handler.ContainerMem)
	v1.GET("/container/metric/mempercent/:id", handler.ContainerMemPercent)
	v1.GET("/container/metric/memlimit/:id", handler.ContainerMemLimit)
	v1.GET("/container/metric/cpu/:id", handler.ContainerCPU)
	v1.GET("/container/metric/networkio/:id", handler.ContainerNetworkIO)
	v1.GET("/container/metric/blockio/:id", handler.ContainerBlockIO)
	v1.GET("/container/info", handler.ContainerInfo)
	v1.GET("/container/logs/:id", handler.ContainerLogs)
	v1.GET("/host/mem", handler.HostMemInfo)

	//v1.POST("/dockerd/add")
	//for profiling
	go func() {
		log.Println(http.ListenAndServe(":8070", nil))
	}()

	for _, ip := range common.HostIPs {
		cli, err := common.InitClient(ip)
		if err != nil {
			log.Println("connect to ", ip, " err :", err)
			continue
		}
		go container.KeepStats(cli, ip)
		container.DockerCliList.Store(ip, cli)
	}
	go container.WriteAllHostInfo()
	//default run at :8080
	_, err := strconv.Atoi(port)
	if !strings.HasPrefix(port, ":") && err == nil {
		port = ":" + port
	} else {
		port = ":8080"
	}
	router.Run(port)
}

func cors(c *gin.Context) {
	// request method
	method := c.Request.Method
	// request header
	origin := c.Request.Header.Get("Origin")

	// request header keys
	var headerKeys []string
	for k := range c.Request.Header {
		headerKeys = append(headerKeys, k)
	}
	headerStr := strings.Join(headerKeys, ", ")
	if headerStr != "" {
		headerStr = fmt.Sprintf("access-control-allow-origin, access-control-allow-headers, %s", headerStr)
	} else {
		headerStr = "access-control-allow-origin, access-control-allow-headers"
	}
	if origin != "" {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		// allow to access all origin
		c.Header("Access-Control-Allow-Origin", "*")
		//服务器支持的所有跨域请求的方法,为了避免浏览次请求的多次'预检'请求
		c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE,UPDATE")
		//  header type
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Length, X-CSRF-Token, Token,session,X_Requested_With,Accept, Origin, Host, Connection, Accept-Encoding, Accept-Language,DNT, X-CustomHeader, Keep-Alive, User-Agent, X-Requested-With, If-Modified-Since, Cache-Control, Content-Type, Pragma")
		// allow across origin setting 可以返回其他子段
		c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers,Cache-Control,Content-Language,Content-Type,Expires,Last-Modified,Pragma,FooBar") // 跨域关键设置 让浏览器可以解析
		c.Header("Access-Control-Max-Age", "172800")                                                                                                                                                           // 缓存请求信息 单位为秒
		c.Header("Access-Control-Allow-Credentials", "false")                                                                                                                                                  //  跨域请求是否需要带cookie信息 默认设置为true
		c.Set("content-type", "application/json")                                                                                                                                                              // 设置返回格式是json
	}

	//allow all OPTIONS method
	if method == "OPTIONS" {
		c.JSON(http.StatusOK, "Options Request!")
	}
	// handle request
	c.Next()
}
