<?php
$config = array();
$config['db_dsnw'] = 'sqlite:////var/www/db/sqlite.db';
$config['default_host'] = 'localhost';
$config['smtp_server'] = 'localhost';
$config['smtp_port'] = 25;
$config['smtp_user'] = '';
$config['smtp_pass'] = '';
$config['support_url'] = '';
$config['product_name'] = '';
$config['des_key'] = '9ff0f804264c25e20e794df8dd9ab63002d85aebd339f6bde2c4406b98bd0689';
$config['plugins'] = array(
    'archive',
    'zipdownload',
);

$config['skin'] = 'elastic';
$config['disabled_actions'] = array('addressbook.index','mail.compose','mail.reply','mail.reply-all','mail.forward');

$config['drafts_mbox'] = '';
$config['junk_mbox'] = '';
$config['sent_mbox'] = '';
$config['trash_mbox'] = '';

$config['protect_default_folders'] = true;
