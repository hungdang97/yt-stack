#!/bin/sh
set -e

# Replace environment variables in nginx config
envsubst '${DOMAIN}' < /etc/nginx/nginx.conf.template > /etc/nginx/nginx.conf

# Check if SSL certificate exists
if [ ! -f "/etc/letsencrypt/live/${DOMAIN}/fullchain.pem" ]; then
    echo "SSL certificate not found. Obtaining new certificate..."

    # Start nginx temporarily for ACME challenge
    nginx

    # Obtain certificate
    certbot certonly --webroot \
        -w /var/www/certbot \
        -d ${DOMAIN} \
        --email ${EMAIL} \
        --agree-tos \
        --non-interactive \
        --keep-until-expiring

    # Stop temporary nginx
    nginx -s stop

    # Reload config with SSL enabled
    envsubst '${DOMAIN}' < /etc/nginx/nginx.conf.template > /etc/nginx/nginx.conf
fi

# Setup auto-renewal cron job (runs daily at 2am)
echo "0 2 * * * certbot renew --nginx --quiet && nginx -s reload" > /etc/crontabs/root
crond

# Execute CMD (start nginx)
exec "$@"
