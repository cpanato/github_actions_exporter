FROM ubuntu:latest

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates

RUN update-ca-certificates

COPY github-actions-exporter /github-actions-exporter

USER nobody
ENTRYPOINT ["/github-actions-exporter"]
EXPOSE     9101
