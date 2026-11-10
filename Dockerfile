FROM golang:1.26-alpine AS builder
WORKDIR /app
ENV GOFLAGS=-mod=readonly
COPY go.mod go.sum ./
RUN sha256sum go.mod go.sum > /tmp/dependencies.sha256 && go mod download && go mod verify && sha256sum -c /tmp/dependencies.sha256
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o simplenvr . && sha256sum -c /tmp/dependencies.sha256

FROM alpine:latest
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=builder /app/simplenvr /app/simplenvr
COPY --from=builder /app/templates /app/templates
COPY --from=builder /app/static /app/static
EXPOSE 443
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/simplenvr"]
