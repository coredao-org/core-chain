#!/bin/bash
set -e

CORE_CONFIG=${CORE_HOME}/config/config.toml
CORE_GENESIS=${CORE_HOME}/config/genesis.json

# Init genesis state if geth not exist
DATA_DIR=$(cat ${CORE_CONFIG} | grep -A1 '\[Node\]' | grep -oP '\"\K.*?(?=\")')

GETH_DIR=${DATA_DIR}/geth
if [ ! -d "$GETH_DIR" ]; then
  geth --datadir ${DATA_DIR} init ${CORE_GENESIS}
fi

exec "geth" "--config" ${CORE_CONFIG} "$@"
