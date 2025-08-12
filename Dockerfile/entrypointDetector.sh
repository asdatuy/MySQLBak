#!/bin/sh
nc -zvw 5 ${rHost} ${rPort}

if [[ $? != 0 ]]; then
    echo "Remoute unreachable!  =-("
    exit 1
fi

echo "Remoute reachable! :-)"
echo -e "host: ${rHost}\nport: ${rPort}\ndb: ${rDBName}" > /mysqldesc/file
exit 0
