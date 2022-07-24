# -=-=-=-=-=-=- Compile Go Image -=-=-=-=-=-=-

FROM golang:1 AS stage-compile

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./... && CGO_ENABLED=0 GOOS=linux go build ./cmd/yt-playlist-ripper

# -=-=-=-=- Final Python Image -=-=-=-=-

FROM python:alpine as stage-final

RUN apk update && \
    apk add --no-cache curl=7.83.1-r2 ffmpeg=5.0.1-r1 && \
    curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && \
    chmod a+rx /usr/local/bin/yt-dlp

COPY --from=stage-compile /go/src/app/yt-playlist-ripper /
CMD ["/yt-playlist-ripper"]