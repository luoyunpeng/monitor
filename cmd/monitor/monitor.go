package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"strings"

	"github.com/luoyunpeng/monitor/internal/conf"
	"github.com/luoyunpeng/monitor/internal/monitor"
	"github.com/luoyunpeng/monitor/internal/server"
)

var (
	port string
)

func init() {

}

func main() {
	conf, err := conf.NewConfig()
	if err != nil {
		panic(err)
	}

	if len(conf.Hosts) == 0 {
		panic("at least one host must given")
	}

	flag.StringVar(&port, "port", ":8080", "base image use to create container")
	flag.Parse()
	//for profiling
	go func() {
		log.Println(http.ListenAndServe(":8070", nil))
	}()

	for _, ip := range conf.Hosts {
		ip = strings.TrimSpace(ip)
		cli, err := monitor.InitClient(ip)
		if err != nil {
			log.Println("connect to ", ip, " err :", err)
			continue
		}
		go monitor.Monitor(cli, ip)
	}
	go monitor.WriteAllHostInfo(conf)

	port = parsePort(port)
	server.Start(port)
}

func parsePort(port string) string {
	_, err := strconv.Atoi(port)
	if !strings.HasPrefix(port, ":") && err == nil {
		port = ":" + port
	} else {
		port = ":8080"
	}
	return port
}
