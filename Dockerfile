FROM golang:1.22-alpine AS build

WORKDIR /app

RUN go install github.com/a-h/templ/cmd/templ@v0.2.793

COPY go.mod go.sum ./
RUN go mod download

COPY *.go *.templ ./
RUN templ generate && \
    go build -o nano-analytics


FROM alpine:3.19 AS run

WORKDIR /app

RUN apk add --no-cache gosu --repository https://dl-cdn.alpinelinux.org/alpine/edge/testing/

COPY *.mmdb .
COPY --from=build /app/nano-analytics .

VOLUME /app/database/
EXPOSE 1323
ENTRYPOINT ./nano-analytics
