FROM golang:1.9.1
MAINTAINER Michael Dungan <mpd@catchandrelease.tv>

WORKDIR /go/src/s3zipper

COPY . .
RUN go-wrapper download && \
    go-wrapper install

EXPOSE 8000

CMD ["go-wrapper", "run"]
