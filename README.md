# NakamaServer

NakamaServer is a lightweight, high-performance HTTP file-distribution server designed specifically for hosting and serving game builds and modpack archives. Written in pure Go, it requires no external runtime database thanks to its embedded SQLite architecture.

> [!NOTE]
> This project does NOT provide any games or mods and is not responsible for any misuse.

## Clients

This server is meant to be used with its official client applications:

- **[NakamaAdmin](https://github.com/OmarQurashi868/nakamaadmin)** — Tauri desktop app for managing games, modpacks, and server configuration.
- **[NakamaLauncher](https://github.com/OmarQurashi868/nakamalauncher)** — Tauri desktop launcher for end-users to browse, download, and play games.

---

## Features

- **Embedded Metadata Catalogs**: Uses local SQLite databases (`games.db` and `modpacks.db`) residing directly in the storage directories, making deployments completely self-contained.
- **Granular Security Keys**: 
  - `ADMIN_KEY`: Authorizes administrative requests (uploading/deleting games or modpacks, checking disk usage).
  - `DOWNLOAD_KEY`: Authorizes client-side operations (querying the catalog, downloading game files/modpacks).
- **Concurrency Protection**: Enforces a strict **one active download per IP** rule to prevent bandwidth saturation.
- **Abuse Prevention**: Built-in token-bucket rate limiter configured for **10 requests per minute per IP** (burst of 10).
- **Disk Quota Utility**: Admin endpoint for monitoring disk consumption under storage directories.

---

## Configuration

NakamaServer is configured using environment variables. You can define them in a `.env` file (when using Docker Compose) or set them directly in your environment.

| Environment Variable | Required | Default | Description |
|----------------------|----------|---------|-------------|
| `ADMIN_KEY` | **Yes** | — | Key for `/admin/...` actions (upload/delete/disk-quota) and `/query` |
| `DOWNLOAD_KEY` | **Yes** | — | Key for `/download/...` actions and `/query` |
| `PORT` | No | `8080` | Port the server listens on |
| `MAX_UPLOAD_BYTES` | No | `10737418240` (10 GB) | Maximum allowed size in bytes for uploaded archives |
| `GAMES_DIR` | No | `/data/nakama/games` | Path to store games and `games.db` SQLite catalog |
| `MODPACKS_DIR` | No | `/data/nakama/modpacks` | Path to store modpacks and `modpacks.db` SQLite catalog |

---

## Getting Started

### Prerequisites

- **Go**: Version 1.26 or newer (if building from source)
- **Docker** and **Docker Compose** (if running containerized)

### 1. Building from Source

To compile NakamaServer locally:

```bash
go build -o nakamaserver ./cmd/nakamaserver
```

### 2. Running Locally

Before running the server, export the required environment variables. It is recommended to use local directories for storage when running outside Docker to avoid permission issues.

#### On Linux / macOS (Bash/Zsh):
```bash
export ADMIN_KEY="my-super-secret-admin-key"
export DOWNLOAD_KEY="my-secure-download-key"
export PORT="8080"
export GAMES_DIR="./data/games"
export MODPACKS_DIR="./data/modpacks"

./nakamaserver
```

#### On Windows (PowerShell):
```powershell
$env:ADMIN_KEY="my-super-secret-admin-key"
$env:DOWNLOAD_KEY="my-secure-download-key"
$env:PORT="8080"
$env:GAMES_DIR=".\data\games"
$env:MODPACKS_DIR=".\data\modpacks"

.\nakamaserver
```

---

## Running with Docker

### Using Docker Command Line

You can run NakamaServer directly using Docker commands.

#### Run with the Pre-built Image:
To run the pre-built registry image with the same configuration as the compose example:
```bash
docker run -d \
  --name nakama-server \
  --restart unless-stopped \
  --user 1000:1000 \
  -p 42042:42042 \
  -e ADMIN_KEY="admin" \
  -e DOWNLOAD_KEY="download" \
  -e PORT="42042" \
  -e MAX_UPLOAD_BYTES="32212254720" \
  -e GAMES_DIR="/data/nakama/games" \
  -e MODPACKS_DIR="/data/nakama/modpacks" \
  -v /data/nakama:/data/nakama \
  ghcr.io/omarqurashi868/nakamaserver:main
```

#### Run with a Local Build:
If you prefer to compile and build the image locally from source:

1. **Build the image:**
   ```bash
   docker build -t nakamaserver .
   ```

2. **Run the container:**
   ```bash
   docker run -d \
     --name nakama-server \
     --restart unless-stopped \
     --user 1000:1000 \
     -p 42042:42042 \
     -e ADMIN_KEY="admin" \
     -e DOWNLOAD_KEY="download" \
     -e PORT="42042" \
     -e MAX_UPLOAD_BYTES="32212254720" \
     -e GAMES_DIR="/data/nakama/games" \
     -e MODPACKS_DIR="/data/nakama/modpacks" \
     -v /data/nakama:/data/nakama \
     nakamaserver
   ```

### Using Docker Compose (Recommended)

You can run NakamaServer using Docker Compose.

#### Example `docker-compose.yml`

Here is an example compose file utilizing the pre-built image:

```yaml
services:
  nakama-server:
    image: ghcr.io/omarqurashi868/nakamaserver:main
    container_name: nakama-server
    restart: unless-stopped
    user: 1000:1000
    ports:
      - 42042:42042
    environment:
      - ADMIN_KEY=admin
      - DOWNLOAD_KEY=download
      - PORT=42042
      - MAX_UPLOAD_BYTES=${MAX_UPLOAD_BYTES:-32212254720} # Default 10GB in bytes (30GB)
      - GAMES_DIR=/data/nakama/games
      - MODPACKS_DIR=/data/nakama/modpacks
    volumes:
      - /data/nakama:/data/nakama
networks: {}
```

#### Steps to Deploy:

1. **Prepare the files:**
   Either configure the example `docker-compose.yml` shown above, or use the local `docker-compose.yml` included in this repository (which works with `.env` files). If using the local file:
   ```bash
   cp .env.example .env
   ```
   *(On Windows PowerShell, use: `Copy-Item .env.example .env`)*

2. **Start the server:**
   ```bash
   docker compose up -d
   ```
   *(Or `docker-compose up -d` depending on your Docker version)*

3. **Verify it is running:**
   ```bash
   docker compose ps
   ```

---

## API Documentation

For a detailed reference on available endpoints, payload schemas, query parameters, authentication headers, and error codes, refer to the [API.md](API.md) documentation file.
