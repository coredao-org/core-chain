#!/bin/bash
go build -o ./build/bin/geth   -gcflags=all='-N -l' -v ./cmd/geth
