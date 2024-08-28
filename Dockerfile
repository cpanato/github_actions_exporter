FROM ubuntu:latest

# ADD ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY github-actions-exporter /github-actions-exporter

USER nobody
ENTRYPOINT ["/github-actions-exporter"]
EXPOSE     9101
