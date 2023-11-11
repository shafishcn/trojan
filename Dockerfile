FROM alpine:3.11

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.tuna.tsinghua.edu.cn/g' /etc/apk/repositories
COPY . trojan
RUN apk add --no-cache --virtual .build-deps \
        build-base \
        cmake \
        boost-dev \
        openssl-dev \
        mariadb-connector-c-dev \
    && (cd trojan && cmake . && make -j $(nproc) && strip -s trojan \
    && mv trojan /usr/local/bin) \
    && rm -rf trojan \
    && apk del .build-deps \
    && apk add --no-cache --virtual .trojan-rundeps \
        libstdc++ \
        boost-system \
        boost-program_options \
        mariadb-connector-c \
    && apk add --no-cache privoxy jq

WORKDIR /config

ADD config.json .
ADD entry.sh .
ADD privoxy.config /etc/privoxy/config

CMD ["/config/entry.sh"]
