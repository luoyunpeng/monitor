package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"

	"github.com/luoyunpeng/monitor/common"
	"github.com/luoyunpeng/monitor/internal/conf"
	"github.com/luoyunpeng/monitor/internal/monitor"
	"github.com/luoyunpeng/monitor/internal/server"
)

var (
	port string
)

func init() {
	if len(conf.HostIPs) == 0 {
		panic("at least one host must given")
	}
	numProces := runtime.NumCPU()
	runtime.GOMAXPROCS(numProces)
	log.Println("[ monitor ] set max processor to ", numProces)
	flag.StringVar(&port, "port", ":8080", "base image use to create container")
	flag.Parse()
}

func main() {
	//for profiling
	go func() {
		log.Println(http.ListenAndServe(":8070", nil))
	}()

	for _, ip := range conf.HostIPs {
		cli, err := common.InitClient(ip)
		if err != nil {
			log.Println("connect to ", ip, " err :", err)
			continue
		}
		go monitor.Monitor(cli, ip)
	}
	go monitor.WriteAllHostInfo()

	server.Start(port)
}
