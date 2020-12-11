
# Copyright 2020 Hewlett Packard Enterprise Development LP
FROM dtr.dev.cray.com/baseos/alpine:3.12

RUN set -ex \
    && apk add --no-cache \
        python3 \
        py3-flask

COPY ./data /app/data
COPY ./pdu.py /app/pdu.py

EXPOSE 8090

CMD ["sh", "-c", "cd /app && ./pdu.py"]