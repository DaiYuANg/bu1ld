# syntax=docker/dockerfile:1.7

FROM node:24-alpine AS build

WORKDIR /src/plugins/node
COPY plugins/node/package.json plugins/node/package-lock.json ./

RUN --mount=type=cache,target=/root/.npm \
    npm ci

COPY plugins/node/tsconfig.json ./tsconfig.json
COPY plugins/node/src ./src
COPY plugins/node/plugin.toml ./plugin.toml

RUN npm run build && \
    npm prune --omit=dev && \
    chmod +x dist/main.js

FROM node:24-alpine

RUN apk add --no-cache ca-certificates git openssh-client tzdata && \
    corepack enable

WORKDIR /opt/bu1ld-node-plugin
COPY --from=build /src/plugins/node/package.json ./package.json
COPY --from=build /src/plugins/node/package-lock.json ./package-lock.json
COPY --from=build /src/plugins/node/node_modules ./node_modules
COPY --from=build /src/plugins/node/dist ./dist
COPY --from=build /src/plugins/node/plugin.toml ./plugin.toml

WORKDIR /workspace
ENTRYPOINT ["node", "/opt/bu1ld-node-plugin/dist/main.js"]
