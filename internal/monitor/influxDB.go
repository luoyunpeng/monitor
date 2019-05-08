package monitor

import (
	"log"
	"time"

	"github.com/influxdata/influxdb/client/v2"
	"github.com/luoyunpeng/monitor/internal/config"
)

var (
	influCli   client.Client
	MetricChan chan Metric
	influxDB   string
)

// Metric is basic unit that insert into influxDB
type Metric struct {
	Measurement string
	Tags        map[string]string
	Fields      map[string]interface{}
	ReadTime    time.Time
}

func initInfluxCli() {
	var err error
	conf := config.MonitorInfo
	f := "init influxCli from addr-%s, db: %s ,user: %s, password: %s"
	log.Printf(f, conf.InfluxDB+conf.InfluxDBPort, conf.InfluxDBName, conf.InfluxDBUser, conf.InfluxDBPassword)
	influCli, err = client.NewHTTPClient(client.HTTPConfig{
		Addr:     conf.InfluxDB + conf.InfluxDBPort,
		Username: conf.InfluxDBUser,
		Password: conf.InfluxDBPassword,
	})
	if err != nil {
		log.Fatal(err)
	}
	influxDB = conf.InfluxDBName
	MetricChan = make(chan Metric, 8)
}

// Write giving tag kv and fields kv to influxDB
func Write() {
	initInfluxCli()
	defer influCli.Close()

	for m := range MetricChan {
		internalWrite(m)
	}
}

func internalWrite(m Metric) {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  influxDB,
		Precision: "s",
	})
	if err != nil {
		log.Printf("err happen when new  batch points: %v", err)
	}

	pt, err := client.NewPoint(m.Measurement, m.Tags, m.Fields, m.ReadTime)
	if err != nil {
		log.Printf("err happen when new point: %v", err)
	}
	bp.AddPoint(pt)

	// Write the batch
	if err := influCli.Write(bp); err != nil {
		log.Printf("err happen when write the batch point: %v", err)
	}
}
