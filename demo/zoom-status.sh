#!/bin/bash

FIRST=true

while sleep 1; do
    ZOOM="off"
    PROC_COUNT=`ps aux | grep 'CptHost' | grep -v 'grep' | wc -l`
    if [[ $PROC_COUNT -gt "0" ]]; then
        ZOOM="on"
    fi
    STATE=`cat ~/.gomat/device_status`
    if $FIRST; then
        if [[ $ZOOM == "on" ]]; then
            echo `date` ' Zoom meeting active, turning light on'
            ./nightlight-on.sh
        elif [[ $ZOOM == "off" ]]; then
            echo `date` ' no active Zoom meeting, turning light off'
            ./nightlight-off.sh
        fi
    else
        if [[ $ZOOM == "on" && $STATE == "off" ]]; then
            echo `date` ' Zoom meeting active, turning light on'
            ./nightlight-on.sh
        elif [[ $ZOOM == "off" && $STATE == "on" ]]; then
            echo `date` ' no active Zoom meeting, turning light off'
            ./nightlight-off.sh
        fi
    fi
    FIRST=false
done


