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

COPY *.mmdb ./
COPY --from=build /app/nano-analytics .

EXPOSE 1323
ENTRYPOINT [ "./nano-analytics" ]
