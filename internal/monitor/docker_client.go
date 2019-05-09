package monitor

import (
	"context"
	"log"
	"strings"

	"github.com/docker/docker/client"
)

var (
	hostURL        = "tcp://ip:2375"
	defaultVersion = "1.38"
)

//InitClient init a docker client from give ip, default port is 2375
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

	if _, err = cli.Ping(context.Background()); err != nil {
		return nil, err
	}
	return cli, nil
}