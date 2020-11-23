ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox:latest

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/github_actions_exporter /bin/github_actions_exporter

USER nobody
ENTRYPOINT ["/bin/github_actions_exporter"]
EXPOSE     9101
