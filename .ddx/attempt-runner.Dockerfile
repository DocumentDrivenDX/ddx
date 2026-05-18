ARG DDX_BASE_IMAGE=ddx-attempt-runner:dev
FROM ${DDX_BASE_IMAGE}

WORKDIR /opt/ddx-project

# Cache DDx's Go module dependencies in the project image. Runtime attempts
# mount the working clone at /work, so cached setup belongs outside /work.
COPY cli/go.mod cli/go.sum ./cli/
RUN cd cli && go mod download
