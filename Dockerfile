FROM alpine

RUN echo http://dl-cdn.alpinelinux.org/alpine/edge/testing >> /etc/apk/repositories \
 && apk update \
 && apk upgrade \
 && apk add curlftpfs

RUN curlftpfs --version || :

# downgrade curl to latest working version for curlftpfs ( https://sourceforge.net/p/curlftpfs/bugs/74/ : 7.78 introduced a breaking change )

RUN apk add build-base libressl-dev && \
    wget https://curl.se/download/curl-7.77.0.tar.gz && \
    tar -xf curl-7.77.0.tar.gz && cd curl-7.77.0 && \
    ./configure --with-ssl && make && make install

RUN curlftpfs --version || :

RUN echo "user_allow_other" >> /etc/fuse.conf

RUN mkdir -p /run/docker/plugins /mnt/state /mnt/volumes

COPY docker-volume-curlftpfs /docker-volume-curlftpfs

CMD ["/docker-volume-curlftpfs"]

