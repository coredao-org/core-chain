## Docker Image

Included in this repo is a Dockerfile that you can launch CORE node for trying it out. Docker images are available on `ghcr.io/coredao-org/core`.

You can build the docker image with the following commands:
```bash
make docker
```

If your build machine has an ARM-based chip, like Apple silicon (M1), the image is built for `linux/arm64` by default. To build for `x86_64`, apply the --platform arg:

```bash
docker build --platform linux/amd64 -t coredao-org/core -f Dockerfile .
```

Before start the docker, get a copy of the config.toml & genesis.json from the release: https://github.com/coredao-org/core/releases, and make necessary modification. `config.toml` & `genesis.json` should be mounted into `/core/config` inside the container. Assume `config.toml` & `genesis.json` are under `./config` in your current working directory, you can start your docker container with the following command:
```bash
docker run -v $(pwd)/config:/core/config --rm --name core -it coredao-org/core 
```

You can also use `ETHEREUM OPTIONS` to overwrite settings in the configuration file
```bash
docker run -v $(pwd)/config:/core/config --rm --name core -it coredao-org/core --http.addr 0.0.0.0 --http.port 8579 --http.vhosts '*' --verbosity 3
```

If you need to open another shell, just do:
```bash
docker exec -it coredao-org/core /bin/bash
```

We also provide a `docker-compose` file for local testing

To use the container in kubernetes, you can use a configmap or secret to mount the `config.toml` & `genesis.json` into the container
```bash
containers:
  - name: core
    image: coredao-org/core

    ports:
      - name: p2p
        containerPort: 30311  
      - name: rpc
        containerPort: 8579
      - name: ws
        containerPort: 8580

    volumeMounts:
      - name: core-config
        mountPath: /core/config

  volumes:
    - name: core-config
      configMap:
        name: cm-core-config
```

Your configmap `core-config` should look like this:
```
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-core-config
data:
  config.toml: |
    ...

  genesis.json: |
    ...  

```