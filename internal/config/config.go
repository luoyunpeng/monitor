package config

import (
	"time"

	"github.com/go-ini/ini"
)

var (
	MonitorInfo *configure
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
	conf := configure{}
	err = cfg.Section("monitor").MapTo(&conf)
	if err != nil {
		panic(err)
	}
	if len(MonitorInfo.Hosts) == 0 {
		panic("at least one host must given")
	}
	if conf.CollectDuration < 30 || conf.CollectDuration > 120 {
		conf.CollectDuration = 60
	}
	conf.CollectDuration = conf.CollectDuration * time.Second
	conf.CollectTimeout = conf.CollectDuration + 10*time.Second
	conf.SqlHost = conf.SqlHost + ":" + conf.SqlPort

	MonitorInfo = &conf
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
