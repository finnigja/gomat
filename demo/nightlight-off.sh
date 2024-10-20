#!/bin/bash

IP=`cat ~/.gomat/device_ip`

./gomat cmd off --ip $IP --controller-id 100 --device-id 500

echo "off" > ~/.gomat/device_status
