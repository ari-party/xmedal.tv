# Build layer
FROM node:24-alpine AS build

RUN corepack enable && corepack prepare pnpm@10 --activate

COPY . /build
WORKDIR /build

COPY package.json ./
COPY pnpm-lock.yaml ./

RUN pnpm fetch --frozen-lockfile
RUN pnpm install --frozen-lockfile

RUN pnpm run build

# Package layer
FROM imbios/bun-node:24-alpine AS package

RUN apk --no-cache add curl

WORKDIR /app

COPY --from=build /build/dist dist
COPY --from=build /build/node_modules node_modules
COPY --from=build /build/package.json package.json

CMD ["bun", "run", "dist/index.js"]

EXPOSE 3000
