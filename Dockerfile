FROM golang:1.10-alpine as builder
COPY . /go/src/github.com/G-eos/docker-volume-curlftpfs
WORKDIR /go/src/github.com/G-eos/docker-volume-curlftpfs
RUN set -ex \
    && apk add --no-cache --virtual .build-deps \
    gcc libc-dev \
    && go install --ldflags '-extldflags "-static"' \
    && apk del .build-deps
CMD ["/go/bin/docker-volume-curlftpfs"]

FROM debian:stable-slim
RUN apt-get update \
    && apt-get install -y ca-certificates curlftpfs \
    && rm -rf /var/lib/apt/lists/*
RUN echo "user_allow_other" >> /etc/fuse.conf
RUN mkdir -p /run/docker/plugins /mnt/state /mnt/volumes
COPY --from=builder /go/bin/docker-volume-curlftpfs .
CMD ["docker-volume-curlftpfs"]
