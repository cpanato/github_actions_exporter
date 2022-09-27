FROM cgr.dev/chainguard/static:latest

COPY github-actions-exporter /bin/github-actions-exporter

USER nobody
ENTRYPOINT ["/bin/github-actions-exporter"]
EXPOSE     9101
