package config

import (
	"log"
	"sync"
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

	sync.RWMutex
}

func (c *configure) GetHostsLen() int {
	c.RLock()
	hostLen := len(c.Hosts)
	c.RUnlock()
	return hostLen
}

func (c *configure) GetHosts() []string {
	c.RLock()
	var hosts []string
	hosts = append(hosts, c.Hosts...)
	c.RUnlock()
	return hosts
}

func (c *configure) AddHost(host string) {
	c.Lock()
	c.Hosts = append(c.Hosts, host)
	c.Unlock()
}

func (c *configure) DeleteHost(host string) bool {
	index := c.DockerHostIndex(host)
	if index == -1 {
		return false
	}
	c.Lock()
	c.Hosts = append(c.Hosts[:index], c.Hosts[index+1:]...)
	c.Unlock()
	return true
}

func (c *configure) DockerHostIndex(host string) int {
	c.RLock()
	defer c.RUnlock()

	if MonitorInfo.Hosts == nil {
		panic("please init config first")
	}
	for index, v := range MonitorInfo.Hosts {
		if v == host {
			return index
		}
	}

	return -1
}

func Load() {
	//global logger
	logger := util.InitLog("global")
	if logger == nil {
		panic("global logger nil")
	}
	MonitorInfo.Logger = logger

	isInContainer, err := util.IsInsideContainer()
	if err != nil {
		// do nothing, ignore error
	}
	defer logConfigure()
	if isInContainer {
		logger.Println("[config] monitor is running inside container")
		defaultConfig()
		return
	}
	loadFromConfigureFile()
}

func logConfigure() {
	startF := "\n******      monitor configure      ******\n"
	timeF := "CacheNum: %d\nMaxTimeoutTimes: %d\nCollectDuration: %v\nCollectTimeout: %v\n"
	hostF := "Hosts: %s\n"
	sqlF := "SqlHost: %s\nSqlDBName: %s\nSqlUser/password: %s\n"
	influxF := "InfluxDB: %s\nInfluxDBName: %s\nInfluxUser/Password: %s"
	endF := "\n*******************************************"
	MonitorInfo.Logger.Printf(startF+timeF+hostF+sqlF+influxF+endF,
		MonitorInfo.CacheNum, MonitorInfo.MaxTimeoutTimes, MonitorInfo.CollectDuration, MonitorInfo.CollectTimeout,
		MonitorInfo.Hosts,
		MonitorInfo.SqlHost, MonitorInfo.SqlDBName, MonitorInfo.SqlUser+"/"+MonitorInfo.SqlPassword,
		MonitorInfo.InfluxDB+MonitorInfo.InfluxDBPort, MonitorInfo.InfluxDBName, MonitorInfo.InfluxDBUser+"/"+MonitorInfo.InfluxDBPassword)
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
	if MonitorInfo.GetHostsLen() == 0 {
		panic("at least one host must given")
	}
	if MonitorInfo.CollectDuration < 30 || MonitorInfo.CollectDuration > 120 {
		MonitorInfo.CollectDuration = 60
	}
	adaptConfigure()
}

func adaptConfigure() {
	MonitorInfo.CollectDuration = MonitorInfo.CollectDuration * time.Second
	MonitorInfo.CollectTimeout = MonitorInfo.CollectDuration * 2
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
		Logger:           MonitorInfo.Logger,
	}
	adaptConfigure()
}
