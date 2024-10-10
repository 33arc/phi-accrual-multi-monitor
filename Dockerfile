FROM golang:1.23.2-alpine3.20 AS builder

RUN apk update && apk add curl git
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

ADD ./ /go/src/github.com/33arc/phi-accrual-multi-monitor

WORKDIR /go/src/github.com/33arc/phi-accrual-multi-monitor/

RUN go build -o /tmp/phi ./*.go


FROM golang:1.23.2-alpine3.20
COPY --from=builder /tmp/phi /app/
COPY --from=builder /go/src/github.com/33arc/phi-accrual-multi-monitor/servers.yml /app/
WORKDIR /app
# CMD ["./phi", "--config", "./servers.yml"]
