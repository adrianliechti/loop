FROM debian:buster-slim

ENV DEBIAN_FRONTEND=noninteractive

ENV USERNAME mailtrap
ENV PASSWORD mailtrap

ENV ROUNDCUBE_VERSION="1.4.11"

RUN apt-get update && apt-get install --no-install-recommends -y \
    tini \
    wget \
    ca-certificates \
    sysvinit-utils \
 && rm -rf /var/lib/apt/lists/*

RUN apt-get update && apt-get install --no-install-recommends -y \
    apache2 \
    postfix \
    dovecot-imapd \
    sqlite \
    php \
    php-mbstring \
    php-sqlite3 \
    php-pear \
    rsyslog \
 && rm -rf /var/lib/apt/lists/*

RUN a2ensite 000-default \
 && a2enmod expires \
 && a2enmod headers \
 && pear channel-update pear.php.net \
 && pear install mail_mime mail_mimedecode net_smtp net_idna2-beta Auth_SASL Horde_ManageSieve crypt_gpg

COPY postfix/* /etc/postfix/
COPY dovecot/* /etc/dovecot/conf.d/
COPY apache2/* /etc/apache2/sites-available/

RUN rm -rf /var/www \
 && wget https://github.com/roundcube/roundcubemail/releases/download/$ROUNDCUBE_VERSION/roundcubemail-$ROUNDCUBE_VERSION-complete.tar.gz -O roundcube.tar.gz \
 && tar -zxf roundcube.tar.gz \
 && mv roundcubemail-$ROUNDCUBE_VERSION /var/www \
 && rm -rf /var/www/installer \
 && mkdir /var/www/db \
 && . /etc/apache2/envvars \
 && chown -R ${APACHE_RUN_USER}:${APACHE_RUN_GROUP} /var/www/temp /var/www/logs /var/www/db \
 && chmod 777 -R /var/mail

COPY mailtrap /usr/local/bin/
COPY roundcube/* /var/www/config/

EXPOSE 25 80

ENTRYPOINT ["tini", "--"]
CMD [ "mailtrap" ]
