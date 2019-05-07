package conf

import (
	"time"

	"github.com/go-ini/ini"
)

var (
	C *Config
)

type Config struct {
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

func NewConfig() (*Config, error) {
	cfg, err := ini.Load("monitor.ini")
	if err != nil {
		return nil, err
	}
	conf := Config{}
	err = cfg.Section("monitor").MapTo(&conf)
	if err != nil {
		return nil, err
	}
	if conf.CollectDuration < 30 || conf.CollectDuration > 120 {
		conf.CollectDuration = 60
	}
	conf.CollectDuration = conf.CollectDuration * time.Second
	conf.CollectTimeout = conf.CollectDuration + 10*time.Second
	conf.SqlHost = conf.SqlHost + ":" + conf.SqlPort

	C = &conf
	return &conf, nil
}

func (c *Config) GetCacheNum() int {
	return c.CacheNum
}

func IsKnownHost(host string) bool {
	if C.Hosts == nil {
		panic("please init config first")
	}
	for _, v := range C.Hosts {
		if v == host {
			return true
		}
	}

	return false
}
