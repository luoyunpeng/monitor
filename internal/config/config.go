package config

import (
	"time"

	"github.com/go-ini/ini"
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
}

func Load() {
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
	MonitorInfo.CollectDuration = MonitorInfo.CollectDuration * time.Second
	MonitorInfo.CollectTimeout = MonitorInfo.CollectDuration + 10*time.Second
	MonitorInfo.SqlHost = MonitorInfo.SqlHost + ":" + MonitorInfo.SqlPort
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
