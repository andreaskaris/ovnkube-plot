#!/bin/bash -x

if ! [ -d bin ]; then
	mkdir bin
fi

go build -o bin/ovnkube-plot
