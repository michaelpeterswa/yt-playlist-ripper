# -=-=-=-=-=-=- Compile Go Image -=-=-=-=-=-=-

FROM golang:1.25 AS stage-compile

WORKDIR /go/src/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build ./cmd/yt-playlist-ripper

# -=-=-=-=- Bun Runtime Image -=-=-=-=-

FROM oven/bun:1.3.14-alpine AS stage-bun

# -=-=-=-=- Final Python Alpine Image -=-=-=-=-

FROM python:3.13-alpine AS stage-final

COPY --from=stage-bun /usr/local/bin/bun /usr/local/bin/bun

# hadolint ignore=DL3018
RUN apk update && \
    apk add --no-cache curl ffmpeg && \
    curl -L https://github.com/yt-dlp/yt-dlp/releases/download/2026.03.17/yt-dlp -o /usr/local/bin/yt-dlp && \
    chmod a+rx /usr/local/bin/yt-dlp

COPY --from=stage-compile /go/src/app/yt-playlist-ripper /
CMD ["/yt-playlist-ripper"]