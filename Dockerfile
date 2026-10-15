FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.21
RUN addgroup -S app && adduser -S app -G app && mkdir /data && chown app:app /data
USER app
COPY --from=builder /server /usr/local/bin/zzzmessage
ENV DATA_DIR=/data LISTEN_ADDR=:8080
EXPOSE 8080
CMD ["zzzmessage"]
