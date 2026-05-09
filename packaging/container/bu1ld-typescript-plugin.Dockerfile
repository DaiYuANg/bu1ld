# syntax=docker/dockerfile:1.7

FROM node:24-alpine AS build

WORKDIR /src/plugins/typescript
COPY plugins/typescript/package.json plugins/typescript/package-lock.json ./

RUN --mount=type=cache,target=/root/.npm \
    npm ci

COPY plugins/typescript/tsconfig.json ./tsconfig.json
COPY plugins/typescript/src ./src
COPY plugins/typescript/plugin.toml ./plugin.toml

RUN npm run build && \
    npm prune --omit=dev && \
    chmod +x dist/main.js

FROM node:24-alpine

RUN apk add --no-cache ca-certificates git openssh-client tzdata

WORKDIR /opt/bu1ld-typescript-plugin
COPY --from=build /src/plugins/typescript/package.json ./package.json
COPY --from=build /src/plugins/typescript/package-lock.json ./package-lock.json
COPY --from=build /src/plugins/typescript/node_modules ./node_modules
COPY --from=build /src/plugins/typescript/dist ./dist
COPY --from=build /src/plugins/typescript/plugin.toml ./plugin.toml

WORKDIR /workspace
ENTRYPOINT ["node", "/opt/bu1ld-typescript-plugin/dist/main.js"]
