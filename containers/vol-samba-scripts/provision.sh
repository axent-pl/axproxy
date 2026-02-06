export LDB_MODULES_PATH=/usr/lib/samba/ldb

# user account
samba-tool user create testuser Passw0rd!      
samba-tool group add testgroup                 
samba-tool group addmembers testgroup testuser 
samba-tool group add testgroup2                
samba-tool group addmembers testgroup2 testuser

# kerberos application account
samba-tool user create app_svc 'Passw0rd!'
samba-tool user setexpiry app_svc --noexpiry
samba-tool spn add HTTP/app_svc app_svc
samba-tool spn add HTTP/app_svc.example.local app_svc

# proxy service account
samba-tool user create proxy_svc 'Passw0rd!'
samba-tool user setexpiry proxy_svc --noexpiry
samba-tool spn add HTTP/proxy_svc proxy_svc
samba-tool spn add HTTP/proxy_svc.example.local proxy_svc

# delegation
samba-tool delegation for-any-protocol proxy_svc on
samba-tool delegation add-service proxy_svc HTTP/app_svc
samba-tool delegation add-service proxy_svc HTTP/app_svc.${DOMAIN}

# keytab generation
rm /keytabs/*
samba-tool domain exportkeytab /keytabs/app_svc.0.keytab --principal=app_svc@${REALM}
samba-tool domain exportkeytab /keytabs/app_svc.1.keytab --principal=HTTP/app_svc@${REALM}
samba-tool domain exportkeytab /keytabs/app_svc.2.keytab --principal=HTTP/app_svc.${DOMAIN}@${REALM}
samba-tool domain exportkeytab /keytabs/proxy_svc.0.keytab --principal=proxy_svc@${REALM}
samba-tool domain exportkeytab /keytabs/proxy_svc.1.keytab --principal=HTTP/proxy_svc.${DOMAIN}@${REALM}
samba-tool domain exportkeytab /keytabs/proxy_svc.2.keytab --principal=HTTP/proxy_svc@${REALM}

ktutil <<EOF
rkt /keytabs/app_svc.0.keytab
rkt /keytabs/app_svc.1.keytab
rkt /keytabs/app_svc.2.keytab
wkt /keytabs/app_svc.keytab
quit
EOF

ktutil <<EOF
rkt /keytabs/proxy_svc.0.keytab
rkt /keytabs/proxy_svc.1.keytab
rkt /keytabs/proxy_svc.2.keytab
wkt /keytabs/proxy_svc.keytab
quit
EOF

# user employeeNumber attribute (links accounts)
cat > /tmp/set-employeeNumber.ldif <<LDIF
dn: CN=testuser,CN=Users,DC=example,DC=local
changetype: modify
replace: employeeNumber
employeeNumber: demo
-
LDIF

ldbmodify -H /var/lib/samba/private/sam.ldb /tmp/set-employeeNumber.ldif
