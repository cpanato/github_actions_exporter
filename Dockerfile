FROM cgr.dev/chainguard/static:latest

COPY github-actions-exporter /github-actions-exporter

USER nobody
ENTRYPOINT ["/github-actions-exporter"]
EXPOSE     9101
