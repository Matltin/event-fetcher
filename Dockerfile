# syntax=docker/dockerfile:1

FROM golang:1.23.5-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./

RUN go env -w GOPROXY=https://goproxy.cn,direct

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o symmioeventsdb ./service.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/symmioeventsdb .
COPY abi ./abi

ENTRYPOINT ["./symmioeventsdb"]