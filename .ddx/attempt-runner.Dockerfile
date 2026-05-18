ARG DDX_BASE_IMAGE=ddx-attempt-runner:dev
FROM ${DDX_BASE_IMAGE}

WORKDIR /opt/ddx-project

COPY cli/go.mod cli/go.sum ./cli/
RUN cd cli && go mod download
