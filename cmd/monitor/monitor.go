package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"strings"

	"github.com/luoyunpeng/monitor/internal/config"
	"github.com/luoyunpeng/monitor/internal/monitor"
	"github.com/luoyunpeng/monitor/internal/server"
)

var (
	port string
)

func main() {
	config.Load()

	flag.StringVar(&port, "port", ":8080", "base image use to create container")
	flag.Parse()
	port = parsePort(port)

	for _, ip := range config.MonitorInfo.Hosts {
		ip = strings.TrimSpace(ip)
		cli, err := monitor.InitClient(ip)
		if err != nil {
			log.Printf("connect to host-%s occur error: %v ", ip, err)
			continue
		}
		go monitor.Monitor(cli, ip)
	}
	go monitor.WriteAllHostInfo()

	//for profiling
	go func() {
		log.Println(http.ListenAndServe(":8070", nil))
	}()
	server.Start(port)
}

func parsePort(port string) string {
	_, err := strconv.Atoi(port)

	if !strings.HasPrefix(port, ":") && err == nil {
		return ":" + port
	}
	return ":8080"
}
