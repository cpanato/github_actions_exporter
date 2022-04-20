FROM ghcr.io/distroless/busybox:latest

COPY github-actions-exporter /bin/github-actions-exporter

USER nobody
ENTRYPOINT ["/bin/github-actions-exporter"]
EXPOSE     9101
