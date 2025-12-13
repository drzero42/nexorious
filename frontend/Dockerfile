# frontend/Dockerfile
FROM docker.io/node:22-bookworm-slim

WORKDIR /app

# Copy dependency files first (better layer caching)
COPY package.json package-lock.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY . .

# Expose Vite dev server port
EXPOSE 5173

# Run dev server (host 0.0.0.0 to be accessible from outside container)
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
