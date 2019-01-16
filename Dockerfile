FROM alpine:latest

MAINTAINER luoyunpeng
RUN mkdir /var/log/monitor
COPY monitor /usr/local/bin/

EXPOSE 8080
EXPOSE 8070

ENTRYPOINT ["monitor"]
