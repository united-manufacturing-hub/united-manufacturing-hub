docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli validate -i /local/factoryinput.yml
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g python -i /local/factoryinput.yml -o /local/output/factory_input/python
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g go -i /local/factoryinput.yml -o /local/output/factory_input/go
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g markdown -i /local/factoryinput.yml -o /local/output/factory_input/markdown
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g rust -i /local/factoryinput.yml -o /local/output/factory_input/rust
docker run --rm -v "%cd%:/local" openapitools/openapi-generator-cli generate -g html2 --generate-alias-as-model  -i /local/factoryinput.yml -o /local/output/factory_input/html