#!/bin/bash

IP=`cat ~/.gomat/device_ip`

# args are hue, saturation, transition time
# 256, 200, 2 is good
./gomat cmd color --ip $IP --controller-id 100 --device-id 500 $1 $2 2
