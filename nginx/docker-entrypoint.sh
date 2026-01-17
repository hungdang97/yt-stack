#!/bin/sh
set -e

# Check if SSL certificate exists
if [ ! -f "/etc/letsencrypt/live/${DOMAIN}/fullchain.pem" ]; then
    echo "SSL certificate not found. Obtaining new certificate..."

    # Use HTTP-only config for initial cert request
    echo "Starting nginx with HTTP-only config for ACME challenge..."
    envsubst '${DOMAIN}' < /etc/nginx/nginx-http-only.conf > /etc/nginx/nginx.conf

    # Start nginx in background
    nginx

    # Wait for nginx to be ready
    sleep 3

    # Obtain certificate
    echo "Requesting SSL certificate from Let's Encrypt..."
    certbot certonly --webroot \
        -w /var/www/certbot \
        -d ${DOMAIN} \
        --email ${EMAIL} \
        --agree-tos \
        --non-interactive \
        --keep-until-expiring

    # Stop temporary nginx
    echo "Certificate obtained. Stopping temporary nginx..."
    nginx -s stop
    sleep 2
fi

# Use full config with SSL
echo "Loading full nginx config with SSL..."
envsubst '${DOMAIN}' < /etc/nginx/nginx.conf.template > /etc/nginx/nginx.conf

# Test nginx config
nginx -t

# Setup auto-renewal cron job (runs daily at 2am)
echo "0 2 * * * certbot renew --nginx --quiet && nginx -s reload" > /etc/crontabs/root
crond

# Execute CMD (start nginx)
echo "Starting nginx with SSL enabled..."
exec "$@"
