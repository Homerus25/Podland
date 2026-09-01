from node:alpine as builder

WORKDIR /app

COPY package.json package.json
COPY package-lock.json package-lock.json
 
RUN npm install

copy . .

RUN npm run build


FROM caddy:latest

expose 3000
WORKDIR /app

COPY --from=builder /app/dist /app

COPY ./Caddyfile /app

cmd caddy run
