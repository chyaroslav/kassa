#!/usr/bin/bash
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:./lib/
./kassa > kassa.log 2>&1 &

