#!/bin/bash
echo "Note: IP value is not given, please consider your local test env"
cd $GOPATH/src/github.com/luoyunpeng/monitor
go build -tags=jsonniter monitor.go
