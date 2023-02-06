FROM cgr.dev/chainguard/static:latest

COPY --chown=nobody github-actions-exporter /github-actions-exporter

USER nobody
ENTRYPOINT ["/github-actions-exporter"]
EXPOSE     9101
