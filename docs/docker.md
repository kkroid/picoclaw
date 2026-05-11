# Docker

OneAppFactory ships two Docker surfaces:

- `oneappfactory`: the launcher service, built from [../docker/Dockerfile](../docker/Dockerfile)
- `oneappfactory-builder`: optional Flutter/Android builder image, built from [../docker/Dockerfile.appfactory-builder](../docker/Dockerfile.appfactory-builder)

The compose file is [../docker/docker-compose.oneappfactory.yml](../docker/docker-compose.oneappfactory.yml).

## Start The Launcher

```bash
docker compose -f docker/docker-compose.oneappfactory.yml up --build
```

The launcher listens on `127.0.0.1:18800` by default. Open `http://localhost:18800` for jobs, configuration, and logs.

Runtime data is stored in the `oneappfactory-home` Docker volume and mounted at `/root/.appfactory` inside the container.

## Build The Builder Image

```bash
docker compose -f docker/docker-compose.oneappfactory.yml --profile builder build oneappfactory-builder
```

The launcher uses `ONEAPPFACTORY_BUILDER_IMAGE=oneappfactory/builder:local` by default. The compose service mounts `/var/run/docker.sock` so the launcher can start builder containers from the host Docker daemon.

## Proxy Settings

The builder image accepts common proxy build args:

```bash
HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=socks5://127.0.0.1:7890 \
docker compose -f docker/docker-compose.oneappfactory.yml --profile builder build oneappfactory-builder
```

`NO_PROXY` defaults to `127.0.0.1,localhost,host.docker.internal`.

## Useful Commands

```bash
# Validate compose syntax
docker compose -f docker/docker-compose.oneappfactory.yml config

# Start launcher in the background
docker compose -f docker/docker-compose.oneappfactory.yml up -d --build

# View logs
docker compose -f docker/docker-compose.oneappfactory.yml logs -f oneappfactory

# Stop services
docker compose -f docker/docker-compose.oneappfactory.yml down
```

## Notes

Legacy non-AppFactory compose profiles are no longer part of the product. The Docker surface should stay limited to the launcher service and the optional AppFactory builder image.
