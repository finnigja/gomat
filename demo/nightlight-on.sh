#!/bin/bash

IP=`cat ~/.gomat/device_ip`

./gomat cmd on --ip $IP --controller-id 100 --device-id 500

echo "on" > ~/.gomat/device_status
