FROM alpine

RUN apk add --no-cache curl jq coreutils

COPY ./data-flow-test-script.sh /data-flow-test-script.sh

RUN chmod +x /data-flow-test-script.sh

ENTRYPOINT ["/data-flow-test-script.sh"]