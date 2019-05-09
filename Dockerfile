# build from golang-alpine
FROM alpine:latest

MAINTAINER luoyunpeng
RUN mkdir /var/log/monitor
COPY monitor /usr/local/bin/

EXPOSE 8080 8070

ENTRYPOINT ["monitor"]
