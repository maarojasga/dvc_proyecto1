FROM node:26-alpine AS deps
WORKDIR /app
# El lockfile se copia y se usa: con `npm install` y sin él, la imagen
# resolvía versiones frescas en cada construcción y podía no coincidir con la
# que validaron las pruebas. Importa especialmente desde que Next está fijado
# por seguridad: una resolución libre puede traer otra cosa.
COPY package.json package-lock.json ./
RUN npm ci

FROM node:26-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# Next incrusta NEXT_PUBLIC_API_URL en el bundle al compilar: pasarla al
# arrancar el contenedor no tiene efecto. El valor por defecto es el del
# compose local; deploy/gcp/publicar.sh pasa el origen HTTPS del Web Server.
ARG NEXT_PUBLIC_API_URL=http://localhost:8080
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
RUN npm run build

FROM node:26-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
EXPOSE 3000
CMD ["node", "server.js"]
