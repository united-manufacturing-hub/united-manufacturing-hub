# United Manufacturing Hub datasource for Grafana 

TODO: Add description

## Getting started (Manual)

   A data source backend plugin consists of both frontend and backend components.

### Frontend

1. Install dependencies

   ```bash
   yarn install
   ```

2. Build plugin in development mode or run in watch mode

   ```bash
   yarn dev
   ```

   or

   ```bash
   yarn watch
   ```

3. Build plugin in production mode

   ```bash
   yarn build
   ```

### Backend

1. Update [Grafana plugin SDK for Go](https://grafana.com/docs/grafana/latest/developers/plugins/backend/grafana-plugin-sdk-for-go/) dependency to the latest minor version:

   ```bash
   go get -u github.com/grafana/grafana-plugin-sdk-go
   ```

2. Build backend plugin binaries for Linux, Windows and Darwin:
   
   2.1 Install [mage](https://github.com/magefile/mage) dependency
      ```base
      git clone https://github.com/magefile/mage
      cd mage
      go run bootstrap.go
      ```
   
   2.2 Build backend
      ```bash
      ~/go/bin/mage -v
      ```

3. List all available Mage targets for additional commands:

   ```bash
   mage -l
   ```
   
## Getting Started (Dockerized)
   ### Build (Release)

   1. Install all dependencies
   ```bash
   yarn install
   ```
   2. Build frontend & backend
   ```bash
   yarn build && ~/go/bin/mage -v build:backend 
   ```
   3. Run docker with plugin
   ```bash
   sudo docker run --rm -p 3000:3000 -v "$(pwd)":/var/lib/grafana/plugins/united-manufacturing-hub -e 'GF_DEFAULT_APP_MODE=development' --name=grafana grafana/grafana
   ```
   
   ### Update (Release)
   ```bash
   yarn build && ~/go/bin/mage -v build:backend && sudo docker container restart grafana
   ```

### Auto rebuild dev builds
   ```bash
   npx @grafana/toolkit plugin:dev --watch
   ```


## Learn more

- [Build a data source backend plugin tutorial](https://grafana.com/tutorials/build-a-data-source-backend-plugin)
- [Grafana documentation](https://grafana.com/docs/)
- [Grafana Tutorials](https://grafana.com/tutorials/) - Grafana Tutorials are step-by-step guides that help you make the most of Grafana
- [Grafana UI Library](https://developers.grafana.com/ui) - UI components to help you build interfaces using Grafana Design System
- [Grafana plugin SDK for Go](https://grafana.com/docs/grafana/latest/developers/plugins/backend/grafana-plugin-sdk-for-go/)
