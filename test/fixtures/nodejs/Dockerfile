# Build context must be the repo root:
#   docker build -f test/fixtures/nodejs/Dockerfile -t coverport-testapp-nodejs .
FROM node:20-slim

WORKDIR /app

COPY test/fixtures/nodejs/package.json .
RUN npm install --production

COPY instrumentation/nodejs/coverage_server.js .

COPY test/fixtures/nodejs/app.js .

EXPOSE 8080 53700

CMD ["node", "/app/coverage_server.js", "/app/app.js"]
