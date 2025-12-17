# frontend-next/Dockerfile
FROM docker.io/node:22-bookworm-slim

WORKDIR /app

# Copy dependency files first (better layer caching)
COPY package.json package-lock.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY . .

# Expose Next.js dev server port
EXPOSE 3000

# Run dev server (turbopack for faster rebuilds)
CMD ["npm", "run", "dev"]
