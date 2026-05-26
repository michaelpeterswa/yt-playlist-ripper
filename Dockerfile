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

# Install yt-dlp via pip rather than the standalone "yt-dlp" zipimport
# binary so we can pull the curl-cffi impersonation extra alongside it.
# Per the yt-dlp README the Unix zipimport build is the one variant that
# ships *without* curl-cffi, which surfaces as the
# "no impersonate target is available" warning on every run.
# hadolint ignore=DL3018
RUN apk update && \
    apk add --no-cache ffmpeg && \
    pip install --no-cache-dir --break-system-packages \
        "yt-dlp[default,curl-cffi]==2026.3.17"

COPY --from=stage-compile /go/src/app/yt-playlist-ripper /
CMD ["/yt-playlist-ripper"]