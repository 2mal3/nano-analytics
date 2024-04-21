FROM golang:1.21-alpine AS build

WORKDIR /app

RUN go install github.com/a-h/templ/cmd/templ@latest

COPY go.mod go.sum ./
RUN go mod download

COPY *.go *.templ ./
RUN templ generate && \
    go build -o nano-analytics


FROM alpine:3.19 AS run

WORKDIR /app

RUN addgroup nonroot && \
    adduser --system -G nonroot --disabled-password nonroot && \
    apk add --no-cache gosu --repository https://dl-cdn.alpinelinux.org/alpine/edge/testing/

COPY *.mmdb docker-entrypoint.sh ./
RUN chmod +x docker-entrypoint.sh
COPY --from=build /app/nano-analytics .

VOLUME /app/database/
EXPOSE 1323
ENTRYPOINT ./docker-entrypoint.sh
