package main

import (
	"flag"
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
	numProces := runtime.NumCPU()
	runtime.GOMAXPROCS(numProces)
	println("[ monitor ] set max processor to ", numProces)
	flag.StringVar(&port, "port", ":8080", "base image use to create container")
	flag.Parse()
}

func initRouter() *gin.Engine {
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
	//v1.GET("/host/mem", handler.HostMemInfo)

	v1.GET("/dockerd/add/:host", handler.AddDockerhost)
	v1.GET("/dockerd/remove/:host", handler.StopDockerHostCollect)
	v1.GET("/dockerd/down/", handler.DownDockerHostInfo)

	return router
}

func main() {
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
		go container.Monitor(cli, ip)
	}
	go container.WriteAllHostInfo()
	//default run at :8080
	_, err := strconv.Atoi(port)
	if !strings.HasPrefix(port, ":") && err == nil {
		port = ":" + port
	} else {
		port = ":8080"
	}
	if err := initRouter().Run(port); err != nil {
		panic(err)
	}
}

func cors(c *gin.Context) {
	whiteList := map[string]int{
		"http://192.168.100.173":     1,
		"http://www.repchain.net.cn": 2,
		"http://localhost:8080":      3,
	}
	// request header
	origin := c.Request.Header.Get("Origin")
	if _, ok := whiteList[origin]; !ok {
		origin = "null"
	}

	if origin != "" {
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		// allow to access all origin
		c.Header("Access-Control-Allow-Origin", origin)
		//all method that server supports, in case of to many pre-checking
		c.Header("Access-Control-Allow-Methods", "POST, GET, PUT, DELETE,UPDATE")
		//  header type
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Length, X-CSRF-Token, Token,session,X_Requested_With,Accept, Origin, Host, Connection, Accept-Encoding, Accept-Language,DNT, X-CustomHeader, Keep-Alive, User-Agent, X-Requested-With, If-Modified-Since, Cache-Control, Content-Type, Pragma")
		// allow across origin setting return other sub fields
		c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers,Cache-Control,Content-Language,Content-Type,Expires,Last-Modified,Pragma,FooBar") // 跨域关键设置 让浏览器可以解析
		c.Header("Access-Control-Max-Age", "172800")                                                                                                                                                           // 缓存请求信息 单位为秒
		c.Header("Access-Control-Allow-Credentials", "false")                                                                                                                                                  //  跨域请求是否需要带cookie信息 默认设置为true
		c.Set("content-type", "application/json")                                                                                                                                                              // 设置返回格式是json
	}

	// handle request
	c.Next()
}
