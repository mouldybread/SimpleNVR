FROM golang:1.23-alpine

# Install FFmpeg (Alpine package)
RUN apk add --no-cache ffmpeg

# Install Air
RUN go install github.com/air-verse/air@v1.61.0

WORKDIR /app

CMD ["air", "-c", ".air.toml"]
