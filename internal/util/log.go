package util

import (
	"log"
	"os"
)

func InitLog(ip string) *log.Logger {
	file, err := os.OpenFile("/var/log/monitor/"+ip+".cmonitor", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Failed to open error log file: %v", err)
		return nil
	}

	//return log.New(bufio.NewWriterSize(file, 128*(len(common.HostIPs)+1)), "", log.Ldate|log.Ltime)
	return log.New(file, "", log.Ldate|log.Ltime)
}
