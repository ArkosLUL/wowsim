# syntax=docker/dockerfile:1

FROM node:19.8.1-bullseye-slim AS node

# Dev toolbox, usable alone with `docker build --target toolchain`
FROM golang:1.23-bookworm AS toolchain

RUN apt-get update \
 && apt-get install -y --no-install-recommends protobuf-compiler \
 && rm -rf /var/lib/apt/lists/*

COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

COPY gitconfig /etc/gitconfig
WORKDIR /wotlk

# no version here on purpose: resolves from go.mod so the generated code matches the protobuf runtime
COPY go.mod go.sum ./
RUN go mod download \
 && go install google.golang.org/protobuf/cmd/protoc-gen-go

EXPOSE 8080/tcp

FROM toolchain AS build

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
# .dockerignore drops .git, so the makefile can't read the commit itself:
# docker build --build-arg SIM_COMMIT=$(git rev-parse HEAD) .
ARG SIM_COMMIT
RUN CGO_ENABLED=0 make wowsimwotlk

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /wotlk/wowsimwotlk /wowsimwotlk

EXPOSE 3333/tcp
# the server only exits on SIGINT, and as PID 1 it'd ignore the default SIGTERM
STOPSIGNAL SIGINT
ENTRYPOINT ["/wowsimwotlk", "--launch=false", "--host=:3333"]
