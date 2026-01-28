#!/bin/sh
set -e

# Check if SSL certificates exist for both domains
MAIN_CERT_EXISTS=false
EXTRACTOR_CERT_EXISTS=false

if [ -f "/etc/letsencrypt/live/${DOWNLOAD_DOMAIN}/fullchain.pem" ]; then
    MAIN_CERT_EXISTS=true
fi

if [ -f "/etc/letsencrypt/live/${EXTRACTOR_DOMAIN}/fullchain.pem" ]; then
    EXTRACTOR_CERT_EXISTS=true
fi

# If either certificate is missing, obtain them
if [ "$MAIN_CERT_EXISTS" = false ] || [ "$EXTRACTOR_CERT_EXISTS" = false ]; then
    echo "SSL certificates missing. Obtaining certificates..."

    # Use HTTP-only config for initial cert request
    echo "Starting nginx with HTTP-only config for ACME challenge..."
    envsubst '${DOWNLOAD_DOMAIN} ${EXTRACTOR_DOMAIN}' < /etc/nginx/nginx-http-only.conf > /etc/nginx/nginx.conf

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

    # Obtain certificate for extractor domain if missing
    if [ "$EXTRACTOR_CERT_EXISTS" = false ]; then
        echo "Requesting SSL certificate for ${EXTRACTOR_DOMAIN}..."
        certbot certonly --webroot \
            -w /var/www/certbot \
            -d ${EXTRACTOR_DOMAIN} \
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
envsubst '${DOWNLOAD_DOMAIN} ${EXTRACTOR_DOMAIN}' < /etc/nginx/nginx.conf.template > /etc/nginx/nginx.conf

# Test nginx config
nginx -t

# Setup auto-renewal cron job (runs daily at 2am)
echo "0 2 * * * certbot renew --nginx --quiet && nginx -s reload" > /etc/crontabs/root
crond

# Execute CMD (start nginx)
echo "Starting nginx with SSL enabled..."
exec "$@"
