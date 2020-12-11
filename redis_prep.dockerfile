# Copyright 2020 Cray Inc. All Rights Reserved.

FROM dtr.dev.cray.com/baseos/alpine:3.12
LABEL maintainer="Cray, Inc."

# Redis
ENV REDIS_HOSTNAME localhost
ENV REDIS_PORT 6379

RUN set -ex \
    && apk update \
    && apk add \
        redis \
        curl

COPY scripts/wait-for.sh /
COPY docker-entrypoint.sh /
COPY scripts /scripts
COPY .version /

ENTRYPOINT ["/docker-entrypoint.sh"]

CMD ["setup_redis"]
