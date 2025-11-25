# Docker Compose Deployment

Deploy UMH Core edge gateway with Docker Compose.

⚠️ **Reference Implementation**: Production-ready example maintained by the community. Review [Security](../../security/umh-core/) before production use. For enterprise support, contact [UMH Systems GmbH](https://www.umh.app/contact).

---

## What This Deploys

**Core deployment includes:**
- **UMH Core**: Edge gateway for data ingestion, processing, and Unified Namespace (Kafka)

**Optional addons** (see [examples/](examples/)):
- Historian: Reference implementation with TimescaleDB + Grafana for local time-series storage (community-maintained)

---

## Prerequisites

1. **Docker and Docker Compose v2** installed
2. **Management Console account** - [Sign up free](https://management.umh.app/) (takes 30 seconds)
   - Creates your AUTH_TOKEN automatically
   - Provides remote monitoring and configuration
   - Optional: Can run fully offline after initial setup
3. **System requirements**: See [sizing-guide.md](../../sizing-guide.md)
4. **Volume permissions**: Non-root user (UID 1000) must own `./data` directory

---

## Getting Your Configuration from Management Console

The Management Console provides a docker run command when you create an instance. This docker-compose deployment uses the same configuration values.

### Step 1: Get Your Docker Run Command

1. Go to [management.umh.app](https://management.umh.app)
2. Create or select your instance
3. Click on deployment instructions
4. Copy the `docker run` command provided

**Example command from UI:**
```bash
docker run -d --restart unless-stopped --name umh-core \
  -v $(pwd)/umh-core-data:/data \
  -e AUTH_TOKEN=a3f29c325f110a757b2635403694accc547a11fba372440c386dfbea16591dde \
  -e RELEASE_CHANNEL=stable \
  -e LOCATION_0=UMH-Systems-GmbH---Dev-Team \
  -e LOCATION_1=jeremy-test-28 \
  -e API_URL=https://management.umh.app/api \
  management.umh.app/oci/united-manufacturing-hub/umh-core:v0.43.16
```

### Step 2: Extract Configuration Values

From the docker run command, extract these values for your `.env` file:

| Parameter | Extract From | Example |
|-----------|-------------|---------|
| `AUTH_TOKEN` | `-e AUTH_TOKEN=...` | `a3f29c32...` |
| `UMH_VERSION` | After `:` in image name | `v0.43.16` |
| `RELEASE_CHANNEL` | `-e RELEASE_CHANNEL=...` | `stable` |
| `API_URL` | `-e API_URL=...` | `https://management.umh.app/api` |
| `LOCATION_0` | `-e LOCATION_0=...` | `UMH-Systems-GmbH---Dev-Team` |
| `LOCATION_1` | `-e LOCATION_1=...` | `jeremy-test-28` |

### Step 3: Configure Your .env File

```bash
# Copy template
cp .env.example .env

# Edit with your values
nano .env  # or your preferred editor
```

Fill in the values you extracted:
```env
AUTH_TOKEN=a3f29c325f110a757b2635403694accc547a11fba372440c386dfbea16591dde
UMH_VERSION=v0.43.16
RELEASE_CHANNEL=stable
API_URL=https://management.umh.app/api
LOCATION_0=UMH-Systems-GmbH---Dev-Team
LOCATION_1=jeremy-test-28
LOCATION_2=
```

---

## Quick Start

### 1. Create data directory with correct permissions

```bash
mkdir -p ./data
sudo chown -R 1000:1000 ./data
```

### 2. Start core services

```bash
docker compose up -d
```

### 3. Verify Installation

Installation successful when:

1. **Container is running:**
   ```bash
   docker compose ps
   ```
   Should show:
   ```
   NAME       STATUS
   nginx      Up
   umh-core   Up (healthy)
   ```

   **Note:** If deployed without NGINX (dev mode), you'll only see umh-core.

2. **Instance appears in Management Console:**
   - Go to https://management.umh.app
   - Your instance should be listed as "Online"
   - Status indicator should be green

3. **Logs show successful connection:**
   ```bash
   docker compose logs umh-core | grep "Successfully connected"
   ```

---

## Deployment Options

Choose your deployment based on environment:

### Production Deployment (Recommended)

Includes NGINX security layer with port isolation, security headers, and webhook routing:

```bash
docker compose up -d
```

**Services started:**
- umh-core (edge gateway)
- NGINX (security layer)

**Access webhooks:** `http://localhost:8081/webhook/*`

### Development Deployment (Simplified)

For local development and testing, you can skip NGINX:

```bash
docker compose up -d umh-core
```

**Services started:**
- umh-core only

**Note:** Webhooks will not be accessible without NGINX. Add NGINX when you're ready to test webhook integrations.

### Why NGINX?

NGINX provides:
- **Port isolation**: umh-core not directly exposed
- **Security headers**: HTTPS, CSP, HSTS
- **Webhook routing**: Single entry point for external systems
- **Production best practice**: Industry standard security layer

**Recommendation:** Use full deployment (with NGINX) for any production or production-like environment.

---

## Next Steps

Once your UMH Core instance is online:

1. **Add protocol converters (bridges)** via Management Console:
   - Modbus TCP/RTU for PLCs
   - OPC UA for industrial equipment
   - S7 for Siemens devices
   - MQTT for IoT sensors

2. **Configure data flows** to process and transform your data

3. **Set up webhook integrations** (requires NGINX to be deployed):
   - External systems can send data to `http://localhost:8081/webhook/<your-webhook-name>`
   - Configure webhooks via Management Console
   - See [webhook documentation](../../features/webhooks.md) for details

4. **Monitor your instance** through the Management Console dashboard

### Add Historian (Optional)

Persist data locally with TimescaleDB + Grafana:

```bash
docker compose -f docker-compose.yaml -f examples/historian/docker-compose.historian.yaml up -d
```

See [examples/historian/](examples/historian/) for details.

---

## Configuration

Edit `.env` file:
- `AUTH_TOKEN` (required): From Management Console
- `LOCATION_0`, `LOCATION_1`, `LOCATION_2` (optional): Organization hierarchy

**Production:** Review [deployment-security.md](../../security/umh-core/deployment-security.md) before exposing ports.
