FROM golang:latest

EXPOSE 5000

WORKDIR /go/src/github.com/TranquilityApp/redis-hub
ADD . /go/src/github.com/TranquilityApp/redis-hub

RUN go get -v
RUN go build
ENV PORT=5000
ENV ENV=production
ENTRYPOINT ["./redis-hub"]
