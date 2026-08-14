FROM oven/bun:alpine AS build
WORKDIR /src

COPY contract/gen/ts/ /contract/gen/ts/
COPY contract/ts/ /contract/ts/

COPY web/package.json web/package-lock.json ./
RUN bun install
# The contract's TypeScript imports @connectrpc/connect, @bufbuild/protobuf,
# etc. Symlink so Vite can resolve them from /contract/ via /src/node_modules.
RUN ln -s /src/node_modules /contract/node_modules
COPY web/ ./

ARG VERSION=dev
ARG BASE_PATH=
ENV BASE_PATH=${BASE_PATH}
ENV APP_VERSION=${VERSION}

RUN bun run build

FROM oven/bun:alpine
COPY --from=build /src/build /app
EXPOSE 3000
ENV PORT=3000
CMD ["bun", "run", "/app/index.js"]
