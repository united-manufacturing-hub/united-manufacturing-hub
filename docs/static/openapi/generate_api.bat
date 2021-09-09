docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli validate -i /local/mqtt_bodies.yaml
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g python -i /local/mqtt_bodies.yaml -o /local/output/python --dry-run
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g html2 --generate-alias-as-model  -i /local/mqtt_bodies.yaml -o /local/output/html2
