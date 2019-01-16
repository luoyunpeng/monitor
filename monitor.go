package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"time"

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
	log.Println("[ monitor ] set max processor to ", numProces)
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
	v1.GET("/container/debug/slicecap/:host", handler.ContainerSliceCap_Debug)

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

	srv := &http.Server{
		Addr:    port,
		Handler: initRouter(),
	}

	go func() {
		// service connections
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("**** Graceful shutdown monitor server ****")

	//release all
	stopAllDockerHost()
	log.Println("closing mysql db")
	dbCloseErr := common.CloseDB()
	if dbCloseErr != nil {
		log.Printf("Close DB err: %v", dbCloseErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Monitor Server shutdown:", err)
	}
	log.Println("**** Monitor server exiting **** ")
}

func cors(c *gin.Context) {
	whiteList := map[string]int{
		"http://192.168.100.173":     1,
		"http://www.repchain.net.cn": 2,
		"http://localhost:8080":      3,
	}

	// request header
	origin := c.Request.Header.Get("Origin")
	if _, ok := whiteList[origin]; ok {
		log.Println("allow access from origin: ", origin)
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		// allow to access all origin
		c.Header("Access-Control-Allow-Origin", origin)
		//all method that server supports, in case of to many pre-checking
		c.Header("Access-Control-Allow-Methods", "POST, GET, PUT, DELETE")
		//  header type
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Length, X-CSRF-Token, Token,session,X_Requested_With,Accept, Origin, Host, Connection, Accept-Encoding, Accept-Language,DNT, X-CustomHeader, Keep-Alive, User-Agent, X-Requested-With, If-Modified-Since, Cache-Control, Content-Type, Pragma")
		// allow across origin setting return other sub fields
		c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers,Cache-Control,Content-Language,Content-Type,Expires,Last-Modified,Pragma,FooBar")
		c.Header("Access-Control-Max-Age", "172800")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Set("content-type", "application/json")
	} else {
		log.Println("forbid access from origin: ", origin)
	}

	// handle request
	c.Next()
}

func stopAllDockerHost() {
	times := 0
	for len(container.AllStoppedDHIP()) != len(common.HostIPs) {
		if times >= 2 {
			break
		}
		container.AllHostList.Range(func(key, value interface{}) bool {
			if dh, ok := value.(*container.DockerHost); ok && dh.IsValid() {
				dh.StopCollect()
			}
			return true
		})
		times++
	}
	log.Printf("stop all docker host monitoring with %d loop", times)
}
