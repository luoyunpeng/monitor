# docker monitor

This Project is based on golang docker engine api, also refer to some docker command line source code,
and offer container memory, CPU, networkIO  blockIO metric rest api with gin and real time log of running 
container with websocket

## Build a linux binary

```sh
go build -tags=jsoniter -mod=vendor
```