# docker monitor
[![Build Status](https://travis-ci.org/luoyunpeng/monitor.svg?branch=master)](https://travis-ci.org/luoyunpeng/monitor)
This Project is based on golang docker engine api, also refer to some docker command line source code,
####

1, offer rest api for select metrics like container memory, CPU, networkIO  blockIO  with gin.
####
2, real time log of running container with websocket.

## Build a linux binary

```sh
go build -tags=jsoniter -o monitor ./cmd/monitor/monitor.go
```
