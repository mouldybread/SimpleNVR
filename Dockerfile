FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && go mod verify
RUN CGO_ENABLED=0 go build -trimpath -o simplenvr .

FROM alpine:latest
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=builder /app/simplenvr /app/simplenvr
COPY --from=builder /app/templates /app/templates
COPY --from=builder /app/static /app/static
EXPOSE 443
ENTRYPOINT ["/app/simplenvr"]
