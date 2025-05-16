<<<<<<< HEAD
FROM golang:1.21-alpine as core
=======
FROM golang:1.23-alpine as bsc
>>>>>>> bsc/v1.5.12

RUN apk add --no-cache make cmake gcc musl-dev linux-headers git bash build-base libc-dev

ADD . /core
WORKDIR /core
ENV CGO_CFLAGS="-O -D__BLST_PORTABLE__" 
ENV CGO_CFLAGS_ALLOW="-O -D__BLST_PORTABLE__"
RUN make geth
RUN mv /core/build/bin/geth /usr/local/bin/geth

EXPOSE 8545 8547 30303 30303/udp
ENTRYPOINT [ "/usr/local/bin/geth" ]

FROM ethereum/solc:0.6.4-alpine as core-genesis

RUN apk add --no-cache ca-certificates npm nodejs bash alpine-sdk expect

RUN git clone https://github.com/coredao-org/core-genesis-contract.git /root/genesis \
    && cd /root/genesis && npm install
#RUN curl -L https://foundry.paradigm.xyz | bash

COPY --from=core /usr/local/bin/geth /usr/local/bin/geth

ENTRYPOINT [ "/bin/bash" ]