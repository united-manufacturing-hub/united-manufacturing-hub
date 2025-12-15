# Nginx Configuration

Nginx is a high-performance web server and reverse proxy. In a umh-core deployment, nginx can be used as the proxy for incoming HTTP requests, handling SSL and routing requests to the appropriate services.

## Why a proxy?

umh-core pipelines can expose HTTP endpoints using the `http_server` input. These endpoints allow external systems to send data to umh-core. However, these endpoints should be secured. It is possible to manage certificates in each of those inputs but it is inconvenient to maintain as certificates have to be replaced regularly.

## Adding Nginx in the existing Docker Compose Configuration

Add nginx to your `docker-compose.yaml` within the `services` section:

```diff
  services:
+   nginx:
+     image: management.umh.app/oci/library/nginx:1.27.3-alpine
+     restart: unless-stopped
+     ports:
+       - 80:80   # for http access
+       - 443:443 # for https access
```

## Nginx Configuration File

Refer to the [Nginx documentation](https://nginx.org/en/docs/index.html) for how to create your own configuration.

If you want to customize the `nginx.conf` create an `nginx.conf` file in the same directory as your `docker-compose.yaml` and mount it: 

```diff
  services:
    umh:
      # ... existing configuration ...
    nginx:
      image: management.umh.app/oci/library/nginx:1.27.3-alpine
      restart: unless-stopped
      ports:
        - 80:80   # for http access
        - 443:443 # for https access
+     volumes:
+       - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
```

Refer to the [Nginx documentation](https://nginx.org/en/docs/index.html) for what you can do in this configuration.

## Adding a simple http/https forward

```conf
# example of an http/https server configuration for forwarding requests to umh
server {
  listen 80;
  listen 443 ssl;
  server_tokens off;

  # Add CORS headers globally for this server
  add_header 'Access-Control-Allow-Origin' '*' always;
  add_header 'Access-Control-Allow-Methods' 'POST,GET,HEAD,OPTIONS' always;
  add_header 'Access-Control-Allow-Headers' 'content-type' always;

  # Handle OPTIONS requests
  if ($request_method = 'OPTIONS') {
      return 204;
  }

  location /example/route/ {
      # docker exposes the service `umh` as a hostname
      # the port is an example for an http_input that has to be created in umh
      proxy_pass http://umh:8285;
      proxy_set_header Host $host;
      proxy_set_header X-Real-IP $remote_addr;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
  }
}
```

## SSL

Obtain certificates from a trusted Certificate Authority (CA) or use Let's Encrypt. Place the `*.crt` and `*.key` file in the `certs` directory and create a `nginx.conf` for nginx which uses those certificates.

The file structure should now look similar to this.

```
.
├── certs
│   ├── example.crt
│   └── example.key
├── docker-compose.yaml
└── nginx.conf
```

The `docker-compose.yaml` has to mount `certs` and `nginx.conf` in appropriate places for nginx. This would look similar to this example:

```diff
  services:
    nginx:
      image: management.umh.app/oci/library/nginx:1.27.3-alpine
      restart: unless-stopped
      ports:
        - 80:80   # for http access
        - 443:443 # for https access
+     volumes:
+       - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
+       - ./certs:/etc/nginx/tls:ro
```

### Allowing forwarded SSL connections in umh-core

umh-core needs to be told to accept forwarded requests when they come from a TLS connection.

```diff
  services:
    umh:
      image: management.umh.app/oci/united-manufacturing-hub/umh-core:v0.43.18
      restart: unless-stopped
      volumes:
        - umh-data:/data
      environment:
        - AUTH_TOKEN=your-auth-token # TODO: you have to replace this
        - RELEASE_CHANNEL=stable
        - API_URL=https://management.umh.app/api
        - LOCATION_0=your-location # TODO: you have to replace this
+       - ALLOW_INSECURE_TLS=true
```
