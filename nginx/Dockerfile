FROM nginx:alpine

# Install certbot for SSL
RUN apk add --no-cache certbot certbot-nginx gettext

# Copy nginx configuration templates
COPY nginx.conf /etc/nginx/nginx.conf.template
COPY nginx-http-only.conf /etc/nginx/nginx-http-only.conf

# Create webroot for ACME challenge
RUN mkdir -p /var/www/certbot

# Copy entrypoint script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 80 443

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
