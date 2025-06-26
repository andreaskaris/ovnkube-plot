FLAGS ?= "-buildvcs=false"
NB_ADDRESS ?= tcp://172.18.0.3:6641,tcp://172.18.0.2:6641,tcp://172.18.0.4:6641 
SB_ADDRESS ?= tcp://172.18.0.3:6642,tcp://172.18.0.2:6642,tcp://172.18.0.4:6642 
MODE ?= auto
SWITCH_FILTER=.*

.DEFAULT_GOAL := build

.PHONY: build
build:
	mkdir -p bin
	go build -o bin/ovnkube-plot $(FLAGS)

.PHONY: plot
plot:
	bin/ovnkube-plot --filter="$(SWITCH_FILTER)" --mode $(MODE) --nb-address=$(NB_ADDRESS) --sb-address=$(SB_ADDRESS) > output/compact.txt
	cat output/compact.txt | dot -Tpdf > output/compact.pdf
	bin/ovnkube-plot --filter="$(SWITCH_FILTER)" --mode $(MODE) --format detailed --nb-address=$(NB_ADDRESS) --sb-address=$(SB_ADDRESS) > output/detailed.txt
	cat output/detailed.txt | dot -Tpdf > output/detailed.pdf

.PHONY: clean
clean:
	rm -f bin/*
	rm -f output/*

.PHONY: lint
lint:
	golangci-lint run
