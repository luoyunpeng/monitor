package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"strings"

	"github.com/luoyunpeng/monitor/internal/config"
	"github.com/luoyunpeng/monitor/internal/models"
	"github.com/luoyunpeng/monitor/internal/monitor"
	"github.com/luoyunpeng/monitor/internal/server"
)

var (
	port string
)

func main() {
	config.Load()
	for _, ip := range config.MonitorInfo.GetHosts() {
		ip = strings.TrimSpace(ip)
		cli, err := monitor.InitClient(ip)
		if err != nil {
			config.MonitorInfo.Logger.Printf("connect to host-%s occur error: %v ", ip, err)
			models.StoppedDockerHost.Store(ip, 1)
			continue
		}
		go monitor.Monitor(cli, ip, config.MonitorInfo.Logger)
	}
	go monitor.WriteAllHostInfo()
	go monitor.RecoveryStopped()
	//for profiling
	go func() {
		log.Println(http.ListenAndServe(":8070", nil))
	}()
	server.Start(parsePort())
}

func parsePort() string {
	flag.StringVar(&port, "port", ":8080", "base image use to create container")
	flag.Parse()

	_, err := strconv.Atoi(port)
	if !strings.HasPrefix(port, ":") && err == nil {
		return ":" + port
	}
	return ":8080"
}
