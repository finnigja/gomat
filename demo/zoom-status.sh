#!/bin/bash

FIRST=true

while sleep 1; do
    ZOOM="off"
    PROC_COUNT=`ps aux | grep 'CptHost' | grep -v 'grep' | wc -l`
    if [[ $PROC_COUNT -gt "0" ]]; then
        echo 'MEETING IS ACTIVE'
        ZOOM="on"
    else
        echo 'no meeting currently active'
    fi
    STATE=`cat ~/.gomat/device_status`
    if $FIRST; then
        if [[ $ZOOM == "on" ]]; then
            ./nightlight-on.sh
        elif [[ $ZOOM == "off" ]]; then
            ./nightlight-off.sh
        fi
    else
        if [[ $ZOOM == "on" && $STATE == "off" ]]; then
            ./nightlight-on.sh
        elif [[ $ZOOM == "off" && $STATE == "on" ]]; then
            ./nightlight-off.sh
        fi
    fi
    FIRST=false
done


