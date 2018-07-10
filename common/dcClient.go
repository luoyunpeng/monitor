package common

import (
	"github.com/docker/docker/client"
	"log"
	"strings"
)

var (
	hostURL = "tcp://ip:2375"
	//run 'docker version' at target host to get the version
	defaultVersion = "1.37"
)

func InitClient(ip string) (*client.Client, error) {
	var (
		err error
		cli *client.Client
	)
	if ip == "localhost" {
		cli, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion(defaultVersion))
		log.Println("[ monitor ]  init docker client from env for localhost")
	} else {
		realURL := strings.Replace(hostURL, "ip", ip, 1)
		cli, err = client.NewClientWithOpts(client.WithHost(realURL), client.WithVersion(defaultVersion))
		log.Println("[ monitor ]  init docker client from remote ip: ", ip)
	}

	if err != nil {
		return nil, err
	}
	return cli, nil
}
