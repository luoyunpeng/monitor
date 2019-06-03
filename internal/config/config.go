package config

import (
	"log"
	"time"

	"github.com/go-ini/ini"
	"github.com/luoyunpeng/monitor/internal/util"
)

var (
	MonitorInfo configure
)

type configure struct {
	CacheNum        int
	MaxTimeoutTimes int
	CollectDuration time.Duration
	CollectTimeout  time.Duration

	Hosts []string

	SqlHost     string
	SqlPort     string
	SqlDBName   string
	SqlUser     string
	SqlPassword string

	InfluxDB         string
	InfluxDBPort     string
	InfluxDBName     string
	InfluxDBUser     string
	InfluxDBPassword string

	Logger *log.Logger
}

func Load() {
	isInContainer, err := util.IsInsideContainer()
	if err != nil {
		// do nothing, ignore error
	}

	if isInContainer {
		log.Println("[config] monitor is running inside container")
		defaultConfig()
		return
	}
	loadFromConfigureFile()
}

func loadFromConfigureFile() {
	cfg, err := ini.Load("monitor.ini")
	if err != nil {
		panic(err)
	}
	err = cfg.Section("monitor").MapTo(&MonitorInfo)
	if err != nil {
		panic(err)
	}
	if len(MonitorInfo.Hosts) == 0 {
		panic("at least one host must given")
	}
	if MonitorInfo.CollectDuration < 30 || MonitorInfo.CollectDuration > 120 {
		MonitorInfo.CollectDuration = 60
	}
	adaptConfigure()
}

func adaptConfigure() {
	MonitorInfo.CollectDuration = MonitorInfo.CollectDuration * time.Second
	MonitorInfo.CollectTimeout = MonitorInfo.CollectDuration + 10*time.Second
	MonitorInfo.SqlHost = MonitorInfo.SqlHost + ":" + MonitorInfo.SqlPort
}

func defaultConfig() {
	MonitorInfo = configure{
		CacheNum:        15,
		MaxTimeoutTimes: 5,
		CollectDuration: 60,

		Hosts:       []string{"localhost"},
		SqlHost:     "localhost",
		SqlPort:     "3306",
		SqlDBName:   "blockchain_db",
		SqlUser:     "root",
		SqlPassword: "123",

		InfluxDB:         "http://localhost:",
		InfluxDBPort:     "8086",
		InfluxDBName:     "docker",
		InfluxDBUser:     "monitor",
		InfluxDBPassword: "iscas123",
	}
	adaptConfigure()
}

func IsKnownHost(host string) bool {
	if MonitorInfo.Hosts == nil {
		panic("please init config first")
	}
	for _, v := range MonitorInfo.Hosts {
		if v == host {
			return true
		}
	}

	return false
}
