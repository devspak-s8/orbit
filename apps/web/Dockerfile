# ORBIT web app: build the static bundle, serve it with nginx, and proxy
# the API behind the same origin. Build from the repository root:
#   docker compose build web
FROM node:22-alpine AS build
WORKDIR /src

COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci

COPY apps/web/ ./
RUN npm run build

FROM nginx:1.27-alpine
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 80