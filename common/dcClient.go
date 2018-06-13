package common

import (
	"github.com/docker/docker/client"
	"log"
	"runtime"
)

func InitClient(hostURL string) (*client.Client, error) {
	var (
		err error
		cli *client.Client
	)
	if runtime.GOOS == "windows" {
		log.Println("[ monitor ]  init docker client from given host")
		if len(hostURL) == 0 {
			log.Fatal("on windows env hostURl must given")
		}
		cli, err = client.NewClientWithOpts(client.WithHost(hostURL))
	} else {
		cli, err = client.NewClientWithOpts(client.FromEnv)
		log.Println("[ monitor ]  init docker client from env")
	}

	if err != nil {
		return nil, err
	}
	return cli, nil
}
