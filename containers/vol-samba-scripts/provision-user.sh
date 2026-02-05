#!/bin/sh
# Usage: sh add_ldap_user.sh <username> <employeeNumber>
# Example: sh add_ldap_user.sh testuser 12345

# Exit on error
set -e

if [ $# -ne 2 ]; then
    echo "Usage: $0 <username> <employeeNumber>"
    exit 1
fi

USERNAME="$1"
EMPLOYEENUMBER="$2"

export LDB_MODULES_PATH=/usr/lib/samba/ldb

# Create the user with a default password
samba-tool user create "$USERNAME" Passw0rd!

# Add the user to the group (adjust 'testgroup' as needed)
samba-tool group addmembers testgroup "$USERNAME"

# Create temporary LDIF file
cat > /tmp/set-employeeNumber.ldif <<LDIF
dn: CN=$USERNAME,CN=Users,DC=example,DC=local
changetype: modify
replace: employeeNumber
employeeNumber: $EMPLOYEENUMBER
-
LDIF

# Apply the modification
ldbmodify -H /var/lib/samba/private/sam.ldb /tmp/set-employeeNumber.ldif