#!/bin/sh
set -e

# Check if SSL certificates exist
MAIN_CERT_EXISTS=false

if [ -f "/etc/letsencrypt/live/${DOWNLOAD_DOMAIN}/fullchain.pem" ]; then
    MAIN_CERT_EXISTS=true
fi

# If certificate is missing, obtain it
if [ "$MAIN_CERT_EXISTS" = false ]; then
    echo "SSL certificates missing. Obtaining certificates..."

    # Use HTTP-only config for initial cert request
    echo "Starting nginx with HTTP-only config for ACME challenge..."
    envsubst '${DOWNLOAD_DOMAIN}' < /etc/nginx/nginx-http-only.conf > /etc/nginx/nginx.conf

    # Start nginx in background
    nginx

    # Wait for nginx to be ready
    sleep 3

    # Obtain certificate for main domain if missing
    if [ "$MAIN_CERT_EXISTS" = false ]; then
        echo "Requesting SSL certificate for ${DOWNLOAD_DOMAIN}..."
        certbot certonly --webroot \
            -w /var/www/certbot \
            -d ${DOWNLOAD_DOMAIN} \
            --email ${EMAIL} \
            --agree-tos \
            --non-interactive \
            --keep-until-expiring
    fi



    # Stop temporary nginx
    echo "Certificates obtained. Stopping temporary nginx..."
    nginx -s stop
    sleep 2
fi

# Use full config with SSL
echo "Loading full nginx config with SSL..."
envsubst '${DOWNLOAD_DOMAIN}' < /etc/nginx/nginx.conf.template > /etc/nginx/nginx.conf

# Test nginx config
nginx -t

# Setup auto-renewal cron job (runs daily at 2am)
echo "0 2 * * * certbot renew --nginx --quiet && nginx -s reload" > /etc/crontabs/root
crond

# Execute CMD (start nginx)
echo "Starting nginx with SSL enabled..."
exec "$@"
