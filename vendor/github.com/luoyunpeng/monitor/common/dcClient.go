package common

import (
	"github.com/docker/docker/client"
	"log"
	"strings"
)

var (
	hostURL = "tcp://ip:2375"
)

func InitClient(ip string) (*client.Client, error) {
	var (
		err error
		cli *client.Client
	)
	if ip == "localhost" {
		cli, err = client.NewClientWithOpts(client.FromEnv)
		log.Println("[ monitor ]  init docker client from env for localhost")
	} else {
		cli, err = client.NewClientWithOpts(client.WithHost(strings.Replace(hostURL, "ip", ip, 1)))
		log.Println("[ monitor ]  init docker client from remote ip: ", ip)
	}

	if err != nil {
		return nil, err
	}
	return cli, nil
}
