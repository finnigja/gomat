#!/bin/bash

# setup ca & user (puts CA bits in ~/.gomat)
./gomat ca-bootstrap
./gomat ca-createuser 100

# find commissionable device & store IP
IP=`./gomat discover commissionable -d | grep addreses | cut -d ' ' -f 3 | cut -d ']' -f 1`
echo $IP > ~/.gomat/device_ip

# setup w/ temp pairing code from 3R-Installer app
PIN=`./gomat decode-mc $1 | grep passcode | cut -d ' ' -f 2`
./gomat commission --ip $IP --pin $PIN --controller-id 100 --device-id 500
